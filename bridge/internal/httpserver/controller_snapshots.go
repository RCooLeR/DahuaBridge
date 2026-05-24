package httpserver

import (
	"io/fs"
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
		writeSnapshotImage(w, body, contentType, err)
	})
	router.With(rateLimitMiddleware(c.snapshotLimiter)).Get("/api/v1/vto/{deviceID}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		body, contentType, err := c.snapshots.VTOSnapshot(r.Context(), deviceID)
		writeSnapshotImage(w, body, contentType, err)
	})
	router.With(rateLimitMiddleware(c.snapshotLimiter)).Get("/api/v1/ipc/{deviceID}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		body, contentType, err := c.snapshots.IPCSnapshot(r.Context(), deviceID)
		writeSnapshotImage(w, body, contentType, err)
	})
}

func writeSnapshotImage(w http.ResponseWriter, body []byte, contentType string, err error) {
	if err != nil || len(body) == 0 {
		writeSnapshotPlaceholder(w, err)
		return
	}
	if contentType == "" {
		contentType = "image/jpeg"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeSnapshotPlaceholder(w http.ResponseWriter, snapshotErr error) {
	body, err := fs.ReadFile(embeddedAdminAssets, "logo.png")
	if err != nil || len(body) == 0 {
		if snapshotErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": snapshotErr.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "snapshot unavailable"})
		return
	}
	if snapshotErr != nil {
		w.Header().Set("X-DahuaBridge-Snapshot-Fallback", "error")
	} else {
		w.Header().Set("X-DahuaBridge-Snapshot-Fallback", "empty")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
