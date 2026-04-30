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
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

type adminActions struct {
	logger            zerolog.Logger
	metrics           *metrics.Registry
	probes            *store.ProbeStore
	configs           deviceConfigStore
	drivers           map[string]dahua.Driver
	lockControllers   map[string]dahua.VTOLockController
	callControllers   map[string]dahua.VTOCallController
	vtoControls       map[string]dahua.VTOControlReader
	vtoAudio          map[string]dahua.VTOAudioController
	vtoRecording      map[string]dahua.VTORecordingController
	channelControls   map[string]dahua.NVRChannelControlReader
	ptzControllers    map[string]dahua.NVRPTZController
	auxControllers    map[string]dahua.NVRAuxController
	audioControllers  map[string]dahua.NVRAudioController
	recordControllers map[string]dahua.NVRRecordingController
	nvrRefresh        map[string]dahua.NVRInventoryRefresher
	configure         map[string]dahua.ConfigurableDriver
}

type deviceConfigStore interface {
	GetDeviceConfig(string) (config.DeviceConfig, bool)
	UpdateDeviceConfig(string, config.DeviceConfig) bool
}

func newAdminActions(
	logger zerolog.Logger,
	metricsRegistry *metrics.Registry,
	probes *store.ProbeStore,
	configs deviceConfigStore,
	drivers []dahua.Driver,
) *adminActions {
	actions := &adminActions{
		logger:            logger.With().Str("component", "admin_actions").Logger(),
		metrics:           metricsRegistry,
		probes:            probes,
		configs:           configs,
		drivers:           make(map[string]dahua.Driver, len(drivers)),
		lockControllers:   make(map[string]dahua.VTOLockController),
		callControllers:   make(map[string]dahua.VTOCallController),
		vtoControls:       make(map[string]dahua.VTOControlReader),
		vtoAudio:          make(map[string]dahua.VTOAudioController),
		vtoRecording:      make(map[string]dahua.VTORecordingController),
		channelControls:   make(map[string]dahua.NVRChannelControlReader),
		ptzControllers:    make(map[string]dahua.NVRPTZController),
		auxControllers:    make(map[string]dahua.NVRAuxController),
		audioControllers:  make(map[string]dahua.NVRAudioController),
		recordControllers: make(map[string]dahua.NVRRecordingController),
		nvrRefresh:        make(map[string]dahua.NVRInventoryRefresher),
		configure:         make(map[string]dahua.ConfigurableDriver),
	}

	for _, driver := range drivers {
		actions.drivers[driver.ID()] = driver
		if controller, ok := driver.(dahua.VTOLockController); ok && driver.Kind() == dahua.DeviceKindVTO {
			actions.lockControllers[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.VTOCallController); ok && driver.Kind() == dahua.DeviceKindVTO {
			actions.callControllers[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.VTOControlReader); ok && driver.Kind() == dahua.DeviceKindVTO {
			actions.vtoControls[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.VTOAudioController); ok && driver.Kind() == dahua.DeviceKindVTO {
			actions.vtoAudio[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.VTORecordingController); ok && driver.Kind() == dahua.DeviceKindVTO {
			actions.vtoRecording[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.NVRChannelControlReader); ok && driver.Kind() == dahua.DeviceKindNVR {
			actions.channelControls[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.NVRPTZController); ok && driver.Kind() == dahua.DeviceKindNVR {
			actions.ptzControllers[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.NVRAuxController); ok && driver.Kind() == dahua.DeviceKindNVR {
			actions.auxControllers[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.NVRAudioController); ok && driver.Kind() == dahua.DeviceKindNVR {
			actions.audioControllers[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.NVRRecordingController); ok && driver.Kind() == dahua.DeviceKindNVR {
			actions.recordControllers[driver.ID()] = controller
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
		log.Error().Err(err).Msg("admin device probe failed")
		return nil, err
	}

	a.metrics.DeviceAvailability.WithLabelValues(driver.ID(), string(driver.Kind())).Set(1)
	a.probes.Set(driver.ID(), result)

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

func (a *adminActions) VTOControlCapabilities(ctx context.Context, deviceID string) (dahua.VTOControlCapabilities, error) {
	controller, ok := a.vtoControls[deviceID]
	if !ok {
		return dahua.VTOControlCapabilities{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.ControlCapabilities(ctx)
}

func (a *adminActions) SetVTOAudioOutputVolume(ctx context.Context, deviceID string, slot int, level int) error {
	controller, ok := a.vtoAudio[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.SetAudioOutputVolume(ctx, slot, level)
}

func (a *adminActions) SetVTOAudioInputVolume(ctx context.Context, deviceID string, slot int, level int) error {
	controller, ok := a.vtoAudio[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.SetAudioInputVolume(ctx, slot, level)
}

func (a *adminActions) SetVTOMute(ctx context.Context, deviceID string, muted bool) error {
	controller, ok := a.vtoAudio[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.SetAudioMute(ctx, muted)
}

func (a *adminActions) SetVTORecordingEnabled(ctx context.Context, deviceID string, enabled bool) error {
	controller, ok := a.vtoRecording[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.SetRecordingEnabled(ctx, enabled)
}

func (a *adminActions) RefreshNVRInventory(ctx context.Context, deviceID string) (*dahua.ProbeResult, error) {
	refresher, ok := a.nvrRefresh[deviceID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}

	refresher.InvalidateInventoryCache()
	return a.ProbeDevice(ctx, deviceID)
}

func (a *adminActions) NVRChannelControlCapabilities(ctx context.Context, deviceID string, channel int) (dahua.NVRChannelControlCapabilities, error) {
	controller, ok := a.channelControls[deviceID]
	if !ok {
		return dahua.NVRChannelControlCapabilities{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.ChannelControlCapabilities(ctx, channel)
}

func (a *adminActions) ControlNVRPTZ(ctx context.Context, deviceID string, request dahua.NVRPTZRequest) error {
	controller, ok := a.ptzControllers[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.PTZ(ctx, request)
}

func (a *adminActions) ControlNVRAux(ctx context.Context, deviceID string, request dahua.NVRAuxRequest) error {
	controller, ok := a.auxControllers[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	if err := controller.Aux(ctx, request); err != nil {
		return err
	}
	a.publishNVRAuxState(deviceID, request, time.Now().UTC())
	return nil
}

func (a *adminActions) ControlNVRAudio(ctx context.Context, deviceID string, request dahua.NVRAudioRequest) error {
	controller, ok := a.audioControllers[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	if err := controller.SetAudioMute(ctx, request); err != nil {
		return err
	}
	a.publishNVRAudioState(deviceID, request)
	return nil
}

func (a *adminActions) ControlNVRRecording(ctx context.Context, deviceID string, request dahua.NVRRecordingRequest) error {
	controller, ok := a.recordControllers[deviceID]
	if !ok {
		return fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return controller.Recording(ctx, request)
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

	_ = ctx
}

func stringStateValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func (a *adminActions) publishNVRAuxState(deviceID string, request dahua.NVRAuxRequest, changedAt time.Time) {
	if a.probes == nil {
		return
	}

	normalizedOutput := strings.ToLower(strings.TrimSpace(request.Output))
	if normalizedOutput == "" {
		return
	}

	a.probes.Update(deviceID, func(result *dahua.ProbeResult) {
		channelDeviceID := findNVRChannelDeviceID(result, request.Channel)
		if channelDeviceID == "" {
			return
		}
		if result.States == nil {
			result.States = make(map[string]dahua.DeviceState)
		}

		state := result.States[channelDeviceID]
		if state.Info == nil {
			state.Info = make(map[string]any)
		}
		state.Available = true

		active := request.Action != dahua.NVRAuxActionStop
		switch normalizedOutput {
		case "aux", "siren":
			setNVRAuxFeatureState(state.Info, "siren", active, changedAt.Add(resolveSirenActiveWindow(request)))
		case "light":
			setNVRAuxFeatureState(state.Info, "light", active, time.Time{})
			if active {
				state.Info["control_aux_current_text_light"] = "White Light"
			} else {
				state.Info["control_aux_current_text_light"] = "Smart Light"
			}
		case "warning_light":
			setNVRAuxFeatureState(state.Info, "warning_light", active, time.Time{})
			if active {
				state.Info["control_aux_current_text_warning_light"] = "Warning Light On"
			} else {
				state.Info["control_aux_current_text_warning_light"] = "Warning Light Off"
			}
		case "wiper":
			setNVRAuxFeatureState(state.Info, "wiper", active, time.Time{})
		}

		result.States[channelDeviceID] = state
	})
}

func (a *adminActions) publishNVRAudioState(deviceID string, request dahua.NVRAudioRequest) {
	if a.probes == nil {
		return
	}
	a.probes.Update(deviceID, func(result *dahua.ProbeResult) {
		channelDeviceID := findNVRChannelDeviceID(result, request.Channel)
		if channelDeviceID == "" {
			return
		}
		if result.States == nil {
			result.States = make(map[string]dahua.DeviceState)
		}
		state := result.States[channelDeviceID]
		if state.Info == nil {
			state.Info = make(map[string]any)
		}
		state.Available = true
		state.Info["control_audio_muted"] = request.Muted
		state.Info["control_audio_stream_enabled"] = !request.Muted
		result.States[channelDeviceID] = state
	})
}

func findNVRChannelDeviceID(result *dahua.ProbeResult, channel int) string {
	if result == nil || channel <= 0 {
		return ""
	}
	expected := strconv.Itoa(channel)
	for _, child := range result.Children {
		if child.Kind != dahua.DeviceKindNVRChannel {
			continue
		}
		if strings.TrimSpace(child.Attributes["channel_index"]) == expected {
			return child.ID
		}
	}
	return ""
}

func setNVRAuxFeatureState(info map[string]any, feature string, active bool, until time.Time) {
	if info == nil {
		return
	}
	feature = strings.TrimSpace(feature)
	if feature == "" {
		return
	}
	info["control_aux_active_"+feature] = active
	if until.IsZero() {
		delete(info, "control_aux_active_until_"+feature)
		return
	}
	info["control_aux_active_until_"+feature] = until.UTC().Format(time.RFC3339Nano)
}

func resolveSirenActiveWindow(request dahua.NVRAuxRequest) time.Duration {
	switch request.Action {
	case dahua.NVRAuxActionStop:
		return 0
	case dahua.NVRAuxActionPulse:
		if request.Duration > 0 && request.Duration < 10*time.Second {
			return request.Duration
		}
		return 10 * time.Second
	default:
		return 10 * time.Second
	}
}
