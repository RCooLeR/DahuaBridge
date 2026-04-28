package app

import (
	"context"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

type stubIntercomController struct {
	enabled       bool
	resetCalls    []string
	uplinkUpdates []struct {
		streamID string
		enabled  bool
	}
}

func (s *stubIntercomController) Enabled() bool {
	return s.enabled
}

func (s *stubIntercomController) StopIntercomSessions(streamID string) media.IntercomStatus {
	s.resetCalls = append(s.resetCalls, streamID)
	return media.IntercomStatus{StreamID: streamID}
}

func (s *stubIntercomController) SetIntercomUplinkEnabled(streamID string, enabled bool) media.IntercomStatus {
	s.uplinkUpdates = append(s.uplinkUpdates, struct {
		streamID string
		enabled  bool
	}{streamID: streamID, enabled: enabled})
	return media.IntercomStatus{StreamID: streamID, ExternalUplinkEnabled: enabled}
}

func TestParseCommandTopic(t *testing.T) {
	deviceID, command, ok := parseCommandTopic("dahuabridge/devices/west20_vto_lock_00/command/press", "dahuabridge")
	if !ok {
		t.Fatal("expected command topic to parse")
	}
	if deviceID != "west20_vto_lock_00" {
		t.Fatalf("unexpected device id %q", deviceID)
	}
	if command != "press" {
		t.Fatalf("unexpected command %q", command)
	}
}

func TestParseCommandTopicRejectsUnexpectedTopic(t *testing.T) {
	if _, _, ok := parseCommandTopic("dahuabridge/devices/west20_vto_lock_00/state/press", "dahuabridge"); ok {
		t.Fatal("expected invalid topic to be rejected")
	}
}

func TestParseCommandTopicSupportsHangup(t *testing.T) {
	deviceID, command, ok := parseCommandTopic("dahuabridge/devices/front_vto/command/hangup", "dahuabridge")
	if !ok {
		t.Fatal("expected hangup topic to parse")
	}
	if deviceID != "front_vto" || command != "hangup" {
		t.Fatalf("unexpected parsed values %q %q", deviceID, command)
	}
}

func TestParseCommandTopicSupportsAnswer(t *testing.T) {
	deviceID, command, ok := parseCommandTopic("dahuabridge/devices/front_vto/command/answer", "dahuabridge")
	if !ok {
		t.Fatal("expected answer topic to parse")
	}
	if deviceID != "front_vto" || command != "answer" {
		t.Fatalf("unexpected parsed values %q %q", deviceID, command)
	}
}

func TestParseCommandTopicSupportsIntercomReset(t *testing.T) {
	deviceID, command, ok := parseCommandTopic("dahuabridge/devices/front_vto/command/intercom_reset", "dahuabridge")
	if !ok {
		t.Fatal("expected intercom reset topic to parse")
	}
	if deviceID != "front_vto" || command != "intercom_reset" {
		t.Fatalf("unexpected parsed values %q %q", deviceID, command)
	}
}

func TestResolveVTOLockTarget(t *testing.T) {
	probes := store.NewProbeStore()
	probes.Set("west20_vto", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "west20_vto",
			Kind: dahua.DeviceKindVTO,
		},
		Children: []dahua.Device{
			{
				ID:   "west20_vto_lock_02",
				Kind: dahua.DeviceKindVTOLock,
				Attributes: map[string]string{
					"lock_index": "2",
				},
			},
		},
	})

	rootID, lockIndex, ok := resolveVTOLockTarget(probes, "west20_vto_lock_02")
	if !ok {
		t.Fatal("expected lock target resolution to succeed")
	}
	if rootID != "west20_vto" {
		t.Fatalf("unexpected root id %q", rootID)
	}
	if lockIndex != 2 {
		t.Fatalf("unexpected lock index %d", lockIndex)
	}
}

func TestResolveNVRChannelTarget(t *testing.T) {
	probes := store.NewProbeStore()
	probes.Set("west20_nvr", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "west20_nvr",
			Kind: dahua.DeviceKindNVR,
		},
		Children: []dahua.Device{
			{
				ID:   "west20_nvr_channel_08",
				Kind: dahua.DeviceKindNVRChannel,
				Attributes: map[string]string{
					"channel_index": "8",
				},
			},
		},
	})

	rootID, channel, ok := resolveNVRChannelTarget(probes, "west20_nvr_channel_08")
	if !ok {
		t.Fatal("expected nvr channel target resolution to succeed")
	}
	if rootID != "west20_nvr" {
		t.Fatalf("unexpected root id %q", rootID)
	}
	if channel != 8 {
		t.Fatalf("unexpected channel %d", channel)
	}
}

