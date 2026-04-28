package streams

import (
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

func TestBuildCatalogForNVRChannel(t *testing.T) {
	catalog := BuildCatalog(CatalogInput{
		Config: config.Config{
			HomeAssistant: config.HomeAssistantConfig{
				PublicBaseURL: "http://bridge.local:8080",
			},
			Media: config.MediaConfig{
				WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
			},
		},
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "west20_nvr",
					Kind: dahua.DeviceKindNVR,
				},
				Children: []dahua.Device{
					{
						ID:       "west20_nvr_channel_01",
						ParentID: "west20_nvr",
						Kind:     dahua.DeviceKindNVRChannel,
						Name:     "Front Gate",
						Attributes: map[string]string{
							"channel_index":   "1",
							"main_codec":      "H.265",
							"main_resolution": "3840x2160",
							"sub_codec":       "H.264",
							"sub_resolution":  "704x576",
						},
					},
				},
			},
		},
		NVRConfigs: map[string]config.DeviceConfig{
			"west20_nvr": {
				BaseURL:  "http://nvr.example.local",
				Username: "admin",
				Password: "secret",
			},
		},
	})

	if len(catalog) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(catalog))
	}

	entry := catalog[0]
	if entry.RecommendedProfile != "stable" {
		t.Fatalf("expected stable recommendation, got %q", entry.RecommendedProfile)
	}
	if entry.Channel != 1 {
		t.Fatalf("expected channel 1, got %d", entry.Channel)
	}
	if !strings.Contains(entry.Profiles["stable"].StreamURL, "subtype=1") {
		t.Fatalf("expected stable profile to use subtype=1, got %q", entry.Profiles["stable"].StreamURL)
	}
	if entry.Profiles["stable"].LocalHLSURL != "http://bridge.local:8080/api/v1/media/hls/west20_nvr_channel_01/stable/index.m3u8" {
		t.Fatalf("unexpected stable hls url %q", entry.Profiles["stable"].LocalHLSURL)
	}
	if entry.SnapshotURL != "http://bridge.local:8080/api/v1/nvr/west20_nvr/channels/1/snapshot" {
		t.Fatalf("unexpected snapshot url %q", entry.SnapshotURL)
	}
	if entry.LocalPreviewURL != "http://bridge.local:8080/api/v1/media/preview/west20_nvr_channel_01?profile=stable" {
		t.Fatalf("unexpected preview url %q", entry.LocalPreviewURL)
	}
	if entry.Profiles["stable"].LocalWebRTCURL != "http://bridge.local:8080/api/v1/media/webrtc/west20_nvr_channel_01/stable" {
		t.Fatalf("unexpected webrtc url %q", entry.Profiles["stable"].LocalWebRTCURL)
	}
}

func TestBuildCatalogIncludesCredentialsWhenRequested(t *testing.T) {
	catalog := BuildCatalog(CatalogInput{
		IncludeCredentials: true,
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard",
					Kind: dahua.DeviceKindIPC,
				},
			},
		},
		IPCConfigs: map[string]config.DeviceConfig{
			"yard_ipc": {
				BaseURL:  "https://ipc20.example.local",
				Username: "admin",
				Password: "secret#1",
			},
		},
	})

	if len(catalog) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(catalog))
	}

	if !strings.Contains(catalog[0].Profiles["quality"].StreamURL, "admin:secret%231@") {
		t.Fatalf("expected credentials in quality stream url, got %q", catalog[0].Profiles["quality"].StreamURL)
	}
	if !strings.Contains(catalog[0].Profiles["quality"].StreamURL, ":554/") {
		t.Fatalf("expected default rtsp port 554, got %q", catalog[0].Profiles["quality"].StreamURL)
	}
}

func TestRecommendProfilePrefersQualityForH264MainStream(t *testing.T) {
	recommended := recommendProfile("H.264", "1920x1080", "H.264", "704x576")
	if recommended != "quality" {
		t.Fatalf("expected quality recommendation, got %q", recommended)
	}
}

