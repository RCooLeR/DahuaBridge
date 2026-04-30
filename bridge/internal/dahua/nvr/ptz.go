package nvr

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

type ptzCommandSpec struct {
	code          string
	supports      func(dahua.NVRPTZCapabilities) bool
	defaultSpeed  func(dahua.NVRPTZCapabilities) int
	validateSpeed func(dahua.NVRPTZCapabilities, int) (int, error)
}

type auxOutputSpec struct {
	code        string
	canonical   string
	feature     string
	controlType auxControlType
}

type auxControlType string

const (
	auxControlTypePTZ       auxControlType = "ptz"
	auxControlTypeLightMode auxControlType = "light_mode"
)

var ptzCommandSpecs = map[dahua.NVRPTZCommand]ptzCommandSpec{
	dahua.NVRPTZCommandUp: {
		code:         "Up",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Tilt },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return firstPositive(c.TiltSpeedMin, c.PanSpeedMin, 1) },
		validateSpeed: func(c dahua.NVRPTZCapabilities, speed int) (int, error) {
			return normalizeSpeed(speed, c.TiltSpeedMin, c.TiltSpeedMax)
		},
	},
	dahua.NVRPTZCommandDown: {
		code:         "Down",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Tilt },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return firstPositive(c.TiltSpeedMin, c.PanSpeedMin, 1) },
		validateSpeed: func(c dahua.NVRPTZCapabilities, speed int) (int, error) {
			return normalizeSpeed(speed, c.TiltSpeedMin, c.TiltSpeedMax)
		},
	},
	dahua.NVRPTZCommandLeft: {
		code:         "Left",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Pan },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1) },
		validateSpeed: func(c dahua.NVRPTZCapabilities, speed int) (int, error) {
			return normalizeSpeed(speed, c.PanSpeedMin, c.PanSpeedMax)
		},
	},
	dahua.NVRPTZCommandRight: {
		code:         "Right",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Pan },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1) },
		validateSpeed: func(c dahua.NVRPTZCapabilities, speed int) (int, error) {
			return normalizeSpeed(speed, c.PanSpeedMin, c.PanSpeedMax)
		},
	},
	dahua.NVRPTZCommandLeftUp: {
		code:         "LeftUp",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Pan && c.Tilt },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1) },
		validateSpeed: func(c dahua.NVRPTZCapabilities, speed int) (int, error) {
			minSpeed := firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1)
			maxSpeed := firstPositive(c.PanSpeedMax, c.TiltSpeedMax, minSpeed)
			return normalizeSpeed(speed, minSpeed, maxSpeed)
		},
	},
	dahua.NVRPTZCommandRightUp: {
		code:         "RightUp",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Pan && c.Tilt },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1) },
		validateSpeed: func(c dahua.NVRPTZCapabilities, speed int) (int, error) {
			minSpeed := firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1)
			maxSpeed := firstPositive(c.PanSpeedMax, c.TiltSpeedMax, minSpeed)
			return normalizeSpeed(speed, minSpeed, maxSpeed)
		},
	},
	dahua.NVRPTZCommandLeftDown: {
		code:         "LeftDown",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Pan && c.Tilt },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1) },
		validateSpeed: func(c dahua.NVRPTZCapabilities, speed int) (int, error) {
			minSpeed := firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1)
			maxSpeed := firstPositive(c.PanSpeedMax, c.TiltSpeedMax, minSpeed)
			return normalizeSpeed(speed, minSpeed, maxSpeed)
		},
	},
	dahua.NVRPTZCommandRightDown: {
		code:         "RightDown",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Pan && c.Tilt },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1) },
		validateSpeed: func(c dahua.NVRPTZCapabilities, speed int) (int, error) {
			minSpeed := firstPositive(c.PanSpeedMin, c.TiltSpeedMin, 1)
			maxSpeed := firstPositive(c.PanSpeedMax, c.TiltSpeedMax, minSpeed)
			return normalizeSpeed(speed, minSpeed, maxSpeed)
		},
	},
	dahua.NVRPTZCommandZoomIn: {
		code:         "ZoomTele",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Zoom },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return 1 },
		validateSpeed: func(_ dahua.NVRPTZCapabilities, speed int) (int, error) {
			if speed <= 0 {
				return 1, nil
			}
			return speed, nil
		},
	},
	dahua.NVRPTZCommandZoomOut: {
		code:         "ZoomWide",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Zoom },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return 1 },
		validateSpeed: func(_ dahua.NVRPTZCapabilities, speed int) (int, error) {
			if speed <= 0 {
				return 1, nil
			}
			return speed, nil
		},
	},
	dahua.NVRPTZCommandFocusNear: {
		code:         "FocusNear",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Focus },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return 1 },
		validateSpeed: func(_ dahua.NVRPTZCapabilities, speed int) (int, error) {
			if speed <= 0 {
				return 1, nil
			}
			return speed, nil
		},
	},
	dahua.NVRPTZCommandFocusFar: {
		code:         "FocusFar",
		supports:     func(c dahua.NVRPTZCapabilities) bool { return c.Focus },
		defaultSpeed: func(c dahua.NVRPTZCapabilities) int { return 1 },
		validateSpeed: func(_ dahua.NVRPTZCapabilities, speed int) (int, error) {
			if speed <= 0 {
				return 1, nil
			}
			return speed, nil
		},
	},
}

