package app

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/ha"
	"RCooLeR/DahuaBridge/internal/haapi"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

type adminActions struct {
	logger          zerolog.Logger
	metrics         *metrics.Registry
	discovery       *ha.DiscoveryPublisher
	probes          *store.ProbeStore
	configs         deviceConfigStore
	haClient        homeAssistantProvisioner
	drivers         map[string]dahua.Driver
	lockControllers map[string]dahua.VTOLockController
	callControllers map[string]dahua.VTOCallController
	nvrRefresh      map[string]dahua.NVRInventoryRefresher
	configure       map[string]dahua.ConfigurableDriver
}

type deviceConfigStore interface {
	GetDeviceConfig(string) (config.DeviceConfig, bool)
	UpdateDeviceConfig(string, config.DeviceConfig) bool
	ListONVIFProvisionTargets([]string, bool) []haapi.ONVIFProvisionTarget
}

type homeAssistantProvisioner interface {
	Enabled() bool
	ProvisionONVIF(context.Context, haapi.ONVIFProvisionTarget) (haapi.ONVIFProvisionResult, error)
}

func newAdminActions(
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	discovery *ha.DiscoveryPublisher,
	probes *store.ProbeStore,
	configs deviceConfigStore,
	haClient homeAssistantProvisioner,
	drivers []dahua.Driver,
) *adminActions {
	actions := &adminActions{
		logger:          logger.With().Str("component", "admin_actions").Logger(),
		metrics:         metricsRegistry,
		discovery:       discovery,
		probes:          probes,
		configs:         configs,
		haClient:        haClient,
		drivers:         make(map[string]dahua.Driver, len(drivers)),
		lockControllers: make(map[string]dahua.VTOLockController),
		callControllers: make(map[string]dahua.VTOCallController),
		nvrRefresh:      make(map[string]dahua.NVRInventoryRefresher),
		configure:       make(map[string]dahua.ConfigurableDriver),
	}

	for _, driver := range drivers {
		actions.drivers[driver.ID()] = driver
		if controller, ok := driver.(dahua.VTOLockController); ok && driver.Kind() == dahua.DeviceKindVTO {
			actions.lockControllers[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.VTOCallController); ok && driver.Kind() == dahua.DeviceKindVTO {
			actions.callControllers[driver.ID()] = controller
		}
		if refresher, ok := driver.(dahua.NVRInventoryRefresher); ok && driver.Kind() == dahua.DeviceKindNVR {
			actions.nvrRefresh[driver.ID()] = refresher
		}
		if configurable, ok := driver.(dahua.ConfigurableDriver); ok {
			actions.configure[driver.ID()] = configurable
		}
	}

	return actions
}

func (a *adminActions) ProbeDevice(ctx context.Context, deviceID string) (*dahua.ProbeResult, error) {
	driver, ok := a.drivers[deviceID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}

	log := a.logger.With().
		Str("device_id", driver.ID()).
		Str("device_type", string(driver.Kind())).
		Str("action", "probe").
		Logger()

	started := time.Now()
	result, err := driver.Probe(ctx)
	a.metrics.ObserveProbe(driver.ID(), string(driver.Kind()), started, err)
	if err != nil {
		a.metrics.DeviceAvailability.WithLabelValues(driver.ID(), string(driver.Kind())).Set(0)
		if publishErr := a.discovery.PublishUnavailable(context.Background(), driver.ID()); publishErr != nil {
			log.Warn().Err(publishErr).Msg("admin unavailable publish failed")
		}
		log.Error().Err(err).Msg("admin device probe failed")
		return nil, err
	}

	a.metrics.DeviceAvailability.WithLabelValues(driver.ID(), string(driver.Kind())).Set(1)
	a.probes.Set(driver.ID(), result)
	if err := a.discovery.PublishProbe(ctx, result); err != nil {
		log.Error().Err(err).Msg("admin probe publish failed")
		return nil, err
	}

	log.Info().
		Str("name", result.Root.Name).
		Str("model", result.Root.Model).
		Str("serial", result.Root.Serial).
		Msg("admin device probe succeeded")
	return result, nil
}

func (a *adminActions) UnlockVTOLock(ctx context.Context, deviceID string, lockIndex int) error {
	controller, ok := a.lockControllers[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.Unlock(ctx, lockIndex)
}

func (a *adminActions) HangupVTOCall(ctx context.Context, deviceID string) error {
	controller, ok := a.callControllers[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	if err := controller.HangupCall(ctx); err != nil {
		return err
	}

	a.publishVTOHangupState(ctx, deviceID, time.Now().UTC())
	return nil
}

func (a *adminActions) AnswerVTOCall(ctx context.Context, deviceID string) error {
	controller, ok := a.callControllers[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.AnswerCall(ctx)
}

func (a *adminActions) RefreshNVRInventory(ctx context.Context, deviceID string) (*dahua.ProbeResult, error) {
	refresher, ok := a.nvrRefresh[deviceID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}

	refresher.InvalidateInventoryCache()
	return a.ProbeDevice(ctx, deviceID)
}

func (a *adminActions) ProbeAllDevices(ctx context.Context) []dahua.ProbeActionResult {
	deviceIDs := make([]string, 0, len(a.drivers))
	for deviceID := range a.drivers {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)

	results := make([]dahua.ProbeActionResult, 0, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		driver := a.drivers[deviceID]
		outcome := dahua.ProbeActionResult{
			DeviceID:   deviceID,
			DeviceKind: driver.Kind(),
		}

		result, err := a.ProbeDevice(ctx, deviceID)
		if err != nil {
			outcome.Error = err.Error()
		} else {
			outcome.Result = result
		}
		results = append(results, outcome)
	}

	return results
}

func (a *adminActions) RotateDeviceCredentials(ctx context.Context, deviceID string, update dahua.DeviceConfigUpdate) (*dahua.ProbeResult, error) {
	driver, ok := a.drivers[deviceID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	configurable, ok := a.configure[deviceID]
	if !ok {
		return nil, fmt.Errorf("device %q does not support live config updates", deviceID)
	}
	if a.configs == nil {
		return nil, fmt.Errorf("device config store is not configured")
	}

	current, ok := a.configs.GetDeviceConfig(deviceID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}

	next, err := applyDeviceConfigUpdate(current, update)
	if err != nil {
		return nil, err
	}
	if err := configurable.UpdateConfig(next); err != nil {
		return nil, err
	}
	if !a.configs.UpdateDeviceConfig(deviceID, next) {
		return nil, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}

	a.logger.Info().
		Str("device_id", driver.ID()).
		Str("device_type", string(driver.Kind())).
		Bool("onvif_enabled", next.ONVIFEnabledValue()).
		Msg("updated device credentials in memory")

	return a.ProbeDevice(ctx, deviceID)
}

func (a *adminActions) ProvisionHomeAssistantONVIF(ctx context.Context, request haapi.ONVIFProvisionRequest) ([]haapi.ONVIFProvisionResult, error) {
	if a.haClient == nil || !a.haClient.Enabled() {
		return nil, fmt.Errorf("home assistant api is not configured")
	}
	if a.configs == nil {
		return nil, fmt.Errorf("device config store is not configured")
	}

	targets := a.configs.ListONVIFProvisionTargets(request.DeviceIDs, request.Force)
	results := make([]haapi.ONVIFProvisionResult, 0, len(targets))
	for _, target := range targets {
		result, err := a.haClient.ProvisionONVIF(ctx, target)
		if err != nil {
			a.logger.Warn().
				Err(err).
				Str("device_id", target.DeviceID).
				Str("device_type", string(target.DeviceKind)).
				Str("host", target.Host).
				Int("port", target.Port).
				Msg("home assistant onvif provisioning failed")
		} else {
			a.logger.Info().
				Str("device_id", target.DeviceID).
				Str("device_type", string(target.DeviceKind)).
				Str("host", target.Host).
				Int("port", target.Port).
				Str("status", result.Status).
				Msg("home assistant onvif provisioning completed")
		}
		results = append(results, result)
	}

	return results, nil
}

func (a *adminActions) RemoveLegacyHomeAssistantMQTTDiscovery(ctx context.Context) (ha.LegacyDiscoveryCleanupResult, error) {
	if a.discovery == nil {
		return ha.LegacyDiscoveryCleanupResult{}, fmt.Errorf("mqtt discovery publisher is not configured")
	}
	if a.probes == nil {
		return ha.LegacyDiscoveryCleanupResult{}, fmt.Errorf("probe store is not configured")
	}

	results := a.probes.List()
	cleanup := ha.LegacyDiscoveryCleanupResult{
		DeviceIDs: make([]string, 0),
	}
	for _, result := range results {
		item, err := a.discovery.RemoveProbeDiscovery(ctx, result)
		if err != nil {
			return ha.LegacyDiscoveryCleanupResult{}, err
		}
		cleanup.RemovedTopics += item.RemovedTopics
		cleanup.DeviceCount += item.DeviceCount
		cleanup.DeviceIDs = append(cleanup.DeviceIDs, item.DeviceIDs...)
	}

	sort.Strings(cleanup.DeviceIDs)
	return cleanup, nil
}

func applyDeviceConfigUpdate(current config.DeviceConfig, update dahua.DeviceConfigUpdate) (config.DeviceConfig, error) {
	next := current

	if update.BaseURL != nil {
		next.BaseURL = strings.TrimSpace(*update.BaseURL)
	}
	if update.Username != nil {
		next.Username = strings.TrimSpace(*update.Username)
	}
	if update.Password != nil {
		next.Password = *update.Password
	}
	if update.OnvifEnabled != nil {
		next.OnvifEnabled = boolPtr(*update.OnvifEnabled)
	}
	if update.OnvifUsername != nil {
		next.OnvifUsername = strings.TrimSpace(*update.OnvifUsername)
	}
	if update.OnvifPassword != nil {
		next.OnvifPassword = *update.OnvifPassword
	}
	if update.OnvifServiceURL != nil {
		next.OnvifServiceURL = strings.TrimSpace(*update.OnvifServiceURL)
	}
	if update.InsecureSkipTLS != nil {
		next.InsecureSkipTLS = *update.InsecureSkipTLS
	}

	normalized, err := config.NormalizeDevice(next)
	if err != nil {
		return config.DeviceConfig{}, err
	}
	if strings.TrimSpace(normalized.Username) == "" {
		return config.DeviceConfig{}, fmt.Errorf("device %q must have a username", normalized.ID)
	}
	if normalized.Password == "" {
		return config.DeviceConfig{}, fmt.Errorf("device %q must have a password", normalized.ID)
	}
	return normalized, nil
}

func boolPtr(value bool) *bool {
	return &value
}

func (a *adminActions) publishVTOHangupState(ctx context.Context, deviceID string, endedAt time.Time) {
	timestamp := endedAt.Format(time.RFC3339Nano)
	duration := ""

	if a.probes != nil {
		a.probes.Update(deviceID, func(result *dahua.ProbeResult) {
			if result.States == nil {
				result.States = make(map[string]dahua.DeviceState)
			}
			state := result.States[deviceID]
			if state.Info == nil {
				state.Info = make(map[string]any)
			}
			state.Available = true
			state.Info["call"] = false
			state.Info["call_state"] = "idle"
			state.Info["last_call_ended_at"] = timestamp
			if startedAt := stringStateValue(state.Info["call_started_at"]); startedAt != "" {
				if parsedStartedAt, err := time.Parse(time.RFC3339Nano, startedAt); err == nil && !endedAt.Before(parsedStartedAt) {
					duration = strconv.Itoa(int(endedAt.Sub(parsedStartedAt).Seconds()))
					state.Info["last_call_duration_seconds"] = duration
				}
			}
			delete(state.Info, "call_started_at")
			result.States[deviceID] = state
		})
	}

	if a.discovery == nil {
		return
	}
	if err := a.discovery.PublishBinaryState(ctx, deviceID, "call", false); err != nil {
		a.logger.Warn().Err(err).Str("device_id", deviceID).Msg("publish call inactive state after hangup failed")
	}
	if err := a.discovery.PublishState(ctx, deviceID, "call_state", "idle", true); err != nil {
		a.logger.Warn().Err(err).Str("device_id", deviceID).Msg("publish call state after hangup failed")
	}
	if err := a.discovery.PublishState(ctx, deviceID, "last_call_ended_at", timestamp, true); err != nil {
		a.logger.Warn().Err(err).Str("device_id", deviceID).Msg("publish last call ended timestamp after hangup failed")
	}
	if duration != "" {
		if err := a.discovery.PublishState(ctx, deviceID, "last_call_duration_seconds", duration, true); err != nil {
			a.logger.Warn().Err(err).Str("device_id", deviceID).Msg("publish call duration after hangup failed")
		}
	}
}

func stringStateValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
