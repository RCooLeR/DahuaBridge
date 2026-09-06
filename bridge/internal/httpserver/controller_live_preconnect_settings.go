package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/go-chi/chi/v5"
)

func (c *controller) registerLivePreconnectSettingsRoutes(router chi.Router) {
	router.Get("/api/v1/settings/live-preconnect", func(w http.ResponseWriter, r *http.Request) {
		reader, ok := c.snapshots.(interface {
			GetLivePreconnect() streams.LivePreconnectSettings
		})
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "live preconnect settings are unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, reader.GetLivePreconnect())
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Put("/api/v1/settings/live-preconnect", func(w http.ResponseWriter, r *http.Request) {
		setter, ok := c.snapshots.(interface {
			SetLivePreconnect(context.Context, streams.LivePreconnectSettings) (streams.LivePreconnectSettings, error)
		})
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "live preconnect settings are unavailable"})
			return
		}
		var request streams.LivePreconnectSettings
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		settings, err := setter.SetLivePreconnect(r.Context(), request)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, streams.ErrInvalidLivePreconnect):
				status = http.StatusBadRequest
			case errors.Is(err, streams.ErrLiveSourcePersistence):
				status = http.StatusServiceUnavailable
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
}
