package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	"RCooLeR/DahuaBridge/internal/ha"
	"RCooLeR/DahuaBridge/internal/haapi"
	"RCooLeR/DahuaBridge/internal/httpserver"
	"RCooLeR/DahuaBridge/internal/logging"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/mqtt"
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
	mqttClient := mqtt.New(cfg.MQTT, logger, metricsRegistry)
	discovery := ha.NewDiscoveryPublisher(cfg, mqttClient, logger)
	probeStore := store.NewProbeStore()
	recentEvents := eventbuffer.New(eventbuffer.DefaultCapacity)
	services := newRuntimeServices(cfg, probeStore)
	mediaManager := media.New(cfg.Media, services, logger, metricsRegistry)
	services.AttachMedia(mediaManager)
	haClient := haapi.New(cfg.HomeAssistant)

	if err := loadPersistedState(cfg, logger, metricsRegistry, probeStore); err != nil {
		return fmt.Errorf("load persisted state: %w", err)
	}

	if err := mqttClient.Connect(ctx); err != nil {
		return fmt.Errorf("connect mqtt: %w", err)
	}
	defer mqttClient.Close()

	if err := republishPersistedState(logger, discovery, probeStore); err != nil {
		return fmt.Errorf("republish persisted state: %w", err)
	}

	var persistenceWG sync.WaitGroup
	if cfg.StateStore.Enabled {
		persistenceWG.Add(1)
		go func() {
			defer persistenceWG.Done()
			runStateStoreLoop(ctx, cfg, logger, metricsRegistry, probeStore)
		}()
	}

	drivers := buildDrivers(cfg, logger, metricsRegistry, services)
	if len(drivers) == 0 {
		return errors.New("no enabled drivers were created from config")
	}
	adminActions := newAdminActions(logger, metricsRegistry, discovery, probeStore, services, haClient, drivers)
	adminServer := httpserver.New(cfg.HTTP, logger, metricsRegistry, probeStore, services, mediaManager, adminActions, recentEvents)
	if err := registerCommandHandlers(ctx, cfg, logger, mqttClient, probeStore, drivers, mediaManager); err != nil {
		return fmt.Errorf("register command handlers: %w", err)
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- adminServer.Start()
	}()

	var wg sync.WaitGroup
	for _, driver := range drivers {
		wg.Add(1)
		go func(driver dahua.Driver) {
			defer wg.Done()
			runProbeLoop(ctx, logger, metricsRegistry, discovery, probeStore, driver)
		}(driver)

		if eventSource, ok := driver.(dahua.EventSource); ok {
			wg.Add(1)
			go func(driver dahua.Driver, eventSource dahua.EventSource) {
				defer wg.Done()
				runEventLoop(ctx, logger, metricsRegistry, discovery, probeStore, recentEvents, driver, eventSource)
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

	if err := persistState(cfg, logger, metricsRegistry, probeStore); err != nil {
		logger.Error().Err(err).Msg("final state store flush failed")
	}
	return nil
}

func loadPersistedState(
	cfg config.Config,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes *store.ProbeStore,
) error {
	if !cfg.StateStore.Enabled {
		return nil
	}

	ok, err := probes.LoadFile(cfg.StateStore.Path)
	metricsRegistry.ObserveStateStore("load", err)
	if err != nil {
		return err
	}
	if ok {
		stats := probes.Stats()
		logger.Info().
			Str("path", cfg.StateStore.Path).
			Int("device_count", stats.DeviceCount).
			Msg("loaded persisted probe state")
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
) {
	ticker := time.NewTicker(cfg.StateStore.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := persistState(cfg, logger, metricsRegistry, probes); err != nil {
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
) error {
	if !cfg.StateStore.Enabled {
		return nil
	}

	err := probes.SaveFile(cfg.StateStore.Path)
	metricsRegistry.ObserveStateStore("save", err)
	return err
}

func republishPersistedState(
	logger zerolog.Logger,
	discovery *ha.DiscoveryPublisher,
	probes *store.ProbeStore,
) error {
	results := probes.List()
	if len(results) == 0 {
		return nil
	}

	for _, result := range results {
		if err := discovery.PublishProbe(context.Background(), result); err != nil {
			return err
		}
	}

	logger.Info().Int("device_count", len(results)).Msg("republished persisted probe state")
	return nil
}

func buildDrivers(
	cfg config.Config,
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	services *runtimeServices,
) []dahua.Driver {
	drivers := make([]dahua.Driver, 0, len(cfg.Devices.NVR)+len(cfg.Devices.VTO)+len(cfg.Devices.IPC))

	for _, deviceCfg := range cfg.Devices.NVR {
		if !deviceCfg.EnabledValue() {
			continue
		}
		driver := nvr.New(deviceCfg, logger, cgi.New(deviceCfg, metricsRegistry))
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
	discovery *ha.DiscoveryPublisher,
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
			if publishErr := discovery.PublishUnavailable(context.Background(), driver.ID()); publishErr != nil {
				log.Warn().Err(publishErr).Msg("publish unavailable state failed")
			}
			return
		}

		metricsRegistry.DeviceAvailability.WithLabelValues(driver.ID(), string(driver.Kind())).Set(1)
		probes.Set(driver.ID(), result)
		if err := discovery.PublishProbe(context.Background(), result); err != nil {
			log.Error().Err(err).Msg("publish probe result failed")
			return
		}
		publishProbeCameraSnapshots(log, discovery, driver, result)

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
	discovery *ha.DiscoveryPublisher,
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
				handleEvent(context.Background(), log, metricsRegistry, discovery, probes, recentEvents, event)
			}
		}
	}()

	err := eventSource.StreamEvents(ctx, events)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("event stream stopped")
	}
}

func handleEvent(
	ctx context.Context,
	log zerolog.Logger,
	metricsRegistry *metrics.Registry,
	discovery *ha.DiscoveryPublisher,
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

	payload := map[string]any{
		"code":        event.Code,
		"action":      event.Action,
		"channel":     event.Channel,
		"index":       event.Index,
		"occurred_at": event.OccurredAt.Format(time.RFC3339Nano),
		"device_id":   event.DeviceID,
	}
	for key, value := range event.Data {
		payload[key] = value
	}

	if event.ChildID != "" {
		if err := publishEventState(ctx, log, discovery, probes, event.DeviceID, event.ChildID, event); err != nil {
			log.Warn().Err(err).Str("child_id", event.ChildID).Msg("publish child event state failed")
		}
		if err := discovery.PublishEvent(ctx, event.ChildID, eventTypeForEvent(event), payload); err != nil {
			log.Warn().Err(err).Str("child_id", event.ChildID).Msg("publish activity event failed")
		}
	}

	if event.DeviceKind == dahua.DeviceKindVTO {
		if err := publishEventState(ctx, log, discovery, probes, event.DeviceID, event.DeviceID, event); err != nil {
			log.Warn().Err(err).Str("device_id", event.DeviceID).Msg("publish root vto event state failed")
		}
		if err := discovery.PublishEvent(ctx, event.DeviceID, eventTypeForEvent(event), payload); err != nil {
			log.Warn().Err(err).Str("device_id", event.DeviceID).Msg("publish root vto activity event failed")
		}
	}

	log.Debug().
		Str("code", event.Code).
		Str("action", string(event.Action)).
		Int("channel", event.Channel).
		Msg("processed device event")
}

func publishEventState(
	ctx context.Context,
	log zerolog.Logger,
	discovery *ha.DiscoveryPublisher,
	probes *store.ProbeStore,
	rootID string,
	targetID string,
	event dahua.Event,
) error {
	stateKey, active, ok := boolStateForEvent(event)
	if !ok {
		return nil
	}

	if err := discovery.PublishBinaryState(ctx, targetID, stateKey, active); err != nil {
		return err
	}

	derivedStates := map[string]string{}
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
				derivedStates[key] = value
			}
		}
		result.States[targetID] = state
	})
	for field, value := range derivedStates {
		if err := discovery.PublishState(ctx, targetID, field, value, true); err != nil {
			return err
		}
	}

	log.Debug().Str("target_id", targetID).Str("state_key", stateKey).Bool("active", active).Msg("updated event state")
	return nil
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

