package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/mqtt"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

type intercomCommandController interface {
	Enabled() bool
	StopIntercomSessions(string) media.IntercomStatus
	SetIntercomUplinkEnabled(string, bool) media.IntercomStatus
}

const (
	defaultNVRSirenPulseDuration        = 3 * time.Second
	defaultNVRWarningLightPulseDuration = 3 * time.Second
	defaultNVRWiperPulseDuration        = 1 * time.Second
)

func registerCommandHandlers(
	ctx context.Context,
	cfg config.Config,
	logger zerolog.Logger,
	mqttClient mqtt.Client,
	probes *store.ProbeStore,
	drivers []dahua.Driver,
	intercom intercomCommandController,
) error {
	lockControllers := make(map[string]dahua.VTOLockController)
	callControllers := make(map[string]dahua.VTOCallController)
	audioControllers := make(map[string]dahua.VTOAudioController)
	vtoRecordingControllers := make(map[string]dahua.VTORecordingController)
	auxControllers := make(map[string]dahua.NVRAuxController)
	nvrRecordingControllers := make(map[string]dahua.NVRRecordingController)
	for _, driver := range drivers {
		switch driver.Kind() {
		case dahua.DeviceKindVTO:
			if controller, ok := driver.(dahua.VTOLockController); ok {
				lockControllers[driver.ID()] = controller
			}
			if controller, ok := driver.(dahua.VTOCallController); ok {
				callControllers[driver.ID()] = controller
			}
			if controller, ok := driver.(dahua.VTOAudioController); ok {
				audioControllers[driver.ID()] = controller
			}
			if controller, ok := driver.(dahua.VTORecordingController); ok {
				vtoRecordingControllers[driver.ID()] = controller
			}
		case dahua.DeviceKindNVR:
			if controller, ok := driver.(dahua.NVRAuxController); ok {
				auxControllers[driver.ID()] = controller
			}
			if controller, ok := driver.(dahua.NVRRecordingController); ok {
				nvrRecordingControllers[driver.ID()] = controller
			}
		}
	}

	if len(lockControllers) == 0 && len(callControllers) == 0 && len(audioControllers) == 0 && len(vtoRecordingControllers) == 0 && len(auxControllers) == 0 && len(nvrRecordingControllers) == 0 && (intercom == nil || !intercom.Enabled()) {
		return nil
	}

	topic := fmt.Sprintf("%s/devices/+/command/+", cfg.MQTT.TopicPrefix)
	log := logger.With().Str("component", "mqtt_commands").Str("topic", topic).Logger()

	return mqttClient.Subscribe(ctx, topic, cfg.MQTT.QoS, func(topic string, payload []byte) {
		deviceID, command, ok := parseCommandTopic(topic, cfg.MQTT.TopicPrefix)
		if !ok {
			log.Warn().Str("received_topic", topic).Msg("received command on unexpected topic")
			return
		}

		switch command {
		case "output_volume":
			level, ok := parseVolumeCommandPayload(payload)
			if !ok {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			controller, ok := audioControllers[deviceID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("no vto audio controller registered for output volume command")
				return
			}

			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.SetAudioOutputVolume(commandCtx, 0, level); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Int("level", level).Msg("failed to execute vto output volume command")
				return
			}

			log.Info().Str("device_id", deviceID).Int("level", level).Msg("executed vto output volume command")
		case "input_volume":
			level, ok := parseVolumeCommandPayload(payload)
			if !ok {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			controller, ok := audioControllers[deviceID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("no vto audio controller registered for input volume command")
				return
			}

			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.SetAudioInputVolume(commandCtx, 0, level); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Int("level", level).Msg("failed to execute vto input volume command")
				return
			}

			log.Info().Str("device_id", deviceID).Int("level", level).Msg("executed vto input volume command")
		case "mute":
			enabled, ok := parseToggleCommandPayload(payload)
			if !ok {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			controller, ok := audioControllers[deviceID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("no vto audio controller registered for mute command")
				return
			}

			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.SetAudioMute(commandCtx, enabled); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Bool("muted", enabled).Msg("failed to execute vto mute command")
				return
			}

			log.Info().Str("device_id", deviceID).Bool("muted", enabled).Msg("executed vto mute command")
		case "auto_record":
			enabled, ok := parseToggleCommandPayload(payload)
			if !ok {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			controller, ok := vtoRecordingControllers[deviceID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("no vto recording controller registered for auto record command")
				return
			}

			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.SetRecordingEnabled(commandCtx, enabled); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Bool("enabled", enabled).Msg("failed to execute vto auto record command")
				return
			}

			log.Info().Str("device_id", deviceID).Bool("enabled", enabled).Msg("executed vto auto record command")
		case "press":
			if !isPressCommandPayload(payload) {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			rootID, lockIndex, ok := resolveVTOLockTarget(probes, deviceID)
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("lock command target is not available in current probe inventory")
				return
			}

			controller, ok := lockControllers[rootID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Str("root_id", rootID).Msg("no vto lock controller registered for command")
				return
			}

			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.Unlock(commandCtx, lockIndex); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Str("root_id", rootID).Int("lock_index", lockIndex).Msg("failed to execute vto unlock command")
				return
			}

			log.Info().Str("device_id", deviceID).Str("root_id", rootID).Int("lock_index", lockIndex).Msg("executed vto unlock command")
		case "hangup":
			if !isPressCommandPayload(payload) {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			controller, ok := callControllers[deviceID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("no vto call controller registered for hangup command")
				return
			}

			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.HangupCall(commandCtx); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Msg("failed to execute vto hangup command")
				return
			}

			log.Info().Str("device_id", deviceID).Msg("executed vto hangup command")
		case "answer":
			if !isPressCommandPayload(payload) {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			controller, ok := callControllers[deviceID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("no vto call controller registered for answer command")
				return
			}

			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.AnswerCall(commandCtx); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Msg("failed to execute vto answer command")
				return
			}

			log.Info().Str("device_id", deviceID).Msg("executed vto answer command")
		case "intercom_reset":
			if !isPressCommandPayload(payload) {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			if intercom == nil || !intercom.Enabled() {
				log.Warn().Str("device_id", deviceID).Msg("bridge media layer is unavailable for intercom reset command")
				return
			}

			status := intercom.StopIntercomSessions(deviceID)
			log.Info().
				Str("device_id", deviceID).
				Int("remaining_sessions", status.SessionCount).
				Msg("executed bridge intercom reset command")
		case "uplink_enable":
			if !isPressCommandPayload(payload) {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			if intercom == nil || !intercom.Enabled() {
				log.Warn().Str("device_id", deviceID).Msg("bridge media layer is unavailable for uplink enable command")
				return
			}

			status := intercom.SetIntercomUplinkEnabled(deviceID, true)
			log.Info().
				Str("device_id", deviceID).
				Bool("external_uplink_enabled", status.ExternalUplinkEnabled).
				Msg("executed bridge intercom uplink enable command")
		case "uplink_disable":
			if !isPressCommandPayload(payload) {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			if intercom == nil || !intercom.Enabled() {
				log.Warn().Str("device_id", deviceID).Msg("bridge media layer is unavailable for uplink disable command")
				return
			}

			status := intercom.SetIntercomUplinkEnabled(deviceID, false)
			log.Info().
				Str("device_id", deviceID).
				Bool("external_uplink_enabled", status.ExternalUplinkEnabled).
				Msg("executed bridge intercom uplink disable command")
		case "siren", "warning_light", "wiper":
			if !isPressCommandPayload(payload) {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			rootID, channel, ok := resolveNVRChannelTarget(probes, deviceID)
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("nvr channel command target is not available in current probe inventory")
				return
			}

			controller, ok := auxControllers[rootID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Str("root_id", rootID).Msg("no nvr aux controller registered for command")
				return
			}

			request := nvrAuxCommandRequest(command, channel)
			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.Aux(commandCtx, request); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Str("root_id", rootID).Int("channel", channel).Str("command", command).Msg("failed to execute nvr aux command")
				return
			}

			log.Info().Str("device_id", deviceID).Str("root_id", rootID).Int("channel", channel).Str("command", command).Msg("executed nvr aux command")
		case "recording_start", "recording_stop":
			if !isPressCommandPayload(payload) {
				log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
				return
			}

			rootID, channel, ok := resolveNVRChannelTarget(probes, deviceID)
			if !ok {
				log.Warn().Str("device_id", deviceID).Msg("nvr channel command target is not available in current probe inventory")
				return
			}

			controller, ok := nvrRecordingControllers[rootID]
			if !ok {
				log.Warn().Str("device_id", deviceID).Str("root_id", rootID).Msg("no nvr recording controller registered for command")
				return
			}

			action := dahua.NVRRecordingActionStart
			if command == "recording_stop" {
				action = dahua.NVRRecordingActionStop
			}

			commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := controller.Recording(commandCtx, dahua.NVRRecordingRequest{
				Channel: channel,
				Action:  action,
			}); err != nil {
				log.Error().Err(err).Str("device_id", deviceID).Str("root_id", rootID).Int("channel", channel).Str("command", command).Msg("failed to execute nvr recording command")
				return
			}

			log.Info().Str("device_id", deviceID).Str("root_id", rootID).Int("channel", channel).Str("command", command).Msg("executed nvr recording command")
		default:
			log.Debug().Str("device_id", deviceID).Str("command", command).Msg("ignoring unsupported command topic")
		}
	})
}

func isPressCommandPayload(payload []byte) bool {
	return strings.TrimSpace(strings.ToUpper(string(payload))) == "PRESS"
}

func parseToggleCommandPayload(payload []byte) (bool, bool) {
	switch strings.TrimSpace(strings.ToUpper(string(payload))) {
	case "ON", "TRUE", "1":
		return true, true
	case "OFF", "FALSE", "0":
		return false, true
	default:
		return false, false
	}
}

func parseVolumeCommandPayload(payload []byte) (int, bool) {
	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return 0, false
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	if value < 0 || value > 100 {
		return 0, false
	}

	return int(value + 0.5), true
}

func parseCommandTopic(topic string, prefix string) (string, string, bool) {
	expectedPrefix := strings.TrimSuffix(prefix, "/") + "/devices/"
	if !strings.HasPrefix(topic, expectedPrefix) {
		return "", "", false
	}

	rest := strings.TrimPrefix(topic, expectedPrefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[1] != "command" {
		return "", "", false
	}

	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[2]) == "" {
		return "", "", false
	}

	return parts[0], parts[2], true
}

func resolveVTOLockTarget(probes *store.ProbeStore, targetID string) (string, int, bool) {
	for _, result := range probes.List() {
		if result == nil || result.Root.Kind != dahua.DeviceKindVTO {
			continue
		}

		for _, child := range result.Children {
			if child.ID != targetID || child.Kind != dahua.DeviceKindVTOLock {
				continue
			}

			index, err := strconv.Atoi(child.Attributes["lock_index"])
			if err != nil || index < 0 {
				return "", 0, false
			}

			return result.Root.ID, index, true
		}
	}

	return "", 0, false
}

func resolveNVRChannelTarget(probes *store.ProbeStore, targetID string) (string, int, bool) {
	for _, result := range probes.List() {
		if result == nil || result.Root.Kind != dahua.DeviceKindNVR {
			continue
		}

		for _, child := range result.Children {
			if child.ID != targetID || child.Kind != dahua.DeviceKindNVRChannel {
				continue
			}

			channel, err := strconv.Atoi(child.Attributes["channel_index"])
			if err != nil || channel <= 0 {
				return "", 0, false
			}

			return result.Root.ID, channel, true
		}
	}

	return "", 0, false
}

func nvrAuxCommandRequest(command string, channel int) dahua.NVRAuxRequest {
	request := dahua.NVRAuxRequest{
		Channel: channel,
		Action:  dahua.NVRAuxActionPulse,
	}

	switch command {
	case "warning_light":
		request.Output = "light"
		request.Duration = defaultNVRWarningLightPulseDuration
	case "wiper":
		request.Output = "wiper"
		request.Duration = defaultNVRWiperPulseDuration
	default:
		request.Output = "aux"
		request.Duration = defaultNVRSirenPulseDuration
	}

	return request
}
