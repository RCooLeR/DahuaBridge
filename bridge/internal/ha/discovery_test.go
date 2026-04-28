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
			Media: config.MediaConfig{
				Enabled:             true,
				WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
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
	}, dahua.DeviceState{}, "dahuabridge/devices/west20_nvr/availability", "dahuabridge/devices/west20_nvr/info")

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
			Media: config.MediaConfig{
				Enabled:             true,
				WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
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
			Media: config.MediaConfig{
				Enabled:             true,
				WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
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
	}, dahua.DeviceState{}, "dahuabridge/devices/front_vto/availability", "dahuabridge/devices/front_vto/info")

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

	answer, ok := found["answer"]
	if !ok {
		t.Fatal("missing vto answer button")
	}
	if answer.component != "button" {
		t.Fatalf("unexpected component for answer: %q", answer.component)
	}
	if answer.config.CommandTopic != "dahuabridge/devices/front_vto/command/answer" {
		t.Fatalf("unexpected command topic for answer: %q", answer.config.CommandTopic)
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

	reset, ok := found["intercom_reset"]
	if !ok {
		t.Fatal("missing vto intercom reset button")
	}
	if reset.config.CommandTopic != "dahuabridge/devices/front_vto/command/intercom_reset" {
		t.Fatalf("unexpected command topic for intercom reset: %q", reset.config.CommandTopic)
	}

	enable, ok := found["uplink_enable"]
	if !ok {
		t.Fatal("missing vto uplink enable button")
	}
	if enable.config.CommandTopic != "dahuabridge/devices/front_vto/command/uplink_enable" {
		t.Fatalf("unexpected command topic for uplink enable: %q", enable.config.CommandTopic)
	}

	disable, ok := found["uplink_disable"]
	if !ok {
		t.Fatal("missing vto uplink disable button")
	}
	if disable.config.CommandTopic != "dahuabridge/devices/front_vto/command/uplink_disable" {
		t.Fatalf("unexpected command topic for uplink disable: %q", disable.config.CommandTopic)
	}
}

func TestExtraEntityConfigsForNVRChannelControls(t *testing.T) {
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
		ID:   "west20_nvr_channel_08",
		Name: "Channel 8",
		Kind: dahua.DeviceKindNVRChannel,
	}, dahua.DeviceState{
		Info: map[string]any{
			"control_aux_features":           []string{"siren", "warning_light", "wiper"},
			"control_audio_supported":        false,
			"control_audio_mute_supported":   false,
			"control_audio_volume_supported": false,
			"control_recording_supported":    true,
			"validation_notes_text":          "non_ptz_channel_feature_surface_detected",
		},
	}, "dahuabridge/devices/west20_nvr_channel_08/availability", "dahuabridge/devices/west20_nvr_channel_08/info")

	found := make(map[string]discoveredEntity, len(entities))
	for _, entity := range entities {
		found[entity.objectID] = entity
	}

	expectedButtons := map[string]string{
		"siren":           "dahuabridge/devices/west20_nvr_channel_08/command/siren",
		"warning_light":   "dahuabridge/devices/west20_nvr_channel_08/command/warning_light",
		"wiper":           "dahuabridge/devices/west20_nvr_channel_08/command/wiper",
		"recording_start": "dahuabridge/devices/west20_nvr_channel_08/command/recording_start",
		"recording_stop":  "dahuabridge/devices/west20_nvr_channel_08/command/recording_stop",
	}

	for objectID, topic := range expectedButtons {
		entity, ok := found[objectID]
		if !ok {
			t.Fatalf("missing nvr channel control entity %q", objectID)
		}
		if entity.component != "button" {
			t.Fatalf("unexpected component for %q: %q", objectID, entity.component)
		}
		if entity.config.CommandTopic != topic {
			t.Fatalf("unexpected command topic for %q: %q", objectID, entity.config.CommandTopic)
		}
	}

	recordingActive, ok := found["recording_active"]
	if !ok {
		t.Fatal("missing recording active entity")
	}
	if recordingActive.component != "binary_sensor" {
		t.Fatalf("unexpected component for recording_active: %q", recordingActive.component)
	}
	if !strings.Contains(recordingActive.config.ValueTemplate, "control_recording_active") {
		t.Fatalf("unexpected value template for recording_active: %q", recordingActive.config.ValueTemplate)
	}

	recordingMode, ok := found["recording_mode"]
	if !ok {
		t.Fatal("missing recording mode entity")
	}
	if recordingMode.component != "sensor" {
		t.Fatalf("unexpected component for recording_mode: %q", recordingMode.component)
	}
	if !strings.Contains(recordingMode.config.ValueTemplate, "control_recording_mode") {
		t.Fatalf("unexpected value template for recording_mode: %q", recordingMode.config.ValueTemplate)
	}

	audioDiagnostics := map[string]string{
		"audio_control_supported":        "control_audio_supported",
		"audio_mute_control_supported":   "control_audio_mute_supported",
		"audio_volume_control_supported": "control_audio_volume_supported",
		"audio_playback_supported":       "control_audio_playback_supported",
		"audio_playback_siren_supported": "control_audio_playback_siren",
		"audio_playback_clip_count":      "control_audio_playback_file_count",
	}
	for objectID, templateField := range audioDiagnostics {
		entity, ok := found[objectID]
		if !ok {
			t.Fatalf("missing audio diagnostic entity %q", objectID)
		}
		if objectID == "audio_playback_clip_count" {
			if entity.component != "sensor" {
				t.Fatalf("unexpected component for %q: %q", objectID, entity.component)
			}
		} else if entity.component != "binary_sensor" {
			t.Fatalf("unexpected component for %q: %q", objectID, entity.component)
		}
		if !strings.Contains(entity.config.ValueTemplate, templateField) {
			t.Fatalf("unexpected value template for %q: %q", objectID, entity.config.ValueTemplate)
		}
	}

	validationNotes, ok := found["validation_notes"]
	if !ok {
		t.Fatal("missing validation notes entity")
	}
	if validationNotes.component != "sensor" {
		t.Fatalf("unexpected component for validation_notes: %q", validationNotes.component)
	}
	if !strings.Contains(validationNotes.config.ValueTemplate, "validation_notes_text") {
		t.Fatalf("unexpected value template for validation_notes: %q", validationNotes.config.ValueTemplate)
	}
}