var auxOutputSpecs = map[string]auxOutputSpec{
	"aux":           {code: "Aux", canonical: "aux", feature: "siren", controlType: auxControlTypePTZ},
	"siren":         {code: "Aux", canonical: "aux", feature: "siren", controlType: auxControlTypePTZ},
	"light":         {canonical: "light", feature: "light", controlType: auxControlTypeLightMode},
	"warning_light": {code: "Light", canonical: "light", feature: "warning_light", controlType: auxControlTypePTZ},
	"wiper":         {code: "Wiper", canonical: "wiper", feature: "wiper", controlType: auxControlTypePTZ},
}

func (d *Driver) ChannelControlCapabilities(ctx context.Context, channel int) (dahua.NVRChannelControlCapabilities, error) {
	cfg := d.currentConfig()
	if channel <= 0 {
		return dahua.NVRChannelControlCapabilities{}, fmt.Errorf("channel must be >= 1")
	}
	if !cfg.AllowsChannel(channel) {
		return dahua.NVRChannelControlCapabilities{}, fmt.Errorf("%w: channel %d is not allowed", dahua.ErrUnsupportedOperation, channel)
	}

	recording, err := d.recordingCapabilities(ctx, channel)
	if err != nil {
		return dahua.NVRChannelControlCapabilities{}, err
	}

	ptz, _ := d.ptzCapabilities(ctx, channel)
	ptz = d.applyPTZOverride(channel, ptz)
	aux, auxErr := d.auxCapabilities(ctx, channel, ptz)
	if auxErr != nil {
		aux = dahua.NVRAuxCapabilities{}
	}
	audio, _ := d.audioCapabilities(ctx, channel)

	return dahua.NVRChannelControlCapabilities{
		DeviceID:  cfg.ID,
		Channel:   channel,
		PTZ:       ptz,
		Aux:       aux,
		Audio:     audio,
		Recording: recording,
	}, nil
}

func (d *Driver) applyPTZOverride(channel int, capabilities dahua.NVRPTZCapabilities) dahua.NVRPTZCapabilities {
	override, ok := d.currentConfig().PTZControlOverride(channel)
	if !ok || override.Enabled == nil {
		return capabilities
	}
	if *override.Enabled {
		return capabilities
	}
	return dahua.NVRPTZCapabilities{}
}