func TestRecommendProfilePrefersStableForDahuaH265Variants(t *testing.T) {
	testCases := []string{
		"H.265",
		"H.265H",
		"Smart H.265+",
		"HEVC",
	}

	for _, codec := range testCases {
		if got := recommendProfile(codec, "1920x1080", "H.264", "704x576"); got != "stable" {
			t.Fatalf("expected stable recommendation for %q, got %q", codec, got)
		}
	}
}

func TestCodecFamilyNormalizesDahuaCodecNames(t *testing.T) {
	testCases := map[string]string{
		"H.264":        "h264",
		"H.264B":       "h264",
		"H.264H":       "h264",
		"H.264M":       "h264",
		"Smart H.264+": "h264",
		"H.265":        "h265",
		"H.265H":       "h265",
		"Smart H.265+": "h265",
		"HEVC":         "h265",
		"MJPEG":        "mjpeg",
		"MJPG":         "mjpeg",
		"MPEG4":        "mpeg4",
		"SVAC":         "svac",
	}

	for input, want := range testCases {
		if got := codecFamily(input); got != want {
			t.Fatalf("codecFamily(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildCatalogAppliesONVIFRecommendation(t *testing.T) {
	catalog := BuildCatalog(CatalogInput{
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard",
					Kind: dahua.DeviceKindIPC,
				},
				States: map[string]dahua.DeviceState{
					"yard_ipc": {
						Available: true,
						Info: map[string]any{
							"onvif_h264_available":       true,
							"onvif_profile_token":        "Profile_1",
							"onvif_profile_name":         "MainStream-H264",
							"onvif_stream_url":           "rtsp://ipc120.example.local:554/onvif1",
							"onvif_snapshot_url":         "http://ipc120.example.local/onvif/snapshot1.jpg",
							"recommended_ha_integration": "onvif",
						},
					},
				},
			},
		},
		IPCConfigs: map[string]config.DeviceConfig{
			"yard_ipc": {
				BaseURL:  "http://ipc120.example.local",
				Username: "admin",
				Password: "secret",
			},
		},
	})

	if len(catalog) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(catalog))
	}
	entry := catalog[0]
	if entry.RecommendedHAIntegration != "onvif" {
		t.Fatalf("expected onvif recommendation, got %q", entry.RecommendedHAIntegration)
	}
	if entry.ONVIFProfileToken != "Profile_1" {
		t.Fatalf("unexpected onvif token %q", entry.ONVIFProfileToken)
	}
	if entry.ONVIFSnapshotURL != "http://ipc120.example.local/onvif/snapshot1.jpg" {
		t.Fatalf("unexpected onvif snapshot url %q", entry.ONVIFSnapshotURL)
	}
	if !entry.ONVIFH264Available {
		t.Fatal("expected onvif h264 availability")
	}
}