func TestExtraEntityConfigsForVTOSupportDiagnostics(t *testing.T) {
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
	}, dahua.DeviceState{
		Info: map[string]any{
			"control_audio_output_volume_supported":  false,
			"control_audio_input_volume_supported":   false,
			"control_audio_mute_supported":           false,
			"control_recording_supported":            false,
			"control_audio_output_volume":            80,
			"control_audio_input_volume":             90,
			"control_audio_muted":                    false,
			"control_recording_auto_enabled":         true,
			"control_recording_auto_time_seconds":    11,
			"control_stream_audio_enabled":           true,
			"control_direct_talkback_supported":      false,
			"control_full_call_acceptance_supported": false,
			"record_storage_event_snapshot_local":    false,
			"validation_notes_text":                  "vto_audio_control_surface_not_exposed",
		},
	}, "dahuabridge/devices/front_vto/availability", "dahuabridge/devices/front_vto/info")

	found := make(map[string]discoveredEntity, len(entities))
	for _, entity := range entities {
		found[entity.objectID] = entity
	}

	expectedDiagnostics := map[string]string{
		"audio_output_volume_control_supported": "control_audio_output_volume_supported",
		"audio_input_volume_control_supported":  "control_audio_input_volume_supported",
		"mute_control_supported":                "control_audio_mute_supported",
		"recording_control_supported":           "control_recording_supported",
		"output_volume":                         "control_audio_output_volume",
		"input_volume":                          "control_audio_input_volume",
		"muted":                                 "control_audio_muted",
		"auto_record_enabled":                   "control_recording_auto_enabled",
		"auto_record_time_seconds":              "control_recording_auto_time_seconds",
		"stream_audio_enabled":                  "control_stream_audio_enabled",
		"direct_talkback_supported":             "control_direct_talkback_supported",
		"full_call_acceptance_supported":        "control_full_call_acceptance_supported",
		"event_snapshot_local_storage":          "record_storage_event_snapshot_local",
		"validation_notes":                      "validation_notes_text",
	}

	for objectID, templateField := range expectedDiagnostics {
		entity, ok := found[objectID]
		if !ok {
			t.Fatalf("missing vto diagnostic entity %q", objectID)
		}
		if objectID == "validation_notes" || objectID == "output_volume" || objectID == "input_volume" || objectID == "auto_record_time_seconds" {
			if entity.component != "sensor" {
				t.Fatalf("unexpected component for %q: %q", objectID, entity.component)
			}
		} else if entity.component != "binary_sensor" {
			t.Fatalf("unexpected component for %q: %q", objectID, entity.component)
		}
		if !strings.Contains(entity.config.ValueTemplate, templateField) {
			t.Fatalf("unexpected value template for %q: %q", objectID, entity.config.ValueTemplate)
		}
	}
}

