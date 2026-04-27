package ha

import (
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

func TestBuildNativeCatalogMergesDevicesStatesAndStreams(t *testing.T) {
	catalog := BuildNativeCatalog([]*dahua.ProbeResult{
		{
			Root: dahua.Device{
				ID:   "west20_nvr",
				Name: "West 20 NVR",
				Kind: dahua.DeviceKindNVR,
			},
			Children: []dahua.Device{
				{
					ID:       "west20_nvr_channel_01",
					ParentID: "west20_nvr",
					Name:     "West 20 Channel 01",
					Kind:     dahua.DeviceKindNVRChannel,
				},
			},
			States: map[string]dahua.DeviceState{
				"west20_nvr": {
					Available: true,
					Info: map[string]any{
						"channel_count": 32,
					},
				},
				"west20_nvr_channel_01": {
					Available: true,
					Info: map[string]any{
						"motion": true,
					},
				},
			},
		},
	}, []streams.Entry{
		{
			ID:           "west20_nvr_channel_01",
			RootDeviceID: "west20_nvr",
			DeviceKind:   dahua.DeviceKindNVRChannel,
			Name:         "West 20 Channel 01",
			SnapshotURL:  "http://bridge.local/api/v1/nvr/west20_nvr/channels/1/snapshot",
		},
		{
			ID:           "front_vto",
			RootDeviceID: "front_vto",
			DeviceKind:   dahua.DeviceKindVTO,
			Name:         "Front VTO",
			Intercom: &streams.IntercomSummary{
				CallState:               "ringing",
				LastCallSource:          "villa_panel",
				BridgeSessionActive:     true,
				BridgeSessionCount:      2,
				ExternalUplinkEnabled:   true,
				BridgeUplinkActive:      true,
				BridgeUplinkCodec:       "audio/opus",
				BridgeForwardedPackets:  30,
				BridgeForwardErrors:     1,
				LastCallDurationSeconds: 11,
			},
		},
	})

	if len(catalog.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(catalog.Devices))
	}

	if catalog.Devices[0].Device.ID != "west20_nvr" {
		t.Fatalf("expected first device to be the root nvr, got %q", catalog.Devices[0].Device.ID)
	}
	if catalog.Devices[0].Stream != nil {
		t.Fatal("expected root nvr to have no stream metadata")
	}

	channel := catalog.Devices[1]
	if channel.Device.ID != "west20_nvr_channel_01" {
		t.Fatalf("expected second device to be channel 01, got %q", channel.Device.ID)
	}
	if channel.Stream == nil {
		t.Fatal("expected nvr channel to include stream metadata")
	}
	if channel.Stream.ID != "west20_nvr_channel_01" {
		t.Fatalf("expected matching stream id, got %q", channel.Stream.ID)
	}
	if value, ok := channel.State.Info["motion"].(bool); !ok || !value {
		t.Fatalf("expected motion state to be preserved, got %#v", channel.State.Info["motion"])
	}
}

func TestBuildNativeCatalogFlattensIntercomSummaryIntoStateInfo(t *testing.T) {
	catalog := BuildNativeCatalog([]*dahua.ProbeResult{
		{
			Root: dahua.Device{
				ID:   "front_vto",
				Name: "Front VTO",
				Kind: dahua.DeviceKindVTO,
			},
			States: map[string]dahua.DeviceState{
				"front_vto": {
					Available: true,
				},
			},
		},
	}, []streams.Entry{
		{
			ID:           "front_vto",
			RootDeviceID: "front_vto",
			DeviceKind:   dahua.DeviceKindVTO,
			Name:         "Front VTO",
			Intercom: &streams.IntercomSummary{
				CallState:               "ringing",
				LastCallSource:          "villa_panel",
				LastCallDurationSeconds: 11,
				BridgeSessionActive:     true,
				BridgeSessionCount:      2,
				ExternalUplinkEnabled:   true,
				BridgeUplinkActive:      true,
				BridgeUplinkCodec:       "audio/opus",
				BridgeForwardedPackets:  30,
				BridgeForwardErrors:     1,
			},
		},
	})

	if len(catalog.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(catalog.Devices))
	}

	info := catalog.Devices[0].State.Info
	if info["call_state"] != "ringing" {
		t.Fatalf("expected call_state to be flattened, got %#v", info["call_state"])
	}
	if info["bridge_session_active"] != true {
		t.Fatalf("expected bridge_session_active to be true, got %#v", info["bridge_session_active"])
	}
	if info["external_uplink_enabled"] != true {
		t.Fatalf("expected external_uplink_enabled to be true, got %#v", info["external_uplink_enabled"])
	}
	if info["bridge_uplink_codec"] != "audio/opus" {
		t.Fatalf("expected bridge_uplink_codec to be preserved, got %#v", info["bridge_uplink_codec"])
	}
	if info["bridge_forwarded_packets"] != uint64(30) {
		t.Fatalf("expected bridge_forwarded_packets to be preserved, got %#v", info["bridge_forwarded_packets"])
	}
}
