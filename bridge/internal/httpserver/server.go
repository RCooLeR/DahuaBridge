package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/store"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type ProbeReader interface {
	List() []*dahua.ProbeResult
	Get(string) (*dahua.ProbeResult, bool)
	Stats() store.Stats
}

type SnapshotReader interface {
	NVRSnapshot(context.Context, string, int) ([]byte, string, error)
	NVRRecordings(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)
	NVRDownloadRecording(context.Context, string, string) (dahua.NVRRecordingDownload, error)
	NVRDownloadRecordingClip(context.Context, string, dahua.NVRRecordingClipRequest) (dahua.NVRRecordingDownload, error)
	CreateNVRPlaybackSession(context.Context, string, dahua.NVRPlaybackSessionRequest) (dahua.NVRPlaybackSession, error)
	GetNVRPlaybackSession(string) (dahua.NVRPlaybackSession, error)
	SeekNVRPlaybackSession(context.Context, string, time.Time) (dahua.NVRPlaybackSession, error)
	VTOSnapshot(context.Context, string) ([]byte, string, error)
	IPCSnapshot(context.Context, string) ([]byte, string, error)
	ListStreams(bool) []streams.Entry
	AdminSettings() map[string]any
}

type MediaReader interface {
	Enabled() bool
	Subscribe(context.Context, string, string) (<-chan []byte, func(), error)
	SubscribeScaled(context.Context, string, string, int) (<-chan []byte, func(), error)
	CaptureFrame(context.Context, string, string, int) ([]byte, string, error)
	HLSPlaylist(context.Context, string, string) ([]byte, error)
	HLSSegment(context.Context, string, string, string) ([]byte, string, error)
	DASHManifest(context.Context, string, string) ([]byte, error)
	DASHAsset(context.Context, string, string, string) ([]byte, string, error)
	StartClip(context.Context, mediaapi.ClipStartRequest) (mediaapi.ClipInfo, error)
	StartDirectClip(context.Context, mediaapi.DirectClipStartRequest) (mediaapi.ClipInfo, error)
	StopClip(context.Context, string) (mediaapi.ClipInfo, error)
	DeleteClip(context.Context, string) error
	GetClip(string) (mediaapi.ClipInfo, error)
	FindClips(mediaapi.ClipQuery) ([]mediaapi.ClipInfo, error)
	ClipFilePath(string) (string, error)
	WebRTCAnswer(context.Context, string, string, mediaapi.WebRTCSessionDescription) (mediaapi.WebRTCSessionDescription, error)
	WebRTCICEServers() []mediaapi.WebRTCICEServer
	IntercomStatus(string) mediaapi.IntercomStatus
	StopIntercomSessions(string) mediaapi.IntercomStatus
	SetIntercomUplinkEnabled(string, bool) mediaapi.IntercomStatus
	ListWorkers() []mediaapi.WorkerStatus
}

type ActionReader interface {
	UnlockVTOLock(context.Context, string, int) error
	AnswerVTOCall(context.Context, string) error
	HangupVTOCall(context.Context, string) error
	VTOControlCapabilities(context.Context, string) (dahua.VTOControlCapabilities, error)
	SetVTORecordingEnabled(context.Context, string, bool) error
	NVRChannelControlCapabilities(context.Context, string, int) (dahua.NVRChannelControlCapabilities, error)
	ControlNVRPTZ(context.Context, string, dahua.NVRPTZRequest) error
	ControlNVRAux(context.Context, string, dahua.NVRAuxRequest) error
	ControlNVRRecording(context.Context, string, dahua.NVRRecordingRequest) error
	NVRDiagnosticAction(context.Context, string, dahua.NVRDiagnosticActionRequest) (dahua.NVRDiagnosticActionResult, error)
	ProbeDevice(context.Context, string) (*dahua.ProbeResult, error)
	ProbeAllDevices(context.Context) []dahua.ProbeActionResult
	RotateDeviceCredentials(context.Context, string, dahua.DeviceConfigUpdate) (*dahua.ProbeResult, error)
	RefreshNVRInventory(context.Context, string) (*dahua.ProbeResult, error)
}

type EventReader interface {
	ListEvents(deviceID string, childID string, deviceKind dahua.DeviceKind, code string, action string, limit int) []dahua.Event
	EventStats() map[string]any
	ClearEvents() int
}

type Server struct {
	httpServer *http.Server
	logger     zerolog.Logger
}

