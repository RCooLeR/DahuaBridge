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
	"RCooLeR/DahuaBridge/internal/ha"
	"RCooLeR/DahuaBridge/internal/haapi"
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
	VTOSnapshot(context.Context, string) ([]byte, string, error)
	IPCSnapshot(context.Context, string) ([]byte, string, error)
	RenderHomeAssistantCameraPackage(ha.CameraPackageOptions) (string, error)
	RenderHomeAssistantDashboardPackage() (string, error)
	RenderHomeAssistantLovelaceDashboard() (string, error)
	ListStreams(bool) []streams.Entry
	AdminSettings() map[string]any
}

type MediaReader interface {
	Enabled() bool
	Subscribe(context.Context, string, string) (<-chan []byte, func(), error)
	HLSPlaylist(context.Context, string, string) ([]byte, error)
	HLSSegment(context.Context, string, string, string) ([]byte, string, error)
	WebRTCAnswer(context.Context, string, string, mediaapi.WebRTCSessionDescription) (mediaapi.WebRTCSessionDescription, error)
	WebRTCICEServers() []mediaapi.WebRTCICEServer
	IntercomStatus(string) mediaapi.IntercomStatus
	StopIntercomSessions(string) mediaapi.IntercomStatus
	SetIntercomUplinkEnabled(string, bool) mediaapi.IntercomStatus
	ListWorkers() []mediaapi.WorkerStatus
}