func (d *Driver) ptzCapabilities(ctx context.Context, channel int) (dahua.NVRPTZCapabilities, error) {
	values, err := d.client.GetKeyValues(ctx, "/cgi-bin/ptz.cgi", url.Values{
		"action":  []string{"getCurrentProtocolCaps"},
		"channel": []string{strconv.Itoa(channel)},
	})
	if err != nil {
		if isUnsupportedPTZSurfaceError(err) {
			return dahua.NVRPTZCapabilities{}, fmt.Errorf("%w: ptz capabilities are not exposed on channel %d", dahua.ErrUnsupportedOperation, channel)
		}
		return dahua.NVRPTZCapabilities{}, err
	}
	return parsePTZCapabilities(values), nil
}

func (d *Driver) PTZ(ctx context.Context, request dahua.NVRPTZRequest) error {
	cfg := d.currentConfig()
	if request.Channel <= 0 {
		return fmt.Errorf("channel must be >= 1")
	}
	if !cfg.AllowsChannel(request.Channel) {
		return fmt.Errorf("%w: channel %d is not allowed", dahua.ErrUnsupportedOperation, request.Channel)
	}

	capabilities, err := d.ChannelControlCapabilities(ctx, request.Channel)
	if err != nil {
		return err
	}

	spec, ok := ptzCommandSpecs[request.Command]
	if !ok {
		return fmt.Errorf("unsupported ptz command %q", request.Command)
	}
	if !capabilities.PTZ.Supported || !spec.supports(capabilities.PTZ) {
		return fmt.Errorf("%w: ptz command %q is not supported on channel %d", dahua.ErrUnsupportedOperation, request.Command, request.Channel)
	}

	speed := request.Speed
	if speed <= 0 {
		speed = spec.defaultSpeed(capabilities.PTZ)
	}
	speed, err = spec.validateSpeed(capabilities.PTZ, speed)
	if err != nil {
		return err
	}

	switch request.Action {
	case dahua.NVRPTZActionStart:
		return d.sendPTZAction(ctx, request.Channel, "start", spec.code, speed)
	case dahua.NVRPTZActionStop:
		return d.sendPTZAction(ctx, request.Channel, "stop", spec.code, speed)
	case dahua.NVRPTZActionPulse:
		duration := request.Duration
		if duration <= 0 {
			duration = 300 * time.Millisecond
		}
		if duration > 10*time.Second {
			return fmt.Errorf("duration must be <= 10000ms")
		}
		if err := d.sendPTZAction(ctx, request.Channel, "start", spec.code, speed); err != nil {
			return err
		}
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = d.sendPTZAction(stopCtx, request.Channel, "stop", spec.code, speed)
			return ctx.Err()
		case <-timer.C:
		}
		return d.sendPTZAction(ctx, request.Channel, "stop", spec.code, speed)
	default:
		return fmt.Errorf("unsupported ptz action %q", request.Action)
	}
}