func New(
	cfg config.HTTPConfig,
	archiveCfg config.ArchiveConfig,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes ProbeReader,
	snapshots SnapshotReader,
	media MediaReader,
	actions ActionReader,
	events EventReader,
) *Server {
	adminLimiter := newPerClientRateLimiter(
		defaultPositiveInt(cfg.AdminRateLimitPerMinute, 30),
		defaultPositiveInt(cfg.AdminRateLimitBurst, 10),
		cfg.TrustedProxies,
	)
	snapshotLimiter := newPerClientRateLimiter(
		defaultPositiveInt(cfg.SnapshotRateLimitPerMinute, 240),
		defaultPositiveInt(cfg.SnapshotRateLimitBurst, 40),
		cfg.TrustedProxies,
	)
	mediaLimiter := newPerClientRateLimiter(
		defaultPositiveInt(cfg.MediaRateLimitPerMinute, 60),
		defaultPositiveInt(cfg.MediaRateLimitBurst, 12),
		cfg.TrustedProxies,
	)
	writeTimeout := cfg.WriteTimeout
	if writeTimeout <= 0 || writeTimeout < 60*time.Second {
		writeTimeout = 60 * time.Second
	}

	httpLogger := logger.With().Str("component", "http").Logger()
	controller := newController(
		cfg,
		archiveCfg.TempDir,
		metricsRegistry,
		probes,
		snapshots,
		media,
		actions,
		adminLimiter,
		snapshotLimiter,
		mediaLimiter,
	)
	router := chi.NewRouter()
	router.Use(securityHeadersMiddleware)
	router.Use(corsMiddleware(cfg))
	router.Use(maxRequestBodyMiddleware(cfg.MaxRequestBodyBytes))
	router.Use(authMiddleware(cfg))
	router.Use(debugAccessLogMiddleware(httpLogger))
	controller.registerRoutes(router)

	return &Server{
		httpServer: &http.Server{
			Addr:                cfg.ListenAddress,
			Handler:             router,
			ReadTimeout:         cfg.ReadTimeout,
			WriteTimeout:        writeTimeout,
			IdleTimeout:         cfg.IdleTimeout,
			MaxHeaderValueCount: 100,
		},
		logger: httpLogger,
	}
}

func (s *Server) Start() error {
	s.logger.Info().Str("listen_address", s.httpServer.Addr).Msg("starting admin http server")

	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}

	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.httpServer.Shutdown(ctx)
	if err != nil {
		// Streaming clients can outlive the grace period. Force their contexts
		// closed so application shutdown can join media and HTTP workers.
		return errors.Join(err, s.httpServer.Close())
	}
	return nil
}

type debugResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *debugResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *debugResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += int64(n)
	return n, err
}

func (w *debugResponseWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *debugResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func debugAccessLogMiddleware(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			debugWriter := &debugResponseWriter{ResponseWriter: w}
			next.ServeHTTP(debugWriter, r)

			status := debugWriter.status
			if status == 0 {
				status = http.StatusOK
			}
			event := logger.Debug().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", status).
				Int64("bytes", debugWriter.bytes).
				Dur("duration", time.Since(started))
			if routePattern := chi.RouteContext(r.Context()).RoutePattern(); routePattern != "" {
				event.Str("route", routePattern)
			}
			if r.URL.RawQuery != "" {
				event.Str("query", redactHTTPQuery(r.URL.Query()))
			}
			event.Msg("bridge http request")
		})
	}
}

func redactHTTPQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	redacted := make(url.Values, len(query))
	for key, values := range query {
		nextValues := append([]string(nil), values...)
		if shouldRedactHTTPQueryKey(key) {
			for index := range nextValues {
				nextValues[index] = "[redacted]"
			}
		}
		redacted[key] = nextValues
	}
	return redacted.Encode()
}

func shouldRedactHTTPQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "pwd") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func clearStreamingWriteDeadline(w http.ResponseWriter) {
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Time{})
}

type apiErrorPayload struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code"`
}

func writeErrorPayload(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, apiErrorPayload{
		Error:     strings.TrimSpace(message),
		ErrorCode: code,
	})
}

func writeInvalidRequestError(w http.ResponseWriter, err error) {
	writeErrorPayload(w, http.StatusBadRequest, "invalid_request", err.Error())
}

func writeServiceUnavailableError(w http.ResponseWriter, message string) {
	writeErrorPayload(w, http.StatusServiceUnavailable, "service_unavailable", message)
}

func writeClassifiedActionError(w http.ResponseWriter, err error, defaultStatus int) {
	status := defaultStatus
	code := "device_failure"

	switch {
	case errors.Is(err, dahua.ErrDeviceNotFound):
		status = http.StatusNotFound
		code = "device_not_found"
	case errors.Is(err, dahua.ErrUnsupportedOperation):
		status = http.StatusBadRequest
		code = "unsupported_operation"
	case errors.Is(err, dahua.ErrPlaybackSessionNotFound):
		status = http.StatusNotFound
		code = "playback_session_not_found"
	case errors.Is(err, mediaapi.ErrClipNotFound):
		status = http.StatusNotFound
		code = "clip_not_found"
	case errors.Is(err, mediaapi.ErrClipAlreadyActive):
		status = http.StatusConflict
		code = "clip_already_active"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		status = http.StatusGatewayTimeout
		code = "transport_failure"
	}

	writeErrorPayload(w, status, code, err.Error())
}

type httpStatus struct {
	Ready         bool   `json:"ready"`
	DeviceCount   int    `json:"device_count"`
	LastUpdatedAt string `json:"last_updated_at,omitempty"`
}

func toHTTPStatus(stats store.Stats) httpStatus {
	lastUpdatedAt := ""
	if !stats.LastUpdatedAt.IsZero() {
		lastUpdatedAt = stats.LastUpdatedAt.Format(time.RFC3339Nano)
	}
	return httpStatus{
		Ready:         stats.DeviceCount > 0,
		DeviceCount:   stats.DeviceCount,
		LastUpdatedAt: lastUpdatedAt,
	}
}

func parseOptionalPositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid integer %q", raw)
	}
	return value, nil
}

func parseClipStartRequest(r *http.Request) (mediaapi.ClipStartRequest, error) {
	request := struct {
		ProfileName     string
		DurationSeconds *int
		DurationMS      *int
	}{}
	request.ProfileName = firstNonEmptyQueryValue(r.URL.Query(), "profile", "profile_name")

	if rawDurationMS := strings.TrimSpace(r.URL.Query().Get("duration_ms")); rawDurationMS != "" {
		value, err := parseOptionalPositiveInt(rawDurationMS)
		if err != nil {
			return mediaapi.ClipStartRequest{}, fmt.Errorf("invalid duration_ms")
		}
		request.DurationMS = &value
	} else if rawDurationSeconds := strings.TrimSpace(r.URL.Query().Get("duration_seconds")); rawDurationSeconds != "" {
		value, err := parseOptionalPositiveInt(rawDurationSeconds)
		if err != nil {
			return mediaapi.ClipStartRequest{}, fmt.Errorf("invalid duration_seconds")
		}
		request.DurationSeconds = &value
	}

	var bodyRequest struct {
		ProfileName     string `json:"profile"`
		DurationSeconds *int   `json:"duration_seconds"`
		DurationMS      *int   `json:"duration_ms"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&bodyRequest); err != nil {
			if !errors.Is(err, io.EOF) {
				return mediaapi.ClipStartRequest{}, fmt.Errorf("invalid json body")
			}
		} else {
			if strings.TrimSpace(bodyRequest.ProfileName) != "" {
				request.ProfileName = bodyRequest.ProfileName
			}
			if bodyRequest.DurationMS != nil {
				request.DurationMS = bodyRequest.DurationMS
				request.DurationSeconds = nil
			} else if bodyRequest.DurationSeconds != nil {
				request.DurationSeconds = bodyRequest.DurationSeconds
				request.DurationMS = nil
			}
		}
	}

	duration := time.Duration(0)
	switch {
	case request.DurationMS != nil:
		if *request.DurationMS < 0 {
			return mediaapi.ClipStartRequest{}, fmt.Errorf("duration_ms must be zero or positive")
		}
		duration = time.Duration(*request.DurationMS) * time.Millisecond
	case request.DurationSeconds != nil:
		if *request.DurationSeconds < 0 {
			return mediaapi.ClipStartRequest{}, fmt.Errorf("duration_seconds must be zero or positive")
		}
		duration = time.Duration(*request.DurationSeconds) * time.Second
	}

	return mediaapi.ClipStartRequest{
		ProfileName: firstNonEmpty(strings.TrimSpace(request.ProfileName), "quality"),
		Duration:    duration,
	}, nil
}

func parseClipQuery(r *http.Request) (mediaapi.ClipQuery, error) {
	channel, err := parseOptionalPositiveInt(r.URL.Query().Get("channel"))
	if err != nil {
		return mediaapi.ClipQuery{}, fmt.Errorf("invalid channel")
	}

	var startTime time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("start")); raw != "" {
		startTime, err = parseFlexibleTimestamp(raw, "start")
		if err != nil {
			return mediaapi.ClipQuery{}, err
		}
	}
	var endTime time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("end")); raw != "" {
		endTime, err = parseFlexibleTimestamp(raw, "end")
		if err != nil {
			return mediaapi.ClipQuery{}, err
		}
	}
	if !startTime.IsZero() && !endTime.IsZero() && endTime.Before(startTime) {
		return mediaapi.ClipQuery{}, fmt.Errorf("end must not be before start")
	}

	limit, err := parseOptionalPositiveInt(r.URL.Query().Get("limit"))
	if err != nil {
		return mediaapi.ClipQuery{}, err
	}
	if limit > 200 {
		limit = 200
	}

	return mediaapi.ClipQuery{
		StreamID:     strings.TrimSpace(r.URL.Query().Get("stream_id")),
		RootDeviceID: strings.TrimSpace(r.URL.Query().Get("root_device_id")),
		Channel:      channel,
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
	}, nil
}

func parseNVRRecordingQuery(r *http.Request) (dahua.NVRRecordingQuery, error) {
	channel, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("channel")))
	if err != nil || channel <= 0 {
		return dahua.NVRRecordingQuery{}, fmt.Errorf("invalid channel")
	}

	values := r.URL.Query()
	startTime, err := parseFlexibleTimestamp(firstNonEmptyQueryValue(values, "start", "start_time"), "start")
	if err != nil {
		return dahua.NVRRecordingQuery{}, err
	}
	endTime, err := parseFlexibleTimestamp(firstNonEmptyQueryValue(values, "end", "end_time"), "end")
	if err != nil {
		return dahua.NVRRecordingQuery{}, err
	}
	if endTime.Before(startTime) {
		return dahua.NVRRecordingQuery{}, fmt.Errorf("end must not be before start")
	}

	limit, err := parseOptionalPositiveInt(r.URL.Query().Get("limit"))
	if err != nil {
		return dahua.NVRRecordingQuery{}, err
	}
	if limit == 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}

	eventOnly := parseQueryBool(values, "event_only", "events_only")
	includeAssets := parseQueryBool(values, "include_assets", "with_assets")
	skipAssetEnrichment := parseQueryBool(values, "db_only", "skip_assets", "skip_asset_enrichment")
	if eventOnly && !includeAssets {
		skipAssetEnrichment = true
	}

	return dahua.NVRRecordingQuery{
		Channel:             channel,
		StartTime:           startTime,
		EndTime:             endTime,
		Limit:               limit,
		EventCode:           firstNonEmptyQueryValue(values, "event", "event_type", "event_code"),
		EventOnly:           eventOnly,
		SkipAssetEnrichment: skipAssetEnrichment,
	}, nil
}

type nvrEventSummaryQuery struct {
	StartTime time.Time
	EndTime   time.Time
	EventCode string
	Channel   int
}

func parseNVREventSummaryQuery(r *http.Request) (nvrEventSummaryQuery, error) {
	startTime, err := parseFlexibleTimestamp(strings.TrimSpace(r.URL.Query().Get("start")), "start")
	if err != nil {
		return nvrEventSummaryQuery{}, err
	}
	endTime, err := parseFlexibleTimestamp(strings.TrimSpace(r.URL.Query().Get("end")), "end")
	if err != nil {
		return nvrEventSummaryQuery{}, err
	}
	if endTime.Before(startTime) {
		return nvrEventSummaryQuery{}, fmt.Errorf("end must not be before start")
	}
	channel, err := parseOptionalPositiveInt(r.URL.Query().Get("channel"))
	if err != nil {
		return nvrEventSummaryQuery{}, err
	}

	return nvrEventSummaryQuery{
		StartTime: startTime,
		EndTime:   endTime,
		EventCode: firstNonEmptyQueryValue(r.URL.Query(), "event", "event_type", "event_code"),
		Channel:   channel,
	}, nil
}

func parseQueryBool(values url.Values, keys ...string) bool {
	for _, key := range keys {
		switch strings.ToLower(strings.TrimSpace(values.Get(key))) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func firstNonEmptyQueryValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func parseNVRPlaybackSessionRequest(r *http.Request) (dahua.NVRPlaybackSessionRequest, error) {
	if r.Body == nil {
		return dahua.NVRPlaybackSessionRequest{}, fmt.Errorf("json body is required")
	}

	var request struct {
		Channel     int    `json:"channel"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		SeekTime    string `json:"seek_time"`
		FilePath    string `json:"file_path"`
		Source      string `json:"source"`
		Type        string `json:"type"`
		VideoStream string `json:"video_stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return dahua.NVRPlaybackSessionRequest{}, fmt.Errorf("json body is required")
		}
		return dahua.NVRPlaybackSessionRequest{}, fmt.Errorf("invalid json body")
	}

	startTime, err := parseFlexibleTimestamp(strings.TrimSpace(request.StartTime), "start_time")
	if err != nil {
		return dahua.NVRPlaybackSessionRequest{}, err
	}
	endTime, err := parseFlexibleTimestamp(strings.TrimSpace(request.EndTime), "end_time")
	if err != nil {
		return dahua.NVRPlaybackSessionRequest{}, err
	}

	var seekTime time.Time
	if strings.TrimSpace(request.SeekTime) != "" {
		seekTime, err = parseFlexibleTimestamp(strings.TrimSpace(request.SeekTime), "seek_time")
		if err != nil {
			return dahua.NVRPlaybackSessionRequest{}, err
		}
	}

	return dahua.NVRPlaybackSessionRequest{
		Channel:     request.Channel,
		StartTime:   startTime,
		EndTime:     endTime,
		SeekTime:    seekTime,
		FilePath:    strings.TrimSpace(request.FilePath),
		Source:      strings.TrimSpace(request.Source),
		Type:        strings.TrimSpace(request.Type),
		VideoStream: strings.TrimSpace(request.VideoStream),
	}, nil
}

func parseNVRPlaybackSeekTime(r *http.Request) (time.Time, error) {
	if r.Body == nil {
		return time.Time{}, fmt.Errorf("json body is required")
	}

	var request struct {
		SeekTime string `json:"seek_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return time.Time{}, fmt.Errorf("json body is required")
		}
		return time.Time{}, fmt.Errorf("invalid json body")
	}

	return parseFlexibleTimestamp(strings.TrimSpace(request.SeekTime), "seek_time")
}