func TestExtraEntityConfigsForVTOControlSwitches(t *testing.T) {
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
	}, dahua.DeviceState{
		Info: map[string]any{
			"control_audio_mute_supported":        true,
			"control_recording_supported":         true,
			"control_audio_muted":                 true,
			"control_recording_auto_enabled":      false,
			"control_recording_auto_time_seconds": 11,
		},
	}, "dahuabridge/devices/front_vto/availability", "dahuabridge/devices/front_vto/info")

	found := make(map[string]discoveredEntity, len(entities))
	for _, entity := range entities {
		found[entity.objectID] = entity
	}

	mute, ok := found["mute"]
	if !ok {
		t.Fatal("missing mute switch entity")
	}
	if mute.component != "switch" {
		t.Fatalf("unexpected component for mute: %q", mute.component)
	}
	if mute.config.CommandTopic != "dahuabridge/devices/front_vto/command/mute" {
		t.Fatalf("unexpected mute command topic %q", mute.config.CommandTopic)
	}
	if mute.config.PayloadOn != "ON" || mute.config.PayloadOff != "OFF" {
		t.Fatalf("unexpected mute payloads %+v", mute.config)
	}
	if !strings.Contains(mute.config.ValueTemplate, "control_audio_muted") {
		t.Fatalf("unexpected mute value template %q", mute.config.ValueTemplate)
	}

	autoRecord, ok := found["auto_record"]
	if !ok {
		t.Fatal("missing auto_record switch entity")
	}
	if autoRecord.component != "switch" {
		t.Fatalf("unexpected component for auto_record: %q", autoRecord.component)
	}
	if autoRecord.config.CommandTopic != "dahuabridge/devices/front_vto/command/auto_record" {
		t.Fatalf("unexpected auto_record command topic %q", autoRecord.config.CommandTopic)
	}
	if autoRecord.config.PayloadOn != "ON" || autoRecord.config.PayloadOff != "OFF" {
		t.Fatalf("unexpected auto_record payloads %+v", autoRecord.config)
	}
	if !strings.Contains(autoRecord.config.ValueTemplate, "control_recording_auto_enabled") {
		t.Fatalf("unexpected auto_record value template %q", autoRecord.config.ValueTemplate)
	}
}

func TestExtraEntityConfigsForVTOVolumeControls(t *testing.T) {
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
	}, dahua.DeviceState{
		Info: map[string]any{
			"control_audio_output_volume_supported": true,
			"control_audio_input_volume_supported":  true,
			"control_audio_output_volume":           80,
			"control_audio_input_volume":            65,
		},
	}, "dahuabridge/devices/front_vto/availability", "dahuabridge/devices/front_vto/info")

	found := make(map[string]discoveredEntity, len(entities))
	for _, entity := range entities {
		found[entity.objectID] = entity
	}

	outputVolume, ok := found["output_volume_control"]
	if !ok {
		t.Fatal("missing output_volume_control number entity")
	}
	if outputVolume.component != "number" {
		t.Fatalf("unexpected component for output_volume_control: %q", outputVolume.component)
	}
	if outputVolume.config.CommandTopic != "dahuabridge/devices/front_vto/command/output_volume" {
		t.Fatalf("unexpected output volume command topic %q", outputVolume.config.CommandTopic)
	}
	if outputVolume.config.Min == nil || *outputVolume.config.Min != 0 || outputVolume.config.Max == nil || *outputVolume.config.Max != 100 || outputVolume.config.Step == nil || *outputVolume.config.Step != 1 {
		t.Fatalf("unexpected output volume bounds %+v", outputVolume.config)
	}
	if outputVolume.config.Mode != "slider" {
		t.Fatalf("unexpected output volume mode %q", outputVolume.config.Mode)
	}
	if !strings.Contains(outputVolume.config.ValueTemplate, "control_audio_output_volume") {
		t.Fatalf("unexpected output volume template %q", outputVolume.config.ValueTemplate)
	}

	inputVolume, ok := found["input_volume_control"]
	if !ok {
		t.Fatal("missing input_volume_control number entity")
	}
	if inputVolume.component != "number" {
		t.Fatalf("unexpected component for input_volume_control: %q", inputVolume.component)
	}
	if inputVolume.config.CommandTopic != "dahuabridge/devices/front_vto/command/input_volume" {
		t.Fatalf("unexpected input volume command topic %q", inputVolume.config.CommandTopic)
	}
	if inputVolume.config.Min == nil || *inputVolume.config.Min != 0 || inputVolume.config.Max == nil || *inputVolume.config.Max != 100 || inputVolume.config.Step == nil || *inputVolume.config.Step != 1 {
		t.Fatalf("unexpected input volume bounds %+v", inputVolume.config)
	}
	if inputVolume.config.Mode != "slider" {
		t.Fatalf("unexpected input volume mode %q", inputVolume.config.Mode)
	}
	if !strings.Contains(inputVolume.config.ValueTemplate, "control_audio_input_volume") {
		t.Fatalf("unexpected input volume template %q", inputVolume.config.ValueTemplate)
	}
}

