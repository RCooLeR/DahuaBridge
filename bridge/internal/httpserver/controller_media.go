package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/ha"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/streams"

	"github.com/go-chi/chi/v5"
)

func (c *controller) registerCatalogRoutes(router chi.Router) {
	router.Get("/api/v1/home-assistant/native/catalog", func(w http.ResponseWriter, r *http.Request) {
		includeCredentials := r.URL.Query().Get("include_credentials") == "true"
		writeJSON(w, http.StatusOK, ha.BuildNativeCatalog(c.probes.List(), c.snapshots.ListStreams(includeCredentials)))
	})
	router.Get("/api/v1/streams", func(w http.ResponseWriter, r *http.Request) {
		includeCredentials := r.URL.Query().Get("include_credentials") == "true"
		entries := c.snapshots.ListStreams(includeCredentials)
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
		for _, entry := range c.snapshots.ListStreams(includeCredentials) {
			if entry.ID == streamID {
				writeJSON(w, http.StatusOK, entry)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream not found"})
	})
}

func (c *controller) registerMediaRoutes(router chi.Router) {
	router.Get("/api/v1/media/workers", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}
		writeJSON(w, http.StatusOK, c.media.ListWorkers())
	})
	router.With(rateLimitMiddleware(c.snapshotLimiter)).Get("/api/v1/media/snapshot/{streamID}", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		scaleWidth, err := parseOptionalPositiveInt(r.URL.Query().Get("width"))
		if err != nil {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		body, contentType, err := c.media.CaptureFrame(r.Context(), chi.URLParam(r, "streamID"), strings.TrimSpace(r.URL.Query().Get("profile")), scaleWidth)
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
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/media/streams/{streamID}/recordings", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		request, err := parseClipStartRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}
		request.StreamID = chi.URLParam(r, "streamID")

		clip, err := c.media.StartClip(r.Context(), request)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, clipAPIResponse(r, clip))
	})
	router.Get("/api/v1/media/recordings", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}

		query, err := parseClipQuery(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}
		clips, err := c.media.FindClips(query)
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
		if c.media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}
		clip, err := c.media.GetClip(chi.URLParam(r, "clipID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, clipAPIResponse(r, clip))
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/media/recordings/{clipID}/stop", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}
		clip, err := c.media.StopClip(r.Context(), chi.URLParam(r, "clipID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, clipAPIResponse(r, clip))
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/media/recordings/{clipID}/download", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}
		path, err := c.media.ClipFilePath(chi.URLParam(r, "clipID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
		http.ServeFile(w, r, path)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/media/recordings/{clipID}/play", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is not configured"})
			return
		}
		path, err := c.media.ClipFilePath(chi.URLParam(r, "clipID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		http.ServeFile(w, r, path)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/media/preview/{streamID}", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		streamID := chi.URLParam(r, "streamID")
		entry, ok := findStreamEntry(c.snapshots.ListStreams(false), streamID)
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
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/media/webrtc/{streamID}/{profile}", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		streamID := chi.URLParam(r, "streamID")
		profileName := chi.URLParam(r, "profile")
		entry, ok := findStreamEntry(c.snapshots.ListStreams(false), streamID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream not found"})
			return
		}
		profile, ok := entry.Profiles[profileName]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}

		body := renderWebRTCPage(entry, profileName, profile, c.media.WebRTCICEServers())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/media/webrtc/{streamID}/{profile}/offer", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
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

		answer, err := c.media.WebRTCAnswer(offerCtx, chi.URLParam(r, "streamID"), chi.URLParam(r, "profile"), offer)
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
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/media/mjpeg/{streamID}", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
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

		frames, unsubscribe, err := c.media.SubscribeScaled(r.Context(), streamID, profile, scaleWidth)
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
		clearStreamingWriteDeadline(w)

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
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/media/hls/{streamID}/{profile}/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		body, err := c.media.HLSPlaylist(r.Context(), chi.URLParam(r, "streamID"), chi.URLParam(r, "profile"))
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
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/media/hls/{streamID}/{profile}/{segmentName}", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		body, contentType, err := c.media.HLSSegment(r.Context(), chi.URLParam(r, "streamID"), chi.URLParam(r, "profile"), chi.URLParam(r, "segmentName"))
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
}
