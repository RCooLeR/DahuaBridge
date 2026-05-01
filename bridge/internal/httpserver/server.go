package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/ha"
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
	StartClip(context.Context, mediaapi.ClipStartRequest) (mediaapi.ClipInfo, error)
	StopClip(context.Context, string) (mediaapi.ClipInfo, error)
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
	SetVTOAudioOutputVolume(context.Context, string, int, int) error
	SetVTOAudioInputVolume(context.Context, string, int, int) error
	SetVTOMute(context.Context, string, bool) error
	SetVTORecordingEnabled(context.Context, string, bool) error
	NVRChannelControlCapabilities(context.Context, string, int) (dahua.NVRChannelControlCapabilities, error)
	ControlNVRPTZ(context.Context, string, dahua.NVRPTZRequest) error
	ControlNVRAux(context.Context, string, dahua.NVRAuxRequest) error
	ControlNVRAudio(context.Context, string, dahua.NVRAudioRequest) error
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
	)
	snapshotLimiter := newPerClientRateLimiter(
		defaultPositiveInt(cfg.SnapshotRateLimitPerMinute, 240),
		defaultPositiveInt(cfg.SnapshotRateLimitBurst, 40),
	)
	mediaLimiter := newPerClientRateLimiter(
		defaultPositiveInt(cfg.MediaRateLimitPerMinute, 60),
		defaultPositiveInt(cfg.MediaRateLimitBurst, 12),
	)
	writeTimeout := cfg.WriteTimeout
	if writeTimeout <= 0 || writeTimeout < 60*time.Second {
		writeTimeout = 60 * time.Second
	}

	httpLogger := logger.With().Str("component", "http").Logger()
	router := chi.NewRouter()
	router.Use(corsMiddleware)
	router.Use(debugAccessLogMiddleware(httpLogger))
	router.Get(cfg.HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	router.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		stats := toHTTPStatus(probes.Stats())
		if !stats.Ready {
			writeJSON(w, http.StatusServiceUnavailable, stats)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	router.Get("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, toHTTPStatus(probes.Stats()))
	})
	router.Handle("/admin/assets/*", adminAssetHandler())
	router.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		status := toHTTPStatus(probes.Stats())
		entries := snapshots.ListStreams(false)
		settings := snapshots.AdminSettings()
		eventStats := map[string]any{}
		if events != nil {
			eventStats = events.EventStats()
		}
		workerStatuses := []mediaapi.WorkerStatus{}
		mediaEnabled := false
		if media != nil {
			workerStatuses = media.ListWorkers()
			mediaEnabled = media.Enabled()
		}

		body := renderAdminPage(
			status,
			probes.List(),
			entries,
			settings,
			eventStats,
			workerStatuses,
			actions != nil,
			events != nil,
			mediaEnabled,
			cfg.HealthPath,
			cfg.MetricsPath,
		)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.Get("/admin/test-bridge", func(w http.ResponseWriter, r *http.Request) {
		mediaEnabled := false
		if media != nil {
			mediaEnabled = media.Enabled()
		}
		body := renderAdminTestBridgePage(snapshots.ListStreams(false), actions != nil, mediaEnabled)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.Handle(cfg.MetricsPath, metricsRegistry.Handler())
	router.Get("/api/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, probes.List())
	})
	router.Get("/api/v1/devices/{deviceID}", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		result, ok := probes.Get(deviceID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/devices/probe-all", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		results := actions.ProbeAllDevices(actionCtx)
		successCount := 0
		errorCount := 0
		for _, result := range results {
			if strings.TrimSpace(result.Error) == "" {
				successCount++
			} else {
				errorCount++
			}
		}

		statusCode := http.StatusOK
		statusText := "ok"
		if errorCount > 0 {
			statusCode = http.StatusMultiStatus
			statusText = "partial_error"
		}

		writeJSON(w, statusCode, map[string]any{
			"status":        statusText,
			"device_count":  len(results),
			"success_count": successCount,
			"error_count":   errorCount,
			"results":       results,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/devices/{deviceID}/probe", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := actions.ProbeDevice(actionCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/devices/{deviceID}/credentials", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		var update dahua.DeviceConfigUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid json body")
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := actions.RotateDeviceCredentials(actionCtx, chi.URLParam(r, "deviceID"), update)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/nvr/{deviceID}/inventory/refresh", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := actions.RefreshNVRInventory(actionCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Get("/api/v1/nvr/{deviceID}/recordings", func(w http.ResponseWriter, r *http.Request) {
		query, err := parseNVRRecordingQuery(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		searchCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := snapshots.NVRRecordings(searchCtx, chi.URLParam(r, "deviceID"), query)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		normalizeNVRRecordingSearchResult(&result)
		attachNVRRecordingExportURLs(r, chi.URLParam(r, "deviceID"), &result)
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(adminLimiter)).Get("/api/v1/nvr/{deviceID}/events/summary", func(w http.ResponseWriter, r *http.Request) {
		query, err := parseNVREventSummaryQuery(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		summaryCtx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()

		summary, err := buildNVREventSummary(summaryCtx, snapshots, chi.URLParam(r, "deviceID"), query)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, summary)
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/nvr/{deviceID}/recordings/export", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		playbackRequest, profile, duration, err := parseNVRRecordingExportRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		session, err := snapshots.CreateNVRPlaybackSession(r.Context(), chi.URLParam(r, "deviceID"), playbackRequest)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		clip, err := media.StartClip(r.Context(), mediaapi.ClipStartRequest{
			StreamID:    session.StreamID,
			ProfileName: profile,
			Duration:    duration,
		})
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"session": session,
			"clip":    clipAPIResponse(r, clip),
		})
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/nvr/{deviceID}/recordings/download", func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimSpace(r.URL.Query().Get("file_path"))
		if filePath == "" {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "file_path is required")
			return
		}

		download, err := snapshots.NVRDownloadRecording(r.Context(), chi.URLParam(r, "deviceID"), filePath)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		defer download.Body.Close()

		contentType := strings.TrimSpace(download.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		fileName := strings.TrimSpace(download.FileName)
		if fileName == "" {
			fileName = "recording.dav"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(fileName)+`"`)
		if download.ContentLength > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(download.ContentLength, 10))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, download.Body)
	})
	router.With(rateLimitMiddleware(adminLimiter)).Get("/api/v1/nvr/{deviceID}/channels/{channel}/controls", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}

		controlCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		capabilities, err := actions.NVRChannelControlCapabilities(controlCtx, chi.URLParam(r, "deviceID"), channel)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, capabilities)
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/ptz", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		request, err := parseNVRPTZRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		request.Channel = channel

		controlCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		if err := actions.ControlNVRPTZ(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"device_id":   chi.URLParam(r, "deviceID"),
			"channel":     channel,
			"action":      request.Action,
			"command":     request.Command,
			"speed":       request.Speed,
			"duration_ms": request.Duration.Milliseconds(),
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/aux", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		request, err := parseNVRAuxRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		request.Channel = channel

		controlCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		if err := actions.ControlNVRAux(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"device_id":   chi.URLParam(r, "deviceID"),
			"channel":     channel,
			"action":      request.Action,
			"output":      request.Output,
			"duration_ms": request.Duration.Milliseconds(),
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/audio/mute", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}
		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		muted, err := parseVTOMuteRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}
		controlCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := actions.ControlNVRAudio(controlCtx, chi.URLParam(r, "deviceID"), dahua.NVRAudioRequest{
			Channel: channel,
			Muted:   muted,
		}); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":               "ok",
			"device_id":            chi.URLParam(r, "deviceID"),
			"channel":              channel,
			"muted":                muted,
			"stream_audio_enabled": !muted,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/recording", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		request, err := parseNVRRecordingRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		request.Channel = channel

		controlCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		if err := actions.ControlNVRRecording(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": chi.URLParam(r, "deviceID"),
			"channel":   channel,
			"action":    request.Action,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		request, err := parseNVRDiagnosticActionRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		request.Channel = channel

		controlCtx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()

		result, err := actions.NVRDiagnosticAction(controlCtx, chi.URLParam(r, "deviceID"), request)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/nvr/{deviceID}/playback/sessions", func(w http.ResponseWriter, r *http.Request) {
		request, err := parseNVRPlaybackSessionRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		session, err := snapshots.CreateNVRPlaybackSession(r.Context(), chi.URLParam(r, "deviceID"), request)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/nvr/playback/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		session, err := snapshots.GetNVRPlaybackSession(chi.URLParam(r, "sessionID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/nvr/playback/sessions/{sessionID}/seek", func(w http.ResponseWriter, r *http.Request) {
		seekTime, err := parseNVRPlaybackSeekTime(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		session, err := snapshots.SeekNVRPlaybackSession(r.Context(), chi.URLParam(r, "sessionID"), seekTime)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
	router.Get("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if events == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event buffer is not configured"})
			return
		}

		limit, err := parseOptionalPositiveInt(r.URL.Query().Get("limit"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
		childID := strings.TrimSpace(r.URL.Query().Get("child_id"))
		deviceKind := dahua.DeviceKind(strings.TrimSpace(r.URL.Query().Get("device_kind")))
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		action := strings.TrimSpace(r.URL.Query().Get("action"))
		writeJSON(w, http.StatusOK, map[string]any{
			"stats":  events.EventStats(),
			"events": events.ListEvents(deviceID, childID, deviceKind, code, action, limit),
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Delete("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if events == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event buffer is not configured"})
			return
		}

		removed := events.ClearEvents()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":        "ok",
			"removed_count": removed,
			"stats":         events.EventStats(),
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/vto/{deviceID}/locks/{lockIndex}/unlock", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		lockIndex, err := strconv.Atoi(chi.URLParam(r, "lockIndex"))
		if err != nil || lockIndex < 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid lock index")
			return
		}

		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.UnlockVTOLock(controlCtx, deviceID, lockIndex); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"device_id":  deviceID,
			"lock_index": lockIndex,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/vto/{deviceID}/call/answer", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.AnswerVTOCall(controlCtx, deviceID); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": deviceID,
			"action":    "answer_call",
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/vto/{deviceID}/call/hangup", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.HangupVTOCall(controlCtx, deviceID); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": deviceID,
			"action":    "hangup_call",
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Get("/api/v1/vto/{deviceID}/controls", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		controlCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		capabilities, err := actions.VTOControlCapabilities(controlCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, capabilities)
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/vto/{deviceID}/audio/output-volume", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		level, slot, err := parseVTOVolumeRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.SetVTOAudioOutputVolume(controlCtx, deviceID, slot, level); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": deviceID,
			"slot":      slot,
			"level":     level,
			"target":    "output_volume",
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/vto/{deviceID}/audio/input-volume", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		level, slot, err := parseVTOVolumeRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.SetVTOAudioInputVolume(controlCtx, deviceID, slot, level); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": deviceID,
			"slot":      slot,
			"level":     level,
			"target":    "input_volume",
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/vto/{deviceID}/audio/mute", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		muted, err := parseVTOMuteRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.SetVTOMute(controlCtx, deviceID, muted); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": deviceID,
			"muted":     muted,
			"target":    "mute",
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/vto/{deviceID}/recording", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		enabled, err := parseVTORecordingRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.SetVTORecordingEnabled(controlCtx, deviceID, enabled); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":              "ok",
			"device_id":           deviceID,
			"auto_record_enabled": enabled,
		})
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/vto/{deviceID}/intercom", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}

		profileName := strings.TrimSpace(r.URL.Query().Get("profile"))
		if profileName == "" {
			profileName = entry.RecommendedProfile
		}
		if profileName == "" {
			profileName = "stable"
		}

		profile, ok := entry.Profiles[profileName]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream profile not found"})
			return
		}

		body := renderVTOIntercomPage(entry, profileName, profile, media.WebRTCICEServers())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.Get("/api/v1/vto/{deviceID}/intercom/status", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}

		writeJSON(w, http.StatusOK, media.IntercomStatus(entry.ID))
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/vto/{deviceID}/intercom/reset", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}

		writeJSON(w, http.StatusOK, media.StopIntercomSessions(entry.ID))
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/vto/{deviceID}/intercom/uplink/enable", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}
		if entry.Intercom == nil || !entry.Intercom.SupportsExternalAudioExport {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "external uplink export is not configured"})
			return
		}

		writeJSON(w, http.StatusOK, media.SetIntercomUplinkEnabled(entry.ID, true))
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/vto/{deviceID}/intercom/uplink/disable", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}
		if entry.Intercom == nil || !entry.Intercom.SupportsExternalAudioExport {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "external uplink export is not configured"})
			return
		}

		writeJSON(w, http.StatusOK, media.SetIntercomUplinkEnabled(entry.ID, false))
	})
	router.Get("/api/v1/home-assistant/native/catalog", func(w http.ResponseWriter, r *http.Request) {
		includeCredentials := r.URL.Query().Get("include_credentials") == "true"
		writeJSON(w, http.StatusOK, ha.BuildNativeCatalog(probes.List(), snapshots.ListStreams(includeCredentials)))
	})
	router.Get("/api/v1/streams", func(w http.ResponseWriter, r *http.Request) {
		includeCredentials := r.URL.Query().Get("include_credentials") == "true"
		entries := snapshots.ListStreams(includeCredentials)
		if deviceID := strings.TrimSpace(r.URL.Query().Get("device_id")); deviceID != "" {
			filtered := make([]streams.Entry, 0, len(entries))
			for _, entry := range entries {
				if entry.RootDeviceID == deviceID || entry.SourceDeviceID == deviceID || entry.ID == deviceID {
					filtered = append(filtered, entry)
				}
			}
			entries = filtered
		}
		writeJSON(w, http.StatusOK, entries)
	})
	router.Get("/api/v1/streams/{streamID}", func(w http.ResponseWriter, r *http.Request) {
		includeCredentials := r.URL.Query().Get("include_credentials") == "true"
		streamID := chi.URLParam(r, "streamID")
		for _, entry := range snapshots.ListStreams(includeCredentials) {
			if entry.ID == streamID {
				writeJSON(w, http.StatusOK, entry)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream not found"})
	})
	router.Get("/api/v1/media/workers", func(w http.ResponseWriter, r *http.Request) {
		if media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, media.ListWorkers())
	})
	router.With(rateLimitMiddleware(snapshotLimiter)).Get("/api/v1/media/snapshot/{streamID}", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		scaleWidth, err := parseOptionalPositiveInt(r.URL.Query().Get("width"))
		if err != nil {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		body, contentType, err := media.CaptureFrame(r.Context(), chi.URLParam(r, "streamID"), strings.TrimSpace(r.URL.Query().Get("profile")), scaleWidth)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		if contentType == "" {
			contentType = "image/jpeg"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/media/streams/{streamID}/recordings", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		request, err := parseClipStartRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}
		request.StreamID = chi.URLParam(r, "streamID")

		clip, err := media.StartClip(r.Context(), request)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, clipAPIResponse(r, clip))
	})
	router.Get("/api/v1/media/recordings", func(w http.ResponseWriter, r *http.Request) {
		if media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}

		query, err := parseClipQuery(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}
		clips, err := media.FindClips(query)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		payload := make([]map[string]any, 0, len(clips))
		for _, clip := range clips {
			payload = append(payload, clipAPIResponse(r, clip))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":          payload,
			"returned_count": len(payload),
		})
	})
	router.Get("/api/v1/media/recordings/{clipID}", func(w http.ResponseWriter, r *http.Request) {
		if media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}
		clip, err := media.GetClip(chi.URLParam(r, "clipID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, clipAPIResponse(r, clip))
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/media/recordings/{clipID}/stop", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}
		clip, err := media.StopClip(r.Context(), chi.URLParam(r, "clipID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, clipAPIResponse(r, clip))
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/media/recordings/{clipID}/download", func(w http.ResponseWriter, r *http.Request) {
		if media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}
		path, err := media.ClipFilePath(chi.URLParam(r, "clipID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
		http.ServeFile(w, r, path)
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/media/preview/{streamID}", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		streamID := chi.URLParam(r, "streamID")
		entry, ok := findStreamEntry(snapshots.ListStreams(false), streamID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream not found"})
			return
		}

		profileName := strings.TrimSpace(r.URL.Query().Get("profile"))
		if profileName == "" {
			profileName = entry.RecommendedProfile
		}
		if profileName == "" {
			profileName = "stable"
		}

		selectedProfile, ok := entry.Profiles[profileName]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}

		body := renderMediaPreviewPage(entry, profileName, selectedProfile)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/media/webrtc/{streamID}/{profile}", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		streamID := chi.URLParam(r, "streamID")
		profileName := chi.URLParam(r, "profile")
		entry, ok := findStreamEntry(snapshots.ListStreams(false), streamID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream not found"})
			return
		}
		profile, ok := entry.Profiles[profileName]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}

		body := renderWebRTCPage(entry, profileName, profile, media.WebRTCICEServers())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Post("/api/v1/media/webrtc/{streamID}/{profile}/offer", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)

		var offer mediaapi.WebRTCSessionDescription
		if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if strings.TrimSpace(offer.Type) == "" || strings.TrimSpace(offer.SDP) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "webrtc offer is missing type or sdp"})
			return
		}

		offerCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		answer, err := media.WebRTCAnswer(offerCtx, chi.URLParam(r, "streamID"), chi.URLParam(r, "profile"), offer)
		if err != nil {
			if errors.Is(err, mediaapi.ErrWorkerLimitReached) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, answer)
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/media/mjpeg/{streamID}", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		streamID := chi.URLParam(r, "streamID")
		profile := strings.TrimSpace(r.URL.Query().Get("profile"))
		if profile == "" {
			profile = "stable"
		}
		scaleWidth := 0
		if rawWidth := strings.TrimSpace(r.URL.Query().Get("width")); rawWidth != "" {
			parsedWidth, err := strconv.Atoi(rawWidth)
			if err != nil || parsedWidth < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid width"})
				return
			}
			scaleWidth = parsedWidth
		}

		frames, unsubscribe, err := media.SubscribeScaled(r.Context(), streamID, profile, scaleWidth)
		if err != nil {
			if errors.Is(err, mediaapi.ErrWorkerLimitReached) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		defer unsubscribe()

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported by the response writer"})
			return
		}

		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.WriteHeader(http.StatusOK)

		for {
			select {
			case <-r.Context().Done():
				return
			case frame, ok := <-frames:
				if !ok {
					return
				}
				if len(frame) == 0 {
					continue
				}
				if _, err := fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame)); err != nil {
					return
				}
				if _, err := w.Write(frame); err != nil {
					return
				}
				if _, err := w.Write([]byte("\r\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/media/hls/{streamID}/{profile}/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		body, err := media.HLSPlaylist(r.Context(), chi.URLParam(r, "streamID"), chi.URLParam(r, "profile"))
		if err != nil {
			if errors.Is(err, mediaapi.ErrWorkerLimitReached) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	router.With(rateLimitMiddleware(mediaLimiter)).Get("/api/v1/media/hls/{streamID}/{profile}/{segmentName}", func(w http.ResponseWriter, r *http.Request) {
		if media == nil || !media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		body, contentType, err := media.HLSSegment(r.Context(), chi.URLParam(r, "streamID"), chi.URLParam(r, "profile"), chi.URLParam(r, "segmentName"))
		if err != nil {
			if errors.Is(err, mediaapi.ErrWorkerLimitReached) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	router.With(rateLimitMiddleware(snapshotLimiter)).Get("/api/v1/nvr/{deviceID}/channels/{channel}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid channel"})
			return
		}

		body, contentType, err := snapshots.NVRSnapshot(r.Context(), deviceID, channel)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		if contentType == "" {
			contentType = "image/jpeg"
		}

		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	router.With(rateLimitMiddleware(snapshotLimiter)).Get("/api/v1/vto/{deviceID}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		body, contentType, err := snapshots.VTOSnapshot(r.Context(), deviceID)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		if contentType == "" {
			contentType = "image/jpeg"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	router.With(rateLimitMiddleware(snapshotLimiter)).Get("/api/v1/ipc/{deviceID}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		body, contentType, err := snapshots.IPCSnapshot(r.Context(), deviceID)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		if contentType == "" {
			contentType = "image/jpeg"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.ListenAddress,
			Handler:      router,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  cfg.IdleTimeout,
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
	return s.httpServer.Shutdown(ctx)
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
		ProfileName: strings.TrimSpace(request.ProfileName),
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

	startTime, err := parseFlexibleTimestamp(strings.TrimSpace(r.URL.Query().Get("start")), "start")
	if err != nil {
		return dahua.NVRRecordingQuery{}, err
	}
	endTime, err := parseFlexibleTimestamp(strings.TrimSpace(r.URL.Query().Get("end")), "end")
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

	return dahua.NVRRecordingQuery{
		Channel:   channel,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     limit,
		EventCode: firstNonEmptyQueryValue(r.URL.Query(), "event", "event_type", "event_code"),
		EventOnly: parseQueryBool(r.URL.Query(), "event_only", "events_only"),
	}, nil
}

type nvrEventSummaryQuery struct {
	StartTime time.Time
	EndTime   time.Time
	EventCode string
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

	return nvrEventSummaryQuery{
		StartTime: startTime,
		EndTime:   endTime,
		EventCode: firstNonEmptyQueryValue(r.URL.Query(), "event", "event_type", "event_code"),
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
		Channel   int    `json:"channel"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
		SeekTime  string `json:"seek_time"`
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
		Channel:   request.Channel,
		StartTime: startTime,
		EndTime:   endTime,
		SeekTime:  seekTime,
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
		Profile         string
		DurationSeconds *int
		DurationMS      *int
	}{
		StartTime: firstNonEmptyQueryValue(values, "start_time", "start"),
		EndTime:   firstNonEmptyQueryValue(values, "end_time", "end"),
		SeekTime:  firstNonEmptyQueryValue(values, "seek_time", "seek"),
		Profile:   strings.TrimSpace(values.Get("profile")),
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
		Channel:   request.Channel,
		StartTime: startTime,
		EndTime:   endTime,
		SeekTime:  seekTime,
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

func parseVTOVolumeRequest(r *http.Request) (int, int, error) {
	if r.Body == nil {
		return 0, 0, fmt.Errorf("json body is required")
	}

	var request struct {
		Level int `json:"level"`
		Slot  int `json:"slot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, 0, fmt.Errorf("json body is required")
		}
		return 0, 0, fmt.Errorf("invalid json body")
	}
	if request.Level < 0 || request.Level > 100 {
		return 0, 0, fmt.Errorf("invalid level")
	}
	if request.Slot < 0 {
		return 0, 0, fmt.Errorf("invalid slot")
	}
	return request.Level, request.Slot, nil
}

func parseVTOMuteRequest(r *http.Request) (bool, error) {
	if r.Body == nil {
		return false, fmt.Errorf("json body is required")
	}

	var request struct {
		Muted *bool `json:"muted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return false, fmt.Errorf("json body is required")
		}
		return false, fmt.Errorf("invalid json body")
	}
	if request.Muted == nil {
		return false, fmt.Errorf("muted is required")
	}
	return *request.Muted, nil
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
	}
	for _, layout := range layouts {
		var (
			parsed time.Time
			err    error
		)
		switch layout {
		case "2006-01-02 15:04:05", "2006-01-02T15:04:05":
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
		"download_url":     buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clip.ID)+"/download"),
		"self_url":         buildAbsoluteRequestURL(r, "/api/v1/media/recordings/"+url.PathEscape(clip.ID)),
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
		item.ExportURL = buildNVRRecordingExportURL(r, deviceID, channel, startTime, endTime)
	}
}

func buildNVREventSummary(ctx context.Context, snapshots SnapshotReader, deviceID string, query nvrEventSummaryQuery) (dahua.NVREventSummary, error) {
	summary := dahua.NVREventSummary{
		DeviceID:  strings.TrimSpace(deviceID),
		StartTime: query.StartTime.Format(time.RFC3339),
		EndTime:   query.EndTime.Format(time.RFC3339),
		Items:     []dahua.NVREventSummaryItem{},
		Channels:  []dahua.NVREventChannelSummary{},
	}
	if snapshots == nil {
		return summary, fmt.Errorf("snapshot reader is not configured")
	}

	channels := nvrSummaryChannels(snapshots.ListStreams(false), summary.DeviceID)
	if len(channels) == 0 {
		return summary, nil
	}

	const summaryLimitPerChannel = 2000

	totalByCode := make(map[string]int)
	channelSummaries := make([]dahua.NVREventChannelSummary, 0, len(channels))
	for _, channel := range channels {
		result, err := snapshots.NVRRecordings(ctx, summary.DeviceID, dahua.NVRRecordingQuery{
			Channel:   channel,
			StartTime: query.StartTime,
			EndTime:   query.EndTime,
			Limit:     summaryLimitPerChannel,
			EventCode: query.EventCode,
			EventOnly: true,
		})
		if err != nil {
			return dahua.NVREventSummary{}, err
		}

		channelByCode := make(map[string]int)
		for _, item := range result.Items {
			code := nvrSummaryCodeForRecording(item)
			if code == "" {
				continue
			}
			channelByCode[code]++
			totalByCode[code]++
			summary.TotalCount++
		}
		channelItems := makeNVREventSummaryItems(channelByCode)
		channelSummaries = append(channelSummaries, dahua.NVREventChannelSummary{
			Channel:    channel,
			TotalCount: countNVREventSummaryItems(channelItems),
			Items:      channelItems,
		})
	}

	sort.Slice(channelSummaries, func(i, j int) bool {
		return channelSummaries[i].Channel < channelSummaries[j].Channel
	})
	summary.Items = makeNVREventSummaryItems(totalByCode)
	summary.Channels = channelSummaries
	return summary, nil
}

func nvrSummaryChannels(entries []streams.Entry, deviceID string) []int {
	seen := make(map[int]struct{})
	channels := make([]int, 0)
	for _, entry := range entries {
		if entry.RootDeviceID != deviceID || entry.DeviceKind != dahua.DeviceKindNVRChannel || entry.Channel <= 0 {
			continue
		}
		if _, ok := seen[entry.Channel]; ok {
			continue
		}
		seen[entry.Channel] = struct{}{}
		channels = append(channels, entry.Channel)
	}
	sort.Ints(channels)
	return channels
}

func makeNVREventSummaryItems(countByCode map[string]int) []dahua.NVREventSummaryItem {
	items := make([]dahua.NVREventSummaryItem, 0, len(countByCode))
	for code, count := range countByCode {
		if count <= 0 {
			continue
		}
		items = append(items, dahua.NVREventSummaryItem{
			Code:  code,
			Label: nvrEventSummaryLabel(code),
			Count: count,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Code < items[j].Code
		}
		return items[i].Count > items[j].Count
	})
	return items
}

func countNVREventSummaryItems(items []dahua.NVREventSummaryItem) int {
	total := 0
	for _, item := range items {
		total += item.Count
	}
	return total
}

func nvrSummaryCodeForRecording(item dahua.NVRRecording) string {
	candidates := make([]string, 0, len(item.Flags)+1)
	candidates = append(candidates, item.Type)
	candidates = append(candidates, item.Flags...)
	for _, candidate := range candidates {
		normalized := normalizeNVREventSummaryCode(candidate)
		if normalized != "" && !strings.EqualFold(normalized, "event") {
			return normalized
		}
	}
	return ""
}

func normalizeNVREventSummaryCode(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "event.") {
		trimmed = strings.TrimSpace(trimmed[len("event."):])
	}
	return trimmed
}

func nvrEventSummaryLabel(value string) string {
	switch strings.ToLower(normalizeNVREventSummaryCode(value)) {
	case "motion", "videomotion", "movedetection", "alarmpir":
		return "Motion"
	case "human", "humandetection", "smartmotionhuman", "intelliframehuman", "smdtypehuman":
		return "Human"
	case "vehicle", "vehicledetection", "smartmotionvehicle", "motorvehicle", "smdtypevehicle":
		return "Vehicle"
	case "animal", "animaldetection", "smdtypeanimal":
		return "Animal"
	case "crosslinedetection", "tripwire":
		return "Cross Line"
	case "crossregiondetection", "intrusion":
		return "Cross Region"
	case "leftdetection":
		return "Left Detection"
	case "access", "accesscontrol", "accessctl":
		return "Access"
	default:
		return humanizeNVREventSummaryValue(value)
	}
}

func humanizeNVREventSummaryValue(value string) string {
	return strings.TrimSpace(strings.NewReplacer("_", " ", "-", " ").Replace(value))
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

func buildNVRRecordingExportURL(r *http.Request, deviceID string, channel int, startTime string, endTime string) string {
	query := url.Values{
		"channel":    []string{strconv.Itoa(channel)},
		"start_time": []string{startTime},
		"end_time":   []string{endTime},
		"profile":    []string{"quality"},
	}
	path := "/api/v1/nvr/" + url.PathEscape(deviceID) + "/recordings/export?" + query.Encode()
	return buildAbsoluteRequestURL(r, path)
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

type adminEndpoint struct {
	Method      string
	Path        string
	Description string
	Linkable    bool
}

type testBridgeChannel struct {
	ID                 string                         `json:"id"`
	DeviceID           string                         `json:"device_id"`
	Name               string                         `json:"name"`
	Channel            int                            `json:"channel"`
	SnapshotURL        string                         `json:"snapshot_url"`
	RecommendedProfile string                         `json:"recommended_profile"`
	MainVideo          string                         `json:"main_video,omitempty"`
	SubVideo           string                         `json:"sub_video,omitempty"`
	AudioCodec         string                         `json:"audio_codec,omitempty"`
	Profiles           []testBridgeProfile            `json:"profiles"`
	Controls           *streams.ChannelControlSummary `json:"controls,omitempty"`
	Features           []streams.FeatureSummary       `json:"features,omitempty"`
}

type testBridgeProfile struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	SnapshotURL string `json:"snapshot_url"`
	MJPEGURL    string `json:"mjpeg_url"`
	HLSURL      string `json:"hls_url"`
	PreviewURL  string `json:"preview_url"`
	WebRTCURL   string `json:"webrtc_url"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

func renderAdminPage(
	status httpStatus,
	probeResults []*dahua.ProbeResult,
	streamEntries []streams.Entry,
	settings map[string]any,
	eventStats map[string]any,
	workerStatuses []mediaapi.WorkerStatus,
	actionsAvailable bool,
	eventsAvailable bool,
	mediaEnabled bool,
	healthPath string,
	metricsPath string,
) string {
	endpointSections := buildAdminEndpointSections(healthPath, metricsPath)
	controlStats := summarizeAdminControlStats(streamEntries)
	deviceCards := buildAdminDeviceCards(probeResults, streamEntries)
	streamCards := buildAdminStreamCards(streamEntries)
	settingsJSON := htmlEscape(marshalIndentedJSON(settings))
	eventStatsJSON := htmlEscape(marshalIndentedJSON(eventStats))
	workerJSON := htmlEscape(marshalIndentedJSON(workerStatuses))
	deviceCount := len(probeResults)
	streamCount := len(streamEntries)
	workerCount := len(workerStatuses)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>DahuaBridge Admin</title>
  <link rel="stylesheet" href="/admin/assets/bootstrap.min.css">
  <style>
    :root {
      color-scheme: dark;
      --bg: #0c1114;
      --bg-soft: #151e21;
      --panel: #111a1d;
      --line: rgba(153, 174, 166, 0.2);
      --text: #f1f5f2;
      --muted: #a8b8b2;
      --accent: #5ed0ac;
      --accent-soft: rgba(94, 208, 172, 0.14);
      --warm: #d9a441;
      --danger: #ef747b;
      --shadow: 0 12px 32px rgba(0, 0, 0, 0.24);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Segoe UI", Tahoma, Geneva, Verdana, sans-serif;
      color: var(--text);
      background: linear-gradient(180deg, #151e21 0%%, #0c1114 72%%);
    }
    main {
      max-width: 1320px;
      margin: 0 auto;
      padding: 28px;
      display: grid;
      gap: 18px;
    }
    .hero {
      display: grid;
      gap: 18px;
      grid-template-columns: auto minmax(0, 1fr);
      align-items: center;
      padding: 26px 28px;
      border-radius: 8px;
      background: #121d20;
      border: 1px solid var(--line);
      box-shadow: var(--shadow);
    }
    .hero-mark {
      width: 104px;
      height: 104px;
      padding: 12px;
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      background: rgba(255,255,255,0.06);
      border: 1px solid rgba(255,255,255,0.08);
      box-shadow: inset 0 1px 0 rgba(255,255,255,0.08);
    }
    .hero-mark img {
      width: 100%%;
      height: 100%%;
      object-fit: contain;
      filter: drop-shadow(0 14px 18px rgba(0,0,0,0.30));
    }
    .hero-copy {
      display: grid;
      gap: 10px;
    }
    .eyebrow {
      display: inline-flex;
      width: fit-content;
      padding: 6px 12px;
      border-radius: 8px;
      background: var(--accent-soft);
      color: var(--accent);
      letter-spacing: 0;
      text-transform: uppercase;
      font-size: 12px;
    }
    h1 {
      margin: 0;
      font-size: clamp(34px, 5vw, 60px);
      line-height: 0.94;
      font-weight: 700;
      letter-spacing: 0;
    }
    .lead {
      margin: 0;
      max-width: 76ch;
      color: var(--muted);
      font-size: 17px;
      line-height: 1.45;
    }
    .grid {
      display: grid;
      gap: 18px;
    }
    .summary-grid {
      display: grid;
      gap: 14px;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    }
    .summary-card, .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
    }
    .summary-card {
      padding: 18px;
      display: grid;
      gap: 8px;
    }
    .summary-label {
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: 0;
      font-size: 12px;
    }
    .summary-value {
      font-size: 30px;
      line-height: 1;
      font-weight: 600;
    }
    .summary-subtle {
      color: var(--muted);
      font-size: 14px;
    }
    .panel {
      padding: 20px;
      display: grid;
      gap: 16px;
    }
    .panel h2 {
      margin: 0;
      font-size: 22px;
      font-weight: 600;
    }
    .panel p {
      margin: 0;
      color: var(--muted);
      line-height: 1.4;
    }
    .layout {
      display: grid;
      grid-template-columns: minmax(0, 1.2fr) minmax(0, 0.8fr);
      gap: 18px;
    }
    .stack {
      display: grid;
      gap: 18px;
    }
    .action-row {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    .action-row .btn {
      appearance: none;
      border-radius: 8px;
      padding: 12px 16px;
      font-weight: 600;
      box-shadow: 0 12px 28px rgba(0, 0, 0, 0.18);
    }
    .action-row .btn-success {
      color: #08100d;
      background: var(--accent);
      border-color: rgba(150, 240, 203, 0.22);
    }
    .action-row .btn-outline-light {
      color: var(--text);
      border-color: rgba(255,255,255,0.22);
      background: rgba(255,255,255,0.02);
    }
    .action-row .btn-warning {
      color: #241600;
      background: var(--warm);
      border-color: rgba(255, 210, 120, 0.24);
    }
    .action-row .btn-outline-danger {
      border-color: rgba(255, 140, 148, 0.38);
    }
    .action-row .btn:disabled {
      cursor: not-allowed;
      opacity: 0.55;
    }
    .result-box, pre {
      margin: 0;
      padding: 14px 16px;
      border-radius: 8px;
      background: rgba(0,0,0,0.22);
      border: 1px solid rgba(255,255,255,0.06);
      color: var(--text);
      overflow: auto;
      font-family: Consolas, "Courier New", monospace;
      font-size: 13px;
      line-height: 1.5;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .endpoint-group, .card-grid {
      display: grid;
      gap: 12px;
    }
    .endpoint-list {
      display: grid;
      gap: 8px;
    }
    .endpoint-row, .device-card, .stream-card {
      display: grid;
      gap: 8px;
      padding: 14px 16px;
      border-radius: 8px;
      background: rgba(255,255,255,0.02);
      border: 1px solid rgba(255,255,255,0.06);
    }
    .endpoint-row {
      grid-template-columns: auto minmax(0, 1fr);
      align-items: center;
      column-gap: 12px;
    }
    .method {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      min-width: 64px;
      padding: 6px 8px;
      border-radius: 8px;
      background: rgba(255,255,255,0.07);
      color: var(--accent);
      font-size: 12px;
      letter-spacing: 0;
      text-transform: uppercase;
    }
    .endpoint-main {
      display: grid;
      gap: 4px;
    }
    .endpoint-main a, .chip a {
      color: var(--text);
      text-decoration: none;
    }
    .endpoint-main code {
      color: var(--text);
      font-size: 13px;
    }
    .endpoint-desc {
      color: var(--muted);
      font-size: 14px;
    }
    .card-grid {
      grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    }
    .card-title {
      margin: 0;
      font-size: 19px;
      font-weight: 600;
    }
    .card-meta {
      color: var(--muted);
      font-size: 14px;
    }
    .chip-row {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }
    .chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 8px 10px;
      border-radius: 8px;
      background: rgba(150, 240, 203, 0.08);
      border: 1px solid rgba(150, 240, 203, 0.12);
      font-size: 13px;
    }
    .chip code {
      color: var(--text);
    }
    .chip.subtle {
      background: rgba(255,255,255,0.04);
      border-color: rgba(255,255,255,0.08);
    }
    .muted-note {
      color: var(--muted);
      font-size: 14px;
    }
    @media (max-width: 1120px) {
      .layout, .summary-grid {
        grid-template-columns: 1fr 1fr;
      }
      .hero {
        grid-template-columns: 1fr;
      }
    }
    @media (max-width: 760px) {
      main {
        padding: 18px;
      }
      .layout, .summary-grid {
        grid-template-columns: 1fr;
      }
      .action-row {
        flex-direction: column;
      }
      .action-row .btn {
        width: 100%%;
      }
      .hero-mark {
        width: 88px;
        height: 88px;
      }
    }
  </style>
</head>
<body data-bs-theme="dark">
  <main class="container-xxl py-4">
    <section class="hero">
      <div class="hero-mark">
        <img src="/admin/assets/logo.png" alt="DahuaBridge logo">
      </div>
      <div class="hero-copy">
        <div class="eyebrow">DahuaBridge Admin</div>
        <h1>Operator Surface</h1>
        <p class="lead">This page exposes the real bridge control surface in one place: health and runtime summaries, concrete endpoint links, redacted config, stream and device shortcuts, and the highest-value mutating actions without needing to remember URLs.</p>
      </div>
    </section>

    <section class="summary-grid">
      <article class="summary-card">
        <div class="summary-label">Readiness</div>
        <div class="summary-value">%s</div>
        <div class="summary-subtle">%s</div>
      </article>
      <article class="summary-card">
        <div class="summary-label">Devices</div>
        <div class="summary-value">%d</div>
        <div class="summary-subtle">Last update: %s</div>
      </article>
      <article class="summary-card">
        <div class="summary-label">Streams</div>
        <div class="summary-value">%d</div>
        <div class="summary-subtle">Concrete preview/intercom links listed below.</div>
      </article>
      <article class="summary-card">
        <div class="summary-label">Media Workers</div>
        <div class="summary-value">%d</div>
        <div class="summary-subtle">Media enabled: %t</div>
      </article>
      <article class="summary-card">
        <div class="summary-label">Control Surface</div>
        <div class="summary-value">%d</div>
        <div class="summary-subtle">%s</div>
      </article>
    </section>

    <section class="layout">
      <div class="stack">
        <section class="panel">
          <h2>Admin Actions</h2>
          <p>These buttons call the same authenticated bridge endpoints the rest of the system uses. Responses are shown inline exactly as the API returns them.</p>
            <div class="action-row">
              <a class="btn btn-outline-light" href="/admin/test-bridge">Open Bridge Test Page</a>
              <button type="button" class="btn btn-success" data-method="POST" data-url="/api/v1/devices/probe-all" data-success="Probe-all requested." %s>Probe All Devices</button>
              <button type="button" class="btn btn-outline-danger" data-method="DELETE" data-url="/api/v1/events" data-success="Event buffer clear requested." %s>Clear Event Buffer</button>
            </div>
          <pre id="admin-action-result" class="result-box">No action has been run yet.</pre>
        </section>

        <section class="panel">
          <h2>Endpoint Inventory</h2>
          <p>Generic bridge routes are grouped here. Use the device and stream sections below for concrete per-device links.</p>
          %s
        </section>

        <section class="panel">
          <h2>Discovered Devices</h2>
          <p>Root-device links are built from the current probe state and stream inventory.</p>
          %s
        </section>

        <section class="panel">
          <h2>Streams</h2>
          <p>These are concrete preview, WebRTC, HLS, snapshot, and intercom shortcuts from the current stream catalog.</p>
          %s
        </section>
      </div>

      <div class="stack">
        <section class="panel">
          <h2>Redacted Settings</h2>
          <p>Passwords, access tokens, ICE credentials, and ONVIF passwords are redacted before reaching this page.</p>
          <pre>%s</pre>
        </section>

        <section class="panel">
          <h2>Event Buffer Stats</h2>
          <pre>%s</pre>
        </section>

        <section class="panel">
          <h2>Media Worker Status</h2>
          <pre>%s</pre>
        </section>
      </div>
    </section>
  </main>
  <script>
    const resultBox = document.getElementById('admin-action-result');
    const actionButtons = Array.from(document.querySelectorAll('[data-method][data-url]'));

    function setResult(title, payload, isError) {
      resultBox.textContent = title + "\n\n" + payload;
      resultBox.style.borderColor = isError ? 'rgba(255, 140, 148, 0.35)' : 'rgba(150, 240, 203, 0.22)';
    }

    async function runAdminAction(button) {
      const method = button.getAttribute('data-method') || 'GET';
      const url = button.getAttribute('data-url');
      const body = button.getAttribute('data-body');
      const success = button.getAttribute('data-success') || 'Action finished.';
      const previous = button.textContent;
      button.disabled = true;
      setResult('Running ' + method + ' ' + url, 'Please wait...', false);
      try {
        const response = await fetch(url, {
          method,
          headers: body ? { 'Content-Type': 'application/json' } : undefined,
          body: body || undefined,
        });
        const text = await response.text();
        if (!response.ok) {
          throw new Error(text || response.statusText || 'request failed');
        }
        setResult(success, text || '(empty response)', false);
      } catch (error) {
        setResult('Action failed', error && error.message ? error.message : String(error), true);
      } finally {
        button.disabled = false;
        button.textContent = previous;
      }
    }

    for (const button of actionButtons) {
      button.addEventListener('click', () => {
        runAdminAction(button);
      });
    }
  </script>
</body>
</html>`,
		boolText(status.Ready, "Ready", "Not Ready"),
		htmlEscape(firstNonEmpty(status.LastUpdatedAt, "No successful probe yet.")),
		deviceCount,
		htmlEscape(firstNonEmpty(status.LastUpdatedAt, "unknown")),
		streamCount,
		workerCount,
		mediaEnabled,
		controlStats.ActionableEntries,
		htmlEscape(controlStats.Summary()),
		boolHTMLAttr(actionsAvailable),
		boolHTMLAttr(eventsAvailable),
		endpointSections,
		deviceCards,
		streamCards,
		settingsJSON,
		eventStatsJSON,
		workerJSON,
	)
}

func renderAdminTestBridgePage(streamEntries []streams.Entry, actionsAvailable bool, mediaEnabled bool) string {
	channels := buildTestBridgeChannels(streamEntries)
	channelsJSON := marshalJSONForScript(channels)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>DahuaBridge Bridge Test</title>
  <link rel="stylesheet" href="/admin/assets/bootstrap.min.css">
  <style>
    :root {
      color-scheme: dark;
      --bg: #101316;
      --panel: #171d20;
      --panel-soft: #20272b;
      --line: rgba(220, 230, 224, 0.16);
      --text: #f4f7f5;
      --muted: #aeb8b3;
      --accent: #62d0aa;
      --accent-2: #d7a64a;
      --danger: #eb747a;
      --ok: rgba(98, 208, 170, 0.16);
      --warn: rgba(215, 166, 74, 0.16);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Segoe UI", Tahoma, Geneva, Verdana, sans-serif;
      background: #101316;
      color: var(--text);
    }
    main {
      max-width: 1480px;
      margin: 0 auto;
      padding: 24px;
      display: grid;
      gap: 16px;
    }
    .topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      flex-wrap: wrap;
    }
    .topbar a {
      color: var(--text);
      text-decoration: none;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 8px 12px;
      background: rgba(255,255,255,0.03);
    }
    .hero {
      display: grid;
      gap: 10px;
      padding: 22px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
    }
    .eyebrow {
      color: var(--accent);
      text-transform: uppercase;
      letter-spacing: 0;
      font-size: 12px;
    }
    h1, h2, h3, p {
      margin: 0;
    }
    h1 {
      font-size: clamp(30px, 4vw, 52px);
      line-height: 1;
      letter-spacing: 0;
      font-weight: 700;
    }
    h2 {
      font-size: 19px;
      line-height: 1.2;
      font-weight: 650;
    }
    h3 {
      font-size: 15px;
      line-height: 1.2;
      font-weight: 650;
      color: var(--accent);
    }
    .muted {
      color: var(--muted);
      line-height: 1.45;
    }
    .status-strip {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
    }
    .pill {
      display: inline-flex;
      align-items: center;
      min-height: 32px;
      padding: 7px 10px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: rgba(255,255,255,0.035);
      color: var(--muted);
      font-size: 13px;
    }
    .pill.good {
      color: var(--accent);
      background: var(--ok);
    }
    .pill.warn {
      color: var(--accent-2);
      background: var(--warn);
    }
    .workspace {
      display: grid;
      grid-template-columns: minmax(280px, 360px) minmax(0, 1fr);
      gap: 16px;
      align-items: start;
    }
    .panel {
      display: grid;
      gap: 14px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
      padding: 16px;
      min-width: 0;
    }
    .sidebar {
      position: sticky;
      top: 16px;
    }
    label {
      display: grid;
      gap: 6px;
      color: var(--muted);
      font-size: 13px;
    }
    select, input {
      width: 100%%;
      min-height: 40px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: #0f1416;
      color: var(--text);
      padding: 8px 10px;
      font: inherit;
    }
    .viewer {
      min-height: 360px;
      aspect-ratio: 16 / 9;
      display: flex;
      align-items: center;
      justify-content: center;
      overflow: hidden;
      border-radius: 8px;
      border: 1px solid rgba(255,255,255,0.09);
      background: #050607;
    }
    .viewer img, .viewer video, .viewer iframe {
      width: 100%%;
      height: 100%%;
      border: 0;
      object-fit: contain;
      background: #050607;
    }
    .viewer iframe {
      object-fit: unset;
    }
    .viewer-empty {
      color: var(--muted);
      padding: 20px;
      text-align: center;
    }
    .button-row {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }
    button {
      min-height: 38px;
      border: 1px solid rgba(255,255,255,0.16);
      border-radius: 8px;
      background: var(--panel-soft);
      color: var(--text);
      padding: 8px 11px;
      font: inherit;
      font-weight: 600;
      cursor: pointer;
    }
    button.primary {
      background: var(--accent);
      color: #07110d;
      border-color: rgba(160, 240, 205, 0.3);
    }
    button.warn {
      background: var(--accent-2);
      color: #1d1300;
      border-color: rgba(255, 220, 150, 0.3);
    }
    button.danger {
      border-color: rgba(235, 116, 122, 0.45);
      color: #ffd7da;
    }
    button:disabled {
      cursor: not-allowed;
      opacity: 0.5;
    }
    .control-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
      gap: 16px;
    }
    pre {
      margin: 0;
      min-height: 120px;
      max-height: 420px;
      overflow: auto;
      padding: 12px;
      border-radius: 8px;
      border: 1px solid rgba(255,255,255,0.08);
      background: #0a0d0e;
      color: var(--text);
      font: 13px/1.45 Consolas, "Courier New", monospace;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .log {
      min-height: 260px;
    }
    @media (max-width: 980px) {
      main {
        padding: 16px;
      }
      .workspace {
        grid-template-columns: 1fr;
      }
      .sidebar {
        position: static;
      }
      .viewer {
        min-height: 240px;
      }
    }
  </style>
</head>
<body data-bs-theme="dark">
  <main>
    <nav class="topbar">
      <a href="/admin">Admin</a>
      <a href="/api/v1/streams">Streams JSON</a>
    </nav>

    <section class="hero">
      <div class="eyebrow">DahuaBridge Admin</div>
      <h1>Bridge Test Bench</h1>
      <p class="muted">Select one NVR channel, switch stream transports, then run bridge, NVR CGI, NVR RPC, and direct IPC control attempts from the same page.</p>
      <div class="status-strip">
        <span class="pill good">%d NVR channels</span>
        <span class="pill %s">media %s</span>
        <span class="pill %s">actions %s</span>
      </div>
    </section>

    <section class="workspace">
      <aside class="panel sidebar">
        <h2>Channel</h2>
        <label>Camera channel
          <select id="channel-select"></select>
        </label>
        <label>Stream profile
          <select id="profile-select"></select>
        </label>
        <pre id="channel-meta">No channel selected.</pre>
      </aside>

      <section class="panel">
        <h2>Video</h2>
        <div class="button-row">
          <button type="button" data-stream-mode="snapshot" class="primary">Snapshot</button>
          <button type="button" data-stream-mode="mjpeg">MJPEG</button>
          <button type="button" data-stream-mode="hls">HLS</button>
          <button type="button" data-stream-mode="preview">Preview Page</button>
          <button type="button" data-stream-mode="webrtc">WebRTC Page</button>
        </div>
        <div id="viewer" class="viewer"><div class="viewer-empty">Select a channel to load video.</div></div>
      </section>
    </section>

    <section class="control-grid">
      <section class="panel">
        <h2>Bridge Controls</h2>
        <div class="button-row">
          <button type="button" data-call="controls">Read Capabilities</button>
          <button type="button" data-call="probe">Probe Device</button>
          <button type="button" data-call="refresh">Refresh NVR Inventory</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="ptz" data-command="left">PTZ Left Pulse</button>
          <button type="button" data-call="ptz" data-command="right">PTZ Right Pulse</button>
          <button type="button" data-call="ptz" data-command="up">PTZ Up Pulse</button>
          <button type="button" data-call="ptz" data-command="down">PTZ Down Pulse</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="aux" data-output="light" data-action="start" class="warn">White Light On</button>
          <button type="button" data-call="aux" data-output="light" data-action="stop">White Light Off</button>
          <button type="button" data-call="aux" data-output="warning_light" data-action="start" class="warn">Warning Light On</button>
          <button type="button" data-call="aux" data-output="warning_light" data-action="stop">Warning Light Off</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="aux" data-output="aux" data-action="pulse" data-duration="800" class="danger">Siren Pulse</button>
          <button type="button" data-call="aux" data-output="aux" data-action="stop">Siren Stop</button>
          <button type="button" data-call="aux" data-output="wiper" data-action="pulse">Wiper Pulse</button>
          <button type="button" data-call="aux" data-output="wiper" data-action="stop">Wiper Stop</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="audio" data-muted="true">Mute Stream Audio</button>
          <button type="button" data-call="audio" data-muted="false">Unmute Stream Audio</button>
          <button type="button" data-call="recording" data-action="start">Recording Start</button>
          <button type="button" data-call="recording" data-action="stop">Recording Stop</button>
          <button type="button" data-call="recording" data-action="auto">Recording Auto</button>
        </div>
      </section>

      <section class="panel">
        <h2>Raw NVR PTZ CGI</h2>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_aux" data-action="start" class="danger">Aux Start</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_aux" data-action="stop">Aux Stop</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_aux" data-action="pulse" data-duration="800" class="danger">Aux Pulse</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_light" data-action="start" class="warn">Light Start</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_light" data-action="stop">Light Stop</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_light" data-action="pulse" data-duration="800">Light Pulse</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_wiper" data-action="start">Wiper Start</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_wiper" data-action="stop">Wiper Stop</button>
          <button type="button" data-call="diagnostic" data-method="nvr_ptz_wiper" data-action="pulse">Wiper Pulse</button>
        </div>
      </section>

      <section class="panel">
        <h2>Lighting APIs</h2>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="bridge_light" data-action="start" class="warn">Bridge Light On</button>
          <button type="button" data-call="diagnostic" data-method="bridge_light" data-action="stop">Bridge Light Off</button>
          <button type="button" data-call="diagnostic" data-method="bridge_warning_light" data-action="start" class="warn">Bridge Warning On</button>
          <button type="button" data-call="diagnostic" data-method="bridge_warning_light" data-action="stop">Bridge Warning Off</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="nvr_lighting_config" data-action="start" class="warn">NVR Lighting_V2 On</button>
          <button type="button" data-call="diagnostic" data-method="nvr_lighting_config" data-action="stop">NVR Lighting_V2 Off</button>
          <button type="button" data-call="diagnostic" data-method="nvr_video_input_light_param" data-action="start" class="warn">VideoIn Light On</button>
          <button type="button" data-call="diagnostic" data-method="nvr_video_input_light_param" data-action="stop">VideoIn Light Off</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="direct_ipc_lighting" data-action="start" class="warn">Direct IPC Lighting On</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_lighting" data-action="stop">Direct IPC Lighting Off</button>
        </div>
      </section>

      <section class="panel">
        <h2>Direct IPC PTZ CGI</h2>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_light" data-action="start" class="warn">Light ch1 Start</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_light" data-action="stop">Light ch1 Stop</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_light_ch0" data-action="start" class="warn">Light ch0 Start</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_light_ch0" data-action="stop">Light ch0 Stop</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_aux" data-action="pulse" data-duration="800" class="danger">Aux ch1 Pulse</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_aux_ch0" data-action="pulse" data-duration="800" class="danger">Aux ch0 Pulse</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_wiper" data-action="pulse">Wiper ch1 Pulse</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_ptz_wiper_ch0" data-action="pulse">Wiper ch0 Pulse</button>
        </div>
      </section>

      <section class="panel">
        <h2>Audio And Recording APIs</h2>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="bridge_audio" data-action="mute">Bridge Audio Mute</button>
          <button type="button" data-call="diagnostic" data-method="bridge_audio" data-action="unmute">Bridge Audio Unmute</button>
          <button type="button" data-call="diagnostic" data-method="nvr_audio_config" data-action="off">NVR Audio Off</button>
          <button type="button" data-call="diagnostic" data-method="nvr_audio_config" data-action="on">NVR Audio On</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_audio" data-action="off">Direct IPC Audio Off</button>
          <button type="button" data-call="diagnostic" data-method="direct_ipc_audio" data-action="on">Direct IPC Audio On</button>
        </div>
        <div class="button-row">
          <button type="button" data-call="diagnostic" data-method="record_mode" data-action="start">RecordMode Manual</button>
          <button type="button" data-call="diagnostic" data-method="record_mode" data-action="stop">RecordMode Stop</button>
          <button type="button" data-call="diagnostic" data-method="record_mode" data-action="auto">RecordMode Auto</button>
        </div>
      </section>

      <section class="panel">
        <h2>Result Log</h2>
        <pre id="result-log" class="log">No test action has run yet.</pre>
      </section>
    </section>
  </main>

  <script>
    const CHANNELS = %s;
    const ACTIONS_AVAILABLE = %t;
    const MEDIA_ENABLED = %t;
    const channelSelect = document.getElementById('channel-select');
    const profileSelect = document.getElementById('profile-select');
    const channelMeta = document.getElementById('channel-meta');
    const viewer = document.getElementById('viewer');
    const resultLog = document.getElementById('result-log');
    const streamButtons = Array.from(document.querySelectorAll('[data-stream-mode]'));
    const actionButtons = Array.from(document.querySelectorAll('[data-call]'));
    let lastStreamMode = 'snapshot';

    function selectedChannel() {
      const index = Number(channelSelect.value);
      if (!Number.isFinite(index) || index < 0 || index >= CHANNELS.length) {
        return null;
      }
      return CHANNELS[index];
    }

    function selectedProfile() {
      const channel = selectedChannel();
      if (!channel || !channel.profiles || channel.profiles.length === 0) {
        return null;
      }
      return channel.profiles.find(function(profile) {
        return profile.name === profileSelect.value;
      }) || channel.profiles[0];
    }

    function pretty(value) {
      return JSON.stringify(value, null, 2);
    }

    function setViewerMessage(message) {
      viewer.innerHTML = '';
      const node = document.createElement('div');
      node.className = 'viewer-empty';
      node.textContent = message;
      viewer.appendChild(node);
    }

    function renderViewer(mode) {
      const channel = selectedChannel();
      const profile = selectedProfile();
      lastStreamMode = mode;
      viewer.innerHTML = '';
      if (!channel) {
        setViewerMessage('No channel selected.');
        return;
      }
      if (mode !== 'snapshot' && !MEDIA_ENABLED) {
        setViewerMessage('Media layer is disabled for stream transports.');
        return;
      }
      if (mode !== 'snapshot' && !profile) {
        setViewerMessage('No stream profile is available for this channel.');
        return;
      }

      if (mode === 'snapshot') {
        const img = document.createElement('img');
        img.alt = channel.name + ' snapshot';
        img.src = channel.snapshot_url + '?_=' + Date.now();
        viewer.appendChild(img);
        return;
      }
      if (mode === 'mjpeg') {
        const img = document.createElement('img');
        img.alt = channel.name + ' MJPEG';
        img.src = profile.mjpeg_url;
        viewer.appendChild(img);
        return;
      }
      if (mode === 'hls') {
        const video = document.createElement('video');
        video.controls = true;
        video.autoplay = true;
        video.muted = true;
        video.playsInline = true;
        video.src = profile.hls_url;
        viewer.appendChild(video);
        video.play().catch(function() {});
        return;
      }
      if (mode === 'preview' || mode === 'webrtc') {
        const iframe = document.createElement('iframe');
        iframe.title = channel.name + ' ' + mode;
        iframe.src = mode === 'preview' ? profile.preview_url : profile.webrtc_url;
        viewer.appendChild(iframe);
        return;
      }
      setViewerMessage('Unknown stream mode.');
    }

    function updateChannelMeta() {
      const channel = selectedChannel();
      if (!channel) {
        channelMeta.textContent = 'No channel selected.';
        return;
      }
      channelMeta.textContent = pretty({
        name: channel.name,
        stream_id: channel.id,
        device_id: channel.device_id,
        channel: channel.channel,
        recommended_profile: channel.recommended_profile,
        main_video: channel.main_video || '',
        sub_video: channel.sub_video || '',
        audio_codec: channel.audio_codec || '',
        controls: channel.controls || null,
        features: channel.features || [],
      });
    }

    function populateProfiles() {
      const channel = selectedChannel();
      profileSelect.innerHTML = '';
      if (!channel || !channel.profiles) {
        profileSelect.disabled = true;
        return;
      }
      channel.profiles.forEach(function(profile) {
        const option = document.createElement('option');
        option.value = profile.name;
        option.textContent = profile.label;
        if (profile.name === channel.recommended_profile) {
          option.selected = true;
        }
        profileSelect.appendChild(option);
      });
      profileSelect.disabled = channel.profiles.length === 0;
    }

    function populateChannels() {
      channelSelect.innerHTML = '';
      CHANNELS.forEach(function(channel, index) {
        const option = document.createElement('option');
        option.value = String(index);
        option.textContent = 'ch ' + channel.channel + ' - ' + channel.name + ' (' + channel.device_id + ')';
        channelSelect.appendChild(option);
      });
      const hasChannels = CHANNELS.length > 0;
      channelSelect.disabled = !hasChannels;
      actionButtons.forEach(function(button) {
        button.disabled = !hasChannels || !ACTIONS_AVAILABLE;
      });
      streamButtons.forEach(function(button) {
        button.disabled = !hasChannels;
      });
      populateProfiles();
      updateChannelMeta();
      renderViewer('snapshot');
      if (!hasChannels) {
        setViewerMessage('No NVR channel streams are currently in the catalog.');
      }
    }

    function logResult(title, payload, isError) {
      const stamp = new Date().toISOString();
      const body = typeof payload === 'string' ? payload : pretty(payload);
      const prior = resultLog.textContent === 'No test action has run yet.' ? '' : '\n\n' + resultLog.textContent;
      resultLog.textContent = '[' + stamp + '] ' + title + '\n' + body + prior;
      resultLog.style.borderColor = isError ? 'rgba(235, 116, 122, 0.45)' : 'rgba(98, 208, 170, 0.28)';
    }

    function channelURL(channel, suffix) {
      return '/api/v1/nvr/' + encodeURIComponent(channel.device_id) + '/channels/' + channel.channel + suffix;
    }

    async function requestJSON(method, path, payload) {
      const init = { method: method };
      if (payload !== undefined && payload !== null) {
        init.headers = { 'Content-Type': 'application/json' };
        init.body = JSON.stringify(payload);
      }
      const response = await fetch(path, init);
      const text = await response.text();
      let body = text;
      try {
        body = text ? JSON.parse(text) : {};
      } catch (error) {
        body = text;
      }
      if (!response.ok) {
        const message = typeof body === 'string' ? body : pretty(body);
        throw new Error(message || response.statusText || 'request failed');
      }
      return body;
    }

    async function runAction(button) {
      const channel = selectedChannel();
      if (!channel) {
        logResult('No channel selected', 'Select a channel first.', true);
        return;
      }
      const call = button.dataset.call;
      const previous = button.textContent;
      button.disabled = true;
      button.textContent = 'Running...';
      try {
        let method = 'POST';
        let path = '';
        let payload = null;
        if (call === 'controls') {
          method = 'GET';
          path = channelURL(channel, '/controls');
        } else if (call === 'probe') {
          path = '/api/v1/devices/' + encodeURIComponent(channel.device_id) + '/probe';
        } else if (call === 'refresh') {
          path = '/api/v1/nvr/' + encodeURIComponent(channel.device_id) + '/inventory/refresh';
        } else if (call === 'ptz') {
          path = channelURL(channel, '/ptz');
          payload = { action: 'pulse', command: button.dataset.command, speed: 3, duration_ms: 350 };
        } else if (call === 'aux') {
          path = channelURL(channel, '/aux');
          payload = {
            action: button.dataset.action,
            output: button.dataset.output,
            duration_ms: Number(button.dataset.duration || 300),
          };
        } else if (call === 'audio') {
          path = channelURL(channel, '/audio/mute');
          payload = { muted: button.dataset.muted === 'true' };
        } else if (call === 'recording') {
          path = channelURL(channel, '/recording');
          payload = { action: button.dataset.action };
        } else if (call === 'diagnostic') {
          path = channelURL(channel, '/diagnostics');
          payload = {
            method: button.dataset.method,
            action: button.dataset.action,
            duration_ms: Number(button.dataset.duration || 300),
          };
        } else {
          throw new Error('unknown action ' + call);
        }
        const result = await requestJSON(method, path, payload);
        logResult(method + ' ' + path, result, false);
      } catch (error) {
        logResult('Action failed', error && error.message ? error.message : String(error), true);
      } finally {
        button.disabled = !ACTIONS_AVAILABLE;
        button.textContent = previous;
      }
    }

    channelSelect.addEventListener('change', function() {
      populateProfiles();
      updateChannelMeta();
      renderViewer(lastStreamMode);
    });
    profileSelect.addEventListener('change', function() {
      updateChannelMeta();
      renderViewer(lastStreamMode);
    });
    streamButtons.forEach(function(button) {
      button.addEventListener('click', function() {
        renderViewer(button.dataset.streamMode);
      });
    });
    actionButtons.forEach(function(button) {
      button.addEventListener('click', function() {
        runAction(button);
      });
    });
    populateChannels();
  </script>
</body>
</html>`,
		len(channels),
		boolText(mediaEnabled, "good", "warn"),
		boolText(mediaEnabled, "enabled", "disabled"),
		boolText(actionsAvailable, "good", "warn"),
		boolText(actionsAvailable, "enabled", "disabled"),
		channelsJSON,
		actionsAvailable,
		mediaEnabled,
	)
}

func buildTestBridgeChannels(streamEntries []streams.Entry) []testBridgeChannel {
	channels := make([]testBridgeChannel, 0)
	for _, entry := range streamEntries {
		if entry.DeviceKind != dahua.DeviceKindNVRChannel || entry.Channel <= 0 || strings.TrimSpace(entry.RootDeviceID) == "" {
			continue
		}
		profiles := buildTestBridgeProfiles(entry)
		channels = append(channels, testBridgeChannel{
			ID:                 entry.ID,
			DeviceID:           entry.RootDeviceID,
			Name:               firstNonEmpty(entry.Name, entry.ID),
			Channel:            entry.Channel,
			SnapshotURL:        buildTestNVRChannelSnapshotURL(entry.RootDeviceID, entry.Channel),
			RecommendedProfile: firstNonEmpty(entry.RecommendedProfile, firstTestBridgeProfileName(profiles)),
			MainVideo:          strings.TrimSpace(firstNonEmpty(entry.MainCodec+" "+entry.MainResolution, entry.MainCodec, entry.MainResolution)),
			SubVideo:           strings.TrimSpace(firstNonEmpty(entry.SubCodec+" "+entry.SubResolution, entry.SubCodec, entry.SubResolution)),
			AudioCodec:         entry.AudioCodec,
			Profiles:           profiles,
			Controls:           entry.Controls,
			Features:           entry.Features,
		})
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].DeviceID != channels[j].DeviceID {
			return channels[i].DeviceID < channels[j].DeviceID
		}
		if channels[i].Channel != channels[j].Channel {
			return channels[i].Channel < channels[j].Channel
		}
		return channels[i].ID < channels[j].ID
	})
	return channels
}

func buildTestBridgeProfiles(entry streams.Entry) []testBridgeProfile {
	names := orderedTestBridgeProfileNames(entry.Profiles, entry.RecommendedProfile)
	profiles := make([]testBridgeProfile, 0, len(names))
	for _, name := range names {
		profile := entry.Profiles[name]
		label := name
		if profile.Recommended || name == entry.RecommendedProfile {
			label += " (recommended)"
		}
		profiles = append(profiles, testBridgeProfile{
			Name:        name,
			Label:       label,
			SnapshotURL: buildTestMediaSnapshotURL(entry.ID, name),
			MJPEGURL:    buildTestMediaMJPEGURL(entry.ID, name),
			HLSURL:      buildTestMediaHLSURL(entry.ID, name),
			PreviewURL:  buildTestMediaPreviewURL(entry.ID, name),
			WebRTCURL:   buildTestMediaWebRTCURL(entry.ID, name),
			Width:       profile.SourceWidth,
			Height:      profile.SourceHeight,
		})
	}
	return profiles
}

func orderedTestBridgeProfileNames(profiles map[string]streams.Profile, recommended string) []string {
	if len(profiles) == 0 {
		return nil
	}

	ordered := make([]string, 0, len(profiles))
	seen := make(map[string]struct{}, len(profiles))
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := profiles[name]; !ok {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		ordered = append(ordered, name)
	}

	appendName(recommended)
	for _, name := range []string{"stable", "substream", "quality", "default"} {
		appendName(name)
	}
	extras := make([]string, 0, len(profiles))
	for name := range profiles {
		if _, ok := seen[name]; ok {
			continue
		}
		extras = append(extras, name)
	}
	sort.Strings(extras)
	for _, name := range extras {
		appendName(name)
	}
	return ordered
}

func firstTestBridgeProfileName(profiles []testBridgeProfile) string {
	if len(profiles) == 0 {
		return ""
	}
	return profiles[0].Name
}

func buildTestNVRChannelSnapshotURL(deviceID string, channel int) string {
	return "/api/v1/nvr/" + url.PathEscape(deviceID) + "/channels/" + strconv.Itoa(channel) + "/snapshot"
}

func buildTestMediaSnapshotURL(streamID string, profile string) string {
	return "/api/v1/media/snapshot/" + url.PathEscape(streamID) + "?profile=" + url.QueryEscape(profile) + "&width=960"
}

func buildTestMediaMJPEGURL(streamID string, profile string) string {
	return "/api/v1/media/mjpeg/" + url.PathEscape(streamID) + "?profile=" + url.QueryEscape(profile) + "&width=960"
}

func buildTestMediaHLSURL(streamID string, profile string) string {
	return "/api/v1/media/hls/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile) + "/index.m3u8"
}

func buildTestMediaPreviewURL(streamID string, profile string) string {
	return "/api/v1/media/preview/" + url.PathEscape(streamID) + "?profile=" + url.QueryEscape(profile)
}

func buildTestMediaWebRTCURL(streamID string, profile string) string {
	return "/api/v1/media/webrtc/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile)
}

func buildAdminEndpointSections(healthPath string, metricsPath string) string {
	sections := []struct {
		Title string
		Items []adminEndpoint
	}{
		{
			Title: "Status And Inventory",
			Items: []adminEndpoint{
				{Method: "GET", Path: "/admin", Description: "Operator page", Linkable: true},
				{Method: "GET", Path: "/admin/test-bridge", Description: "NVR channel stream and control diagnostics page", Linkable: true},
				{Method: "GET", Path: healthPath, Description: "Liveness probe", Linkable: true},
				{Method: "GET", Path: "/readyz", Description: "Readiness probe", Linkable: true},
				{Method: "GET", Path: metricsPath, Description: "Prometheus metrics", Linkable: true},
				{Method: "GET", Path: "/api/v1/status", Description: "Bridge status JSON", Linkable: true},
				{Method: "GET", Path: "/api/v1/devices", Description: "Current probe results", Linkable: true},
				{Method: "GET", Path: "/api/v1/streams", Description: "Full stream catalog", Linkable: true},
				{Method: "GET", Path: "/api/v1/media/workers", Description: "Runtime media worker state", Linkable: true},
			},
		},
		{
			Title: "Events And Media",
			Items: []adminEndpoint{
				{Method: "GET", Path: "/api/v1/events", Description: "Recent event buffer", Linkable: true},
				{Method: "DELETE", Path: "/api/v1/events", Description: "Clear event buffer", Linkable: false},
				{Method: "GET", Path: "/api/v1/home-assistant/native/catalog", Description: "Bridge-native Home Assistant catalog", Linkable: true},
			},
		},
		{
			Title: "Mutating Admin APIs",
			Items: []adminEndpoint{
				{Method: "POST", Path: "/api/v1/devices/probe-all", Description: "Probe every configured device", Linkable: false},
				{Method: "POST", Path: "/api/v1/devices/{deviceID}/probe", Description: "Probe one specific device", Linkable: false},
				{Method: "POST", Path: "/api/v1/devices/{deviceID}/credentials", Description: "Rotate bridge-side device credentials", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/inventory/refresh", Description: "Refresh NVR channel/disk inventory", Linkable: false},
				{Method: "GET", Path: "/api/v1/nvr/{deviceID}/recordings?channel=1&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=25", Description: "Search NVR archive recordings by channel and time range", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/recordings/export", Description: "Export an NVR archive playback window as a bridge MP4 clip", Linkable: false},
				{Method: "GET", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/controls", Description: "Read NVR per-channel PTZ capability data", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/ptz", Description: "Send PTZ start/stop/pulse command to an NVR channel", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/aux", Description: "Send aux/light/wiper start/stop/pulse command to an NVR channel", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/recording", Description: "Set manual recording mode for an NVR channel", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/channels/{channel}/diagnostics", Description: "Run one NVR/direct-IPC diagnostic control strategy for a channel", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/playback/sessions", Description: "Create an NVR archive playback session backed by bridge media endpoints", Linkable: false},
				{Method: "GET", Path: "/api/v1/nvr/playback/sessions/{sessionID}", Description: "Inspect an active NVR archive playback session", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/playback/sessions/{sessionID}/seek", Description: "Create a new playback session starting from a different archive timestamp", Linkable: false},
				{Method: "GET", Path: "/api/v1/vto/{deviceID}/controls", Description: "Inspect detected VTO call, lock, audio, recording, and talkback capabilities", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/call/answer", Description: "Request VTO call answer", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/call/hangup", Description: "Request VTO hangup", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/locks/{lockIndex}/unlock", Description: "Trigger VTO door unlock for one configured lock", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/audio/output-volume", Description: "Set VTO output volume for a specific slot", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/audio/input-volume", Description: "Set VTO input volume for a specific slot", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/audio/mute", Description: "Set VTO silent mode", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/recording", Description: "Set VTO automatic call recording", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/intercom/reset", Description: "Reset active bridge WebRTC intercom session", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/intercom/uplink/enable", Description: "Enable external RTP uplink forwarding for the VTO intercom session", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/intercom/uplink/disable", Description: "Disable external RTP uplink forwarding for the VTO intercom session", Linkable: false},
			},
		},
	}

	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		rows := make([]string, 0, len(section.Items))
		for _, item := range section.Items {
			target := "<code>" + htmlEscape(item.Path) + "</code>"
			if item.Linkable {
				target = `<a href="` + htmlEscape(item.Path) + `"><code>` + htmlEscape(item.Path) + `</code></a>`
			}
			rows = append(rows, fmt.Sprintf(
				`<div class="endpoint-row"><span class="method">%s</span><div class="endpoint-main">%s<div class="endpoint-desc">%s</div></div></div>`,
				htmlEscape(item.Method),
				target,
				htmlEscape(item.Description),
			))
		}
		parts = append(parts, `<div class="endpoint-group"><h3 class="card-title">`+htmlEscape(section.Title)+`</h3><div class="endpoint-list">`+strings.Join(rows, "")+`</div></div>`)
	}
	return strings.Join(parts, "")
}

type adminControlStats struct {
	ActionableEntries   int
	NVRPTZEntries       int
	NVRAuxEntries       int
	NVRRecordingEntries int
	VTOVolumeEntries    int
	VTOMuteEntries      int
	VTORecordingEntries int
}

func (s adminControlStats) Summary() string {
	return fmt.Sprintf(
		"%d PTZ | %d aux | %d NVR rec | %d VTO vol | %d VTO mute | %d VTO rec",
		s.NVRPTZEntries,
		s.NVRAuxEntries,
		s.NVRRecordingEntries,
		s.VTOVolumeEntries,
		s.VTOMuteEntries,
		s.VTORecordingEntries,
	)
}

func summarizeAdminControlStats(streamEntries []streams.Entry) adminControlStats {
	var stats adminControlStats
	for _, entry := range streamEntries {
		actionable := false
		if entry.Controls != nil {
			if entry.Controls.PTZ != nil && entry.Controls.PTZ.Supported {
				stats.NVRPTZEntries++
				actionable = true
			}
			if entry.Controls.Aux != nil && entry.Controls.Aux.Supported {
				stats.NVRAuxEntries++
				actionable = true
			}
			if entry.Controls.Recording != nil && entry.Controls.Recording.Supported {
				stats.NVRRecordingEntries++
				actionable = true
			}
		}
		if entry.Intercom != nil {
			if entry.Intercom.SupportsVTOOutputVolumeControl || entry.Intercom.SupportsVTOInputVolumeControl {
				stats.VTOVolumeEntries++
				actionable = true
			}
			if entry.Intercom.SupportsVTOMuteControl {
				stats.VTOMuteEntries++
				actionable = true
			}
			if entry.Intercom.SupportsVTORecordingControl {
				stats.VTORecordingEntries++
				actionable = true
			}
		}
		if actionable {
			stats.ActionableEntries++
		}
	}
	return stats
}

func buildAdminDeviceCards(probeResults []*dahua.ProbeResult, streamEntries []streams.Entry) string {
	if len(probeResults) == 0 {
		return `<p class="muted-note">No devices are currently available in the probe store.</p>`
	}

	streamsByRoot := make(map[string][]streams.Entry)
	for _, entry := range streamEntries {
		if entry.RootDeviceID == "" {
			continue
		}
		streamsByRoot[entry.RootDeviceID] = append(streamsByRoot[entry.RootDeviceID], entry)
	}

	cards := make([]string, 0, len(probeResults))
	for _, result := range probeResults {
		if result == nil {
			continue
		}
		root := result.Root
		rootStreams := streamsByRoot[root.ID]
		chips := []string{
			adminLinkChip("device detail", "/api/v1/devices/"+url.PathEscape(root.ID), false),
			adminLinkChip("device streams", "/api/v1/streams?device_id="+url.QueryEscape(root.ID), true),
		}
		switch root.Kind {
		case dahua.DeviceKindVTO:
			chips = append(chips, adminLinkChip("snapshot", "/api/v1/vto/"+url.PathEscape(root.ID)+"/snapshot", false))
			chips = append(chips, buildAdminRootControlChips(root.ID, rootStreams)...)
		case dahua.DeviceKindIPC:
			chips = append(chips, adminLinkChip("snapshot", "/api/v1/ipc/"+url.PathEscape(root.ID)+"/snapshot", false))
		case dahua.DeviceKindNVR:
			if len(rootStreams) > 0 && rootStreams[0].Channel > 0 {
				recordingsURL := fmt.Sprintf(
					"/api/v1/nvr/%s/recordings?channel=%d&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=25",
					url.PathEscape(root.ID),
					rootStreams[0].Channel,
				)
				chips = append(chips, adminLinkChip("recordings", recordingsURL, true))
			}
		}

		for _, entry := range rootStreams {
			if entry.LocalPreviewURL != "" {
				chips = append(chips, adminLinkChip("preview", entry.LocalPreviewURL, true))
			}
			if entry.LocalIntercomURL != "" {
				chips = append(chips, adminLinkChip("intercom", entry.LocalIntercomURL, false))
			}
			break
		}

		metaParts := []string{
			string(root.Kind),
			"id=" + root.ID,
			"model=" + firstNonEmpty(root.Model, "unknown"),
			fmt.Sprintf("children=%d", len(result.Children)),
			fmt.Sprintf("streams=%d", len(rootStreams)),
		}
		if controlSummary := buildAdminRootControlSummary(rootStreams); controlSummary != "" {
			metaParts = append(metaParts, controlSummary)
		}
		if notes := buildAdminValidationNoteMarkup(collectAdminValidationNotes(rootStreams)); notes != "" {
			chips = append(chips, notes)
		}

		cards = append(cards, fmt.Sprintf(
			`<article class="device-card"><h3 class="card-title">%s</h3><div class="card-meta">%s</div><div class="chip-row">%s</div></article>`,
			htmlEscape(firstNonEmpty(root.Name, root.ID)),
			adminMetaLine(metaParts...),
			strings.Join(chips, ""),
		))
	}
	return `<div class="card-grid">` + strings.Join(cards, "") + `</div>`
}

func buildAdminStreamCards(streamEntries []streams.Entry) string {
	if len(streamEntries) == 0 {
		return `<p class="muted-note">No stream entries are currently available.</p>`
	}

	cards := make([]string, 0, len(streamEntries))
	for _, entry := range streamEntries {
		chips := []string{
			adminLinkChip("stream detail", "/api/v1/streams/"+url.PathEscape(entry.ID), false),
		}
		if entry.SnapshotURL != "" {
			chips = append(chips, adminLinkChip("snapshot", entry.SnapshotURL, true))
		}
		if entry.LocalPreviewURL != "" {
			chips = append(chips, adminLinkChip("preview", entry.LocalPreviewURL, false))
		}
		if entry.LocalIntercomURL != "" {
			chips = append(chips, adminLinkChip("intercom", entry.LocalIntercomURL, false))
		}
		if profile, ok := entry.Profiles[entry.RecommendedProfile]; ok {
			if profile.LocalWebRTCURL != "" {
				chips = append(chips, adminLinkChip("webrtc", profile.LocalWebRTCURL, true))
			}
			if profile.LocalHLSURL != "" {
				chips = append(chips, adminLinkChip("hls", profile.LocalHLSURL, true))
			}
			if profile.LocalMJPEGURL != "" {
				chips = append(chips, adminLinkChip("mjpeg", profile.LocalMJPEGURL, true))
			}
		}
		chips = append(chips, buildAdminStreamControlChips(entry)...)
		if notes := buildAdminValidationNoteMarkup(adminValidationNotesForEntry(entry)); notes != "" {
			chips = append(chips, notes)
		}

		videoSummary := strings.TrimSpace(firstNonEmpty(entry.MainCodec, "unknown") + " " + firstNonEmpty(entry.MainResolution, ""))
		metaParts := []string{
			string(entry.DeviceKind),
			"recommended=" + firstNonEmpty(entry.RecommendedProfile, "unknown"),
			"video=" + videoSummary,
			"audio=" + firstNonEmpty(entry.AudioCodec, "none"),
		}
		if entry.Channel > 0 {
			metaParts = append(metaParts, fmt.Sprintf("channel=%d", entry.Channel))
		}
		if controlSummary := buildAdminStreamControlSummary(entry); controlSummary != "" {
			metaParts = append(metaParts, controlSummary)
		}

		cards = append(cards, fmt.Sprintf(
			`<article class="stream-card"><h3 class="card-title">%s</h3><div class="card-meta">%s</div><div class="chip-row">%s</div></article>`,
			htmlEscape(firstNonEmpty(entry.Name, entry.ID)),
			adminMetaLine(metaParts...),
			strings.Join(chips, ""),
		))
	}
	return `<div class="card-grid">` + strings.Join(cards, "") + `</div>`
}

func buildAdminRootControlSummary(entries []streams.Entry) string {
	var parts []string
	for _, entry := range entries {
		if entry.Intercom == nil {
			continue
		}
		if entry.Intercom.SupportsVTOCallAnswer || entry.Intercom.SupportsHangup {
			parts = append(parts, "call control")
		}
		if len(entry.Intercom.LockURLs) > 0 {
			parts = append(parts, fmt.Sprintf("locks=%d", len(entry.Intercom.LockURLs)))
		}
		if entry.Intercom.SupportsVTOOutputVolumeControl || entry.Intercom.SupportsVTOInputVolumeControl || entry.Intercom.SupportsVTOMuteControl {
			parts = append(parts, "audio control")
		}
		if entry.Intercom.SupportsVTORecordingControl {
			parts = append(parts, "auto record")
		}
		break
	}
	return strings.Join(parts, ", ")
}

func buildAdminRootControlChips(rootID string, entries []streams.Entry) []string {
	for _, entry := range entries {
		if entry.Intercom == nil {
			continue
		}
		var chips []string
		chips = append(chips, adminLinkChip("vto controls", "/api/v1/vto/"+url.PathEscape(rootID)+"/controls", false))
		if entry.Intercom.AnswerURL != "" {
			chips = append(chips, adminLinkChip("answer", entry.Intercom.AnswerURL, true))
		}
		if entry.Intercom.HangupURL != "" {
			chips = append(chips, adminLinkChip("hangup", entry.Intercom.HangupURL, true))
		}
		for index, lockURL := range entry.Intercom.LockURLs {
			chips = append(chips, adminLinkChip(fmt.Sprintf("unlock %d", index+1), lockURL, false))
		}
		if entry.Intercom.OutputVolumeURL != "" {
			chips = append(chips, adminLinkChip("output volume", entry.Intercom.OutputVolumeURL, true))
		}
		if entry.Intercom.InputVolumeURL != "" {
			chips = append(chips, adminLinkChip("input volume", entry.Intercom.InputVolumeURL, true))
		}
		if entry.Intercom.MuteURL != "" {
			chips = append(chips, adminLinkChip("mute", entry.Intercom.MuteURL, true))
		}
		if entry.Intercom.RecordingURL != "" {
			chips = append(chips, adminLinkChip("auto record", entry.Intercom.RecordingURL, true))
		}
		if entry.Intercom.BridgeSessionResetURL != "" {
			chips = append(chips, adminLinkChip("reset intercom", entry.Intercom.BridgeSessionResetURL, true))
		}
		return chips
	}
	return nil
}

func buildAdminStreamControlSummary(entry streams.Entry) string {
	var parts []string
	if entry.Controls != nil {
		if entry.Controls.PTZ != nil && entry.Controls.PTZ.Supported {
			parts = append(parts, "ptz")
		}
		if entry.Controls.Aux != nil && entry.Controls.Aux.Supported {
			auxSummary := "aux"
			if len(entry.Controls.Aux.Features) > 0 {
				auxSummary = "aux=" + strings.Join(entry.Controls.Aux.Features, ",")
			} else if len(entry.Controls.Aux.Outputs) > 0 {
				auxSummary = "aux=" + strings.Join(entry.Controls.Aux.Outputs, ",")
			}
			parts = append(parts, auxSummary)
		}
		if entry.Controls.Audio != nil {
			audioParts := make([]string, 0, 3)
			if entry.Controls.Audio.Mute {
				audioParts = append(audioParts, "mute")
			}
			if entry.Controls.Audio.Volume {
				audioParts = append(audioParts, "volume")
			}
			if entry.Controls.Audio.PlaybackSupported {
				audioParts = append(audioParts, "playback")
			}
			if len(audioParts) > 0 {
				parts = append(parts, "audio="+strings.Join(audioParts, ","))
			}
		}
		if entry.Controls.Recording != nil && entry.Controls.Recording.Supported {
			recordingSummary := "recording=" + firstNonEmpty(entry.Controls.Recording.Mode, "supported")
			if entry.Controls.Recording.Active {
				recordingSummary += " active"
			}
			parts = append(parts, recordingSummary)
		}
	}
	if entry.Intercom != nil {
		if entry.Intercom.SupportsVTOOutputVolumeControl || entry.Intercom.SupportsVTOInputVolumeControl || entry.Intercom.SupportsVTOMuteControl {
			parts = append(parts, "vto audio")
		}
		if entry.Intercom.SupportsVTORecordingControl {
			parts = append(parts, "vto recording")
		}
		if entry.Intercom.CallState != "" {
			parts = append(parts, "call="+entry.Intercom.CallState)
		}
	}
	return strings.Join(parts, ", ")
}

func buildAdminStreamControlChips(entry streams.Entry) []string {
	var chips []string
	if entry.Controls != nil {
		if entry.Channel > 0 {
			chips = append(chips, adminLinkChip("channel controls", fmt.Sprintf("/api/v1/nvr/%s/channels/%d/controls", url.PathEscape(entry.RootDeviceID), entry.Channel), true))
			chips = append(chips, adminLinkChip("recordings", fmt.Sprintf("/api/v1/nvr/%s/recordings?channel=%d&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=25", url.PathEscape(entry.RootDeviceID), entry.Channel), true))
		}
		if entry.Controls.PTZ != nil && entry.Controls.PTZ.URL != "" {
			chips = append(chips, adminLinkChip("ptz", entry.Controls.PTZ.URL, false))
		}
		if entry.Controls.Aux != nil && entry.Controls.Aux.URL != "" {
			chips = append(chips, adminLinkChip("aux", entry.Controls.Aux.URL, false))
		}
		if entry.Controls.Recording != nil && entry.Controls.Recording.URL != "" {
			chips = append(chips, adminLinkChip("recording", entry.Controls.Recording.URL, false))
		}
	}
	if entry.Intercom != nil {
		if entry.Intercom.AnswerURL != "" {
			chips = append(chips, adminLinkChip("answer", entry.Intercom.AnswerURL, true))
		}
		if entry.Intercom.HangupURL != "" {
			chips = append(chips, adminLinkChip("hangup", entry.Intercom.HangupURL, true))
		}
		if entry.Intercom.OutputVolumeURL != "" {
			chips = append(chips, adminLinkChip("output volume", entry.Intercom.OutputVolumeURL, true))
		}
		if entry.Intercom.InputVolumeURL != "" {
			chips = append(chips, adminLinkChip("input volume", entry.Intercom.InputVolumeURL, true))
		}
		if entry.Intercom.MuteURL != "" {
			chips = append(chips, adminLinkChip("mute", entry.Intercom.MuteURL, true))
		}
		if entry.Intercom.RecordingURL != "" {
			chips = append(chips, adminLinkChip("auto record", entry.Intercom.RecordingURL, true))
		}
	}
	return chips
}

func collectAdminValidationNotes(entries []streams.Entry) []string {
	seen := make(map[string]struct{})
	notes := make([]string, 0)
	for _, entry := range entries {
		for _, note := range adminValidationNotesForEntry(entry) {
			if note == "" {
				continue
			}
			if _, ok := seen[note]; ok {
				continue
			}
			seen[note] = struct{}{}
			notes = append(notes, note)
		}
	}
	return notes
}

func adminValidationNotesForEntry(entry streams.Entry) []string {
	notes := make([]string, 0)
	if entry.Controls != nil {
		notes = append(notes, entry.Controls.ValidationNotes...)
	}
	if entry.Intercom != nil {
		notes = append(notes, entry.Intercom.ValidationNotes...)
	}
	return notes
}

func buildAdminValidationNoteMarkup(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	summary := strings.Join(notes, " | ")
	if len(summary) > 180 {
		summary = summary[:177] + "..."
	}
	return adminTextChip("validated: "+summary, true)
}

func adminLinkChip(label string, href string, subtle bool) string {
	className := "chip"
	if subtle {
		className += " subtle"
	}
	return `<span class="` + className + `"><a href="` + htmlEscape(href) + `"><code>` + htmlEscape(label) + `</code></a></span>`
}

func adminTextChip(label string, subtle bool) string {
	className := "chip"
	if subtle {
		className += " subtle"
	}
	return `<span class="` + className + `"><code>` + htmlEscape(label) + `</code></span>`
}

func adminMetaLine(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		filtered = append(filtered, htmlEscape(strings.TrimSpace(part)))
	}
	return strings.Join(filtered, " &bull; ")
}

func marshalIndentedJSON(payload any) string {
	if payload == nil {
		return "{}"
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(body)
}

func marshalJSONForScript(payload any) string {
	body, err := json.Marshal(payload)
	if err != nil {
		return "null"
	}
	return string(body)
}

func boolText(value bool, trueText string, falseText string) string {
	if value {
		return trueText
	}
	return falseText
}

func boolHTMLAttr(enabled bool) string {
	if enabled {
		return ""
	}
	return "disabled"
}

func renderMediaPreviewPage(entry streams.Entry, profileName string, profile streams.Profile) string {
	title := htmlEscape(entry.Name) + " Preview"
	profileLinks := buildPreviewProfileLinks(entry, profileName)
	audioNote := "Audio is available only when the browser can play the HLS stream directly."
	if strings.TrimSpace(entry.AudioCodec) == "" {
		audioNote = "The source stream does not advertise audio, so preview is video-only."
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0c1114;
      --panel: #111a1d;
      --panel-alt: #151e21;
      --text: #f1f5f2;
      --muted: #a8b8b2;
      --line: rgba(153, 174, 166, 0.2);
      --accent: #5ed0ac;
      --accent-soft: rgba(94, 208, 172, 0.14);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background: linear-gradient(180deg, #151e21 0, var(--bg) 72%%);
      color: var(--text);
      min-height: 100vh;
    }
    main {
      max-width: 1040px;
      margin: 0 auto;
      padding: 24px;
    }
    .hero {
      display: grid;
      gap: 12px;
      margin-bottom: 18px;
    }
    .eyebrow {
      display: inline-flex;
      width: fit-content;
      gap: 8px;
      padding: 6px 10px;
      border-radius: 8px;
      background: var(--accent-soft);
      color: var(--accent);
      font-size: 13px;
      letter-spacing: 0;
      text-transform: uppercase;
    }
    h1 {
      margin: 0;
      font-size: clamp(28px, 5vw, 44px);
      line-height: 1.05;
    }
    .subtle {
      color: var(--muted);
      margin: 0;
      max-width: 72ch;
    }
    .layout {
      display: grid;
      grid-template-columns: minmax(0, 2fr) minmax(280px, 1fr);
      gap: 18px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
      box-shadow: 0 18px 60px rgba(0, 0, 0, 0.28);
    }
    .player-wrap {
      position: relative;
      background: #040a10;
      aspect-ratio: 16 / 9;
    }
    video, img {
      width: 100%%;
      height: 100%%;
      object-fit: contain;
      display: none;
      background: #040a10;
    }
    .fallback-note {
      position: absolute;
      inset: auto 16px 16px 16px;
      padding: 12px 14px;
      border-radius: 8px;
      background: rgba(4, 10, 16, 0.74);
      border: 1px solid rgba(255,255,255,0.08);
      color: var(--muted);
      font-size: 14px;
      display: none;
    }
    .panel-body {
      padding: 18px;
      display: grid;
      gap: 14px;
    }
    .profile-links {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    .profile-links a {
      padding: 10px 12px;
      border-radius: 8px;
      text-decoration: none;
      color: var(--text);
      background: rgba(255,255,255,0.04);
      border: 1px solid var(--line);
      font-size: 14px;
    }
    .profile-links a.active {
      color: #04261b;
      background: var(--accent);
      border-color: var(--accent);
      font-weight: 600;
    }
    .meta {
      display: grid;
      gap: 10px;
    }
    .meta-row {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      padding-bottom: 10px;
      border-bottom: 1px solid rgba(255,255,255,0.05);
    }
    .meta-row:last-child {
      border-bottom: 0;
      padding-bottom: 0;
    }
    .meta-key {
      color: var(--muted);
    }
    code {
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 100%%;
    }
    @media (max-width: 860px) {
      .layout {
        grid-template-columns: 1fr;
      }
      main {
        padding: 16px;
      }
    }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <div class="eyebrow">Bridge Preview</div>
      <h1>%s</h1>
      <p class="subtle">This page stays on the bridge host and chooses the best browser-safe live path available for this stream. Native HLS is preferred when supported; otherwise the page falls back to low-latency MJPEG.</p>
    </section>
    <section class="layout">
      <article class="panel">
        <div class="player-wrap">
          <video id="video" controls autoplay muted playsinline preload="auto"></video>
          <img id="mjpeg" alt="%s live preview">
          <div id="fallback-note" class="fallback-note"></div>
        </div>
        <div class="panel-body">
          <div class="profile-links">%s</div>
          <p class="subtle">%s</p>
        </div>
      </article>
      <aside class="panel">
        <div class="panel-body meta">
          <div class="meta-row"><span class="meta-key">Device</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Kind</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Profile</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Video</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Audio</span><span>%s</span></div>
          <div class="meta-row"><span class="meta-key">Snapshot</span><code>%s</code></div>
          <div class="meta-row"><span class="meta-key">HLS</span><code>%s</code></div>
          <div class="meta-row"><span class="meta-key">MJPEG</span><code>%s</code></div>
        </div>
      </aside>
    </section>
  </main>
  <script>
    const video = document.getElementById('video');
    const mjpeg = document.getElementById('mjpeg');
    const fallback = document.getElementById('fallback-note');
    const hlsURL = %q;
    const mjpegURL = %q;

    const canPlayNativeHLS = !!video.canPlayType('application/vnd.apple.mpegurl') || !!video.canPlayType('application/x-mpegURL');
    if (canPlayNativeHLS && hlsURL) {
      video.src = hlsURL;
      video.style.display = 'block';
      video.play().catch(() => {});
    } else {
      mjpeg.src = mjpegURL;
      mjpeg.style.display = 'block';
      fallback.style.display = 'block';
      fallback.textContent = canPlayNativeHLS ? '' : 'Native HLS is not available in this browser, so the preview is using MJPEG for low-latency playback.';
    }
  </script>
</body>
</html>`,
		title,
		htmlEscape(entry.Name),
		htmlEscape(entry.Name),
		profileLinks,
		htmlEscape(audioNote),
		htmlEscape(entry.Name),
		htmlEscape(string(entry.DeviceKind)),
		htmlEscape(profileName),
		htmlEscape(firstNonEmpty(entry.MainCodec+" "+entry.MainResolution, "unknown")),
		htmlEscape(firstNonEmpty(entry.AudioCodec, "none")),
		htmlEscape(entry.SnapshotURL),
		htmlEscape(profile.LocalHLSURL),
		htmlEscape(profile.LocalMJPEGURL),
		profile.LocalHLSURL,
		profile.LocalMJPEGURL,
	)
}

func renderWebRTCPage(entry streams.Entry, profileName string, profile streams.Profile, iceServers []mediaapi.WebRTCICEServer) string {
	title := htmlEscape(entry.Name) + " WebRTC"
	offerURL := "/api/v1/media/webrtc/" + url.PathEscape(entry.ID) + "/" + url.PathEscape(profileName) + "/offer"
	iceServersJSON := marshalWebRTCICEServers(iceServers)
	iceModeLabel := "default host candidates"
	if len(iceServers) > 0 {
		iceModeLabel = fmt.Sprintf("configured STUN/TURN (%d)", len(iceServers))
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0c1114;
      --panel: #111a1d;
      --line: rgba(153, 174, 166, 0.2);
      --text: #f1f5f2;
      --muted: #a8b8b2;
      --accent: #5ed0ac;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background: linear-gradient(180deg, #151e21 0, var(--bg) 72%%);
      color: var(--text);
      min-height: 100vh;
    }
    main {
      max-width: 920px;
      margin: 0 auto;
      padding: 24px;
      display: grid;
      gap: 18px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
    }
    .hero {
      padding: 22px;
      display: grid;
      gap: 10px;
    }
    .eyebrow {
      color: var(--accent);
      text-transform: uppercase;
      letter-spacing: 0;
      font-size: 12px;
    }
    h1 {
      margin: 0;
      font-size: clamp(28px, 5vw, 42px);
      line-height: 1.05;
    }
    p {
      margin: 0;
      color: var(--muted);
    }
    video {
      width: 100%%;
      aspect-ratio: 16 / 9;
      background: #000;
      display: block;
    }
    .meta {
      padding: 18px 22px 22px;
      display: grid;
      gap: 10px;
    }
    .row {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      padding-bottom: 10px;
      border-bottom: 1px solid rgba(255,255,255,0.06);
    }
    .row:last-child {
      border-bottom: 0;
      padding-bottom: 0;
    }
    .label {
      color: var(--muted);
    }
    code {
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 100%%;
    }
  </style>
</head>
<body>
  <main>
    <section class="panel hero">
      <div class="eyebrow">Bridge WebRTC</div>
      <h1>%s</h1>
      <p>This page negotiates direct WebRTC playback through the bridge for lower-latency live media than HLS. It is playback-only and currently does not include talkback.</p>
    </section>
    <section class="panel">
      <video id="player" autoplay playsinline controls></video>
        <div class="meta">
          <div class="row"><span class="label">Status</span><span id="status">Negotiating...</span></div>
          <div class="row"><span class="label">Profile</span><span>%s</span></div>
          <div class="row"><span class="label">Audio</span><span>%s</span></div>
          <div class="row"><span class="label">ICE</span><span>%s</span></div>
          <div class="row"><span class="label">Fallback HLS</span><code>%s</code></div>
          <div class="row"><span class="label">Fallback MJPEG</span><code>%s</code></div>
        </div>
    </section>
  </main>
  <script>
    const video = document.getElementById('player');
    const statusEl = document.getElementById('status');
    const offerURL = %q;
    const iceServers = %s;
    let peer = null;
    let reconnectTimer = null;
    let reconnectAttempts = 0;

    async function waitForIceComplete(pc) {
      if (pc.iceGatheringState === 'complete') {
        return;
      }
      await new Promise(resolve => {
        const onChange = () => {
          if (pc.iceGatheringState === 'complete') {
            pc.removeEventListener('icegatheringstatechange', onChange);
            resolve();
          }
        };
        pc.addEventListener('icegatheringstatechange', onChange);
      });
    }

    function clearReconnectTimer() {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    }

    function closePeer() {
      if (!peer) {
        return;
      }
      try {
        peer.ontrack = null;
        peer.onconnectionstatechange = null;
        peer.close();
      } catch (_) {}
      peer = null;
    }

    function reconnectDelayMilliseconds() {
      return Math.min(1000 * Math.pow(2, Math.min(reconnectAttempts, 4)), 10000);
    }

    function scheduleReconnect(reason) {
      if (reconnectTimer) {
        return;
      }
      reconnectAttempts += 1;
      const delay = reconnectDelayMilliseconds();
      statusEl.textContent = 'Reconnecting in ' + Math.max(1, Math.round(delay / 1000)) + 's';
      if (reason) {
        console.warn('webrtc reconnect scheduled:', reason);
      }
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        start(true).catch(handleStartError);
      }, delay);
    }

    function handleStartError(error) {
      console.error(error);
      statusEl.textContent = 'Retrying...';
      scheduleReconnect(error && error.message ? error.message : String(error));
    }

    async function start(isReconnect) {
      clearReconnectTimer();
      closePeer();
      statusEl.textContent = isReconnect ? 'Reconnecting...' : 'Negotiating...';

      const pc = new RTCPeerConnection({ iceServers });
      peer = pc;
      const stream = new MediaStream();
      video.srcObject = stream;

      pc.addTransceiver('video', { direction: 'recvonly' });
      pc.addTransceiver('audio', { direction: 'recvonly' });
      pc.ontrack = event => {
        if (peer !== pc) {
          return;
        }
        stream.addTrack(event.track);
        reconnectAttempts = 0;
        statusEl.textContent = 'Connected';
      };
      pc.onconnectionstatechange = () => {
        if (peer !== pc || !pc.connectionState) {
          return;
        }
        switch (pc.connectionState) {
        case 'connected':
          reconnectAttempts = 0;
          statusEl.textContent = 'Connected';
          return;
        case 'new':
          statusEl.textContent = 'Negotiating...';
          return;
        case 'connecting':
          statusEl.textContent = 'Connecting...';
          return;
        case 'disconnected':
        case 'failed':
          statusEl.textContent = pc.connectionState;
          scheduleReconnect('connection ' + pc.connectionState);
          return;
        case 'closed':
          if (peer === pc) {
            scheduleReconnect('connection closed');
          }
          return;
        default:
          statusEl.textContent = pc.connectionState;
        }
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      await waitForIceComplete(pc);

      const response = await fetch(offerURL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(pc.localDescription),
      });
      if (!response.ok) {
        closePeer();
        throw new Error(await response.text());
      }

      const answer = await response.json();
      await pc.setRemoteDescription(answer);
      video.play().catch(() => {
        statusEl.textContent = 'Ready - press play if autoplay was blocked';
      });
      if (statusEl.textContent === 'Connected') {
        return;
      }
      statusEl.textContent = 'Waiting for media...';
    }

    window.addEventListener('beforeunload', () => {
      clearReconnectTimer();
      closePeer();
    });
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
        return;
      }
      if (!peer || peer.connectionState === 'failed' || peer.connectionState === 'disconnected' || peer.connectionState === 'closed') {
        scheduleReconnect('page became visible');
      }
    });

    start(false).catch(handleStartError);
  </script>
</body>
</html>`,
		title,
		htmlEscape(entry.Name),
		htmlEscape(profileName),
		htmlEscape(firstNonEmpty(entry.AudioCodec, "none")),
		htmlEscape(iceModeLabel),
		htmlEscape(profile.LocalHLSURL),
		htmlEscape(profile.LocalMJPEGURL),
		offerURL,
		iceServersJSON,
	)
}

func renderVTOIntercomPage(entry streams.Entry, profileName string, profile streams.Profile, iceServers []mediaapi.WebRTCICEServer) string {
	title := htmlEscape(entry.Name) + " Intercom"
	offerURL := "/api/v1/media/webrtc/" + url.PathEscape(entry.ID) + "/" + url.PathEscape(profileName) + "/offer"
	deviceURL := "/api/v1/devices/" + url.PathEscape(entry.ID)
	intercomStatusURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/intercom/status"
	intercomResetURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/intercom/reset"
	answerURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/call/answer"
	hangupURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/call/hangup"
	uplinkEnableURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/intercom/uplink/enable"
	uplinkDisableURL := "/api/v1/vto/" + url.PathEscape(entry.ID) + "/intercom/uplink/disable"
	profileLinks := buildIntercomProfileLinks(entry, profileName)
	audioLabel := firstNonEmpty(entry.AudioCodec, "none")
	iceServersJSON := marshalWebRTCICEServers(iceServers)
	iceModeLabel := "default host candidates"
	if len(iceServers) > 0 {
		iceModeLabel = fmt.Sprintf("configured STUN/TURN (%d)", len(iceServers))
	}
	externalUplinkTargetsLabel := "none"
	if entry.Intercom != nil && entry.Intercom.ConfiguredExternalUplinkTargetCount > 0 {
		externalUplinkTargetsLabel = fmt.Sprintf("%d configured", entry.Intercom.ConfiguredExternalUplinkTargetCount)
	}
	showAnswerControl := entry.Intercom != nil && entry.Intercom.SupportsVTOCallAnswer
	showExternalUplinkControl := entry.Intercom != nil && entry.Intercom.SupportsExternalAudioExport

	lockButtonCapacity := entry.LockCount
	if lockButtonCapacity < 1 {
		lockButtonCapacity = 1
	}
	lockButtons := make([]string, 0, lockButtonCapacity)
	if entry.LockCount <= 0 {
		lockButtons = append(lockButtons, `<div class="empty-note">No lock actions were discovered for this VTO.</div>`)
	} else {
		for index := 0; index < entry.LockCount; index++ {
			label := "Open Door"
			if index > 0 {
				label = fmt.Sprintf("Open Door %d", index+1)
			}
			lockButtons = append(lockButtons, fmt.Sprintf(
				`<button class="action-button secondary" type="button" data-lock-index="%d">%s</button>`,
				index,
				htmlEscape(label),
			))
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0c1114;
      --panel: #111a1d;
      --panel-alt: #151e21;
      --line: rgba(153, 174, 166, 0.2);
      --text: #f1f5f2;
      --muted: #a8b8b2;
      --accent: #5ed0ac;
      --accent-soft: rgba(94, 208, 172, 0.14);
      --danger: #ef747b;
      --danger-soft: rgba(239, 116, 123, 0.16);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background: linear-gradient(180deg, #151e21 0%%, var(--bg) 74%%);
      color: var(--text);
      min-height: 100vh;
    }
    main {
      max-width: 1180px;
      margin: 0 auto;
      padding: 24px;
      display: grid;
      gap: 18px;
    }
    .hero {
      display: grid;
      gap: 10px;
    }
    .eyebrow {
      display: inline-flex;
      width: fit-content;
      padding: 6px 10px;
      border-radius: 8px;
      background: var(--accent-soft);
      color: var(--accent);
      font-size: 13px;
      letter-spacing: 0;
      text-transform: uppercase;
    }
    h1 {
      margin: 0;
      font-size: clamp(28px, 5vw, 44px);
      line-height: 1.04;
    }
    .subtle {
      margin: 0;
      color: var(--muted);
      max-width: 76ch;
    }
    .layout {
      display: grid;
      grid-template-columns: minmax(0, 2fr) minmax(300px, 1fr);
      gap: 18px;
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
      box-shadow: 0 18px 52px rgba(0, 0, 0, 0.28);
    }
    .media-frame {
      position: relative;
      background: #000;
    }
    video {
      width: 100%%;
      aspect-ratio: 16 / 9;
      display: block;
      background: #000;
    }
    .status-pill {
      position: absolute;
      top: 16px;
      left: 16px;
      padding: 8px 12px;
      border-radius: 8px;
      background: rgba(4, 10, 16, 0.82);
      border: 1px solid rgba(255,255,255,0.08);
      color: var(--text);
      font-size: 14px;
      backdrop-filter: blur(10px);
    }
    .panel-body {
      padding: 18px;
      display: grid;
      gap: 16px;
    }
    .section-title {
      margin: 0;
      font-size: 15px;
      letter-spacing: 0;
      text-transform: uppercase;
      color: var(--muted);
    }
    .profile-links {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    .profile-links a {
      padding: 10px 12px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: rgba(255,255,255,0.02);
      color: var(--text);
      text-decoration: none;
    }
    .profile-links a.active {
      background: var(--accent-soft);
      color: var(--accent);
      border-color: rgba(91, 227, 189, 0.42);
    }
    .actions {
      display: grid;
      gap: 12px;
    }
    .action-row {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
    }
    .action-button {
      appearance: none;
      border: 0;
      border-radius: 8px;
      padding: 12px 16px;
      font: inherit;
      cursor: pointer;
      color: #08111a;
      background: var(--accent);
      font-weight: 600;
      min-width: 148px;
    }
    .action-button.secondary {
      background: #e8eef5;
    }
    .action-button.danger {
      background: var(--danger);
      color: white;
    }
    .action-button:disabled {
      cursor: wait;
      opacity: 0.6;
    }
    .status-grid {
      display: grid;
      gap: 12px;
    }
    .status-row {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      padding-bottom: 10px;
      border-bottom: 1px solid rgba(255,255,255,0.07);
    }
    .status-row:last-child {
      border-bottom: 0;
      padding-bottom: 0;
    }
    .status-label {
      color: var(--muted);
    }
    .call-state {
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0;
    }
    .call-state.ringing { color: var(--accent); }
    .call-state.idle { color: var(--muted); }
    code {
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 100%%;
    }
    .toast {
      min-height: 20px;
      color: var(--muted);
      font-size: 14px;
    }
    .toast.error {
      color: var(--danger);
    }
    .empty-note {
      padding: 12px 14px;
      border-radius: 8px;
      border: 1px dashed rgba(255,255,255,0.16);
      color: var(--muted);
    }
    @media (max-width: 940px) {
      .layout {
        grid-template-columns: 1fr;
      }
      .action-row {
        flex-direction: column;
      }
      .action-button {
        width: 100%%;
      }
    }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <div class="eyebrow">Bridge VTO Intercom</div>
      <h1>%s</h1>
      <p class="subtle">This page combines low-latency WebRTC playback, live call-state refresh, answer and hangup controls, and door actions for a Dahua VTO. It can also send browser microphone audio up to the bridge session and, when configured, export that incoming RTP to external bridge-side targets. That uplink is still not directly connected through to VTO talkback.</p>
    </section>
    <section class="layout">
      <article class="panel">
        <div class="media-frame">
          <video id="player" autoplay playsinline controls></video>
          <div class="status-pill" id="webrtc-status">Negotiating media...</div>
        </div>
        <div class="panel-body">
          <div>
            <p class="section-title">Profiles</p>
            <div class="profile-links">%s</div>
          </div>
            <div class="actions">
              <p class="section-title">Actions</p>
              <div class="action-row">
                %s
                <button id="hangup-button" class="action-button danger" type="button">Hang Up Call</button>
                <button id="mic-button" class="action-button" type="button">Enable Microphone</button>
                <button id="reset-button" class="action-button secondary" type="button">Reset Bridge Session</button>
                %s
              </div>
              <div class="action-row">%s</div>
              <div id="action-toast" class="toast"></div>
          </div>
        </div>
      </article>
      <aside class="panel">
        <div class="panel-body">
          <p class="section-title">Call Session</p>
            <div class="status-grid">
              <div class="status-row"><span class="status-label">Call State</span><span id="call-state" class="call-state idle">unknown</span></div>
              <div class="status-row"><span class="status-label">Bridge Session</span><span id="bridge-session">inactive</span></div>
              <div class="status-row"><span class="status-label">Last Source</span><span id="last-source">unknown</span></div>
              <div class="status-row"><span class="status-label">Last Ring</span><span id="last-ring">unknown</span></div>
              <div class="status-row"><span class="status-label">Call Started</span><span id="last-started">unknown</span></div>
              <div class="status-row"><span class="status-label">Call Ended</span><span id="last-ended">unknown</span></div>
              <div class="status-row"><span class="status-label">Duration</span><span id="duration">unknown</span></div>
              <div class="status-row"><span class="status-label">Microphone Uplink</span><span id="mic-state">inactive</span></div>
              <div class="status-row"><span class="status-label">Forwarded RTP Packets</span><span id="forwarded-packets">0</span></div>
              <div class="status-row"><span class="status-label">External RTP Targets</span><span>%s</span></div>
              <div class="status-row"><span class="status-label">ICE</span><span>%s</span></div>
              <div class="status-row"><span class="status-label">Profile</span><span>%s</span></div>
            <div class="status-row"><span class="status-label">Video</span><span>%s</span></div>
            <div class="status-row"><span class="status-label">Audio</span><span>%s</span></div>
            <div class="status-row"><span class="status-label">Snapshot</span><code>%s</code></div>
          </div>
        </div>
      </aside>
    </section>
  </main>
  <script>
    const video = document.getElementById('player');
    const webrtcStatus = document.getElementById('webrtc-status');
    const actionToast = document.getElementById('action-toast');
    const answerButton = document.getElementById('answer-button');
    const hangupButton = document.getElementById('hangup-button');
    const micButton = document.getElementById('mic-button');
    const resetButton = document.getElementById('reset-button');
    const lockButtons = Array.from(document.querySelectorAll('[data-lock-index]'));
    const offerURL = %q;
    const deviceURL = %q;
    const intercomStatusURL = %q;
    const intercomResetURL = %q;
    const answerURL = %q;
    const hangupURL = %q;
    const uplinkEnableURL = %q;
    const uplinkDisableURL = %q;
    const lockURLBase = %q;
    const iceServers = %s;
    let peer = null;
    let reconnectTimer = null;
    let reconnectAttempts = 0;
    let micStream = null;
    let micEnabled = false;
    const exportButton = document.getElementById('export-button');

    function setToast(message, isError) {
      actionToast.textContent = message;
      actionToast.className = isError ? 'toast error' : 'toast';
    }

    function formatValue(value, suffix = '') {
      if (value === null || value === undefined || value === '') {
        return 'unknown';
      }
      return String(value) + suffix;
    }

    function clearReconnectTimer() {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    }

    function closePeer() {
      if (!peer) {
        return;
      }
      try {
        peer.ontrack = null;
        peer.onconnectionstatechange = null;
        peer.close();
      } catch (_) {}
      peer = null;
    }

    function reconnectDelayMilliseconds() {
      return Math.min(1000 * Math.pow(2, Math.min(reconnectAttempts, 4)), 10000);
    }

    function scheduleReconnect(reason) {
      if (reconnectTimer) {
        return;
      }
      reconnectAttempts += 1;
      const delay = reconnectDelayMilliseconds();
      webrtcStatus.textContent = 'Reconnecting in ' + Math.max(1, Math.round(delay / 1000)) + 's';
      if (reason) {
        console.warn('intercom reconnect scheduled:', reason);
      }
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connectMedia(micEnabled, true).catch(handleMediaError);
      }, delay);
    }

    function handleMediaError(error) {
      console.error(error);
      webrtcStatus.textContent = 'Retrying...';
      scheduleReconnect(error && error.message ? error.message : String(error));
    }

    async function postAction(url, button, successMessage) {
      const previous = button.textContent;
      button.disabled = true;
      setToast('');
      try {
        const response = await fetch(url, { method: 'POST' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        setToast(successMessage, false);
        await refreshState();
        await refreshIntercomStatus();
      } catch (error) {
        setToast(error.message || String(error), true);
      } finally {
        button.disabled = false;
        button.textContent = previous;
      }
    }

    async function refreshState() {
      try {
        const response = await fetch(deviceURL, { cache: 'no-store' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        const payload = await response.json();
        const state = payload && payload.states ? payload.states[%q] : null;
        const info = state && state.info ? state.info : {};
        const callState = formatValue(info.call_state).toLowerCase();
        const callStateEl = document.getElementById('call-state');
        callStateEl.textContent = formatValue(info.call_state);
        callStateEl.className = 'call-state ' + (callState === 'ringing' ? 'ringing' : 'idle');
        document.getElementById('last-source').textContent = formatValue(info.last_call_source);
        document.getElementById('last-ring').textContent = formatValue(info.last_ring_at);
        document.getElementById('last-started').textContent = formatValue(info.last_call_started_at);
        document.getElementById('last-ended').textContent = formatValue(info.last_call_ended_at);
        document.getElementById('duration').textContent = formatValue(info.last_call_duration_seconds, info.last_call_duration_seconds ? ' s' : '');
      } catch (error) {
        setToast('State refresh failed: ' + (error.message || String(error)), true);
      }
    }

    async function refreshIntercomStatus() {
      try {
        const response = await fetch(intercomStatusURL, { cache: 'no-store' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        const status = await response.json();
        const sessionCount = Number(status.session_count || 0);
        const forwardedPackets = Number(status.uplink_forwarded_packets || 0);
        const uplinkActive = Boolean(status.uplink_active);
        const externalUplinkEnabled = Boolean(status.external_uplink_enabled);
        document.getElementById('bridge-session').textContent = sessionCount > 0 ? ('active (' + sessionCount + ')') : 'inactive';
        document.getElementById('forwarded-packets').textContent = String(forwardedPackets);
        if (exportButton) {
          exportButton.textContent = externalUplinkEnabled ? 'Disable RTP Export' : 'Enable RTP Export';
        }
        if (uplinkActive) {
          document.getElementById('mic-state').textContent = 'active in bridge';
        } else if (micEnabled) {
          document.getElementById('mic-state').textContent = 'browser armed';
        } else {
          document.getElementById('mic-state').textContent = 'inactive';
        }
      } catch (error) {
        setToast('Intercom status refresh failed: ' + (error.message || String(error)), true);
      }
    }

    async function resetBridgeSession() {
      resetButton.disabled = true;
      setToast('');
      try {
        const response = await fetch(intercomResetURL, { method: 'POST' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        clearReconnectTimer();
        closePeer();
        await refreshIntercomStatus();
        await connectMedia(micEnabled, true);
        setToast('Bridge media session reset.', false);
      } catch (error) {
        setToast('Bridge session reset failed: ' + (error.message || String(error)), true);
      } finally {
        resetButton.disabled = false;
      }
    }

    async function toggleExport() {
      if (!exportButton) {
        return;
      }
      exportButton.disabled = true;
      setToast('');
      try {
        const currentlyEnabled = exportButton.textContent.indexOf('Disable') === 0;
        const response = await fetch(currentlyEnabled ? uplinkDisableURL : uplinkEnableURL, { method: 'POST' });
        if (!response.ok) {
          throw new Error(await response.text());
        }
        await refreshIntercomStatus();
        setToast(currentlyEnabled ? 'External RTP export disabled.' : 'External RTP export enabled.', false);
      } catch (error) {
        setToast('External RTP export update failed: ' + (error.message || String(error)), true);
      } finally {
        exportButton.disabled = false;
      }
    }

    async function connectMedia(withMicrophone, isReconnect) {
      clearReconnectTimer();
      closePeer();
      webrtcStatus.textContent = isReconnect ? 'Reconnecting...' : 'Negotiating media...';

      const pc = new RTCPeerConnection({ iceServers });
      peer = pc;
      const stream = new MediaStream();
      video.srcObject = stream;

      pc.addTransceiver('video', { direction: 'recvonly' });
      pc.addTransceiver('audio', { direction: 'recvonly' });
      if (withMicrophone) {
        if (!micStream) {
          micStream = await navigator.mediaDevices.getUserMedia({ audio: true });
        }
        for (const track of micStream.getAudioTracks()) {
          pc.addTrack(track, micStream);
        }
      }
      pc.ontrack = event => {
        if (peer !== pc) {
          return;
        }
        stream.addTrack(event.track);
        reconnectAttempts = 0;
        webrtcStatus.textContent = 'Connected';
      };
      pc.onconnectionstatechange = () => {
        if (peer !== pc || !pc.connectionState) {
          return;
        }
        switch (pc.connectionState) {
        case 'connected':
          reconnectAttempts = 0;
          webrtcStatus.textContent = 'Connected';
          return;
        case 'new':
          webrtcStatus.textContent = 'Negotiating media...';
          return;
        case 'connecting':
          webrtcStatus.textContent = 'Connecting...';
          return;
        case 'disconnected':
        case 'failed':
          webrtcStatus.textContent = pc.connectionState;
          scheduleReconnect('connection ' + pc.connectionState);
          return;
        case 'closed':
          if (peer === pc) {
            scheduleReconnect('connection closed');
          }
          return;
        default:
          webrtcStatus.textContent = pc.connectionState;
        }
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      if (pc.iceGatheringState !== 'complete') {
        await new Promise(resolve => {
          const onChange = () => {
            if (pc.iceGatheringState === 'complete') {
              pc.removeEventListener('icegatheringstatechange', onChange);
              resolve();
            }
          };
          pc.addEventListener('icegatheringstatechange', onChange);
        });
      }

      const response = await fetch(offerURL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(pc.localDescription),
      });
      if (!response.ok) {
        closePeer();
        throw new Error(await response.text());
      }
      const answer = await response.json();
      await pc.setRemoteDescription(answer);
      video.play().catch(() => {
        webrtcStatus.textContent = 'Ready - press play if autoplay was blocked';
      });
      if (webrtcStatus.textContent === 'Connected') {
        return;
      }
      webrtcStatus.textContent = 'Waiting for media...';
    }

    async function toggleMicrophone() {
      const nextEnabled = !micEnabled;
      micButton.disabled = true;
      setToast('');
      try {
        if (!nextEnabled && micStream) {
          for (const track of micStream.getTracks()) {
            track.stop();
          }
          micStream = null;
        }
        await connectMedia(nextEnabled, true);
        micEnabled = nextEnabled;
        micButton.textContent = micEnabled ? 'Disable Microphone' : 'Enable Microphone';
        document.getElementById('mic-state').textContent = micEnabled ? 'browser armed' : 'inactive';
        await refreshIntercomStatus();
        if (micEnabled) {
          setToast('Browser microphone uplink is now connected to the bridge session.', false);
        } else {
          setToast('Browser microphone uplink is now disconnected.', false);
        }
      } catch (error) {
        if (!nextEnabled) {
          micStream = null;
        }
        setToast('Microphone setup failed: ' + (error.message || String(error)), true);
      } finally {
        micButton.disabled = false;
      }
    }

    if (answerButton) {
      answerButton.addEventListener('click', () => {
        postAction(answerURL, answerButton, 'Call answer requested.');
      });
    }
    hangupButton.addEventListener('click', () => {
      postAction(hangupURL, hangupButton, 'Call hangup requested.');
    });
    micButton.addEventListener('click', () => {
      toggleMicrophone();
    });
    resetButton.addEventListener('click', () => {
      resetBridgeSession();
    });
    if (exportButton) {
      exportButton.addEventListener('click', () => {
        toggleExport();
      });
    }
    for (const button of lockButtons) {
      button.addEventListener('click', () => {
        const index = button.getAttribute('data-lock-index');
        postAction(lockURLBase + '/' + index + '/unlock', button, 'Door action sent.');
      });
    }

    window.addEventListener('beforeunload', () => {
      clearReconnectTimer();
      closePeer();
    });
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
        return;
      }
      if (!peer || peer.connectionState === 'failed' || peer.connectionState === 'disconnected' || peer.connectionState === 'closed') {
        scheduleReconnect('page became visible');
      }
    });

    refreshState();
    refreshIntercomStatus();
    setInterval(refreshState, 2000);
    setInterval(refreshIntercomStatus, 2000);
    connectMedia(false, false).catch(error => {
      setToast('Media negotiation failed: ' + (error.message || String(error)), true);
      handleMediaError(error);
    });
  </script>
</body>
</html>`,
		title,
		htmlEscape(entry.Name),
		profileLinks,
		boolHTMLButton(showAnswerControl, `<button id="answer-button" class="action-button" type="button">Answer Call</button>`),
		boolHTMLButton(showExternalUplinkControl, `<button id="export-button" class="action-button secondary" type="button">Disable RTP Export</button>`),
		strings.Join(lockButtons, ""),
		htmlEscape(externalUplinkTargetsLabel),
		htmlEscape(iceModeLabel),
		htmlEscape(profileName),
		htmlEscape(firstNonEmpty(entry.MainCodec+" "+entry.MainResolution, "unknown")),
		htmlEscape(audioLabel),
		htmlEscape(entry.SnapshotURL),
		offerURL,
		deviceURL,
		intercomStatusURL,
		intercomResetURL,
		answerURL,
		hangupURL,
		uplinkEnableURL,
		uplinkDisableURL,
		"/api/v1/vto/"+url.PathEscape(entry.ID)+"/locks",
		iceServersJSON,
		entry.ID,
	)
}

func marshalWebRTCICEServers(iceServers []mediaapi.WebRTCICEServer) string {
	if len(iceServers) == 0 {
		return "[]"
	}
	body, err := json.Marshal(iceServers)
	if err != nil {
		return "[]"
	}
	return string(body)
}

func buildPreviewProfileLinks(entry streams.Entry, selectedProfile string) string {
	ordered := []string{"quality", "default", "stable", "substream"}
	links := make([]string, 0, len(entry.Profiles))
	seen := map[string]struct{}{}

	appendLink := func(profileName string) {
		profile, ok := entry.Profiles[profileName]
		if !ok {
			return
		}
		if _, ok := seen[profileName]; ok {
			return
		}
		seen[profileName] = struct{}{}

		className := ""
		if profileName == selectedProfile {
			className = ` class="active"`
		}
		label := profile.Name
		if label == "" {
			label = profileName
		}
		links = append(links, fmt.Sprintf(
			`<a%s href="/api/v1/media/preview/%s?profile=%s">%s</a>`,
			className,
			url.PathEscape(entry.ID),
			url.QueryEscape(profileName),
			htmlEscape(label),
		))
	}

	for _, profileName := range ordered {
		appendLink(profileName)
	}
	for profileName := range entry.Profiles {
		appendLink(profileName)
	}

	return strings.Join(links, "")
}

func buildIntercomProfileLinks(entry streams.Entry, selectedProfile string) string {
	ordered := []string{"quality", "default", "stable", "substream"}
	links := make([]string, 0, len(entry.Profiles))
	seen := map[string]struct{}{}

	appendLink := func(profileName string) {
		profile, ok := entry.Profiles[profileName]
		if !ok {
			return
		}
		if _, ok := seen[profileName]; ok {
			return
		}
		seen[profileName] = struct{}{}

		className := ""
		if profileName == selectedProfile {
			className = ` class="active"`
		}
		label := profile.Name
		if label == "" {
			label = profileName
		}
		links = append(links, fmt.Sprintf(
			`<a%s href="/api/v1/vto/%s/intercom?profile=%s">%s</a>`,
			className,
			url.PathEscape(entry.ID),
			url.QueryEscape(profileName),
			htmlEscape(label),
		))
	}

	for _, profileName := range ordered {
		appendLink(profileName)
	}
	for profileName := range entry.Profiles {
		appendLink(profileName)
	}

	return strings.Join(links, "")
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}

func boolHTMLButton(enabled bool, body string) string {
	if !enabled {
		return ""
	}
	return body
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Access-Control-Allow-Origin", "*")
		headers.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		headers.Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")
		headers.Add("Vary", "Origin")
		headers.Add("Vary", "Access-Control-Request-Method")
		headers.Add("Vary", "Access-Control-Request-Headers")

		requestHeaders := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers"))
		if requestHeaders != "" {
			headers.Set("Access-Control-Allow-Headers", requestHeaders)
		} else {
			headers.Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