func TestPublishProbeInNativeEntityModeSkipsHomeAssistantDiscovery(t *testing.T) {
	mqttClient := &mockMQTTClient{}
	publisher := NewDiscoveryPublisher(
		config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix:     "dahuabridge",
				DiscoveryPrefix: "homeassistant",
				QoS:             1,
				Retain:          true,
			},
			HomeAssistant: config.HomeAssistantConfig{
				Enabled:    true,
				NodeID:     "dahuabridge",
				EntityMode: "native",
			},
		},
		mqttClient,
		zerolog.Nop(),
	)

	result := &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "yard_ipc",
			Name: "Yard IPC",
			Kind: dahua.DeviceKindIPC,
		},
		States: map[string]dahua.DeviceState{
			"yard_ipc": {
				Available: true,
				Info: map[string]any{
					"motion": true,
				},
			},
		},
	}

	if err := publisher.PublishProbe(context.Background(), result); err != nil {
		t.Fatalf("PublishProbe returned error: %v", err)
	}

	for _, published := range mqttClient.published {
		if strings.HasPrefix(published.topic, "homeassistant/") {
			t.Fatalf("expected no home assistant discovery publish in native mode, got topic %q", published.topic)
		}
	}

	if !hasPublishedMessage(mqttClient.published, "dahuabridge/devices/yard_ipc/availability", "online") {
		t.Fatalf("expected availability publish, got %+v", mqttClient.published)
	}
	if !hasPublishedMessage(mqttClient.published, "dahuabridge/devices/yard_ipc/info", `"name":"Yard IPC"`) {
		t.Fatalf("expected info publish, got %+v", mqttClient.published)
	}
}

func TestRemoveProbeDiscoveryPublishesEmptyPayloads(t *testing.T) {
	mqttClient := &mockMQTTClient{}
	publisher := NewDiscoveryPublisher(
		config.Config{
			MQTT: config.MQTTConfig{
				TopicPrefix:     "dahuabridge",
				DiscoveryPrefix: "homeassistant",
				QoS:             1,
				Retain:          true,
			},
			HomeAssistant: config.HomeAssistantConfig{
				Enabled:    true,
				NodeID:     "dahuabridge",
				EntityMode: "hybrid",
			},
			Media: config.MediaConfig{
				Enabled: true,
			},
		},
		mqttClient,
		zerolog.Nop(),
	)

	result := &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "yard_ipc",
			Name: "Yard IPC",
			Kind: dahua.DeviceKindIPC,
		},
		States: map[string]dahua.DeviceState{
			"yard_ipc": {Available: true},
		},
	}

	cleanup, err := publisher.RemoveProbeDiscovery(context.Background(), result)
	if err != nil {
		t.Fatalf("RemoveProbeDiscovery returned error: %v", err)
	}
	if cleanup.DeviceCount != 1 {
		t.Fatalf("expected 1 device in cleanup result, got %+v", cleanup)
	}
	if cleanup.RemovedTopics == 0 {
		t.Fatalf("expected at least one removed topic, got %+v", cleanup)
	}

	var clearedOnline bool
	var clearedCamera bool
	for _, published := range mqttClient.published {
		if published.topic == "homeassistant/binary_sensor/dahuabridge/yard_ipc_online/config" && len(published.payload) == 0 {
			clearedOnline = true
		}
		if published.topic == "homeassistant/camera/dahuabridge/yard_ipc_camera/config" && len(published.payload) == 0 {
			clearedCamera = true
		}
	}
	if !clearedOnline {
		t.Fatalf("expected online discovery topic to be cleared, got %+v", mqttClient.published)
	}
	if !clearedCamera {
		t.Fatalf("expected camera discovery topic to be cleared, got %+v", mqttClient.published)
	}
}

func hasPublishedMessage(messages []publishedMessage, topic string, payloadSubstring string) bool {
	for _, message := range messages {
		if message.topic != topic {
			continue
		}
		if strings.Contains(string(message.payload), payloadSubstring) {
			return true
		}
	}
	return false
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
