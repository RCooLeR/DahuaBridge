package httpserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"

	"github.com/go-chi/chi/v5"
)

func (c *controller) registerVTORoutes(router chi.Router) {
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/vto/{deviceID}/locks/{lockIndex}/unlock", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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

		if err := c.actions.UnlockVTOLock(controlCtx, deviceID, lockIndex); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"device_id":  deviceID,
			"lock_index": lockIndex,
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/vto/{deviceID}/call/answer", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := c.actions.AnswerVTOCall(controlCtx, deviceID); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": deviceID,
			"action":    "answer_call",
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/vto/{deviceID}/call/hangup", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		controlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := c.actions.HangupVTOCall(controlCtx, deviceID); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": deviceID,
			"action":    "hangup_call",
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Get("/api/v1/vto/{deviceID}/controls", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		controlCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		capabilities, err := c.actions.VTOControlCapabilities(controlCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, capabilities)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/vto/{deviceID}/recording", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
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

		if err := c.actions.SetVTORecordingEnabled(controlCtx, deviceID, enabled); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":              "ok",
			"device_id":           deviceID,
			"auto_record_enabled": enabled,
		})
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/vto/{deviceID}/intercom", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(c.snapshots.ListStreams(false), deviceID)
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

		body := renderVTOIntercomPage(entry, profileName, profile, c.media.WebRTCICEServers())
		writeHTMLPage(w, c.cfg, body)
	})
	router.Get("/api/v1/vto/{deviceID}/intercom/status", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(c.snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}

		writeJSON(w, http.StatusOK, c.media.IntercomStatus(entry.ID))
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/vto/{deviceID}/intercom/reset", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(c.snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}

		writeJSON(w, http.StatusOK, c.media.StopIntercomSessions(entry.ID))
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/vto/{deviceID}/intercom/uplink/enable", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(c.snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}
		if entry.Intercom == nil || !entry.Intercom.SupportsExternalAudioExport {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "external uplink export is not configured"})
			return
		}

		writeJSON(w, http.StatusOK, c.media.SetIntercomUplinkEnabled(entry.ID, true))
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/vto/{deviceID}/intercom/uplink/disable", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		entry, ok := findStreamEntry(c.snapshots.ListStreams(false), deviceID)
		if !ok || entry.DeviceKind != dahua.DeviceKindVTO {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "vto intercom stream not found"})
			return
		}
		if entry.Intercom == nil || !entry.Intercom.SupportsExternalAudioExport {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "external uplink export is not configured"})
			return
		}

		writeJSON(w, http.StatusOK, c.media.SetIntercomUplinkEnabled(entry.ID, false))
	})
}
