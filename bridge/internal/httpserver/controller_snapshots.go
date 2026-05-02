package httpserver

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (c *controller) registerSnapshotRoutes(router chi.Router) {
	router.With(rateLimitMiddleware(c.snapshotLimiter)).Get("/api/v1/nvr/{deviceID}/channels/{channel}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid channel"})
			return
		}

		body, contentType, err := c.snapshots.NVRSnapshot(r.Context(), deviceID, channel)
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
	router.With(rateLimitMiddleware(c.snapshotLimiter)).Get("/api/v1/vto/{deviceID}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		body, contentType, err := c.snapshots.VTOSnapshot(r.Context(), deviceID)
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
	router.With(rateLimitMiddleware(c.snapshotLimiter)).Get("/api/v1/ipc/{deviceID}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		body, contentType, err := c.snapshots.IPCSnapshot(r.Context(), deviceID)
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
}