func (d *Driver) Aux(ctx context.Context, request dahua.NVRAuxRequest) error {
	cfg := d.currentConfig()
	if request.Channel <= 0 {
		return fmt.Errorf("channel must be >= 1")
	}
	if !cfg.AllowsChannel(request.Channel) {
		return fmt.Errorf("%w: channel %d is not allowed", dahua.ErrUnsupportedOperation, request.Channel)
	}

	outputKey := strings.ToLower(strings.TrimSpace(request.Output))
	spec, ok := auxOutputSpecs[outputKey]
	if !ok {
		return fmt.Errorf("unsupported aux output %q", request.Output)
	}
	if handled, err := d.handleImouAux(ctx, request); handled {
		return err
	}

	ptz, _ := d.ptzCapabilities(ctx, request.Channel)
	auxCapabilities, err := d.auxCapabilities(ctx, request.Channel, ptz)
	if err != nil {
		return err
	}
	if !auxCapabilities.Supported {
		return fmt.Errorf("%w: aux outputs are not supported on channel %d", dahua.ErrUnsupportedOperation, request.Channel)
	}
	if !hasAuxOutput(auxCapabilities.Outputs, spec.canonical) {
		return fmt.Errorf("%w: aux output %q is not supported on channel %d", dahua.ErrUnsupportedOperation, request.Output, request.Channel)
	}

	switch spec.controlType {
	case auxControlTypeLightMode:
		return d.setChannelLightingMode(ctx, request.Channel, request.Action)
	case auxControlTypePTZ:
		switch request.Action {
		case dahua.NVRAuxActionStart:
			return d.sendAuxAction(ctx, request.Channel, "start", spec.code)
		case dahua.NVRAuxActionStop:
			return d.sendAuxAction(ctx, request.Channel, "stop", spec.code)
		case dahua.NVRAuxActionPulse:
			duration := request.Duration
			if duration <= 0 {
				duration = 300 * time.Millisecond
			}
			if duration > 10*time.Second {
				return fmt.Errorf("duration must be <= 10000ms")
			}
			if err := d.sendAuxAction(ctx, request.Channel, "start", spec.code); err != nil {
				return err
			}
			timer := time.NewTimer(duration)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = d.sendAuxAction(stopCtx, request.Channel, "stop", spec.code)
				return ctx.Err()
			case <-timer.C:
			}
			return d.sendAuxAction(ctx, request.Channel, "stop", spec.code)
		default:
			return fmt.Errorf("unsupported aux action %q", request.Action)
		}
	default:
		return fmt.Errorf("unsupported aux control type %q", spec.controlType)
	}
}

func (d *Driver) auxCapabilities(_ context.Context, channel int, ptz dahua.NVRPTZCapabilities) (dahua.NVRAuxCapabilities, error) {
	cfg := d.currentConfig()

	var outputs []string
	var features []string
	if ptz.Aux {
		outputs = normalizeAuxOutputs(ptz.AuxFunctions)
		if !hasAuxOutput(outputs, "aux") {
			outputs = append(outputs, "aux")
		}
		outputs = uniqueSortedStrings(outputs)
		features = auxFeatureAliases(outputs)
	}

	if override, ok := cfg.AuxControlOverride(channel); ok {
		outputs = uniqueSortedStrings(append(outputs, override.Outputs...))
		features = uniqueSortedStrings(append(features, override.Features...))
		if len(outputs) > 0 {
			features = uniqueSortedStrings(append(features, auxFeatureAliases(outputs)...))
		}
	}

	if len(outputs) == 0 && len(features) == 0 {
		capabilities := d.augmentAuxCapabilitiesWithImou(channel, dahua.NVRAuxCapabilities{})
		if capabilities.Supported {
			return capabilities, nil
		}
		return dahua.NVRAuxCapabilities{}, fmt.Errorf("%w: aux outputs are not supported on channel %d", dahua.ErrUnsupportedOperation, channel)
	}

	capabilities := dahua.NVRAuxCapabilities{
		Supported: true,
		Outputs:   outputs,
		Features:  features,
	}
	return d.augmentAuxCapabilitiesWithImou(channel, capabilities), nil
}

func (d *Driver) sendPTZAction(ctx context.Context, channel int, action string, code string, speed int) error {
	body, err := d.client.GetText(ctx, "/cgi-bin/ptz.cgi", url.Values{
		"action":  []string{action},
		"channel": []string{strconv.Itoa(channel)},
		"code":    []string{code},
		"arg1":    []string{"0"},
		"arg2":    []string{strconv.Itoa(speed)},
		"arg3":    []string{"0"},
	})
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(body), "OK") {
		return fmt.Errorf("ptz action %s %s returned %q", action, code, strings.TrimSpace(body))
	}
	return nil
}

