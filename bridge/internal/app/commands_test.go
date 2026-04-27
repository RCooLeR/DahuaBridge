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
