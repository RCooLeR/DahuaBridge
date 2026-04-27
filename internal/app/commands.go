package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/mqtt"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

func registerCommandHandlers(
	ctx context.Context,
	cfg config.Config,
	logger zerolog.Logger,
	mqttClient mqtt.Client,
	probes *store.ProbeStore,
	drivers []dahua.Driver,
) error {
	lockControllers := make(map[string]dahua.VTOLockController)
	callControllers := make(map[string]dahua.VTOCallController)
	for _, driver := range drivers {
		if driver.Kind() != dahua.DeviceKindVTO {
			continue
		}
		if controller, ok := driver.(dahua.VTOLockController); ok {
			lockControllers[driver.ID()] = controller
		}
		if controller, ok := driver.(dahua.VTOCallController); ok {
			callControllers[driver.ID()] = controller
		}
	}

	if len(lockControllers) == 0 && len(callControllers) == 0 {
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

		if strings.TrimSpace(strings.ToUpper(string(payload))) != "PRESS" {
			log.Debug().Str("device_id", deviceID).Str("command", command).Str("payload", string(payload)).Msg("ignoring unsupported command payload")
			return
		}

		switch command {
		case "press":
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
		default:
			log.Debug().Str("device_id", deviceID).Str("command", command).Msg("ignoring unsupported command topic")
		}
	})
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
