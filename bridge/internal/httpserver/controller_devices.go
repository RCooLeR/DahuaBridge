package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"

	"github.com/go-chi/chi/v5"
)

func (c *controller) registerDeviceRoutes(router chi.Router) {
	router.Get("/api/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, c.probes.List())
	})
	router.Get("/api/v1/devices/{deviceID}", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		result, ok := c.probes.Get(deviceID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/devices/probe-all", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		results := c.actions.ProbeAllDevices(actionCtx)
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
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/devices/{deviceID}/probe", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := c.actions.ProbeDevice(actionCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/devices/{deviceID}/credentials", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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

		result, err := c.actions.RotateDeviceCredentials(actionCtx, chi.URLParam(r, "deviceID"), update)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
}

func (c *controller) registerNVRRoutes(router chi.Router) {
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/inventory/refresh", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := c.actions.RefreshNVRInventory(actionCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Get("/api/v1/nvr/{deviceID}/recordings", func(w http.ResponseWriter, r *http.Request) {
		query, err := parseNVRRecordingQuery(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		searchCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := c.snapshots.NVRRecordings(searchCtx, chi.URLParam(r, "deviceID"), query)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		normalizeNVRRecordingSearchResult(&result)
		attachNVRRecordingExportURLs(r, chi.URLParam(r, "deviceID"), &result)
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Get("/api/v1/nvr/{deviceID}/events/summary", func(w http.ResponseWriter, r *http.Request) {
		query, err := parseNVREventSummaryQuery(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		summaryCtx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()

		summary, err := buildNVREventSummary(summaryCtx, c.snapshots, chi.URLParam(r, "deviceID"), query)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, summary)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/nvr/{deviceID}/recordings/export", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		playbackRequest, profile, duration, err := parseNVRRecordingExportRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		session, err := c.snapshots.CreateNVRPlaybackSession(r.Context(), chi.URLParam(r, "deviceID"), playbackRequest)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		clip, err := c.media.StartClip(r.Context(), mediaapi.ClipStartRequest{
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
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/nvr/{deviceID}/recordings/download", func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimSpace(r.URL.Query().Get("file_path"))
		if filePath == "" {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "file_path is required")
			return
		}

		download, err := c.snapshots.NVRDownloadRecording(r.Context(), chi.URLParam(r, "deviceID"), filePath)
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
	router.With(rateLimitMiddleware(c.adminLimiter)).Get("/api/v1/nvr/{deviceID}/channels/{channel}/controls", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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

		capabilities, err := c.actions.NVRChannelControlCapabilities(controlCtx, chi.URLParam(r, "deviceID"), channel)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, capabilities)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/ptz", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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

		if err := c.actions.ControlNVRPTZ(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
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
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/aux", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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

		if err := c.actions.ControlNVRAux(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
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
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/audio/mute", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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
		if err := c.actions.ControlNVRAudio(controlCtx, chi.URLParam(r, "deviceID"), dahua.NVRAudioRequest{
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
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/recording", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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

		if err := c.actions.ControlNVRRecording(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
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
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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

		result, err := c.actions.NVRDiagnosticAction(controlCtx, chi.URLParam(r, "deviceID"), request)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/nvr/{deviceID}/playback/sessions", func(w http.ResponseWriter, r *http.Request) {
		request, err := parseNVRPlaybackSessionRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		session, err := c.snapshots.CreateNVRPlaybackSession(r.Context(), chi.URLParam(r, "deviceID"), request)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/nvr/playback/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		session, err := c.snapshots.GetNVRPlaybackSession(chi.URLParam(r, "sessionID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/nvr/playback/sessions/{sessionID}/seek", func(w http.ResponseWriter, r *http.Request) {
		seekTime, err := parseNVRPlaybackSeekTime(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		session, err := c.snapshots.SeekNVRPlaybackSession(r.Context(), chi.URLParam(r, "sessionID"), seekTime)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
}

func (c *controller) registerEventRoutes(router chi.Router) {
	router.Get("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if c.events == nil {
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
			"stats":  c.events.EventStats(),
			"events": c.events.ListEvents(deviceID, childID, deviceKind, code, action, limit),
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Delete("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if c.events == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event buffer is not configured"})
			return
		}

		removed := c.events.ClearEvents()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":        "ok",
			"removed_count": removed,
			"stats":         c.events.EventStats(),
		})
	})
}
