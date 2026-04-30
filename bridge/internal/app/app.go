package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
	"RCooLeR/DahuaBridge/internal/dahua/ipc"
	"RCooLeR/DahuaBridge/internal/dahua/nvr"
	"RCooLeR/DahuaBridge/internal/dahua/vto"
	"RCooLeR/DahuaBridge/internal/eventbuffer"
	"RCooLeR/DahuaBridge/internal/httpserver"
	"RCooLeR/DahuaBridge/internal/imou"
	"RCooLeR/DahuaBridge/internal/logging"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

func Run(ctx context.Context, cfg config.Config, info buildinfo.BuildInfo) error {
	logger := logging.New(cfg.Log).With().
		Str("version", info.Version).
		Str("commit", info.Commit).
		Str("build_date", info.BuildDate).
		Logger()

	metricsRegistry := metrics.New(info)
	probeStore := store.NewProbeStore()
	recentEvents := eventbuffer.New(eventbuffer.DefaultCapacity)
	services := newRuntimeServices(cfg, probeStore)
	mediaManager := media.New(cfg.Media, services, logger, metricsRegistry)
	imouClient := imou.NewClient(cfg.Imou)
	services.AttachMedia(mediaManager)

	if err := loadPersistedState(cfg, logger, metricsRegistry, probeStore, imouClient); err != nil {
		return fmt.Errorf("load persisted state: %w", err)
	}

	var persistenceWG sync.WaitGroup
	if cfg.StateStore.Enabled {
		persistenceWG.Add(1)
		go func() {
			defer persistenceWG.Done()
			runStateStoreLoop(ctx, cfg, logger, metricsRegistry, probeStore, imouClient)
		}()
	}

	drivers := buildDrivers(cfg, logger, metricsRegistry, services, imouClient)
	if len(drivers) == 0 {
		return errors.New("no enabled drivers were created from config")
	}
	adminActions := newAdminActions(logger, metricsRegistry, probeStore, services, drivers)
	adminServer := httpserver.New(cfg.HTTP, logger, metricsRegistry, probeStore, services, mediaManager, adminActions, recentEvents)

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- adminServer.Start()
	}()

	var wg sync.WaitGroup
	for _, driver := range drivers {
		wg.Add(1)
		go func(driver dahua.Driver) {
			defer wg.Done()
			runProbeLoop(ctx, logger, metricsRegistry, probeStore, driver)
		}(driver)

		if eventSource, ok := driver.(dahua.EventSource); ok {
			wg.Add(1)
			go func(driver dahua.Driver, eventSource dahua.EventSource) {
				defer wg.Done()
				runEventLoop(ctx, logger, metricsRegistry, probeStore, recentEvents, driver, eventSource)
			}(driver, eventSource)
		}
	}

	select {
	case <-ctx.Done():
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("admin http shutdown failed")
	}

	wg.Wait()
	persistenceWG.Wait()

	if err := persistState(cfg, logger, metricsRegistry, probeStore, imouClient); err != nil {
		logger.Error().Err(err).Msg("final state store flush failed")
	}
	return nil
}

func loadPersistedState(
	cfg config.Config,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes *store.ProbeStore,
	imouClient *imou.Client,
) error {
	if !cfg.StateStore.Enabled {
		return nil
	}

	ok, authState, err := probes.LoadFileWithMetadata(cfg.StateStore.Path)
	metricsRegistry.ObserveStateStore("load", err)
	if err != nil {
		return err
	}
	if ok {
		if imouClient != nil && imouClient.Enabled() && authState != nil {
			imouClient.ImportAuthState(authState)
		}
		stats := probes.Stats()
		event := logger.Info().
			Str("path", cfg.StateStore.Path).
			Int("device_count", stats.DeviceCount)
		if authState != nil && authState.ExpiresAt.After(time.Now()) {
			event = event.Bool("imou_auth_restored", true)
		}
		event.Msg("loaded persisted probe state")
		return nil
	}

	logger.Info().Str("path", cfg.StateStore.Path).Msg("no persisted probe state found")
	return nil
}

func runStateStoreLoop(
	ctx context.Context,
	cfg config.Config,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes *store.ProbeStore,
	imouClient *imou.Client,
) {
	ticker := time.NewTicker(cfg.StateStore.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := persistState(cfg, logger, metricsRegistry, probes, imouClient); err != nil {
				logger.Error().Err(err).Str("path", cfg.StateStore.Path).Msg("state store flush failed")
			}
		}
	}
}

func persistState(
	cfg config.Config,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes *store.ProbeStore,
	imouClient *imou.Client,
) error {
	if !cfg.StateStore.Enabled {
		return nil
	}

	err := probes.SaveFileWithMetadata(cfg.StateStore.Path, imouClient.ExportAuthState())
	metricsRegistry.ObserveStateStore("save", err)
	return err
}

