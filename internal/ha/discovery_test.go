package ha

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/mqtt"
	"github.com/rs/zerolog"
)

type mockMQTTClient struct {
	published []publishedMessage
}

type publishedMessage struct {
	topic   string
	payload []byte
}

func (m *mockMQTTClient) Connect(context.Context) error { return nil }

func (m *mockMQTTClient) Publish(_ context.Context, topic string, _ byte, _ bool, payload []byte) error {
	cloned := make([]byte, len(payload))
	copy(cloned, payload)
	m.published = append(m.published, publishedMessage{topic: topic, payload: cloned})
	return nil
}

func (m *mockMQTTClient) PublishJSON(ctx context.Context, topic string, qos byte, retain bool, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return m.Publish(ctx, topic, qos, retain, data)
}

func (m *mockMQTTClient) Subscribe(context.Context, string, byte, func(string, []byte)) error {
	return nil
}

func (m *mockMQTTClient) Close() {}

var _ mqtt.Client = (*mockMQTTClient)(nil)

func TestExtraTriggerConfigsForVTO(t *testing.T) {
	publisher := &DiscoveryPublisher{
		cfg: config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
				QoS:         1,
			},
		},
	}

	triggers := publisher.extraTriggerConfigs(dahua.Device{
		ID:   "front_vto",
		Name: "Front Door",
		Kind: dahua.DeviceKindVTO,
	})

	if len(triggers) != 5 {
		t.Fatalf("expected 5 triggers, got %d", len(triggers))
	}

	expected := map[string]struct {
		payload string
		typ     string
		subtype string
	}{
		"doorbell_start":      {payload: "doorbell_start", typ: "doorbell_pressed", subtype: "doorbell"},
		"call_start":          {payload: "call_start", typ: "call_started", subtype: "call"},
		"call_stop":           {payload: "call_stop", typ: "call_ended", subtype: "call"},
		"accesscontrol_start": {payload: "accesscontrol_start", typ: "access_granted", subtype: "door_access"},
		"tamper_start":        {payload: "tamper_start", typ: "tamper_detected", subtype: "tamper"},
	}

	for _, trigger := range triggers {
		want, ok := expected[trigger.objectID]
		if !ok {
			t.Fatalf("unexpected trigger object id %q", trigger.objectID)
		}
		if trigger.config.Topic != "dahuabridge/devices/front_vto/event/activity" {
			t.Fatalf("unexpected topic for %q: %q", trigger.objectID, trigger.config.Topic)
		}
		if trigger.config.Payload != want.payload {
			t.Fatalf("unexpected payload for %q: %q", trigger.objectID, trigger.config.Payload)
		}
		if trigger.config.Type != want.typ {
			t.Fatalf("unexpected type for %q: %q", trigger.objectID, trigger.config.Type)
		}
		if trigger.config.Subtype != want.subtype {
			t.Fatalf("unexpected subtype for %q: %q", trigger.objectID, trigger.config.Subtype)
		}
		if trigger.config.ValueTemplate != "{{ value_json.event_type }}" {
			t.Fatalf("unexpected value template for %q: %q", trigger.objectID, trigger.config.ValueTemplate)
		}
	}
}

func TestExtraEntityConfigsForNVRHealth(t *testing.T) {
	publisher := &DiscoveryPublisher{
		cfg: config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
			},
			HomeAssistant: config.HomeAssistantConfig{
				NodeID: "dahuabridge",
			},
		},
	}

	entities := publisher.extraEntityConfigs(dahua.Device{
		ID:   "west20_nvr",
		Name: "West 20 NVR",
		Kind: dahua.DeviceKindNVR,
	}, "dahuabridge/devices/west20_nvr/availability", "dahuabridge/devices/west20_nvr/info")

	expected := map[string]struct {
		component string
		template  string
	}{
		"disk_fault":       {component: "binary_sensor", template: "disk_fault"},
		"disk_error_count": {component: "sensor", template: "disk_error_count"},
		"total_bytes":      {component: "sensor", template: "total_bytes"},
		"used_bytes":       {component: "sensor", template: "used_bytes"},
		"free_bytes":       {component: "sensor", template: "free_bytes"},
		"used_percent":     {component: "sensor", template: "used_percent"},
	}

	found := make(map[string]discoveredEntity, len(entities))
	for _, entity := range entities {
		found[entity.objectID] = entity
	}

	for objectID, want := range expected {
		entity, ok := found[objectID]
		if !ok {
			t.Fatalf("missing nvr health entity %q", objectID)
		}
		if entity.component != want.component {
			t.Fatalf("unexpected component for %q: %q", objectID, entity.component)
		}
		if !strings.Contains(entity.config.ValueTemplate, want.template) {
			t.Fatalf("unexpected value template for %q: %q", objectID, entity.config.ValueTemplate)
		}
	}
}

