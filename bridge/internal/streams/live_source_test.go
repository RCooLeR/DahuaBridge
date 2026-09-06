package streams

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

func TestDualLensCameraUsesExplicitInputsAndKeepsRecorderChannels(t *testing.T) {
	input := directSourceCatalogInput()
	first := input.ProbeResults[0].Children[0]
	second := first
	second.ID = "nvr_channel_06"
	second.Attributes = map[string]string{"channel_index": "6", "sub_codec": "H264", "direct_ipc_rtsp_port": "554"}
	input.ProbeResults[0].Children = append(input.ProbeResults[0].Children, second)
	cfg := input.NVRConfigs["nvr"]
	cfg.DirectIPCCredentials[0].DirectIPCChannel = 1
	secondCredential := cfg.DirectIPCCredentials[0]
	secondCredential.NVRChannel = 6
	secondCredential.DirectIPCChannel = 2
	cfg.DirectIPCCredentials = append(cfg.DirectIPCCredentials, secondCredential)
	input.NVRConfigs["nvr"] = cfg
	input.LiveSources = map[string]string{"nvr_channel_05": LiveSourceCamera, "nvr_channel_06": LiveSourceCamera}
	input.IncludeCredentials = true
	for _, entry := range BuildCatalog(input) {
		for _, profile := range entry.Profiles {
			live, err := url.Parse(profile.StreamURL)
			if err != nil {
				t.Fatal(err)
			}
			recorder, err := url.Parse(profile.RecorderStreamURL)
			if err != nil {
				t.Fatal(err)
			}
			if live.Hostname() != "camera.local" || live.User.Username() != "camera-user" || live.Query().Get("channel") != strconv.Itoa(entry.Channel-4) {
				t.Fatalf("NVR channel %d has incorrect direct camera input", entry.Channel)
			}
			if recorder.Hostname() != "nvr.local" || recorder.Query().Get("channel") != strconv.Itoa(entry.Channel) {
				t.Fatal("direct input changed the archive channel")
			}
		}
	}
}

func TestGlobalLiveSourceInheritanceAndOverrides(t *testing.T) {
	for _, item := range []struct {
		name, global, override, wantSource, wantDefault string
		missingCredentials                              bool
	}{
		{"initial default", "", "", "nvr", "nvr", false},
		{"inherit camera", "camera", "", "camera", "camera", false},
		{"override nvr", "camera", "nvr", "nvr", "camera", false},
		{"override camera", "nvr", "camera", "camera", "nvr", false},
		{"unavailable inherited camera", "camera", "", "nvr", "camera", true},
		{"unavailable explicit camera", "nvr", "camera", "nvr", "nvr", true},
	} {
		t.Run(item.name, func(t *testing.T) {
			input := directSourceCatalogInput()
			input.DefaultLiveSource = item.global
			input.LiveSources = map[string]string{"nvr_channel_05": item.override}
			if item.missingCredentials {
				cfg := input.NVRConfigs["nvr"]
				cfg.DirectIPCCredentials = nil
				input.NVRConfigs["nvr"] = cfg
			}
			entry := BuildCatalog(input)[0]
			if entry.LiveSource.Source != item.wantSource || entry.LiveSource.DefaultSource != item.wantDefault || entry.LiveSource.OverrideSource != item.override {
				t.Fatalf("unexpected effective source: %+v", entry.LiveSource)
			}
			if item.missingCredentials && entry.LiveSource.CameraUnavailableReason == "" {
				t.Fatal("unavailable camera reason missing")
			}
			if item.wantSource == "nvr" && !strings.Contains(entry.Profiles["quality"].StreamURL, "nvr.local:554") {
				t.Fatal("NVR fallback was not preserved")
			}
			body, err := json.Marshal(entry.LiveSource)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `"default_source":`) || !strings.Contains(string(body), `"override_source":`) {
				t.Fatalf("inheritance metadata must always be present: %s", body)
			}
		})
	}
}

func directSourceCatalogInput() CatalogInput {
	return CatalogInput{
		Config: config.Config{HomeAssistant: config.HomeAssistantConfig{PublicBaseURL: "http://bridge.local"}},
		ProbeResults: []*dahua.ProbeResult{{
			Root: dahua.Device{ID: "nvr", Kind: dahua.DeviceKindNVR},
			Children: []dahua.Device{{ID: "nvr_channel_05", Kind: dahua.DeviceKindNVRChannel, Name: "Gate", Attributes: map[string]string{
				"channel_index": "5", "sub_codec": "H264", "direct_ipc_rtsp_port": "8554",
			}}},
			States: map[string]dahua.DeviceState{"nvr_channel_05": {Info: map[string]any{
				"onvif_h264_available": true, "onvif_stream_url": "rtsp://nvr.local:554/onvif",
				"onvif_snapshot_url": "http://nvr.local/snapshot", "recommended_ha_integration": "onvif",
			}}},
		}},
		NVRConfigs: map[string]config.DeviceConfig{"nvr": {
			BaseURL: "http://nvr.local", Username: "nvr-user", Password: "nvr-pass",
			DirectIPCCredentials: []config.ChannelDirectIPCCredential{{
				NVRChannel: 5, DirectIPCIP: "http://camera.local:8080", DirectIPCBaseURL: "https://camera.local:8443",
				DirectIPCUser: "camera-user", DirectIPCPassword: "camera:p@ss",
			}},
		}},
	}
}

