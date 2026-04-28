package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/ha"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/mqtt"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

func TestSnapshotTargetsForProbeResult(t *testing.T) {
	tests := []struct {
		name   string
		result *dahua.ProbeResult
		want   []cameraSnapshotTarget
	}{
		{
			name: "nvr channels",
			result: &dahua.ProbeResult{
				Root: dahua.Device{
					ID:   "west20_nvr",
					Kind: dahua.DeviceKindNVR,
				},
				Children: []dahua.Device{
					{
						ID:   "west20_nvr_channel_01",
						Kind: dahua.DeviceKindNVRChannel,
						Attributes: map[string]string{
							"channel_index": "1",
						},
					},
					{
						ID:   "west20_nvr_disk_00",
						Kind: dahua.DeviceKindNVRDisk,
					},
				},
			},
			want: []cameraSnapshotTarget{
				{deviceID: "west20_nvr_channel_01", channel: 1},
			},
		},
		{
			name: "vto root",
			result: &dahua.ProbeResult{
				Root: dahua.Device{
					ID:   "front_vto",
					Kind: dahua.DeviceKindVTO,
				},
			},
			want: []cameraSnapshotTarget{
				{deviceID: "front_vto", channel: 0},
			},
		},
		{
			name: "ipc root",
			result: &dahua.ProbeResult{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Kind: dahua.DeviceKindIPC,
				},
			},
			want: []cameraSnapshotTarget{
				{deviceID: "yard_ipc", channel: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapshotTargetsForProbeResult(tt.result)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("snapshotTargetsForProbeResult() mismatch:\nwant: %#v\ngot:  %#v", tt.want, got)
			}
		})
	}
}

type stubRuntimeMedia struct {
	status map[string]media.IntercomStatus
}

type stubSnapshotProvider struct {
	body        []byte
	contentType string
	calls       int
	lastChannel int
	err         error
}

func (s *stubSnapshotProvider) Snapshot(_ context.Context, channel int) ([]byte, string, error) {
	s.calls++
	s.lastChannel = channel
	if s.err != nil {
		return nil, "", s.err
	}
	return append([]byte(nil), s.body...), s.contentType, nil
}

type stubSnapshotDriver struct {
	*stubSnapshotProvider
}

func (d *stubSnapshotDriver) ID() string { return "stub" }

func (d *stubSnapshotDriver) Kind() dahua.DeviceKind { return dahua.DeviceKindIPC }

func (d *stubSnapshotDriver) PollInterval() time.Duration { return 0 }

func (d *stubSnapshotDriver) Probe(context.Context) (*dahua.ProbeResult, error) { return nil, nil }

type appMockMQTTClient struct {
	published []appPublishedMessage
}

type appPublishedMessage struct {
	topic   string
	payload []byte
}

func (m *appMockMQTTClient) Connect(context.Context) error { return nil }
func (m *appMockMQTTClient) Subscribe(context.Context, string, byte, func(string, []byte)) error {
	return nil
}
func (m *appMockMQTTClient) Close() {}
func (m *appMockMQTTClient) Publish(_ context.Context, topic string, _ byte, _ bool, payload []byte) error {
	cloned := append([]byte(nil), payload...)
	m.published = append(m.published, appPublishedMessage{topic: topic, payload: cloned})
	return nil
}
func (m *appMockMQTTClient) PublishJSON(context.Context, string, byte, bool, any) error { return nil }

var _ mqtt.Client = (*appMockMQTTClient)(nil)

func (s stubRuntimeMedia) IntercomStatus(streamID string) media.IntercomStatus {
	if s.status != nil {
		if status, ok := s.status[streamID]; ok {
			return status
		}
	}
	return media.IntercomStatus{StreamID: streamID}
}

