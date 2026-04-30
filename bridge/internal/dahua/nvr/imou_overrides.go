package nvr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/imou"
)

const (
	imouFeatureEvents       = "events"
	imouFeatureLight        = "light"
	imouFeatureWarningLight = "warning_light"
	imouFeatureSiren        = "siren"
)

var imouEnableTypeByFeature = map[string]string{
	imouFeatureWarningLight: "linkageWhiteLight",
	imouFeatureSiren:        "linkageSiren",
}

var imouAudioEnableTypes = []string{"audioEncodeControl", "aecv3"}

func (d *Driver) imouOverride(channel int) (config.ChannelImouOverride, bool) {
	if d.imou == nil || !d.imou.Enabled() {
		return config.ChannelImouOverride{}, false
	}
	return d.currentConfig().ImouOverride(channel)
}

func (d *Driver) channelUsesImouFeature(channel int, feature string) bool {
	override, ok := d.imouOverride(channel)
	if !ok {
		return false
	}
	feature = strings.ToLower(strings.TrimSpace(feature))
	for _, value := range override.Features {
		if strings.EqualFold(strings.TrimSpace(value), feature) {
			return true
		}
	}
	return false
}

func (d *Driver) auxFeatureForOutput(output string) string {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "light":
		return imouFeatureLight
	case "warning_light":
		return imouFeatureWarningLight
	case "aux", "siren":
		return imouFeatureSiren
	default:
		return ""
	}
}

func (d *Driver) augmentAuxCapabilitiesWithImou(channel int, capabilities dahua.NVRAuxCapabilities) dahua.NVRAuxCapabilities {
	override, ok := d.imouOverride(channel)
	if !ok {
		return capabilities
	}

	outputs := append([]string(nil), capabilities.Outputs...)
	features := append([]string(nil), capabilities.Features...)
	for _, feature := range override.Features {
		switch strings.ToLower(strings.TrimSpace(feature)) {
		case imouFeatureLight:
			outputs = append(outputs, "light")
			features = append(features, imouFeatureLight)
		case imouFeatureWarningLight:
			outputs = append(outputs, "light")
			features = append(features, imouFeatureWarningLight)
		case imouFeatureSiren:
			outputs = append(outputs, "aux")
			features = append(features, imouFeatureSiren)
		}
	}
	outputs = uniqueSortedStrings(outputs)
	features = uniqueSortedStrings(features)
	if len(outputs) == 0 && len(features) == 0 {
		return capabilities
	}
	return dahua.NVRAuxCapabilities{
		Supported: true,
		Outputs:   outputs,
		Features:  features,
	}
}

func (d *Driver) handleImouAux(ctx context.Context, request dahua.NVRAuxRequest) (bool, error) {
	override, ok := d.imouOverride(request.Channel)
	if !ok {
		return false, nil
	}
	feature := d.auxFeatureForOutput(request.Output)
	if feature == "" || !containsString(override.Features, feature) {
		return false, nil
	}

	enableType, ok := imouEnableTypeByFeature[feature]
	if feature == imouFeatureLight {
		mode, err := d.imou.GetNightVisionMode(ctx, imou.NightVisionModeRequest{
			DeviceID:  override.DeviceID,
			ChannelID: override.ChannelID,
		})
		if err != nil {
			return true, err
		}
		targetMode, err := resolveTargetNightVisionMode(mode, request.Action)
		if err != nil {
			return true, err
		}
		return true, d.imou.SetNightVisionMode(ctx, imou.NightVisionModeChange{
			DeviceID:  override.DeviceID,
			ChannelID: override.ChannelID,
			Mode:      targetMode,
		})
	}
	if !ok {
		return false, fmt.Errorf("%w: imou aux feature %q is not mapped", dahua.ErrUnsupportedOperation, feature)
	}

	change := imou.CameraStatusChange{
		DeviceID:   override.DeviceID,
		ChannelID:  override.ChannelID,
		EnableType: enableType,
	}

	switch request.Action {
	case dahua.NVRAuxActionStart:
		change.Enable = true
		return true, d.imou.SetCameraStatus(ctx, change)
	case dahua.NVRAuxActionStop:
		change.Enable = false
		return true, d.imou.SetCameraStatus(ctx, change)
	case dahua.NVRAuxActionPulse:
		duration := request.Duration
		if duration <= 0 {
			duration = 300 * time.Millisecond
		}
		if duration > 10*time.Second {
			duration = 10 * time.Second
		}
		change.Enable = true
		if err := d.imou.SetCameraStatus(ctx, change); err != nil {
			return true, err
		}
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			change.Enable = false
			_ = d.imou.SetCameraStatus(stopCtx, change)
			return true, ctx.Err()
		case <-timer.C:
		}
		change.Enable = false
		return true, d.imou.SetCameraStatus(ctx, change)
	default:
		return true, fmt.Errorf("unsupported aux action %q", request.Action)
	}
}

