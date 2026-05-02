package httpserver

import (
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/metrics"

	"github.com/go-chi/chi/v5"
)

type controller struct {
	cfg             config.HTTPConfig
	archiveTempDir  string
	metricsRegistry *metrics.Registry
	probes          ProbeReader
	snapshots       SnapshotReader
	media           MediaReader
	actions         ActionReader
	events          EventReader
	adminLimiter    *perClientRateLimiter
	snapshotLimiter *perClientRateLimiter
	mediaLimiter    *perClientRateLimiter
}

func newController(
	cfg config.HTTPConfig,
	archiveTempDir string,
	metricsRegistry *metrics.Registry,
	probes ProbeReader,
	snapshots SnapshotReader,
	media MediaReader,
	actions ActionReader,
	events EventReader,
	adminLimiter *perClientRateLimiter,
	snapshotLimiter *perClientRateLimiter,
	mediaLimiter *perClientRateLimiter,
) *controller {
	return &controller{
		cfg:             cfg,
		archiveTempDir:  archiveTempDir,
		metricsRegistry: metricsRegistry,
		probes:          probes,
		snapshots:       snapshots,
		media:           media,
		actions:         actions,
		events:          events,
		adminLimiter:    adminLimiter,
		snapshotLimiter: snapshotLimiter,
		mediaLimiter:    mediaLimiter,
	}
}

func (c *controller) registerRoutes(router chi.Router) {
	c.registerCoreRoutes(router)
	c.registerDeviceRoutes(router)
	c.registerNVRRoutes(router)
	c.registerEventRoutes(router)
	c.registerVTORoutes(router)
	c.registerCatalogRoutes(router)
	c.registerMediaRoutes(router)
	c.registerSnapshotRoutes(router)
}
