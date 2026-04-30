package nvr

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

const (
	diagnosticDefaultPulse = 300 * time.Millisecond
	diagnosticMaxPulse     = 10 * time.Second
)

func (d *Driver) DiagnosticAction(ctx context.Context, request dahua.NVRDiagnosticActionRequest) (dahua.NVRDiagnosticActionResult, error) {
	cfg := d.currentConfig()
	if request.Channel <= 0 {
		return dahua.NVRDiagnosticActionResult{}, fmt.Errorf("channel must be >= 1")
	}
	if !cfg.AllowsChannel(request.Channel) {
		return dahua.NVRDiagnosticActionResult{}, fmt.Errorf("%w: channel %d is not allowed", dahua.ErrUnsupportedOperation, request.Channel)
	}

	method := strings.ToLower(strings.TrimSpace(request.Method))
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if method == "" {
		return dahua.NVRDiagnosticActionResult{}, fmt.Errorf("method is required")
	}
	if action == "" {
		return dahua.NVRDiagnosticActionResult{}, fmt.Errorf("action is required")
	}

	duration, err := normalizeDiagnosticDuration(request.Duration)
	if err != nil {
		return dahua.NVRDiagnosticActionResult{}, err
	}

	result := dahua.NVRDiagnosticActionResult{
		Status:     "ok",
		DeviceID:   d.ID(),
		Channel:    request.Channel,
		Method:     method,
		Action:     action,
		DurationMS: duration.Milliseconds(),
	}

	switch method {
	case "bridge_siren":
		result.Description = "Bridge-selected NVR aux strategy for siren/Aux output."
		result.Endpoint = "/api/v1/nvr/" + url.PathEscape(d.ID()) + "/channels/" + strconv.Itoa(request.Channel) + "/aux"
		err = d.Aux(ctx, dahua.NVRAuxRequest{Channel: request.Channel, Output: "aux", Action: diagnosticAuxAction(action), Duration: duration})
	case "bridge_light":
		result.Description = "Bridge-selected white-light strategy."
		result.Endpoint = "/api/v1/nvr/" + url.PathEscape(d.ID()) + "/channels/" + strconv.Itoa(request.Channel) + "/aux"
		err = d.Aux(ctx, dahua.NVRAuxRequest{Channel: request.Channel, Output: "light", Action: diagnosticAuxAction(action), Duration: duration})
	case "bridge_warning_light":
		result.Description = "Bridge-selected warning-light strategy."
		result.Endpoint = "/api/v1/nvr/" + url.PathEscape(d.ID()) + "/channels/" + strconv.Itoa(request.Channel) + "/aux"
		err = d.Aux(ctx, dahua.NVRAuxRequest{Channel: request.Channel, Output: "warning_light", Action: diagnosticAuxAction(action), Duration: duration})
	case "bridge_wiper":
		result.Description = "Bridge-selected wiper strategy."
		result.Endpoint = "/api/v1/nvr/" + url.PathEscape(d.ID()) + "/channels/" + strconv.Itoa(request.Channel) + "/aux"
		err = d.Aux(ctx, dahua.NVRAuxRequest{Channel: request.Channel, Output: "wiper", Action: diagnosticAuxAction(action), Duration: duration})
	case "bridge_audio":
		enabled, enableErr := diagnosticAudioEnabled(action)
		if enableErr != nil {
			err = enableErr
			break
		}
		result.Description = "Bridge-selected stream-audio mute strategy."
		result.Endpoint = "/api/v1/nvr/" + url.PathEscape(d.ID()) + "/channels/" + strconv.Itoa(request.Channel) + "/audio/mute"
		err = d.SetAudioMute(ctx, dahua.NVRAudioRequest{Channel: request.Channel, Muted: !enabled})
	case "nvr_ptz_aux":
		result.Description = "Raw NVR PTZ CGI Aux command."
		result.Endpoint = diagnosticPTZEndpoint(request.Channel, "Aux")
		err = d.runNVRAuxDiagnostic(ctx, request.Channel, action, "Aux", duration)
	case "nvr_ptz_light":
		result.Description = "Raw NVR PTZ CGI Light command."
		result.Endpoint = diagnosticPTZEndpoint(request.Channel, "Light")
		err = d.runNVRAuxDiagnostic(ctx, request.Channel, action, "Light", duration)
	case "nvr_ptz_wiper":
		result.Description = "Raw NVR PTZ CGI Wiper command."
		result.Endpoint = diagnosticPTZEndpoint(request.Channel, "Wiper")
		err = d.runNVRAuxDiagnostic(ctx, request.Channel, action, "Wiper", duration)
	case "nvr_lighting_config":
		auxAction, actionErr := diagnosticLightingAction(action)
		if actionErr != nil {
			err = actionErr
			break
		}
		result.Description = "NVR RPC configManager.setConfig Lighting_V2 and LightingScheme."
		result.Endpoint = "RPC configManager.setConfig Lighting_V2 + LightingScheme"
		err = d.setChannelLightingModeViaConfig(ctx, request.Channel, auxAction)
	case "nvr_video_input_light_param":
		auxAction, actionErr := diagnosticLightingAction(action)
		if actionErr != nil {
			err = actionErr
			break
		}
		result.Description = "NVR RPC VideoIn.setLightParam legacy light control path."
		result.Endpoint = "RPC VideoIn.setLightParam"
		err = d.setChannelVideoInputLightingMode(ctx, request.Channel, auxAction)
	case "direct_ipc_lighting":
		auxAction, actionErr := diagnosticLightingAction(action)
		if actionErr != nil {
			err = actionErr
			break
		}
		result.Description = "Direct IPC Lighting_V2 configManager setConfig path."
		result.Endpoint = "/cgi-bin/configManager.cgi?action=setConfig&Lighting_V2..."
		err = d.setDirectIPCLightingMode(ctx, request.Channel, auxAction)
	case "direct_ipc_ptz_aux":
		result.Description = "Direct IPC raw PTZ CGI Aux command using camera channel 1."
		result.Endpoint = diagnosticDirectIPCPTZEndpoint(1, "Aux")
		err = d.runDirectIPCAuxDiagnostic(ctx, request.Channel, 1, action, "Aux", duration)
	case "direct_ipc_ptz_aux_ch0":
		result.Description = "Direct IPC raw PTZ CGI Aux command using camera channel 0."
		result.Endpoint = diagnosticDirectIPCPTZEndpoint(0, "Aux")
		err = d.runDirectIPCAuxDiagnostic(ctx, request.Channel, 0, action, "Aux", duration)
	case "direct_ipc_ptz_light":
		result.Description = "Direct IPC raw PTZ CGI Light command using camera channel 1."
		result.Endpoint = diagnosticDirectIPCPTZEndpoint(1, "Light")
		err = d.runDirectIPCAuxDiagnostic(ctx, request.Channel, 1, action, "Light", duration)
	case "direct_ipc_ptz_light_ch0":
		result.Description = "Direct IPC raw PTZ CGI Light command using camera channel 0."
		result.Endpoint = diagnosticDirectIPCPTZEndpoint(0, "Light")
		err = d.runDirectIPCAuxDiagnostic(ctx, request.Channel, 0, action, "Light", duration)
	case "direct_ipc_ptz_wiper":
		result.Description = "Direct IPC raw PTZ CGI Wiper command using camera channel 1."
		result.Endpoint = diagnosticDirectIPCPTZEndpoint(1, "Wiper")
		err = d.runDirectIPCAuxDiagnostic(ctx, request.Channel, 1, action, "Wiper", duration)
	case "direct_ipc_ptz_wiper_ch0":
		result.Description = "Direct IPC raw PTZ CGI Wiper command using camera channel 0."
		result.Endpoint = diagnosticDirectIPCPTZEndpoint(0, "Wiper")
		err = d.runDirectIPCAuxDiagnostic(ctx, request.Channel, 0, action, "Wiper", duration)
	case "nvr_audio_config":
		enabled, enableErr := diagnosticAudioEnabled(action)
		if enableErr != nil {
			err = enableErr
			break
		}
		result.Description = "NVR Encode AudioEnable config paths for all discovered streams."
		result.Endpoint = "/cgi-bin/configManager.cgi?action=setConfig&Encode...AudioEnable"
		err = d.setChannelMainAudioEnabled(ctx, request.Channel, enabled)
	case "direct_ipc_audio":
		enabled, enableErr := diagnosticAudioEnabled(action)
		if enableErr != nil {
			err = enableErr
			break
		}
		result.Description = "Direct IPC Encode AudioEnable config paths for all discovered streams."
		result.Endpoint = "/cgi-bin/configManager.cgi?action=setConfig&Encode...AudioEnable"
		err = d.setDirectIPCAudioEnabled(ctx, request.Channel, enabled)
	case "record_mode":
		mode, modeErr := diagnosticRecordMode(action)
		if modeErr != nil {
			err = modeErr
			break
		}
		recordModes, loadErr := d.loadRecordModes(ctx)
		if loadErr != nil {
			err = loadErr
			break
		}
		state, ok := recordModes[request.Channel-1]
		if !ok {
			err = fmt.Errorf("%w: recording controls are not supported on channel %d", dahua.ErrUnsupportedOperation, request.Channel)
			break
		}
		result.Description = "NVR RecordMode config path."
		result.Endpoint = "/cgi-bin/configManager.cgi?action=setConfig&RecordMode..."
		err = d.setChannelRecordMode(ctx, request.Channel, mode, state)
	default:
		err = fmt.Errorf("%w: unsupported diagnostic method %q", dahua.ErrUnsupportedOperation, method)
	}
	if err != nil {
		return dahua.NVRDiagnosticActionResult{}, err
	}

	if len(result.Notes) == 0 {
		result.Notes = diagnosticNotes(method)
	}
	return result, nil
}