func TestNVRLiveSourceKeepsRecorderIdentityAndArchiveAddress(t *testing.T) {
	input := directSourceCatalogInput()
	defaultEntry := BuildCatalog(input)[0]
	if defaultEntry.LiveSource.Source != LiveSourceNVR || !defaultEntry.LiveSource.CameraAvailable {
		t.Fatalf("unexpected default source: %+v", defaultEntry.LiveSource)
	}
	input.LiveSources = map[string]string{"nvr_channel_05": LiveSourceCamera}
	for _, credentials := range []bool{false, true} {
		input.IncludeCredentials = credentials
		entry := BuildCatalog(input)[0]
		if entry.ID != defaultEntry.ID || entry.Channel != 5 || entry.RootDeviceID != "nvr" || entry.DeviceKind != dahua.DeviceKindNVRChannel || entry.SnapshotURL != defaultEntry.SnapshotURL {
			t.Fatalf("camera selection changed recorder identity: %+v", entry)
		}
		if entry.ONVIFStreamURL != "" || entry.ONVIFSnapshotURL != "" || entry.ONVIFH264Available || entry.RecommendedHAIntegration != "bridge_media" {
			t.Fatal("recorder ONVIF profile remained active after direct selection")
		}
		for name, profile := range entry.Profiles {
			live, err := url.Parse(profile.StreamURL)
			if err != nil {
				t.Fatal(err)
			}
			recorder, err := url.Parse(profile.RecorderStreamURL)
			if err != nil {
				t.Fatal(err)
			}
			if live.Host != "camera.local:8554" || live.Query().Get("channel") != "1" || recorder.Host != "nvr.local:554" || recorder.Query().Get("channel") != "5" {
				t.Fatalf("incorrect live/recorder source: %s / %s", profile.StreamURL, profile.RecorderStreamURL)
			}
			if live.Query().Get("subtype") != recorder.Query().Get("subtype") || profile.LocalHLSURL != defaultEntry.Profiles[name].LocalHLSURL {
				t.Fatalf("profile routing changed: %+v", profile)
			}
			if credentials {
				password, _ := live.User.Password()
				if live.User.Username() != "camera-user" || password != "camera:p@ss" || recorder.User.Username() != "nvr-user" {
					t.Fatal("incorrect source credentials")
				}
			} else if live.User != nil || recorder.User != nil {
				t.Fatal("credentials exposed in public catalog")
			}
		}
	}
}

func TestDirectCameraRTSPPortUsesInventoryAndDefaults(t *testing.T) {
	for _, item := range []struct{ port, address, want string }{
		{"", "https://camera.local:8443", "camera.local:554"},
		{"80", "camera.local:8080", "camera.local:80"},
		{"443", "https://camera.local", "camera.local:443"},
		{"8554", "2001:db8::5", "[2001:db8::5]:8554"},
	} {
		t.Run(item.want, func(t *testing.T) {
			input := directSourceCatalogInput()
			input.LiveSources = map[string]string{"nvr_channel_05": LiveSourceCamera}
			input.ProbeResults[0].Children[0].Attributes["direct_ipc_rtsp_port"] = item.port
			cfg := input.NVRConfigs["nvr"]
			cfg.DirectIPCCredentials[0].DirectIPCIP = item.address
			input.NVRConfigs["nvr"] = cfg
			parsed, err := url.Parse(BuildCatalog(input)[0].Profiles["quality"].StreamURL)
			if err != nil || parsed.Host != item.want {
				t.Fatalf("unexpected RTSP address: %v, %v", parsed, err)
			}
		})
	}
}

func TestMissingDirectCredentialsReportNVRFallback(t *testing.T) {
	input := directSourceCatalogInput()
	cfg := input.NVRConfigs["nvr"]
	cfg.DirectIPCCredentials = nil
	input.NVRConfigs["nvr"] = cfg
	input.LiveSources = map[string]string{"nvr_channel_05": LiveSourceCamera}
	entry := BuildCatalog(input)[0]
	if entry.LiveSource.Source != LiveSourceNVR || entry.LiveSource.PreferredSource != LiveSourceCamera || entry.LiveSource.CameraAvailable || entry.LiveSource.CameraUnavailableReason == "" || entry.LiveSource.FallbackReason == "" || entry.Profiles["quality"].StreamURL == "" {
		t.Fatalf("missing camera source should report recorder fallback: %+v", entry)
	}
	if entry.Profiles["quality"].RecorderStreamURL == "" {
		t.Fatal("recorder URL must remain available")
	}
}