func buildDrivers(
	cfg config.Config,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	services *runtimeServices,
	imouClient imou.Service,
) []dahua.Driver {
	drivers := make([]dahua.Driver, 0, len(cfg.Devices.NVR)+len(cfg.Devices.VTO)+len(cfg.Devices.IPC))

	for _, deviceCfg := range cfg.Devices.NVR {
		if !deviceCfg.EnabledValue() {
			continue
		}
		driver := nvr.New(deviceCfg, cfg.Imou, imouClient, cfg.Devices.IPC, logger, metricsRegistry, cgi.New(deviceCfg, metricsRegistry))
		drivers = append(drivers, driver)
		services.RegisterNVR(deviceCfg.ID, driver, driver, deviceCfg)
	}

	for _, deviceCfg := range cfg.Devices.VTO {
		if !deviceCfg.EnabledValue() {
			continue
		}

		driver := vto.New(deviceCfg, logger, cgi.New(deviceCfg, metricsRegistry))
		drivers = append(drivers, driver)
		services.RegisterVTO(deviceCfg.ID, driver, deviceCfg)
	}

	for _, deviceCfg := range cfg.Devices.IPC {
		if !deviceCfg.EnabledValue() {
			continue
		}

		driver := ipc.New(deviceCfg, logger, cgi.New(deviceCfg, metricsRegistry))
		drivers = append(drivers, driver)
		services.RegisterIPC(deviceCfg.ID, driver, deviceCfg)
	}

	return drivers
}

func runProbeLoop(
	ctx context.Context,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes *store.ProbeStore,
	driver dahua.Driver,
) {
	interval := driver.PollInterval()
	if interval <= 0 {
		interval = 30 * time.Second
	}

	log := logger.With().
		Str("device_id", driver.ID()).
		Str("device_type", string(driver.Kind())).
		Logger()

	probeOnce := func() {
		started := time.Now()
		result, err := driver.Probe(ctx)
		metricsRegistry.ObserveProbe(driver.ID(), string(driver.Kind()), started, err)
		if err != nil {
			metricsRegistry.DeviceAvailability.WithLabelValues(driver.ID(), string(driver.Kind())).Set(0)
			log.Error().Err(err).Msg("device probe failed")
			return
		}

		metricsRegistry.DeviceAvailability.WithLabelValues(driver.ID(), string(driver.Kind())).Set(1)
		probes.Set(driver.ID(), result)

		log.Info().
			Str("name", result.Root.Name).
			Str("model", result.Root.Model).
			Str("serial", result.Root.Serial).
			Msg("device probe succeeded")
	}

	probeOnce()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probeOnce()
		}
	}
}

func runEventLoop(
	ctx context.Context,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes *store.ProbeStore,
	recentEvents *eventbuffer.Buffer,
	driver dahua.Driver,
	eventSource dahua.EventSource,
) {
	log := logger.With().
		Str("device_id", driver.ID()).
		Str("device_type", string(driver.Kind())).
		Str("component", "event_stream").
		Logger()

	events := make(chan dahua.Event, 32)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-events:
				handleEvent(log, metricsRegistry, probes, recentEvents, event)
			}
		}
	}()

	err := eventSource.StreamEvents(ctx, events)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("event stream stopped")
	}
}

func handleEvent(
	log zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes *store.ProbeStore,
	recentEvents *eventbuffer.Buffer,
	event dahua.Event,
) {
	if recentEvents != nil {
		recentEvents.Add(event)
	}

	metricsRegistry.ObserveEvent(
		event.DeviceID,
		string(event.DeviceKind),
		event.Code,
		string(event.Action),
		fmt.Sprintf("%d", event.Channel),
	)

	if event.ChildID != "" {
		updateEventState(log, probes, event.DeviceID, event.ChildID, event)
	}

	if event.DeviceKind == dahua.DeviceKindVTO {
		updateEventState(log, probes, event.DeviceID, event.DeviceID, event)
	}

	log.Debug().
		Str("code", event.Code).
		Str("action", string(event.Action)).
		Int("channel", event.Channel).
		Msg("processed device event")
}

func updateEventState(
	log zerolog.Logger,
	probes *store.ProbeStore,
	rootID string,
	targetID string,
	event dahua.Event,
) {
	stateKey, active, ok := boolStateForEvent(event)
	if !ok {
		return
	}

	probes.Update(rootID, func(result *dahua.ProbeResult) {
		state := result.States[targetID]
		if state.Info == nil {
			state.Info = make(map[string]any)
		}
		state.Available = true
		state.Info[stateKey] = active
		state.Info["last_event_type"] = eventTypeForEvent(event)
		state.Info["last_event_at"] = event.OccurredAt.Format(time.RFC3339Nano)
		if event.DeviceKind == dahua.DeviceKindVTO && targetID == rootID {
			for key, value := range vto.SessionStateUpdatesForApp(state.Info, event) {
				state.Info[key] = value
			}
		}
		result.States[targetID] = state
	})

	log.Debug().Str("target_id", targetID).Str("state_key", stateKey).Bool("active", active).Msg("updated event state")
}

func boolStateForEvent(event dahua.Event) (string, bool, bool) {
	switch event.DeviceKind {
	case dahua.DeviceKindNVR:
		return nvr.BoolStateFromEventForApp(event)
	case dahua.DeviceKindIPC:
		return ipc.BoolStateFromEventForApp(event)
	case dahua.DeviceKindVTO:
		return vto.BoolStateFromEventForApp(event)
	default:
		return "", false, false
	}
}

func eventTypeForEvent(event dahua.Event) string {
	switch event.DeviceKind {
	case dahua.DeviceKindNVR:
		return nvr.EventTypeForApp(event)
	case dahua.DeviceKindIPC:
		return ipc.EventTypeForApp(event)
	case dahua.DeviceKindVTO:
		return vto.EventTypeForApp(event)
	default:
		return "unknown_state"
	}
}