type ActionReader interface {
	UnlockVTOLock(context.Context, string, int) error
	HangupVTOCall(context.Context, string) error
	ProbeDevice(context.Context, string) (*dahua.ProbeResult, error)
	ProbeAllDevices(context.Context) []dahua.ProbeActionResult
	RotateDeviceCredentials(context.Context, string, dahua.DeviceConfigUpdate) (*dahua.ProbeResult, error)
	RefreshNVRInventory(context.Context, string) (*dahua.ProbeResult, error)
	ProvisionHomeAssistantONVIF(context.Context, haapi.ONVIFProvisionRequest) ([]haapi.ONVIFProvisionResult, error)
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

	router := chi.NewRouter()
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
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := actions.ProbeDevice(actionCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			switch {
			case errors.Is(err, dahua.ErrDeviceNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/devices/{deviceID}/credentials", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		var update dahua.DeviceConfigUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := actions.RotateDeviceCredentials(actionCtx, chi.URLParam(r, "deviceID"), update)
		if err != nil {
			switch {
			case errors.Is(err, dahua.ErrDeviceNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/home-assistant/onvif/provision", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		request, err := parseONVIFProvisionRequest(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		results, err := actions.ProvisionHomeAssistantONVIF(actionCtx, request)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}

		statusCode := http.StatusOK
		statusText := "ok"
		createdCount := 0
		alreadyConfiguredCount := 0
		skippedCount := 0
		errorCount := 0
		for _, result := range results {
			switch result.Status {
			case "created":
				createdCount++
			case "already_configured":
				alreadyConfiguredCount++
			case "skipped":
				skippedCount++
			case "error":
				errorCount++
			}
		}
		if errorCount > 0 {
			statusCode = http.StatusMultiStatus
			statusText = "partial_error"
		}

		writeJSON(w, statusCode, map[string]any{
			"status":                   statusText,
			"requested_count":          len(request.DeviceIDs),
			"result_count":             len(results),
			"created_count":            createdCount,
			"already_configured_count": alreadyConfiguredCount,
			"skipped_count":            skippedCount,
			"error_count":              errorCount,
			"results":                  results,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/nvr/{deviceID}/inventory/refresh", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := actions.RefreshNVRInventory(actionCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			switch {
			case errors.Is(err, dahua.ErrDeviceNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
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
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		lockIndex, err := strconv.Atoi(chi.URLParam(r, "lockIndex"))
		if err != nil || lockIndex < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lock index"})
			return
		}

		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.UnlockVTOLock(controlCtx, deviceID, lockIndex); err != nil {
			switch {
			case errors.Is(err, dahua.ErrDeviceNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"device_id":  deviceID,
			"lock_index": lockIndex,
		})
	})
	router.With(rateLimitMiddleware(adminLimiter)).Post("/api/v1/vto/{deviceID}/call/hangup", func(w http.ResponseWriter, r *http.Request) {
		if actions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := actions.HangupVTOCall(controlCtx, deviceID); err != nil {
			switch {
			case errors.Is(err, dahua.ErrDeviceNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": deviceID,
			"action":    "hangup_call",
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

		var offer mediaapi.WebRTCSessionDescription
		if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
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

		frames, unsubscribe, err := media.Subscribe(r.Context(), streamID, profile)
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
	router.Get("/api/v1/home-assistant/package/cameras.yaml", func(w http.ResponseWriter, r *http.Request) {
		options, err := parseCameraPackageOptions(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		body, err := snapshots.RenderHomeAssistantCameraPackage(options)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.Get("/api/v1/home-assistant/package/cameras_stable.yaml", func(w http.ResponseWriter, r *http.Request) {
		renderCameraPackage(w, r, snapshots, ha.CameraStreamProfileStable)
	})
	router.Get("/api/v1/home-assistant/package/cameras_quality.yaml", func(w http.ResponseWriter, r *http.Request) {
		renderCameraPackage(w, r, snapshots, ha.CameraStreamProfileQuality)
	})
	router.Get("/api/v1/home-assistant/package/cameras_substream.yaml", func(w http.ResponseWriter, r *http.Request) {
		renderCameraPackage(w, r, snapshots, ha.CameraStreamProfileSubstream)
	})
	router.Get("/api/v1/home-assistant/package/cameras_dashboard.yaml", func(w http.ResponseWriter, r *http.Request) {
		body, err := snapshots.RenderHomeAssistantDashboardPackage()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.Get("/api/v1/home-assistant/dashboard/lovelace.yaml", func(w http.ResponseWriter, r *http.Request) {
		body, err := snapshots.RenderHomeAssistantLovelaceDashboard()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.ListenAddress,
			Handler:      router,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		logger: logger.With().Str("component", "http").Logger(),
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
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

func parseCameraPackageOptions(r *http.Request) (ha.CameraPackageOptions, error) {
	options := ha.CameraPackageOptions{
		IncludeCredentials: r.URL.Query().Get("include_credentials") == "true",
	}

	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("profile"))) {
	case "", "default":
		options.Profile = ha.CameraStreamProfileDefault
	case "stable":
		options.Profile = ha.CameraStreamProfileStable
	case "quality":
		options.Profile = ha.CameraStreamProfileQuality
	case "substream":
		options.Profile = ha.CameraStreamProfileSubstream
	default:
		return ha.CameraPackageOptions{}, fmt.Errorf("invalid profile %q", r.URL.Query().Get("profile"))
	}

	if value := strings.TrimSpace(r.URL.Query().Get("rtsp_transport")); value != "" {
		switch value {
		case "tcp", "udp", "udp_multicast", "http":
			options.RTSPTransport = value
		default:
			return ha.CameraPackageOptions{}, fmt.Errorf("invalid rtsp_transport %q", value)
		}
	}

	if value := strings.TrimSpace(r.URL.Query().Get("frame_rate")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return ha.CameraPackageOptions{}, fmt.Errorf("invalid frame_rate %q", value)
		}
		options.FrameRate = parsed
	}

	if value := strings.TrimSpace(r.URL.Query().Get("use_wallclock_as_timestamps")); value != "" {
		switch strings.ToLower(value) {
		case "true", "1", "yes":
			v := true
			options.UseWallclockAsTimestamps = &v
		case "false", "0", "no":
			v := false
			options.UseWallclockAsTimestamps = &v
		default:
			return ha.CameraPackageOptions{}, fmt.Errorf("invalid use_wallclock_as_timestamps %q", value)
		}
	}

	return options, nil
}

func parseONVIFProvisionRequest(r *http.Request) (haapi.ONVIFProvisionRequest, error) {
	if r.Body == nil {
		return haapi.ONVIFProvisionRequest{}, nil
	}

	var request haapi.ONVIFProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return haapi.ONVIFProvisionRequest{}, nil
		}
		return haapi.ONVIFProvisionRequest{}, fmt.Errorf("invalid json body")
	}
	return request, nil
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

func defaultPositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func renderCameraPackage(w http.ResponseWriter, r *http.Request, snapshots SnapshotReader, defaultProfile ha.CameraStreamProfile) {
	options, err := parseCameraPackageOptions(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if options.Profile == ha.CameraStreamProfileDefault {
		options.Profile = defaultProfile
	}
	body, err := snapshots.RenderHomeAssistantCameraPackage(options)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
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
  <style>
    :root {
      color-scheme: dark;
      --bg: #0a1318;
      --bg-soft: #102028;
      --panel: rgba(15, 29, 37, 0.96);
      --line: rgba(157, 200, 189, 0.18);
      --text: #edf6f2;
      --muted: #9ab7ad;
      --accent: #96f0cb;
      --accent-soft: rgba(150, 240, 203, 0.14);
      --warm: #ffd278;
      --danger: #ff8c94;
      --shadow: 0 20px 54px rgba(0, 0, 0, 0.28);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: Georgia, "Times New Roman", serif;
      color: var(--text);
      background:
        radial-gradient(circle at top right, rgba(150, 240, 203, 0.12), transparent 28%%),
        radial-gradient(circle at top left, rgba(255, 210, 120, 0.10), transparent 22%%),
        linear-gradient(180deg, #13242d 0%%, #0a1318 72%%);
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
      gap: 10px;
      padding: 22px 24px;
      border-radius: 22px;
      background: linear-gradient(180deg, rgba(19,36,45,0.98), rgba(10,19,24,0.95));
      border: 1px solid var(--line);
      box-shadow: var(--shadow);
    }
    .eyebrow {
      display: inline-flex;
      width: fit-content;
      padding: 6px 12px;
      border-radius: 999px;
      background: var(--accent-soft);
      color: var(--accent);
      letter-spacing: 0.08em;
      text-transform: uppercase;
      font-size: 12px;
    }
    h1 {
      margin: 0;
      font-size: clamp(32px, 5vw, 54px);
      line-height: 0.98;
      font-weight: 600;
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
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
    .summary-card, .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 22px;
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
      letter-spacing: 0.08em;
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
    button {
      appearance: none;
      border: 0;
      border-radius: 14px;
      padding: 12px 16px;
      cursor: pointer;
      font: inherit;
      font-weight: 600;
      color: #08100d;
      background: var(--accent);
    }
    button.secondary {
      background: #eff6f2;
    }
    button.warm {
      background: var(--warm);
    }
    button.danger {
      background: var(--danger);
      color: white;
    }
    button:disabled {
      cursor: not-allowed;
      opacity: 0.55;
    }
    .result-box, pre {
      margin: 0;
      padding: 14px 16px;
      border-radius: 16px;
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
      border-radius: 18px;
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
      border-radius: 999px;
      background: rgba(255,255,255,0.07);
      color: var(--accent);
      font-size: 12px;
      letter-spacing: 0.06em;
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
      border-radius: 999px;
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
      button {
        width: 100%%;
      }
    }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <div class="eyebrow">DahuaBridge Admin</div>
      <h1>Operator Surface</h1>
      <p class="lead">This page exposes the real bridge control surface in one place: health and runtime summaries, concrete endpoint links, redacted config, stream and device shortcuts, and the highest-value mutating actions without needing to remember URLs.</p>
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
    </section>

    <section class="layout">
      <div class="stack">
        <section class="panel">
          <h2>Admin Actions</h2>
          <p>These buttons call the same authenticated bridge endpoints the rest of the system uses. Responses are shown inline exactly as the API returns them.</p>
          <div class="action-row">
            <button type="button" data-method="POST" data-url="/api/v1/devices/probe-all" data-success="Probe-all requested." %s>Probe All Devices</button>
            <button type="button" class="secondary" data-method="POST" data-url="/api/v1/home-assistant/onvif/provision" data-success="Recommended ONVIF provisioning requested." %s>Provision Recommended ONVIF</button>
            <button type="button" class="warm" data-method="POST" data-url="/api/v1/home-assistant/onvif/provision" data-body='{"force":true}' data-success="Forced ONVIF provisioning requested." %s>Force ONVIF Provisioning</button>
            <button type="button" class="danger" data-method="DELETE" data-url="/api/v1/events" data-success="Event buffer clear requested." %s>Clear Event Buffer</button>
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
		boolHTMLAttr(actionsAvailable),
		boolHTMLAttr(actionsAvailable),
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

func buildAdminEndpointSections(healthPath string, metricsPath string) string {
	sections := []struct {
		Title string
		Items []adminEndpoint
	}{
		{
			Title: "Status And Inventory",
			Items: []adminEndpoint{
				{Method: "GET", Path: "/admin", Description: "Operator page", Linkable: true},
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
				{Method: "GET", Path: "/api/v1/home-assistant/package/cameras.yaml", Description: "Generated camera package", Linkable: true},
				{Method: "GET", Path: "/api/v1/home-assistant/package/cameras_dashboard.yaml", Description: "Generated dashboard package", Linkable: true},
				{Method: "GET", Path: "/api/v1/home-assistant/dashboard/lovelace.yaml", Description: "Generated Lovelace dashboard", Linkable: true},
			},
		},
		{
			Title: "Mutating Admin APIs",
			Items: []adminEndpoint{
				{Method: "POST", Path: "/api/v1/devices/probe-all", Description: "Probe every configured device", Linkable: false},
				{Method: "POST", Path: "/api/v1/home-assistant/onvif/provision", Description: "Push ONVIF provisioning into Home Assistant", Linkable: false},
				{Method: "POST", Path: "/api/v1/devices/{deviceID}/probe", Description: "Probe one specific device", Linkable: false},
				{Method: "POST", Path: "/api/v1/devices/{deviceID}/credentials", Description: "Rotate bridge-side device credentials", Linkable: false},
				{Method: "POST", Path: "/api/v1/nvr/{deviceID}/inventory/refresh", Description: "Refresh NVR channel/disk inventory", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/call/hangup", Description: "Request VTO hangup", Linkable: false},
				{Method: "POST", Path: "/api/v1/vto/{deviceID}/intercom/reset", Description: "Reset active bridge WebRTC intercom session", Linkable: false},
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
		chips := []string{
			`<span class="chip"><a href="/api/v1/devices/` + url.PathEscape(root.ID) + `"><code>/api/v1/devices/` + htmlEscape(root.ID) + `</code></a></span>`,
			`<span class="chip subtle"><a href="/api/v1/streams?device_id=` + url.QueryEscape(root.ID) + `"><code>/api/v1/streams?device_id=` + htmlEscape(root.ID) + `</code></a></span>`,
		}
		switch root.Kind {
		case dahua.DeviceKindVTO:
			chips = append(chips, `<span class="chip"><a href="/api/v1/vto/`+url.PathEscape(root.ID)+`/snapshot"><code>snapshot</code></a></span>`)
		case dahua.DeviceKindIPC:
			chips = append(chips, `<span class="chip"><a href="/api/v1/ipc/`+url.PathEscape(root.ID)+`/snapshot"><code>snapshot</code></a></span>`)
		}

		for _, entry := range streamsByRoot[root.ID] {
			if entry.LocalPreviewURL != "" {
				chips = append(chips, `<span class="chip subtle"><a href="`+htmlEscape(entry.LocalPreviewURL)+`"><code>preview</code></a></span>`)
			}
			if entry.LocalIntercomURL != "" {
				chips = append(chips, `<span class="chip"><a href="`+htmlEscape(entry.LocalIntercomURL)+`"><code>intercom</code></a></span>`)
			}
			break
		}

		cards = append(cards, fmt.Sprintf(
			`<article class="device-card"><h3 class="card-title">%s</h3><div class="card-meta">%s • id=%s • model=%s • children=%d</div><div class="chip-row">%s</div></article>`,
			htmlEscape(firstNonEmpty(root.Name, root.ID)),
			htmlEscape(string(root.Kind)),
			htmlEscape(root.ID),
			htmlEscape(firstNonEmpty(root.Model, "unknown")),
			len(result.Children),
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
			`<span class="chip"><a href="/api/v1/streams/` + url.PathEscape(entry.ID) + `"><code>stream detail</code></a></span>`,
		}
		if entry.SnapshotURL != "" {
			chips = append(chips, `<span class="chip subtle"><a href="`+htmlEscape(entry.SnapshotURL)+`"><code>snapshot</code></a></span>`)
		}
		if entry.LocalPreviewURL != "" {
			chips = append(chips, `<span class="chip"><a href="`+htmlEscape(entry.LocalPreviewURL)+`"><code>preview</code></a></span>`)
		}
		if entry.LocalIntercomURL != "" {
			chips = append(chips, `<span class="chip"><a href="`+htmlEscape(entry.LocalIntercomURL)+`"><code>intercom</code></a></span>`)
		}
		if profile, ok := entry.Profiles[entry.RecommendedProfile]; ok {
			if profile.LocalWebRTCURL != "" {
				chips = append(chips, `<span class="chip subtle"><a href="`+htmlEscape(profile.LocalWebRTCURL)+`"><code>webrtc</code></a></span>`)
			}
			if profile.LocalHLSURL != "" {
				chips = append(chips, `<span class="chip subtle"><a href="`+htmlEscape(profile.LocalHLSURL)+`"><code>hls</code></a></span>`)
			}
			if profile.LocalMJPEGURL != "" {
				chips = append(chips, `<span class="chip subtle"><a href="`+htmlEscape(profile.LocalMJPEGURL)+`"><code>mjpeg</code></a></span>`)
			}
		}

		cards = append(cards, fmt.Sprintf(
			`<article class="stream-card"><h3 class="card-title">%s</h3><div class="card-meta">%s • recommended=%s • video=%s %s • audio=%s</div><div class="chip-row">%s</div></article>`,
			htmlEscape(firstNonEmpty(entry.Name, entry.ID)),
			htmlEscape(string(entry.DeviceKind)),
			htmlEscape(firstNonEmpty(entry.RecommendedProfile, "unknown")),
			htmlEscape(firstNonEmpty(entry.MainCodec, "unknown")),
			htmlEscape(firstNonEmpty(entry.MainResolution, "")),
			htmlEscape(firstNonEmpty(entry.AudioCodec, "none")),
			strings.Join(chips, ""),
		))
	}
	return `<div class="card-grid">` + strings.Join(cards, "") + `</div>`
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
      --bg: #09111a;
      --panel: #132232;
      --panel-alt: #0d1a29;
      --text: #f4f7fb;
      --muted: #9db0c3;
      --line: #294055;
      --accent: #47d7ac;
      --accent-soft: rgba(71, 215, 172, 0.16);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background: radial-gradient(circle at top, #17304a 0, var(--bg) 52%%);
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
      border-radius: 999px;
      background: var(--accent-soft);
      color: var(--accent);
      font-size: 13px;
      letter-spacing: 0.04em;
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
      background: linear-gradient(180deg, rgba(19,34,50,0.96), rgba(13,26,41,0.98));
      border: 1px solid var(--line);
      border-radius: 18px;
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
      border-radius: 14px;
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
      border-radius: 12px;
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
      --bg: #08131d;
      --panel: #0f1f2f;
      --line: #27435b;
      --text: #eff5fb;
      --muted: #9db2c5;
      --accent: #68d9ff;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background: linear-gradient(180deg, #102132 0, #08131d 72%%);
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
      background: rgba(15, 31, 47, 0.95);
      border: 1px solid var(--line);
      border-radius: 18px;
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
      letter-spacing: 0.06em;
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
      --bg: #08111a;
      --panel: #102130;
      --panel-alt: #0b1a28;
      --line: #284255;
      --text: #eff6fb;
      --muted: #9cb2c7;
      --accent: #5be3bd;
      --accent-soft: rgba(91, 227, 189, 0.15);
      --danger: #ff7f8f;
      --danger-soft: rgba(255, 127, 143, 0.16);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, sans-serif;
      background:
        radial-gradient(circle at top left, rgba(91, 227, 189, 0.15), transparent 34%%),
        linear-gradient(180deg, #112334 0%%, #08111a 74%%);
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
      border-radius: 999px;
      background: var(--accent-soft);
      color: var(--accent);
      font-size: 13px;
      letter-spacing: 0.04em;
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
      background: linear-gradient(180deg, rgba(16,33,48,0.96), rgba(11,26,40,0.98));
      border: 1px solid var(--line);
      border-radius: 20px;
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
      border-radius: 999px;
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
      letter-spacing: 0.03em;
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
      border-radius: 12px;
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
      border-radius: 14px;
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
      letter-spacing: 0.04em;
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
      border-radius: 14px;
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
      <p class="subtle">This page combines low-latency WebRTC playback, live call-state refresh, hangup, and door actions for a Dahua VTO. It can also send browser microphone audio up to the bridge session and, when configured, export that incoming RTP to external bridge-side targets. That uplink is still not directly connected through to VTO talkback.</p>
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
    const hangupButton = document.getElementById('hangup-button');
    const micButton = document.getElementById('mic-button');
    const resetButton = document.getElementById('reset-button');
    const lockButtons = Array.from(document.querySelectorAll('[data-lock-index]'));
    const offerURL = %q;
    const deviceURL = %q;
    const intercomStatusURL = %q;
    const intercomResetURL = %q;
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
	ordered := []string{"stable", "quality", "substream", "default"}
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
	ordered := []string{"stable", "quality", "substream", "default"}
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