func (d *Driver) attachImouChannelState(_ context.Context, channel int, state *dahua.DeviceState) {
	override, ok := d.imouOverride(channel)
	if !ok || state == nil {
		return
	}
	if state.Info == nil {
		state.Info = make(map[string]any)
	}
	state.Info["control_imou_configured"] = true
	state.Info["control_imou_features"] = append([]string(nil), override.Features...)
}

func (d *Driver) imouEventOverrides() []config.ChannelImouOverride {
	cfg := d.currentConfig()
	if d.imou == nil || !d.imou.Enabled() || len(cfg.ChannelImouOverrides) == 0 {
		return nil
	}

	overrides := make([]config.ChannelImouOverride, 0, len(cfg.ChannelImouOverrides))
	for _, override := range cfg.ChannelImouOverrides {
		if containsString(override.Features, imouFeatureEvents) {
			overrides = append(overrides, override)
		}
	}
	return overrides
}

func (d *Driver) shouldSuppressLocalEvent(event dahua.Event) bool {
	return event.Channel > 0 && d.channelUsesImouFeature(event.Channel, imouFeatureEvents)
}

func (d *Driver) runLocalEventSource(ctx context.Context, sink chan<- dahua.Event) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := d.streamLocalEvents(ctx, sink); err != nil && ctx.Err() == nil {
			d.logger.Warn().Err(err).Msg("local event source stopped, retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		return
	}
}

func (d *Driver) runImouEventSource(ctx context.Context, override config.ChannelImouOverride, sink chan<- dahua.Event) {
	queryEnd := time.Now().UTC()
	queryBegin := queryEnd.Add(-d.imouPollInterval())
	seenAlarmIDs := make(map[string]time.Time)
	activeUntil := make(map[string]time.Time)
	pollTicker := time.NewTicker(d.imouPollInterval())
	expiryTicker := time.NewTicker(time.Second)
	defer pollTicker.Stop()
	defer expiryTicker.Stop()

	poll := func(now time.Time) {
		alarms, err := d.imou.ListAlarms(ctx, imou.AlarmQuery{
			DeviceID:  override.DeviceID,
			ChannelID: override.ChannelID,
			BeginTime: queryBegin,
			EndTime:   now,
			Count:     30,
		})
		if err != nil {
			d.logger.Warn().Err(err).Int("channel", override.Channel).Msg("imou alarm poll failed")
			return
		}
		for key, seenAt := range seenAlarmIDs {
			if now.Sub(seenAt) > 10*time.Minute {
				delete(seenAlarmIDs, key)
			}
		}
		for _, alarm := range alarms {
			if alarm.AlarmID == "" {
				continue
			}
			if _, exists := seenAlarmIDs[alarm.AlarmID]; exists {
				continue
			}
			seenAlarmIDs[alarm.AlarmID] = now

			code := imouAlarmCode(alarm.Type)
			if code == "" {
				continue
			}
			event := dahua.Event{
				DeviceID:   d.currentConfig().ID,
				DeviceKind: dahua.DeviceKindNVR,
				ChildID:    channelDeviceID(d.currentConfig().ID, override.Channel-1),
				Code:       code,
				Action:     dahua.EventActionPulse,
				Index:      override.Channel - 1,
				Channel:    override.Channel,
				OccurredAt: alarm.Time,
				Data: map[string]string{
					"alarmId":   alarm.AlarmID,
					"channelId": alarm.ChannelID,
					"deviceId":  alarm.DeviceID,
					"type":      fmt.Sprintf("%d", alarm.Type),
				},
			}
			select {
			case <-ctx.Done():
				return
			case sink <- event:
			}

			until := alarm.Time.Add(d.imouEventActiveWindow())
			if current, ok := activeUntil[code]; !ok || until.After(current) {
				activeUntil[code] = until
			}
		}
		queryEnd = now
		queryBegin = now.Add(-10 * time.Second)
	}

	poll(queryEnd)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-pollTicker.C:
			poll(now.UTC())
		case now := <-expiryTicker.C:
			for code, until := range activeUntil {
				if now.UTC().Before(until) {
					continue
				}
				delete(activeUntil, code)
				select {
				case <-ctx.Done():
					return
				case sink <- dahua.Event{
					DeviceID:   d.currentConfig().ID,
					DeviceKind: dahua.DeviceKindNVR,
					ChildID:    channelDeviceID(d.currentConfig().ID, override.Channel-1),
					Code:       code,
					Action:     dahua.EventActionStop,
					Index:      override.Channel - 1,
					Channel:    override.Channel,
					OccurredAt: now.UTC(),
				}:
				}
			}
		}
	}
}