func parseNVRRecordingExportRequest(r *http.Request) (dahua.NVRPlaybackSessionRequest, string, time.Duration, error) {
	values := r.URL.Query()
	request := struct {
		Channel         int
		StartTime       string
		EndTime         string
		SeekTime        string
		FilePath        string
		Source          string
		Type            string
		VideoStream     string
		Profile         string
		DurationSeconds *int
		DurationMS      *int
	}{
		StartTime:   firstNonEmptyQueryValue(values, "start_time", "start"),
		EndTime:     firstNonEmptyQueryValue(values, "end_time", "end"),
		SeekTime:    firstNonEmptyQueryValue(values, "seek_time", "seek"),
		FilePath:    strings.TrimSpace(values.Get("file_path")),
		Source:      strings.TrimSpace(values.Get("source")),
		Type:        strings.TrimSpace(values.Get("type")),
		VideoStream: strings.TrimSpace(values.Get("video_stream")),
		Profile:     strings.TrimSpace(values.Get("profile")),
	}
	if rawChannel := strings.TrimSpace(values.Get("channel")); rawChannel != "" {
		channel, err := strconv.Atoi(rawChannel)
		if err != nil {
			return dahua.NVRPlaybackSessionRequest{}, "", 0, fmt.Errorf("invalid channel")
		}
		request.Channel = channel
	}
	if rawDurationMS := strings.TrimSpace(values.Get("duration_ms")); rawDurationMS != "" {
		durationMS, err := strconv.Atoi(rawDurationMS)
		if err != nil {
			return dahua.NVRPlaybackSessionRequest{}, "", 0, fmt.Errorf("invalid duration_ms")
		}
		request.DurationMS = &durationMS
	}
	if rawDurationSeconds := strings.TrimSpace(values.Get("duration_seconds")); rawDurationSeconds != "" {
		durationSeconds, err := strconv.Atoi(rawDurationSeconds)
		if err != nil {
			return dahua.NVRPlaybackSessionRequest{}, "", 0, fmt.Errorf("invalid duration_seconds")
		}
		request.DurationSeconds = &durationSeconds
	}

	if r.Body != nil {
		var bodyRequest struct {
			Channel         int    `json:"channel"`
			StartTime       string `json:"start_time"`
			EndTime         string `json:"end_time"`
			SeekTime        string `json:"seek_time"`
			FilePath        string `json:"file_path"`
			Source          string `json:"source"`
			Type            string `json:"type"`
			VideoStream     string `json:"video_stream"`
			Profile         string `json:"profile"`
			DurationSeconds *int   `json:"duration_seconds"`
			DurationMS      *int   `json:"duration_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&bodyRequest); err != nil {
			if !errors.Is(err, io.EOF) {
				return dahua.NVRPlaybackSessionRequest{}, "", 0, fmt.Errorf("invalid json body")
			}
		} else {
			if bodyRequest.Channel != 0 {
				request.Channel = bodyRequest.Channel
			}
			if strings.TrimSpace(bodyRequest.StartTime) != "" {
				request.StartTime = strings.TrimSpace(bodyRequest.StartTime)
			}
			if strings.TrimSpace(bodyRequest.EndTime) != "" {
				request.EndTime = strings.TrimSpace(bodyRequest.EndTime)
			}
			if strings.TrimSpace(bodyRequest.SeekTime) != "" {
				request.SeekTime = strings.TrimSpace(bodyRequest.SeekTime)
			}
			if strings.TrimSpace(bodyRequest.FilePath) != "" {
				request.FilePath = strings.TrimSpace(bodyRequest.FilePath)
			}
			if strings.TrimSpace(bodyRequest.Source) != "" {
				request.Source = strings.TrimSpace(bodyRequest.Source)
			}
			if strings.TrimSpace(bodyRequest.Type) != "" {
				request.Type = strings.TrimSpace(bodyRequest.Type)
			}
			if strings.TrimSpace(bodyRequest.VideoStream) != "" {
				request.VideoStream = strings.TrimSpace(bodyRequest.VideoStream)
			}
			if strings.TrimSpace(bodyRequest.Profile) != "" {
				request.Profile = strings.TrimSpace(bodyRequest.Profile)
			}
			if bodyRequest.DurationSeconds != nil {
				request.DurationSeconds = bodyRequest.DurationSeconds
			}
			if bodyRequest.DurationMS != nil {
				request.DurationMS = bodyRequest.DurationMS
			}
		}
	}

	startTime, err := parseFlexibleTimestamp(strings.TrimSpace(request.StartTime), "start_time")
	if err != nil {
		return dahua.NVRPlaybackSessionRequest{}, "", 0, err
	}
	endTime, err := parseFlexibleTimestamp(strings.TrimSpace(request.EndTime), "end_time")
	if err != nil {
		return dahua.NVRPlaybackSessionRequest{}, "", 0, err
	}
	var seekTime time.Time
	if strings.TrimSpace(request.SeekTime) != "" {
		seekTime, err = parseFlexibleTimestamp(strings.TrimSpace(request.SeekTime), "seek_time")
		if err != nil {
			return dahua.NVRPlaybackSessionRequest{}, "", 0, err
		}
	}

	duration := time.Duration(0)
	switch {
	case request.DurationMS != nil:
		if *request.DurationMS <= 0 {
			return dahua.NVRPlaybackSessionRequest{}, "", 0, fmt.Errorf("duration_ms must be positive")
		}
		duration = time.Duration(*request.DurationMS) * time.Millisecond
	case request.DurationSeconds != nil:
		if *request.DurationSeconds <= 0 {
			return dahua.NVRPlaybackSessionRequest{}, "", 0, fmt.Errorf("duration_seconds must be positive")
		}
		duration = time.Duration(*request.DurationSeconds) * time.Second
	default:
		effectiveStart := startTime
		if !seekTime.IsZero() {
			effectiveStart = seekTime
		}
		duration = endTime.Sub(effectiveStart)
		if duration <= 0 {
			return dahua.NVRPlaybackSessionRequest{}, "", 0, fmt.Errorf("export duration must be positive")
		}
	}

	profile := strings.TrimSpace(request.Profile)
	if profile == "" {
		profile = "quality"
	}

	return dahua.NVRPlaybackSessionRequest{
		Channel:     request.Channel,
		StartTime:   startTime,
		EndTime:     endTime,
		SeekTime:    seekTime,
		FilePath:    strings.TrimSpace(request.FilePath),
		Source:      strings.TrimSpace(request.Source),
		Type:        strings.TrimSpace(request.Type),
		VideoStream: strings.TrimSpace(request.VideoStream),
	}, profile, duration, nil
}

func parseNVRPTZRequest(r *http.Request) (dahua.NVRPTZRequest, error) {
	if r.Body == nil {
		return dahua.NVRPTZRequest{}, fmt.Errorf("json body is required")
	}

	var request struct {
		Action     string `json:"action"`
		Command    string `json:"command"`
		Speed      int    `json:"speed"`
		DurationMS int    `json:"duration_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return dahua.NVRPTZRequest{}, fmt.Errorf("json body is required")
		}
		return dahua.NVRPTZRequest{}, fmt.Errorf("invalid json body")
	}

	action := dahua.NVRPTZAction(strings.ToLower(strings.TrimSpace(request.Action)))
	switch action {
	case dahua.NVRPTZActionStart, dahua.NVRPTZActionStop, dahua.NVRPTZActionPulse:
	default:
		return dahua.NVRPTZRequest{}, fmt.Errorf("invalid action")
	}

	command := dahua.NVRPTZCommand(strings.ToLower(strings.TrimSpace(request.Command)))
	switch command {
	case dahua.NVRPTZCommandUp,
		dahua.NVRPTZCommandDown,
		dahua.NVRPTZCommandLeft,
		dahua.NVRPTZCommandRight,
		dahua.NVRPTZCommandLeftUp,
		dahua.NVRPTZCommandRightUp,
		dahua.NVRPTZCommandLeftDown,
		dahua.NVRPTZCommandRightDown,
		dahua.NVRPTZCommandZoomIn,
		dahua.NVRPTZCommandZoomOut,
		dahua.NVRPTZCommandFocusNear,
		dahua.NVRPTZCommandFocusFar:
	default:
		return dahua.NVRPTZRequest{}, fmt.Errorf("invalid command")
	}

	if request.Speed < 0 {
		return dahua.NVRPTZRequest{}, fmt.Errorf("invalid speed")
	}
	if request.DurationMS < 0 {
		return dahua.NVRPTZRequest{}, fmt.Errorf("invalid duration_ms")
	}

	duration := time.Duration(request.DurationMS) * time.Millisecond
	if action == dahua.NVRPTZActionPulse && duration <= 0 {
		duration = 300 * time.Millisecond
	}

	return dahua.NVRPTZRequest{
		Action:   action,
		Command:  command,
		Speed:    request.Speed,
		Duration: duration,
	}, nil
}

