package httpserver

import (
	"net/http"

	mediaapi "RCooLeR/DahuaBridge/internal/media"

	"github.com/go-chi/chi/v5"
)

func (c *controller) registerCoreRoutes(router chi.Router) {
	router.Get(c.cfg.HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	router.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		stats := toHTTPStatus(c.probes.Stats())
		if !stats.Ready {
			writeJSON(w, http.StatusServiceUnavailable, stats)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	router.Get("/api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, toHTTPStatus(c.probes.Stats()))
	})
	router.Handle("/admin/assets/*", adminAssetHandler())
	router.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		status := toHTTPStatus(c.probes.Stats())
		entries := c.snapshots.ListStreams(false)
		settings := c.snapshots.AdminSettings()
		eventStats := map[string]any{}
		if c.events != nil {
			eventStats = c.events.EventStats()
		}
		workerStatuses := []mediaapi.WorkerStatus{}
		mediaEnabled := false
		if c.media != nil {
			workerStatuses = c.media.ListWorkers()
			mediaEnabled = c.media.Enabled()
		}

		body := renderAdminPage(
			status,
			c.probes.List(),
			entries,
			settings,
			eventStats,
			workerStatuses,
			c.actions != nil,
			c.events != nil,
			mediaEnabled,
			c.cfg.HealthPath,
			c.cfg.MetricsPath,
		)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.Get("/admin/test-bridge", func(w http.ResponseWriter, r *http.Request) {
		mediaEnabled := false
		if c.media != nil {
			mediaEnabled = c.media.Enabled()
		}
		body := renderAdminTestBridgePage(c.snapshots.ListStreams(false), c.actions != nil, mediaEnabled)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	router.Handle(c.cfg.MetricsPath, c.metricsRegistry.Handler())
}