func normalizeDiagnosticDuration(duration time.Duration) (time.Duration, error) {
	if duration < 0 {
		return 0, fmt.Errorf("duration_ms must be zero or positive")
	}
	if duration == 0 {
		duration = diagnosticDefaultPulse
	}
	if duration > diagnosticMaxPulse {
		return 0, fmt.Errorf("duration_ms must be <= 10000")
	}
	return duration, nil
}

func diagnosticAuxAction(action string) dahua.NVRAuxAction {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "on":
		return dahua.NVRAuxActionStart
	case "off":
		return dahua.NVRAuxActionStop
	default:
		return dahua.NVRAuxAction(action)
	}
}

func diagnosticLightingAction(action string) (dahua.NVRAuxAction, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "start", "on":
		return dahua.NVRAuxActionStart, nil
	case "stop", "off":
		return dahua.NVRAuxActionStop, nil
	default:
		return "", fmt.Errorf("%w: lighting diagnostics support start/on and stop/off actions", dahua.ErrUnsupportedOperation)
	}
}

func diagnosticAudioEnabled(action string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "start", "on", "unmute", "enable", "enabled":
		return true, nil
	case "stop", "off", "mute", "disable", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("%w: audio diagnostics support on/unmute and off/mute actions", dahua.ErrUnsupportedOperation)
	}
}