func parseNVRAuxRequest(r *http.Request) (dahua.NVRAuxRequest, error) {
	if r.Body == nil {
		return dahua.NVRAuxRequest{}, fmt.Errorf("json body is required")
	}

	var request struct {
		Action     string `json:"action"`
		Output     string `json:"output"`
		DurationMS int    `json:"duration_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return dahua.NVRAuxRequest{}, fmt.Errorf("json body is required")
		}
		return dahua.NVRAuxRequest{}, fmt.Errorf("invalid json body")
	}

	action := dahua.NVRAuxAction(strings.ToLower(strings.TrimSpace(request.Action)))
	switch action {
	case dahua.NVRAuxActionStart, dahua.NVRAuxActionStop, dahua.NVRAuxActionPulse:
	default:
		return dahua.NVRAuxRequest{}, fmt.Errorf("invalid action")
	}

	output := strings.ToLower(strings.TrimSpace(request.Output))
	switch output {
	case "aux", "siren":
		output = "aux"
	case "light":
		output = "light"
	case "warning_light":
		output = "warning_light"
	case "wiper":
	default:
		return dahua.NVRAuxRequest{}, fmt.Errorf("invalid output")
	}

	if request.DurationMS < 0 {
		return dahua.NVRAuxRequest{}, fmt.Errorf("invalid duration_ms")
	}

	duration := time.Duration(request.DurationMS) * time.Millisecond
	if action == dahua.NVRAuxActionPulse && duration <= 0 {
		duration = 300 * time.Millisecond
	}

	return dahua.NVRAuxRequest{
		Action:   action,
		Output:   output,
		Duration: duration,
	}, nil
}

func parseNVRRecordingRequest(r *http.Request) (dahua.NVRRecordingRequest, error) {
	if r.Body == nil {
		return dahua.NVRRecordingRequest{}, fmt.Errorf("json body is required")
	}

	var request struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return dahua.NVRRecordingRequest{}, fmt.Errorf("json body is required")
		}
		return dahua.NVRRecordingRequest{}, fmt.Errorf("invalid json body")
	}

	action := dahua.NVRRecordingAction(strings.ToLower(strings.TrimSpace(request.Action)))
	switch action {
	case dahua.NVRRecordingActionStart, dahua.NVRRecordingActionStop, dahua.NVRRecordingActionAuto:
	default:
		return dahua.NVRRecordingRequest{}, fmt.Errorf("invalid action")
	}

	return dahua.NVRRecordingRequest{Action: action}, nil
}

func parseNVRDiagnosticActionRequest(r *http.Request) (dahua.NVRDiagnosticActionRequest, error) {
	if r.Body == nil {
		return dahua.NVRDiagnosticActionRequest{}, fmt.Errorf("json body is required")
	}

	var request struct {
		Method     string `json:"method"`
		Action     string `json:"action"`
		DurationMS int    `json:"duration_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return dahua.NVRDiagnosticActionRequest{}, fmt.Errorf("json body is required")
		}
		return dahua.NVRDiagnosticActionRequest{}, fmt.Errorf("invalid json body")
	}

	method := strings.ToLower(strings.TrimSpace(request.Method))
	if method == "" {
		return dahua.NVRDiagnosticActionRequest{}, fmt.Errorf("method is required")
	}
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action == "" {
		return dahua.NVRDiagnosticActionRequest{}, fmt.Errorf("action is required")
	}
	if request.DurationMS < 0 {
		return dahua.NVRDiagnosticActionRequest{}, fmt.Errorf("invalid duration_ms")
	}

	return dahua.NVRDiagnosticActionRequest{
		Method:   method,
		Action:   action,
		Duration: time.Duration(request.DurationMS) * time.Millisecond,
	}, nil
}

