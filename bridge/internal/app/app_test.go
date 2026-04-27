package app

import (
	"reflect"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"
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