func diagnosticRecordMode(action string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "auto":
		return 0, nil
	case "start", "manual":
		return 1, nil
	case "stop":
		return 2, nil
	default:
		return 0, fmt.Errorf("%w: record_mode diagnostics support start, stop, and auto actions", dahua.ErrUnsupportedOperation)
	}
}

func (d *Driver) runNVRAuxDiagnostic(ctx context.Context, channel int, action string, code string, duration time.Duration) error {
	if d.client == nil {
		return fmt.Errorf("%w: nvr cgi client is unavailable", dahua.ErrUnsupportedOperation)
	}
	return runDiagnosticAuxAction(ctx, action, duration, func(callCtx context.Context, callAction string) error {
		return d.sendAuxAction(callCtx, channel, callAction, code)
	})
}

func (d *Driver) runDirectIPCAuxDiagnostic(ctx context.Context, bridgeChannel int, cameraChannel int, action string, code string, duration time.Duration) error {
	target, err := d.directIPCTargetForChannel(ctx, bridgeChannel)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("%w: direct ipc control is not configured on channel %d", dahua.ErrUnsupportedOperation, bridgeChannel)
	}

	client := d.directIPCClient(target)
	return runDiagnosticAuxAction(ctx, action, duration, func(callCtx context.Context, callAction string) error {
		body, err := client.GetText(callCtx, "/cgi-bin/ptz.cgi", url.Values{
			"action":  []string{callAction},
			"channel": []string{strconv.Itoa(cameraChannel)},
			"code":    []string{code},
			"arg1":    []string{"0"},
			"arg2":    []string{"1"},
			"arg3":    []string{"0"},
		})
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(body), "OK") {
			return fmt.Errorf("direct ipc aux action %s %s returned %q", callAction, code, strings.TrimSpace(body))
		}
		return nil
	})
}

func runDiagnosticAuxAction(ctx context.Context, action string, duration time.Duration, send func(context.Context, string) error) error {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "start", "on":
		return send(ctx, "start")
	case "stop", "off":
		return send(ctx, "stop")
	case "pulse":
		if err := send(ctx, "start"); err != nil {
			return err
		}
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = send(stopCtx, "stop")
			return ctx.Err()
		case <-timer.C:
		}
		return send(ctx, "stop")
	default:
		return fmt.Errorf("%w: aux diagnostics support start/on, stop/off, and pulse actions", dahua.ErrUnsupportedOperation)
	}
}

func (d *Driver) setChannelVideoInputLightingMode(ctx context.Context, channel int, action dahua.NVRAuxAction) error {
	if err := d.requireNVRConfigWrite(ctx, channel, "nvr video input lighting control"); err != nil {
		return err
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
		return fmt.Errorf("%w: lighting diagnostics support start/on and stop/off actions", dahua.ErrUnsupportedOperation)
	}
}

func diagnosticPTZEndpoint(channel int, code string) string {
	return "/cgi-bin/ptz.cgi?action=start|stop&channel=" + strconv.Itoa(channel) + "&code=" + url.QueryEscape(code) + "&arg1=0&arg2=1&arg3=0"
}

func diagnosticDirectIPCPTZEndpoint(cameraChannel int, code string) string {
	return "/cgi-bin/ptz.cgi?action=start|stop&channel=" + strconv.Itoa(cameraChannel) + "&code=" + url.QueryEscape(code) + "&arg1=0&arg2=1&arg3=0"
}

func diagnosticNotes(method string) []string {
	switch method {
	case "direct_ipc_lighting", "direct_ipc_audio", "direct_ipc_ptz_aux", "direct_ipc_ptz_aux_ch0", "direct_ipc_ptz_light", "direct_ipc_ptz_light_ch0", "direct_ipc_ptz_wiper", "direct_ipc_ptz_wiper_ch0":
		return []string{"Requires direct_ipc credentials for the selected NVR channel."}
	case "nvr_lighting_config", "nvr_video_input_light_param", "nvr_audio_config", "record_mode":
		return []string{"Requires allow_config_writes for the NVR device."}
	default:
		return nil
	}
}

var _ dahua.NVRDiagnosticController = (*Driver)(nil)