type cameraSnapshotTarget struct {
	deviceID string
	channel  int
}

func publishProbeCameraSnapshots(
	log zerolog.Logger,
	discovery *ha.DiscoveryPublisher,
	driver dahua.Driver,
	result *dahua.ProbeResult,
) {
	if discovery == nil || result == nil {
		return
	}

	if discovery.LogoCameraSnapshots() {
		payload := ha.CameraSnapshotPlaceholder()
		for _, target := range snapshotTargetsForProbeResult(result) {
			if err := discovery.PublishCameraSnapshot(context.Background(), target.deviceID, payload); err != nil {
				log.Debug().Err(err).Str("snapshot_device_id", target.deviceID).Int("snapshot_channel", target.channel).Msg("camera snapshot mqtt publish failed")
			}
		}
		return
	}

	provider, ok := driver.(dahua.SnapshotProvider)
	if !ok {
		return
	}

	for _, target := range snapshotTargetsForProbeResult(result) {
		snapshotCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		body, _, err := provider.Snapshot(snapshotCtx, target.channel)
		cancel()
		if err != nil {
			log.Debug().Err(err).Str("snapshot_device_id", target.deviceID).Int("snapshot_channel", target.channel).Msg("camera snapshot publish skipped")
			continue
		}
		if err := discovery.PublishCameraSnapshot(context.Background(), target.deviceID, body); err != nil {
			log.Debug().Err(err).Str("snapshot_device_id", target.deviceID).Int("snapshot_channel", target.channel).Msg("camera snapshot mqtt publish failed")
		}
	}
}

func snapshotTargetsForProbeResult(result *dahua.ProbeResult) []cameraSnapshotTarget {
	if result == nil {
		return nil
	}

	switch result.Root.Kind {
	case dahua.DeviceKindNVR:
		targets := make([]cameraSnapshotTarget, 0, len(result.Children))
		for _, child := range result.Children {
			if child.Kind != dahua.DeviceKindNVRChannel {
				continue
			}
			channel, err := strconv.Atoi(child.Attributes["channel_index"])
			if err != nil || channel <= 0 {
				continue
			}
			targets = append(targets, cameraSnapshotTarget{
				deviceID: child.ID,
				channel:  channel,
			})
		}
		return targets
	case dahua.DeviceKindVTO:
		return []cameraSnapshotTarget{{deviceID: result.Root.ID, channel: 0}}
	case dahua.DeviceKindIPC:
		return []cameraSnapshotTarget{{deviceID: result.Root.ID, channel: 1}}
	default:
		return nil
	}
}