func TestBuildCatalogForVTOIncludesLockCount(t *testing.T) {
	catalog := BuildCatalog(CatalogInput{
		Config: config.Config{
			HomeAssistant: config.HomeAssistantConfig{
				PublicBaseURL: "http://bridge.local:8080",
			},
			Media: config.MediaConfig{
				WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
			},
		},
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "front_vto",
					Name: "Front Door",
					Kind: dahua.DeviceKindVTO,
					Attributes: map[string]string{
						"lock_count": "2",
					},
				},
				States: map[string]dahua.DeviceState{
					"front_vto": {
						Available: true,
						Info: map[string]any{
							"audio_codec":                "PCM",
							"call_state":                 "ringing",
							"last_ring_at":               "2026-04-27T18:45:00Z",
							"last_call_started_at":       "2026-04-27T18:45:03Z",
							"last_call_ended_at":         "2026-04-27T18:45:21Z",
							"last_call_duration_seconds": 18,
							"last_call_source":           "villa_panel",
						},
					},
				},
			},
		},
		VTOConfigs: map[string]config.DeviceConfig{
			"front_vto": {
				BaseURL:  "http://vto.example.local",
				Username: "admin",
				Password: "secret",
			},
		},
		IntercomStatuses: map[string]RuntimeIntercomStatus{
			"front_vto": {
				Active:                 true,
				SessionCount:           2,
				ExternalUplinkEnabled:  true,
				UplinkActive:           true,
				UplinkCodec:            "audio/opus",
				UplinkPackets:          33,
				UplinkForwardedPackets: 30,
				UplinkForwardErrors:    1,
			},
		},
	})

	if len(catalog) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(catalog))
	}
	if catalog[0].LockCount != 2 {
		t.Fatalf("expected lock count 2, got %d", catalog[0].LockCount)
	}
	if catalog[0].LocalIntercomURL != "http://bridge.local:8080/api/v1/vto/front_vto/intercom?profile=quality" {
		t.Fatalf("unexpected intercom url %q", catalog[0].LocalIntercomURL)
	}
	if catalog[0].Intercom == nil {
		t.Fatal("expected intercom summary")
	}
	if catalog[0].Intercom.CallState != "ringing" {
		t.Fatalf("unexpected call state %q", catalog[0].Intercom.CallState)
	}
	if catalog[0].Intercom.LastCallDurationSeconds != 18 {
		t.Fatalf("unexpected call duration %d", catalog[0].Intercom.LastCallDurationSeconds)
	}
	if catalog[0].Intercom.AnswerURL != "http://bridge.local:8080/api/v1/vto/front_vto/call/answer" {
		t.Fatalf("unexpected answer url %q", catalog[0].Intercom.AnswerURL)
	}
	if catalog[0].Intercom.HangupURL != "http://bridge.local:8080/api/v1/vto/front_vto/call/hangup" {
		t.Fatalf("unexpected hangup url %q", catalog[0].Intercom.HangupURL)
	}
	if catalog[0].Intercom.ExternalUplinkEnableURL != "http://bridge.local:8080/api/v1/vto/front_vto/intercom/uplink/enable" {
		t.Fatalf("unexpected uplink enable url %q", catalog[0].Intercom.ExternalUplinkEnableURL)
	}
	if catalog[0].Intercom.BridgeSessionResetURL != "http://bridge.local:8080/api/v1/vto/front_vto/intercom/reset" {
		t.Fatalf("unexpected bridge session reset url %q", catalog[0].Intercom.BridgeSessionResetURL)
	}
	if catalog[0].Intercom.ExternalUplinkDisableURL != "http://bridge.local:8080/api/v1/vto/front_vto/intercom/uplink/disable" {
		t.Fatalf("unexpected uplink disable url %q", catalog[0].Intercom.ExternalUplinkDisableURL)
	}
	if len(catalog[0].Intercom.LockURLs) != 2 {
		t.Fatalf("expected 2 lock urls, got %d", len(catalog[0].Intercom.LockURLs))
	}
	if catalog[0].Intercom.LockURLs[1] != "http://bridge.local:8080/api/v1/vto/front_vto/locks/1/unlock" {
		t.Fatalf("unexpected second lock url %q", catalog[0].Intercom.LockURLs[1])
	}
	if !catalog[0].Intercom.SupportsHangup || !catalog[0].Intercom.SupportsBridgeSessionReset || !catalog[0].Intercom.SupportsUnlock || !catalog[0].Intercom.SupportsBrowserMicrophone {
		t.Fatalf("expected supported intercom actions, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.SupportsBridgeAudioOutput || !catalog[0].Intercom.SupportsBridgeAudioUplink {
		t.Fatalf("expected bridge audio capabilities, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.SupportsExternalAudioExport || catalog[0].Intercom.ConfiguredExternalUplinkTargetCount != 1 {
		t.Fatalf("expected configured external uplink export, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.BridgeSessionActive || catalog[0].Intercom.BridgeSessionCount != 2 || !catalog[0].Intercom.ExternalUplinkEnabled {
		t.Fatalf("expected bridge intercom runtime state, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.BridgeUplinkActive || catalog[0].Intercom.BridgeUplinkCodec != "audio/opus" || catalog[0].Intercom.BridgeForwardedPackets != 30 {
		t.Fatalf("expected bridge uplink runtime metrics, got %+v", catalog[0].Intercom)
	}
	if !catalog[0].Intercom.SupportsVTOCallAnswer {
		t.Fatalf("expected VTO call answer support, got %+v", catalog[0].Intercom)
	}
	if catalog[0].Intercom.SupportsVTOTalkback || catalog[0].Intercom.SupportsFullCallAcceptance {
		t.Fatalf("expected talkback and full acceptance to remain unsupported, got %+v", catalog[0].Intercom)
	}
}