func parseVTORecordingRequest(r *http.Request) (bool, error) {
	if r.Body == nil {
		return false, fmt.Errorf("json body is required")
	}

	var request struct {
		AutoRecordEnabled *bool `json:"auto_record_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return false, fmt.Errorf("json body is required")
		}
		return false, fmt.Errorf("invalid json body")
	}
	if request.AutoRecordEnabled == nil {
		return false, fmt.Errorf("auto_record_enabled is required")
	}
	return *request.AutoRecordEnabled, nil
}

func parseFlexibleTimestamp(raw string, field string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006_01_02_15_04_05",
	}
	for _, layout := range layouts {
		var (
			parsed time.Time
			err    error
		)
		switch layout {
		case "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006_01_02_15_04_05":
			parsed, err = time.ParseInLocation(layout, raw, time.Local)
		default:
			parsed, err = time.Parse(layout, raw)
		}
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid %s time %q", field, raw)
}

func clipAPIResponse(r *http.Request, clip mediaapi.ClipInfo) map[string]any {
	payload := map[string]any{
		"id":               clip.ID,
		"stream_id":        clip.StreamID,
		"root_device_id":   clip.RootDeviceID,
		"source_device_id": clip.SourceDeviceID,
		"device_kind":      clip.DeviceKind,
		"name":             clip.Name,
		"channel":          clip.Channel,
		"profile":          clip.Profile,
		"status":           clip.Status,
		"started_at":       clip.StartedAt.Format(time.RFC3339),
		"duration_ms":      clip.Duration.Milliseconds(),
		"bytes":            clip.Bytes,
		"file_name":        clip.FileName,
		"playback_url":     buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clip.ID)+"/play"),
		"download_url":     buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clip.ID)+"/download"),
		"self_url":         buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clip.ID)),
	}
	if !clip.SourceStartAt.IsZero() {
		payload["start_time"] = clip.SourceStartAt.Format(time.RFC3339)
	}
	if !clip.SourceEndAt.IsZero() {
		payload["end_time"] = clip.SourceEndAt.Format(time.RFC3339)
	}
	if !clip.EndedAt.IsZero() {
		payload["ended_at"] = clip.EndedAt.Format(time.RFC3339)
	}
	if strings.TrimSpace(clip.Error) != "" {
		payload["error"] = clip.Error
	}
	if clip.Status == mediaapi.ClipStatusRecording {
		payload["stop_url"] = buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clip.ID)+"/stop")
	}
	payload["delete_url"] = buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clip.ID))
	return payload
}

func attachNVRRecordingExportURLs(r *http.Request, deviceID string, result *dahua.NVRRecordingSearchResult) {
	if result == nil {
		return
	}
	for index := range result.Items {
		item := &result.Items[index]
		if strings.EqualFold(strings.TrimSpace(item.Source), "bridge") || item.ClipID != "" {
			continue
		}
		if strings.TrimSpace(item.Source) == "" {
			item.Source = "nvr"
		}
		channel := item.Channel
		if channel <= 0 {
			channel = result.Channel
		}
		startTime := firstNonEmpty(item.StartTime, result.StartTime)
		endTime := firstNonEmpty(item.EndTime, result.EndTime)
		if channel <= 0 || strings.TrimSpace(startTime) == "" || strings.TrimSpace(endTime) == "" {
			continue
		}
		item.ExportURL = buildNVRRecordingExportURL(
			r,
			deviceID,
			channel,
			startTime,
			endTime,
			conditionalArchiveExportFilePath(*item),
			item.Source,
			item.Type,
			item.VideoStream,
		)
		if strings.TrimSpace(item.FilePath) != "" && !isEventRecordingItem(*item) {
			query := url.Values{"file_path": []string{item.FilePath}}
			item.DownloadURL = buildAbsoluteRequestURL(
				r,
				"/api/v1/nvr/"+url.PathEscape(deviceID)+"/recordings/download?"+query.Encode(),
			)
		}
		attachNVRRecordingAssetURLs(r, item)
	}
}

func attachNVRRecordingAssetURLs(r *http.Request, item *dahua.NVRRecording) {
	if item == nil {
		return
	}
	clipID := strings.TrimSpace(item.AssetClipID)
	if clipID == "" {
		return
	}
	status := strings.ToLower(strings.TrimSpace(item.AssetStatus))
	if status == "indexed" || status == "missing" {
		return
	}
	item.AssetSelfURL = buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clipID))
	if status == "ready" {
		item.AssetDownloadURL = buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clipID)+"/download")
		item.AssetPlaybackURL = buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clipID)+"/play")
	}
	if status == "transcoding" {
		item.AssetStopURL = buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clipID)+"/stop")
	}
}

func stripNVRRecordingPlaybackURLs(result *dahua.NVRRecordingSearchResult) {
	if result == nil {
		return
	}
	for index := range result.Items {
		result.Items[index].RTSPMainURL = ""
		result.Items[index].RTSPSubURL = ""
	}
}

func isEventRecordingItem(item dahua.NVRRecording) bool {
	recordKind := strings.ToLower(strings.TrimSpace(item.RecordKind))
	source := strings.ToLower(strings.TrimSpace(item.Source))
	recordingType := strings.ToLower(strings.TrimSpace(item.Type))
	return recordKind == "smd_ivs" ||
		recordKind == "smd-ivs" ||
		recordKind == "event" ||
		source == "smd_ivs" ||
		source == "smd-ivs" ||
		source == "nvr_event" ||
		recordingType == "event" ||
		strings.HasPrefix(recordingType, "event.")
}

func normalizeNVRRecordingSearchResult(result *dahua.NVRRecordingSearchResult) {
	if result == nil {
		return
	}
	if result.Items == nil {
		result.Items = []dahua.NVRRecording{}
	}
	if result.ReturnedCount == 0 && len(result.Items) > 0 {
		result.ReturnedCount = len(result.Items)
	}
}

func buildNVRRecordingExportURL(r *http.Request, deviceID string, channel int, startTime string, endTime string, filePath string, source string, recordingType string, videoStream string) string {
	profileName := "stable"
	if isArchiveEventSource(source, recordingType) {
		profileName = "quality"
	}
	query := url.Values{
		"channel":    []string{strconv.Itoa(channel)},
		"start_time": []string{startTime},
		"end_time":   []string{endTime},
		"profile":    []string{profileName},
	}
	if strings.TrimSpace(filePath) != "" {
		query.Set("file_path", strings.TrimSpace(filePath))
	}
	if strings.TrimSpace(source) != "" {
		query.Set("source", strings.TrimSpace(source))
	}
	if strings.TrimSpace(recordingType) != "" {
		query.Set("type", strings.TrimSpace(recordingType))
	}
	if strings.TrimSpace(videoStream) != "" {
		query.Set("video_stream", strings.TrimSpace(videoStream))
	}
	path := "/api/v1/nvr/" + url.PathEscape(deviceID) + "/recordings/export?" + query.Encode()
	return buildAbsoluteRequestURL(r, path)
}

func conditionalArchiveExportFilePath(item dahua.NVRRecording) string {
	if isEventRecordingItem(item) {
		return ""
	}
	return strings.TrimSpace(item.FilePath)
}

func isArchiveEventSource(source string, recordingType string) bool {
	normalizedSource := strings.ToLower(strings.TrimSpace(source))
	normalizedType := strings.ToLower(strings.TrimSpace(recordingType))
	return normalizedSource == "smd_ivs" ||
		normalizedSource == "smd-ivs" ||
		normalizedSource == "nvr_event" ||
		normalizedType == "event" ||
		strings.HasPrefix(normalizedType, "event.")
}

func buildAbsoluteRequestURL(r *http.Request, path string) string {
	if r == nil {
		return path
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = forwardedProto
	}

	host := strings.TrimSpace(r.Host)
	if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	if host == "" {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return scheme + "://" + host + path
}

func defaultPositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func findStreamEntry(entries []streams.Entry, streamID string) (streams.Entry, bool) {
	for _, entry := range entries {
		if entry.ID == streamID {
			return entry, true
		}
	}
	return streams.Entry{}, false
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(cfg config.HTTPConfig) func(http.Handler) http.Handler {
	allowedOrigins := append([]string(nil), cfg.AllowedOrigins...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			headers := w.Header()
			headers.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			headers.Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")
			headers.Add("Vary", "Origin")
			headers.Add("Vary", "Access-Control-Request-Method")
			headers.Add("Vary", "Access-Control-Request-Headers")

			if origin := allowedCORSOrigin(r.Header.Get("Origin"), allowedOrigins); origin != "" {
				headers.Set("Access-Control-Allow-Origin", origin)
			}

			requestHeaders := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers"))
			if requestHeaders != "" {
				headers.Set("Access-Control-Allow-Headers", requestHeaders)
			} else {
				headers.Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-DahuaBridge-Token")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func allowedCORSOrigin(origin string, allowedOrigins []string) string {
	origin = strings.TrimSpace(origin)
	if len(allowedOrigins) == 0 {
		return "*"
	}
	if origin == "" {
		return ""
	}
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return allowed
		}
	}
	return ""
}