func TestRegisterCommandHandlersHandlesBridgeIntercomCommands(t *testing.T) {
	mqttClient := &mockMQTTClient{}
	probes := store.NewProbeStore()
	intercom := &stubIntercomController{enabled: true}

	err := registerCommandHandlers(
		context.Background(),
		config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
				QoS:         1,
			},
		},
		zerolog.Nop(),
		mqttClient,
		probes,
		[]dahua.Driver{
			stubDriver{id: "front_vto", kind: dahua.DeviceKindVTO},
		},
		intercom,
	)
	if err != nil {
		t.Fatalf("registerCommandHandlers returned error: %v", err)
	}
	if mqttClient.subscribeHandler == nil {
		t.Fatal("expected mqtt subscription handler to be registered")
	}

	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/intercom_reset", []byte("PRESS"))
	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/uplink_enable", []byte("PRESS"))
	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/uplink_disable", []byte("PRESS"))

	if len(intercom.resetCalls) != 1 || intercom.resetCalls[0] != "front_vto" {
		t.Fatalf("unexpected intercom reset calls %+v", intercom.resetCalls)
	}
	if len(intercom.uplinkUpdates) != 2 {
		t.Fatalf("unexpected uplink updates %+v", intercom.uplinkUpdates)
	}
	if !intercom.uplinkUpdates[0].enabled || intercom.uplinkUpdates[0].streamID != "front_vto" {
		t.Fatalf("unexpected uplink enable update %+v", intercom.uplinkUpdates[0])
	}
	if intercom.uplinkUpdates[1].enabled || intercom.uplinkUpdates[1].streamID != "front_vto" {
		t.Fatalf("unexpected uplink disable update %+v", intercom.uplinkUpdates[1])
	}
}

func TestRegisterCommandHandlersHandlesVTOCallCommands(t *testing.T) {
	mqttClient := &mockMQTTClient{}
	probes := store.NewProbeStore()
	answerCalls := 0
	hangupCalls := 0

	err := registerCommandHandlers(
		context.Background(),
		config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
				QoS:         1,
			},
		},
		zerolog.Nop(),
		mqttClient,
		probes,
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				answerFn: func(context.Context) error {
					answerCalls++
					return nil
				},
				hangupFn: func(context.Context) error {
					hangupCalls++
					return nil
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("registerCommandHandlers returned error: %v", err)
	}
	if mqttClient.subscribeHandler == nil {
		t.Fatal("expected mqtt subscription handler to be registered")
	}

	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/answer", []byte("PRESS"))
	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/hangup", []byte("PRESS"))

	if answerCalls != 1 {
		t.Fatalf("expected answer command to run once, got %d", answerCalls)
	}
	if hangupCalls != 1 {
		t.Fatalf("expected hangup command to run once, got %d", hangupCalls)
	}
}

func TestRegisterCommandHandlersHandlesVTOToggleCommands(t *testing.T) {
	mqttClient := &mockMQTTClient{}
	probes := store.NewProbeStore()
	var muteValues []bool
	var recordValues []bool

	err := registerCommandHandlers(
		context.Background(),
		config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
				QoS:         1,
			},
		},
		zerolog.Nop(),
		mqttClient,
		probes,
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				vtoMuteFn: func(_ context.Context, muted bool) error {
					muteValues = append(muteValues, muted)
					return nil
				},
				vtoRecordFn: func(_ context.Context, enabled bool) error {
					recordValues = append(recordValues, enabled)
					return nil
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("registerCommandHandlers returned error: %v", err)
	}
	if mqttClient.subscribeHandler == nil {
		t.Fatal("expected mqtt subscription handler to be registered")
	}

	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/mute", []byte("ON"))
	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/mute", []byte("OFF"))
	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/auto_record", []byte("ON"))
	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/auto_record", []byte("OFF"))

	if len(muteValues) != 2 || !muteValues[0] || muteValues[1] {
		t.Fatalf("unexpected mute command values %+v", muteValues)
	}
	if len(recordValues) != 2 || !recordValues[0] || recordValues[1] {
		t.Fatalf("unexpected auto record command values %+v", recordValues)
	}
}