func TestCameraConfigsForProbeResult(t *testing.T) {
	publisher := &DiscoveryPublisher{
		cfg: config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
			},
			HomeAssistant: config.HomeAssistantConfig{
				NodeID: "dahuabridge",
			},
		},
	}

	cameras := publisher.cameraConfigs(&dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "west20_nvr",
			Kind: dahua.DeviceKindNVR,
		},
		Children: []dahua.Device{
			{
				ID:   "west20_nvr_channel_01",
				Name: "Front Gate",
				Kind: dahua.DeviceKindNVRChannel,
			},
			{
				ID:   "west20_nvr_disk_00",
				Name: "Disk 0",
				Kind: dahua.DeviceKindNVRDisk,
			},
		},
	})

	if len(cameras) != 1 {
		t.Fatalf("expected 1 camera config, got %d", len(cameras))
	}
	if cameras[0].config.Topic != "dahuabridge/devices/west20_nvr_channel_01/camera/snapshot" {
		t.Fatalf("unexpected camera topic: %q", cameras[0].config.Topic)
	}
	if cameras[0].config.Name != "Camera" {
		t.Fatalf("unexpected camera name: %q", cameras[0].config.Name)
	}
}

func TestExtraEntityConfigsForVTOCallSession(t *testing.T) {
	publisher := &DiscoveryPublisher{
		cfg: config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix: "dahuabridge",
			},
			HomeAssistant: config.HomeAssistantConfig{
				NodeID: "dahuabridge",
			},
		},
	}

	entities := publisher.extraEntityConfigs(dahua.Device{
		ID:   "front_vto",
		Name: "Front Door",
		Kind: dahua.DeviceKindVTO,
	}, "dahuabridge/devices/front_vto/availability", "dahuabridge/devices/front_vto/info")

	found := make(map[string]discoveredEntity, len(entities))
	for _, entity := range entities {
		found[entity.objectID] = entity
	}

	expectedTopics := map[string]string{
		"call_state":                 "dahuabridge/devices/front_vto/state/call_state",
		"last_ring_at":               "dahuabridge/devices/front_vto/state/last_ring_at",
		"last_call_started_at":       "dahuabridge/devices/front_vto/state/last_call_started_at",
		"last_call_ended_at":         "dahuabridge/devices/front_vto/state/last_call_ended_at",
		"last_call_duration_seconds": "dahuabridge/devices/front_vto/state/last_call_duration_seconds",
		"last_call_source":           "dahuabridge/devices/front_vto/state/last_call_source",
	}

	for objectID, topic := range expectedTopics {
		entity, ok := found[objectID]
		if !ok {
			t.Fatalf("missing vto call session entity %q", objectID)
		}
		if entity.component != "sensor" {
			t.Fatalf("unexpected component for %q: %q", objectID, entity.component)
		}
		if entity.config.StateTopic != topic {
			t.Fatalf("unexpected topic for %q: %q", objectID, entity.config.StateTopic)
		}
	}

	hangup, ok := found["hangup"]
	if !ok {
		t.Fatal("missing vto hangup button")
	}
	if hangup.component != "button" {
		t.Fatalf("unexpected component for hangup: %q", hangup.component)
	}
	if hangup.config.CommandTopic != "dahuabridge/devices/front_vto/command/hangup" {
		t.Fatalf("unexpected command topic for hangup: %q", hangup.config.CommandTopic)
	}
}

func TestPublishProbeAddsDefaultEntityIDsAndCameraDiscovery(t *testing.T) {
	mqttClient := &mockMQTTClient{}
	publisher := NewDiscoveryPublisher(config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}, mqttClient, zerolog.Nop())

	err := publisher.PublishProbe(context.Background(), &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "yard_ipc",
			Name: "Yard Camera",
			Kind: dahua.DeviceKindIPC,
		},
		States: map[string]dahua.DeviceState{
			"yard_ipc": {Available: true},
		},
	})
	if err != nil {
		t.Fatalf("PublishProbe returned error: %v", err)
	}

	var cameraConfig cameraConfig
	var foundCameraConfig bool
	var foundOnlineConfig bool

	for _, message := range mqttClient.published {
		switch message.topic {
		case "homeassistant/camera/dahuabridge/yard_ipc_camera/config":
			foundCameraConfig = true
			if err := json.Unmarshal(message.payload, &cameraConfig); err != nil {
				t.Fatalf("decode camera config: %v", err)
			}
		case "homeassistant/binary_sensor/dahuabridge/yard_ipc_online/config":
			foundOnlineConfig = true
			var config entityConfig
			if err := json.Unmarshal(message.payload, &config); err != nil {
				t.Fatalf("decode online config: %v", err)
			}
			if config.DefaultEntityID != "binary_sensor.yard_ipc_online" {
				t.Fatalf("unexpected default entity id: %q", config.DefaultEntityID)
			}
		}
	}

	if !foundCameraConfig {
		t.Fatal("expected camera discovery config to be published")
	}
	if !foundOnlineConfig {
		t.Fatal("expected online discovery config to be published")
	}
	if cameraConfig.DefaultEntityID != "camera.yard_ipc_camera" {
		t.Fatalf("unexpected camera default entity id: %q", cameraConfig.DefaultEntityID)
	}
	if cameraConfig.Topic != "dahuabridge/devices/yard_ipc/camera/snapshot" {
		t.Fatalf("unexpected camera topic: %q", cameraConfig.Topic)
	}
}