func TestRuntimeServicesListStreamsIncludesIntercomRuntimeStatus(t *testing.T) {
	probes := store.NewProbeStore()
	probes.Set("front_vto", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "front_vto",
			Name: "Front Door",
			Kind: dahua.DeviceKindVTO,
			Attributes: map[string]string{
				"lock_count": "1",
			},
		},
		States: map[string]dahua.DeviceState{
			"front_vto": {
				Available: true,
				Info: map[string]any{
					"audio_codec": "PCM",
					"call_state":  "ringing",
				},
			},
		},
	})

	services := newRuntimeServices(config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
		Media: config.MediaConfig{
			WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
		},
	}, probes)
	services.RegisterVTO("front_vto", nil, config.DeviceConfig{
		ID:       "front_vto",
		BaseURL:  "http://vto.example.local",
		Username: "admin",
		Password: "secret",
	})
	services.AttachMedia(stubRuntimeMedia{
		status: map[string]media.IntercomStatus{
			"front_vto": {
				StreamID:               "front_vto",
				Active:                 true,
				SessionCount:           1,
				ExternalUplinkEnabled:  true,
				UplinkActive:           true,
				UplinkCodec:            "audio/opus",
				UplinkPackets:          8,
				UplinkForwardedPackets: 6,
				UplinkForwardErrors:    1,
			},
		},
	})

	entries := services.ListStreams(false)
	if len(entries) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(entries))
	}
	if entries[0].Intercom == nil {
		t.Fatal("expected intercom summary")
	}
	if !entries[0].Intercom.BridgeSessionActive || entries[0].Intercom.BridgeSessionCount != 1 {
		t.Fatalf("expected bridge runtime session state, got %+v", entries[0].Intercom)
	}
	if !entries[0].Intercom.ExternalUplinkEnabled || entries[0].Intercom.BridgeUplinkCodec != "audio/opus" {
		t.Fatalf("expected bridge runtime uplink state, got %+v", entries[0].Intercom)
	}
}

func TestRuntimeServicesNVRSnapshotCachesRecentResponses(t *testing.T) {
	probes := store.NewProbeStore()
	services := newRuntimeServices(config.Config{}, probes)
	provider := &stubSnapshotProvider{
		body:        []byte("jpeg-bytes"),
		contentType: "image/jpeg",
	}
	services.RegisterNVR("west20_nvr", provider, config.DeviceConfig{ID: "west20_nvr"})

	body1, contentType1, err := services.NVRSnapshot(context.Background(), "west20_nvr", 2)
	if err != nil {
		t.Fatalf("first NVRSnapshot returned error: %v", err)
	}
	body2, contentType2, err := services.NVRSnapshot(context.Background(), "west20_nvr", 2)
	if err != nil {
		t.Fatalf("second NVRSnapshot returned error: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("expected provider to be called once, got %d", provider.calls)
	}
	if provider.lastChannel != 2 {
		t.Fatalf("expected provider channel 2, got %d", provider.lastChannel)
	}
	if string(body1) != "jpeg-bytes" || string(body2) != "jpeg-bytes" {
		t.Fatalf("unexpected cached snapshot bodies: %q / %q", string(body1), string(body2))
	}
	if contentType1 != "image/jpeg" || contentType2 != "image/jpeg" {
		t.Fatalf("unexpected content types: %q / %q", contentType1, contentType2)
	}
}

func TestPublishProbeCameraSnapshotsUsesLogoWithoutFetchingDeviceSnapshot(t *testing.T) {
	mqttClient := &appMockMQTTClient{}
	discovery := ha.NewDiscoveryPublisher(config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix: "dahuabridge",
			QoS:         1,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled:              true,
			NodeID:               "dahuabridge",
			CameraSnapshotSource: "logo",
		},
	}, mqttClient, zerolog.Nop())

	provider := &stubSnapshotDriver{stubSnapshotProvider: &stubSnapshotProvider{err: errors.New("snapshot should not be called")}}
	result := &dahua.ProbeResult{
		Root: dahua.Device{ID: "yard_ipc", Kind: dahua.DeviceKindIPC},
	}

	publishProbeCameraSnapshots(zerolog.Nop(), discovery, provider, result)

	if provider.calls != 0 {
		t.Fatalf("expected no snapshot provider calls, got %d", provider.calls)
	}
	if len(mqttClient.published) != 1 {
		t.Fatalf("expected one placeholder snapshot publish, got %+v", mqttClient.published)
	}
	if mqttClient.published[0].topic != "dahuabridge/devices/yard_ipc/camera/snapshot" {
		t.Fatalf("unexpected camera snapshot topic %q", mqttClient.published[0].topic)
	}
	if len(mqttClient.published[0].payload) == 0 {
		t.Fatal("expected non-empty placeholder snapshot payload")
	}
}