func TestRegisterCommandHandlersHandlesVTOVolumeCommands(t *testing.T) {
	mqttClient := &mockMQTTClient{}
	probes := store.NewProbeStore()
	var outputLevels []int
	var inputLevels []int

	err := registerCommandHandlers(
		context.Background(),
		config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
				QoS:         1,
			},
		},
		zerolog.Nop(),
		mqttClient,
		probes,
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				vtoOutputFn: func(_ context.Context, slot int, level int) error {
					if slot != 0 {
						t.Fatalf("unexpected output slot %d", slot)
					}
					outputLevels = append(outputLevels, level)
					return nil
				},
				vtoInputFn: func(_ context.Context, slot int, level int) error {
					if slot != 0 {
						t.Fatalf("unexpected input slot %d", slot)
					}
					inputLevels = append(inputLevels, level)
					return nil
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("registerCommandHandlers returned error: %v", err)
	}
	if mqttClient.subscribeHandler == nil {
		t.Fatal("expected mqtt subscription handler to be registered")
	}

	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/output_volume", []byte("73"))
	mqttClient.subscribeHandler("dahuabridge/devices/front_vto/command/input_volume", []byte("64.6"))

	if len(outputLevels) != 1 || outputLevels[0] != 73 {
		t.Fatalf("unexpected output volume commands %+v", outputLevels)
	}
	if len(inputLevels) != 1 || inputLevels[0] != 65 {
		t.Fatalf("unexpected input volume commands %+v", inputLevels)
	}
}

func TestRegisterCommandHandlersHandlesNVRChannelCommands(t *testing.T) {
	mqttClient := &mockMQTTClient{}
	probes := store.NewProbeStore()
	probes.Set("west20_nvr", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "west20_nvr",
			Kind: dahua.DeviceKindNVR,
		},
		Children: []dahua.Device{
			{
				ID:   "west20_nvr_channel_08",
				Kind: dahua.DeviceKindNVRChannel,
				Attributes: map[string]string{
					"channel_index": "8",
				},
			},
		},
	})

	var auxRequests []dahua.NVRAuxRequest
	var recordingRequests []dahua.NVRRecordingRequest

	err := registerCommandHandlers(
		context.Background(),
		config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
				QoS:         1,
			},
		},
		zerolog.Nop(),
		mqttClient,
		probes,
		[]dahua.Driver{
			stubDriver{
				id:   "west20_nvr",
				kind: dahua.DeviceKindNVR,
				auxFn: func(_ context.Context, request dahua.NVRAuxRequest) error {
					auxRequests = append(auxRequests, request)
					return nil
				},
				recordingFn: func(_ context.Context, request dahua.NVRRecordingRequest) error {
					recordingRequests = append(recordingRequests, request)
					return nil
				},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("registerCommandHandlers returned error: %v", err)
	}
	if mqttClient.subscribeHandler == nil {
		t.Fatal("expected mqtt subscription handler to be registered")
	}

	mqttClient.subscribeHandler("dahuabridge/devices/west20_nvr_channel_08/command/siren", []byte("PRESS"))
	mqttClient.subscribeHandler("dahuabridge/devices/west20_nvr_channel_08/command/warning_light", []byte("PRESS"))
	mqttClient.subscribeHandler("dahuabridge/devices/west20_nvr_channel_08/command/wiper", []byte("PRESS"))
	mqttClient.subscribeHandler("dahuabridge/devices/west20_nvr_channel_08/command/recording_start", []byte("PRESS"))
	mqttClient.subscribeHandler("dahuabridge/devices/west20_nvr_channel_08/command/recording_stop", []byte("PRESS"))

	if len(auxRequests) != 3 {
		t.Fatalf("expected 3 aux requests, got %+v", auxRequests)
	}
	if auxRequests[0].Channel != 8 || auxRequests[0].Action != dahua.NVRAuxActionPulse || auxRequests[0].Output != "aux" || auxRequests[0].Duration != defaultNVRSirenPulseDuration {
		t.Fatalf("unexpected siren request %+v", auxRequests[0])
	}
	if auxRequests[1].Channel != 8 || auxRequests[1].Action != dahua.NVRAuxActionPulse || auxRequests[1].Output != "light" || auxRequests[1].Duration != defaultNVRWarningLightPulseDuration {
		t.Fatalf("unexpected warning light request %+v", auxRequests[1])
	}
	if auxRequests[2].Channel != 8 || auxRequests[2].Action != dahua.NVRAuxActionPulse || auxRequests[2].Output != "wiper" || auxRequests[2].Duration != defaultNVRWiperPulseDuration {
		t.Fatalf("unexpected wiper request %+v", auxRequests[2])
	}

	if len(recordingRequests) != 2 {
		t.Fatalf("expected 2 recording requests, got %+v", recordingRequests)
	}
	if recordingRequests[0].Channel != 8 || recordingRequests[0].Action != dahua.NVRRecordingActionStart {
		t.Fatalf("unexpected recording start request %+v", recordingRequests[0])
	}
	if recordingRequests[1].Channel != 8 || recordingRequests[1].Action != dahua.NVRRecordingActionStop {
		t.Fatalf("unexpected recording stop request %+v", recordingRequests[1])
	}
}