func (d *Driver) sendAuxAction(ctx context.Context, channel int, action string, code string) error {
	body, err := d.client.GetText(ctx, "/cgi-bin/ptz.cgi", url.Values{
		"action":  []string{action},
		"channel": []string{strconv.Itoa(channel)},
		"code":    []string{code},
		"arg1":    []string{"0"},
		"arg2":    []string{"1"},
		"arg3":    []string{"0"},
	})
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(body), "OK") {
		return fmt.Errorf("aux action %s %s returned %q", action, code, strings.TrimSpace(body))
	}
	return nil
}

func isUnsupportedPTZSurfaceError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, dahua.ErrUnsupportedOperation) {
		return true
	}
	return strings.Contains(err.Error(), "Bad Request!") || strings.Contains(err.Error(), "Not Implemented!")
}

func parsePTZCapabilities(values map[string]string) dahua.NVRPTZCapabilities {
	capabilities := dahua.NVRPTZCapabilities{
		Pan:                strings.EqualFold(strings.TrimSpace(values["caps.Pan"]), "true"),
		Tilt:               strings.EqualFold(strings.TrimSpace(values["caps.Tile"]), "true"),
		Zoom:               strings.EqualFold(strings.TrimSpace(values["caps.Zoom"]), "true"),
		Focus:              strings.EqualFold(strings.TrimSpace(values["caps.Focus"]), "true"),
		MoveRelatively:     strings.EqualFold(strings.TrimSpace(values["caps.MoveRelatively"]), "true"),
		AutoScan:           strings.EqualFold(strings.TrimSpace(values["caps.AutoScan"]), "true"),
		Preset:             strings.EqualFold(strings.TrimSpace(values["caps.Preset"]), "true"),
		Pattern:            strings.EqualFold(strings.TrimSpace(values["caps.Pattern"]), "true"),
		Tour:               strings.EqualFold(strings.TrimSpace(values["caps.Tour"]), "true"),
		Aux:                strings.EqualFold(strings.TrimSpace(values["caps.Aux"]), "true"),
		PanSpeedMin:        parsedIntOrZero(values["caps.PanSpeedMin"]),
		PanSpeedMax:        parsedIntOrZero(values["caps.PanSpeedMax"]),
		TiltSpeedMin:       parsedIntOrZero(values["caps.TileSpeedMin"]),
		TiltSpeedMax:       parsedIntOrZero(values["caps.TileSpeedMax"]),
		PresetMin:          parsedIntOrZero(values["caps.PresetMin"]),
		PresetMax:          parsedIntOrZero(values["caps.PresetMax"]),
		HorizontalAngleMin: parsedIntOrZero(values["caps.PtzMotionRange.HorizontalAngle[0]"]),
		HorizontalAngleMax: parsedIntOrZero(values["caps.PtzMotionRange.HorizontalAngle[1]"]),
		VerticalAngleMin:   parsedIntOrZero(values["caps.PtzMotionRange.VerticalAngle[0]"]),
		VerticalAngleMax:   parsedIntOrZero(values["caps.PtzMotionRange.VerticalAngle[1]"]),
	}

	auxFunctions := make([]string, 0, 4)
	for key, value := range values {
		if !strings.HasPrefix(key, "caps.Auxs[") {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		auxFunctions = append(auxFunctions, value)
	}
	sort.Strings(auxFunctions)
	capabilities.AuxFunctions = auxFunctions

	commands := make([]string, 0, len(ptzCommandSpecs))
	for command, spec := range ptzCommandSpecs {
		if spec.supports(capabilities) {
			commands = append(commands, string(command))
		}
	}
	sort.Strings(commands)
	capabilities.Commands = commands
	capabilities.Supported = len(commands) > 0 || capabilities.Aux
	return capabilities
}

func attachChannelControlState(state *dahua.DeviceState, capabilities dahua.NVRChannelControlCapabilities) {
	if state == nil {
		return
	}
	if state.Info == nil {
		state.Info = make(map[string]any)
	}

	state.Info["control_ptz_supported"] = capabilities.PTZ.Supported
	state.Info["control_ptz_pan"] = capabilities.PTZ.Pan
	state.Info["control_ptz_tilt"] = capabilities.PTZ.Tilt
	state.Info["control_ptz_zoom"] = capabilities.PTZ.Zoom
	state.Info["control_ptz_focus"] = capabilities.PTZ.Focus
	state.Info["control_ptz_move_relatively"] = capabilities.PTZ.MoveRelatively
	state.Info["control_ptz_auto_scan"] = capabilities.PTZ.AutoScan
	state.Info["control_ptz_preset"] = capabilities.PTZ.Preset
	state.Info["control_ptz_pattern"] = capabilities.PTZ.Pattern
	state.Info["control_ptz_tour"] = capabilities.PTZ.Tour
	state.Info["control_aux_supported"] = capabilities.Aux.Supported
	state.Info["control_audio_supported"] = capabilities.Audio.Supported
	state.Info["control_audio_mute_supported"] = capabilities.Audio.Mute
	state.Info["control_audio_volume_supported"] = capabilities.Audio.Volume
	state.Info["control_audio_volume_permission_denied"] = capabilities.Audio.VolumePermissionDenied
	state.Info["control_audio_muted"] = capabilities.Audio.Muted
	state.Info["control_audio_stream_enabled"] = capabilities.Audio.StreamEnabled
	state.Info["control_audio_playback_supported"] = capabilities.Audio.Playback.Supported
	state.Info["control_audio_playback_siren"] = capabilities.Audio.Playback.Siren
	state.Info["control_audio_playback_quick_reply"] = capabilities.Audio.Playback.QuickReply
	state.Info["control_audio_playback_formats"] = append([]string(nil), capabilities.Audio.Playback.Formats...)
	state.Info["control_audio_playback_file_count"] = capabilities.Audio.Playback.FileCount
	state.Info["control_ptz_commands"] = append([]string(nil), capabilities.PTZ.Commands...)
	state.Info["control_aux_outputs"] = append([]string(nil), capabilities.Aux.Outputs...)
	state.Info["control_aux_features"] = append([]string(nil), capabilities.Aux.Features...)
	attachChannelRecordingState(state, capabilities.Recording)
}

func attachChannelRecordingState(state *dahua.DeviceState, capabilities dahua.NVRRecordingCapabilities) {
	if state == nil {
		return
	}
	if state.Info == nil {
		state.Info = make(map[string]any)
	}
	state.Info["control_recording_supported"] = capabilities.Supported
	state.Info["control_recording_active"] = capabilities.Active
	state.Info["control_recording_mode"] = capabilities.Mode
}

func attachValidationNotes(state *dahua.DeviceState, notes []string) {
	if state == nil || len(notes) == 0 {
		return
	}
	if state.Info == nil {
		state.Info = make(map[string]any)
	}
	cloned := append([]string(nil), notes...)
	state.Info["validation_notes"] = cloned
	state.Info["validation_notes_text"] = strings.Join(cloned, "; ")
}

func parsedIntOrZero(value string) int {
	parsed, _ := parseInt(value)
	return parsed
}

func hasAuxFunction(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func normalizeAuxOutputs(values []string) []string {
	outputs := make([]string, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "aux":
			outputs = append(outputs, "aux")
		case "light":
			outputs = append(outputs, "light")
		case "wiper":
			outputs = append(outputs, "wiper")
		default:
			outputs = append(outputs, strings.ToLower(strings.TrimSpace(value)))
		}
	}
	return uniqueSortedStrings(outputs)
}

func hasAuxOutput(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func auxFeatureAliases(outputs []string) []string {
	features := make([]string, 0, len(outputs))
	for _, output := range outputs {
		switch strings.ToLower(strings.TrimSpace(output)) {
		case "aux":
			features = append(features, "siren")
		case "light":
			features = append(features, "warning_light")
		case "wiper":
			features = append(features, "wiper")
		}
	}
	return uniqueSortedStrings(features)
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

func (d *Driver) setChannelLightingMode(ctx context.Context, channel int, action dahua.NVRAuxAction) error {
	if err := d.setChannelLightingModeViaConfig(ctx, channel, action); err == nil {
		return nil
	} else if !errors.Is(err, dahua.ErrUnsupportedOperation) {
		d.logger.Debug().Err(err).Int("channel", channel).Msg("lighting config RPC update failed, falling back")
	}

	objectID, err := d.videoInputObjectID(ctx, channel-1)
	if err != nil {
		return err
	}

	switch action {
	case dahua.NVRAuxActionStart:
		return d.setVideoInputLightParam(ctx, objectID, channel-1, map[string]any{
			"Channel":        channel - 1,
			"LightType":      "WhiteLight",
			"Mode":           "Manual",
			"LightingScheme": "WhiteMode",
		})
	case dahua.NVRAuxActionStop:
		return d.setVideoInputLightParam(ctx, objectID, channel-1, map[string]any{
			"Channel":        channel - 1,
			"LightType":      "AIMixLight",
			"Mode":           "Auto",
			"LightingScheme": "AIMode",
		})
	default:
		return fmt.Errorf("%w: lighting mode only supports start and stop actions", dahua.ErrUnsupportedOperation)
	}
}

func (d *Driver) setChannelLightingModeViaConfig(ctx context.Context, channel int, action dahua.NVRAuxAction) error {
	if d.rpc == nil {
		return fmt.Errorf("%w: rpc client is unavailable", dahua.ErrUnsupportedOperation)
	}

	zeroBasedChannel := channel - 1
	lightingTable, err := d.getLightingConfigTable(ctx, "Lighting_V2", zeroBasedChannel)
	if err != nil {
		return err
	}
	schemeTable, err := d.getLightingConfigTable(ctx, "LightingScheme", zeroBasedChannel)
	if err != nil {
		return err
	}

	targetMode := ""
	switch action {
	case dahua.NVRAuxActionStart:
		targetMode = "WhiteMode"
	case dahua.NVRAuxActionStop:
		targetMode = "AIMode"
	default:
		return fmt.Errorf("%w: lighting mode only supports start and stop actions", dahua.ErrUnsupportedOperation)
	}

	prepareLightingV2Table(lightingTable)
	applyLightingSchemeMode(schemeTable, targetMode)

	calls := []map[string]any{
		{
			"method": "configManager.setConfig",
			"params": map[string]any{
				"name":    "Lighting_V2",
				"table":   lightingTable,
				"options": []any{},
				"channel": zeroBasedChannel,
			},
		},
		{
			"method": "configManager.setConfig",
			"params": map[string]any{
				"name":    "LightingScheme",
				"table":   schemeTable,
				"options": []any{},
				"channel": zeroBasedChannel,
			},
		},
	}

	if err := d.rpc.Call(ctx, "system.multicall", calls, nil); err != nil {
		if isUnknownRPCError(err) {
			return fmt.Errorf("%w: lighting config RPC surface is not exposed on channel %d", dahua.ErrUnsupportedOperation, channel)
		}
		return fmt.Errorf("apply lighting config for channel %d: %w", channel, err)
	}
	return nil
}

func (d *Driver) getLightingConfigTable(ctx context.Context, name string, zeroBasedChannel int) (any, error) {
	var response struct {
		Table any `json:"table"`
	}
	if err := d.rpc.Call(ctx, "configManager.getConfig", map[string]any{
		"name":    name,
		"channel": zeroBasedChannel,
	}, &response); err != nil {
		if isUnknownRPCError(err) {
			return nil, fmt.Errorf("%w: %s config is not exposed on channel %d", dahua.ErrUnsupportedOperation, name, zeroBasedChannel+1)
		}
		return nil, fmt.Errorf("get %s config for channel %d: %w", name, zeroBasedChannel+1, err)
	}
	if response.Table == nil {
		return nil, fmt.Errorf("%w: %s config did not return a table for channel %d", dahua.ErrUnsupportedOperation, name, zeroBasedChannel+1)
	}
	return response.Table, nil
}

func prepareLightingV2Table(table any) {
	rows, ok := table.([]any)
	if !ok {
		return
	}
	for _, rowValue := range rows {
		profiles, ok := rowValue.([]any)
		if !ok {
			continue
		}
		for _, profileValue := range profiles {
			profile, ok := profileValue.(map[string]any)
			if !ok {
				continue
			}
			lightType := strings.TrimSpace(fmt.Sprint(profile["LightType"]))
			switch lightType {
			case "WhiteLight", "AIMixLight":
				profile["Mode"] = "Auto"
				if _, ok := profile["PercentOfMaxBrightness"]; ok {
					profile["PercentOfMaxBrightness"] = 100
				}
			}
		}
	}
}

func applyLightingSchemeMode(table any, targetMode string) {
	items, ok := table.([]any)
	if !ok {
		return
	}
	for index, itemValue := range items {
		item, ok := itemValue.(map[string]any)
		if !ok {
			continue
		}
		mode := "AIMode"
		if index == 0 {
			mode = targetMode
		}
		if targetMode == "AIMode" {
			mode = "AIMode"
		}
		item["LightingMode"] = mode
	}
}

func (d *Driver) videoInputObjectID(ctx context.Context, channel int) (int64, error) {
	if d.rpc == nil {
		return 0, fmt.Errorf("%w: rpc client is unavailable", dahua.ErrUnsupportedOperation)
	}
	if channel < 0 {
		return 0, fmt.Errorf("channel must be >= 0")
	}

	var objectID int64
	if err := d.rpc.Call(ctx, "devVideoInput.factory.instance", map[string]any{"channel": channel}, &objectID); err != nil {
		if isUnknownRPCError(err) {
			return 0, fmt.Errorf("%w: devVideoInput RPC surface is not exposed on channel %d", dahua.ErrUnsupportedOperation, channel+1)
		}
		return 0, fmt.Errorf("create devVideoInput instance for channel %d: %w", channel+1, err)
	}
	if objectID == 0 {
		return 0, fmt.Errorf("%w: devVideoInput RPC instance did not return an object id for channel %d", dahua.ErrUnsupportedOperation, channel+1)
	}
	return objectID, nil
}

func (d *Driver) setVideoInputLightParam(ctx context.Context, objectID int64, zeroBasedChannel int, params map[string]any) error {
	if d.rpc == nil {
		return fmt.Errorf("%w: rpc client is unavailable", dahua.ErrUnsupportedOperation)
	}

	if params == nil {
		params = make(map[string]any)
	}
	if _, ok := params["Channel"]; !ok {
		params["Channel"] = zeroBasedChannel
	}

	if err := d.rpc.CallObject(ctx, "devVideoInput.setLightParam", params, objectID, nil); err != nil {
		if isUnknownRPCError(err) {
			return fmt.Errorf("%w: lighting control is not exposed on channel %d", dahua.ErrUnsupportedOperation, zeroBasedChannel+1)
		}
		return fmt.Errorf("set light param for channel %d: %w", zeroBasedChannel+1, err)
	}
	return nil
}

func normalizeSpeed(speed int, minSpeed int, maxSpeed int) (int, error) {
	if speed <= 0 {
		speed = firstPositive(minSpeed, 1)
	}
	if minSpeed > 0 && speed < minSpeed {
		return 0, fmt.Errorf("speed must be >= %d", minSpeed)
	}
	if maxSpeed > 0 && speed > maxSpeed {
		return 0, fmt.Errorf("speed must be <= %d", maxSpeed)
	}
	return speed, nil
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

var _ dahua.NVRChannelControlReader = (*Driver)(nil)
var _ dahua.NVRPTZController = (*Driver)(nil)
var _ dahua.NVRAuxController = (*Driver)(nil)