func imouAlarmCode(alarmType int) string {
	switch alarmType {
	case 0, 4:
		return "SmartMotionHuman"
	case 1, 2, 5:
		return "VideoMotion"
	default:
		return "VideoMotion"
	}
}

func resolveTargetNightVisionMode(mode imou.NightVisionMode, action dahua.NVRAuxAction) (string, error) {
	switch action {
	case dahua.NVRAuxActionStart:
		if supportedMode(mode.Modes, "FullColor") {
			return "FullColor", nil
		}
		if supportedMode(mode.Modes, "LowLight") {
			return "LowLight", nil
		}
		if mode.Mode != "" && isWhiteLightNightVisionMode(mode.Mode) {
			return mode.Mode, nil
		}
		return "", fmt.Errorf("%w: imou night vision does not advertise a white-light mode", dahua.ErrUnsupportedOperation)
	case dahua.NVRAuxActionStop:
		for _, candidate := range []string{"SmartLowLight", "Intelligent", "Infrared", "Off"} {
			if supportedMode(mode.Modes, candidate) {
				return candidate, nil
			}
		}
		if mode.Mode != "" && !isWhiteLightNightVisionMode(mode.Mode) {
			return mode.Mode, nil
		}
		return "", fmt.Errorf("%w: imou night vision does not advertise a smart mode", dahua.ErrUnsupportedOperation)
	default:
		return "", fmt.Errorf("%w: lighting mode only supports start and stop actions", dahua.ErrUnsupportedOperation)
	}
}

func isWhiteLightNightVisionMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "FullColor", "LowLight":
		return true
	default:
		return false
	}
}

func supportedMode(modes []string, candidate string) bool {
	for _, mode := range modes {
		if strings.EqualFold(strings.TrimSpace(mode), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func (d *Driver) imouPollInterval() time.Duration {
	interval := d.imouCfg.AlarmPollInterval
	if interval <= 0 {
		return 15 * time.Second
	}
	return interval
}

func (d *Driver) imouEventActiveWindow() time.Duration {
	window := d.imouCfg.EventActiveWindow
	if window <= 0 {
		return 20 * time.Second
	}
	return window
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func (d *Driver) imouAudioMuted(ctx context.Context, override config.ChannelImouOverride) (bool, bool, error) {
	if d.imou == nil || !d.imou.Enabled() {
		return false, false, nil
	}
	var lastErr error
	for _, enableType := range imouAudioEnableTypes {
		status, err := d.imou.GetCameraStatus(ctx, imou.CameraStatusRequest{
			DeviceID:   override.DeviceID,
			ChannelID:  override.ChannelID,
			EnableType: enableType,
		})
		if err == nil {
			return !status.Enabled, true, nil
		}
		lastErr = err
	}
	return false, false, lastErr
}

func (d *Driver) setImouAudioMuted(ctx context.Context, override config.ChannelImouOverride, muted bool) error {
	if d.imou == nil || !d.imou.Enabled() {
		return fmt.Errorf("%w: imou audio control is unavailable", dahua.ErrUnsupportedOperation)
	}
	var lastErr error
	for _, enableType := range imouAudioEnableTypes {
		err := d.imou.SetCameraStatus(ctx, imou.CameraStatusChange{
			DeviceID:   override.DeviceID,
			ChannelID:  override.ChannelID,
			EnableType: enableType,
			Enable:     !muted,
		})
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}
