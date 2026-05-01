package config

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestEnabledDefaultsToTrueWhenUnset(t *testing.T) {
	cfg := DeviceConfig{
		ID:      "nvr",
		BaseURL: "http://127.0.0.1",
	}

	if err := normalizeDevice(&cfg); err != nil {
		t.Fatalf("normalizeDevice returned error: %v", err)
	}

	if !cfg.EnabledValue() {
		t.Fatal("expected device to be enabled by default")
	}
}

func TestEnabledCanBeExplicitlyDisabled(t *testing.T) {
	enabled := false
	cfg := DeviceConfig{
		ID:      "nvr",
		BaseURL: "http://127.0.0.1",
		Enabled: &enabled,
	}

	if err := normalizeDevice(&cfg); err != nil {
		t.Fatalf("normalizeDevice returned error: %v", err)
	}

	if cfg.EnabledValue() {
		t.Fatal("expected device to stay disabled")
	}
}

func TestStateStoreDefaultsFlushInterval(t *testing.T) {
	cfg := defaultConfig()
	if cfg.StateStore.FlushInterval != 5*time.Second {
		t.Fatalf("unexpected default flush interval %s", cfg.StateStore.FlushInterval)
	}
}

func TestMediaDefaults(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Media.MaxWorkers != 32 {
		t.Fatalf("unexpected default media max_workers %d", cfg.Media.MaxWorkers)
	}
	if cfg.Media.VideoEncoder != "software" {
		t.Fatalf("unexpected default media video_encoder %q", cfg.Media.VideoEncoder)
	}
	if cfg.Media.InputPreset != "low_latency" {
		t.Fatalf("unexpected default media input_preset %q", cfg.Media.InputPreset)
	}
	if cfg.Media.ClipPath != "/data/clips" {
		t.Fatalf("unexpected default media clip_path %q", cfg.Media.ClipPath)
	}
	if cfg.Media.StableFrameRate != 5 {
		t.Fatalf("unexpected default media stable_frame_rate %d", cfg.Media.StableFrameRate)
	}
	if cfg.Media.SubstreamFrameRate != 5 {
		t.Fatalf("unexpected default media substream_frame_rate %d", cfg.Media.SubstreamFrameRate)
	}
	if cfg.Media.Threads != 1 {
		t.Fatalf("unexpected default media threads %d", cfg.Media.Threads)
	}
	if cfg.Media.ScaleWidth != 960 {
		t.Fatalf("unexpected default media scale_width %d", cfg.Media.ScaleWidth)
	}
	if cfg.Media.HLSSegmentTime != 2*time.Second {
		t.Fatalf("unexpected default media hls_segment_time %s", cfg.Media.HLSSegmentTime)
	}
	if cfg.Media.HLSListSize != 6 {
		t.Fatalf("unexpected default media hls_list_size %d", cfg.Media.HLSListSize)
	}
	if cfg.Media.HLSTmpDir != "/data/tmp/dahuabridge/hls" {
		t.Fatalf("unexpected default media hls_tmp_dir %q", cfg.Media.HLSTmpDir)
	}
	if cfg.Media.HLSKeepAfterExit != 6*time.Hour {
		t.Fatalf("unexpected default media hls_keep_after_exit %s", cfg.Media.HLSKeepAfterExit)
	}
	if len(cfg.Media.WebRTCICEServers) != 0 {
		t.Fatalf("expected no default webrtc ice servers, got %+v", cfg.Media.WebRTCICEServers)
	}
	if len(cfg.Media.WebRTCUplinkTargets) != 0 {
		t.Fatalf("expected no default webrtc uplink targets, got %+v", cfg.Media.WebRTCUplinkTargets)
	}
}

func TestHTTPRateLimitDefaults(t *testing.T) {
	cfg := defaultConfig()
	if cfg.HTTP.WriteTimeout != 60*time.Second {
		t.Fatalf("unexpected http write timeout default %s", cfg.HTTP.WriteTimeout)
	}
	if cfg.HTTP.AdminRateLimitPerMinute != 30 || cfg.HTTP.AdminRateLimitBurst != 10 {
		t.Fatalf("unexpected admin rate limit defaults: %+v", cfg.HTTP)
	}
	if cfg.HTTP.SnapshotRateLimitPerMinute != 240 || cfg.HTTP.SnapshotRateLimitBurst != 40 {
		t.Fatalf("unexpected snapshot rate limit defaults: %+v", cfg.HTTP)
	}
	if cfg.HTTP.MediaRateLimitPerMinute != 60 || cfg.HTTP.MediaRateLimitBurst != 12 {
		t.Fatalf("unexpected media rate limit defaults: %+v", cfg.HTTP)
	}
}

func TestValidateRequiresStateStorePathWhenEnabled(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.StateStore.Enabled = true
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
	}}

	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "state_store.path") {
		t.Fatalf("expected state_store.path validation error, got %v", err)
	}
}

func TestHomeAssistantAPIDefaultTimeout(t *testing.T) {
	cfg := defaultConfig()
	if cfg.HomeAssistant.RequestTimeout != 15*time.Second {
		t.Fatalf("unexpected home assistant request timeout %s", cfg.HomeAssistant.RequestTimeout)
	}
	if cfg.HomeAssistant.EntityMode != "native" {
		t.Fatalf("unexpected home assistant entity mode %q", cfg.HomeAssistant.EntityMode)
	}
	if cfg.HomeAssistant.CameraSnapshotSource != "device" {
		t.Fatalf("unexpected home assistant camera snapshot source %q", cfg.HomeAssistant.CameraSnapshotSource)
	}
}

func TestValidateRequiresHomeAssistantTokenWhenAPIBaseURLIsSet(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.HomeAssistant.APIBaseURL = "http://homeassistant.local:8123"
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
	}}

	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "home_assistant.access_token") {
		t.Fatalf("expected home assistant access token validation error, got %v", err)
	}
}

func TestValidateRequiresHomeAssistantAPIBaseURLWhenTokenIsSet(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.HomeAssistant.AccessToken = "token"
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
	}}

	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "home_assistant.api_base_url") {
		t.Fatalf("expected home assistant api_base_url validation error, got %v", err)
	}
}

func TestNormalizeHomeAssistantAPIBaseURL(t *testing.T) {
	cfg := defaultConfig()
	cfg.HomeAssistant.APIBaseURL = " http://homeassistant.local:8123/ "
	cfg.HomeAssistant.EntityMode = " Native "
	cfg.HomeAssistant.CameraSnapshotSource = " Logo "

	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	if cfg.HomeAssistant.APIBaseURL != "http://homeassistant.local:8123" {
		t.Fatalf("unexpected normalized api base url %q", cfg.HomeAssistant.APIBaseURL)
	}
	if cfg.HomeAssistant.EntityMode != "native" {
		t.Fatalf("unexpected normalized entity mode %q", cfg.HomeAssistant.EntityMode)
	}
	if cfg.HomeAssistant.CameraSnapshotSource != "logo" {
		t.Fatalf("unexpected normalized camera snapshot source %q", cfg.HomeAssistant.CameraSnapshotSource)
	}
}

func TestNormalizeMediaHLSTmpDirAlias(t *testing.T) {
	cfg := defaultConfig()
	cfg.Media.HLSTmpDir = ""
	cfg.Media.HLSTempPath = " /srv/cache/hls "

	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}

	if cfg.Media.HLSTmpDir != "/srv/cache/hls" {
		t.Fatalf("unexpected normalized media hls_tmp_dir %q", cfg.Media.HLSTmpDir)
	}
	if cfg.Media.HLSTempPath != "/srv/cache/hls" {
		t.Fatalf("unexpected normalized media hls_temp_path %q", cfg.Media.HLSTempPath)
	}
}

func TestYAMLUnmarshalSupportsMediaHLSTmpDir(t *testing.T) {
	cfg := defaultConfig()
	cfg.Media.HLSTmpDir = ""
	cfg.Media.HLSTempPath = ""
	cfg.Media.HLSKeepAfterExit = 0

	data := []byte("media:\n  hls_tmp_dir: /cache/dahuabridge/hls\n  hls_keep_after_exit: 90m\n")
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal returned error: %v", err)
	}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}

	if cfg.Media.HLSTmpDir != "/cache/dahuabridge/hls" {
		t.Fatalf("unexpected media hls_tmp_dir %q", cfg.Media.HLSTmpDir)
	}
	if cfg.Media.HLSTempPath != "/cache/dahuabridge/hls" {
		t.Fatalf("unexpected media hls_temp_path %q", cfg.Media.HLSTempPath)
	}
	if cfg.Media.HLSKeepAfterExit != 90*time.Minute {
		t.Fatalf("unexpected media hls_keep_after_exit %s", cfg.Media.HLSKeepAfterExit)
	}
}

func TestValidateRejectsUnsupportedHomeAssistantEntityMode(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.HomeAssistant.EntityMode = "broken"
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
	}}

	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "home_assistant.entity_mode") {
		t.Fatalf("expected home assistant entity mode validation error, got %v", err)
	}
}

func TestValidateRejectsUnsupportedHomeAssistantCameraSnapshotSource(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.HomeAssistant.CameraSnapshotSource = "broken"
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
	}}

	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "home_assistant.camera_snapshot_source") {
		t.Fatalf("expected home assistant camera snapshot source validation error, got %v", err)
	}
}

func TestNormalizeMediaWebRTCICEServers(t *testing.T) {
	cfg := defaultConfig()
	cfg.Media.VideoEncoder = " QSV "
	cfg.Media.InputPreset = " Stable "
	cfg.Media.WebRTCICEServers = []WebRTCICEServerConfig{
		{
			URLs:       []string{" stun:stun1.example.net:3478 ", "", "turn:turn.example.net:3478?transport=udp"},
			Username:   " user ",
			Credential: " pass ",
		},
	}

	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	if cfg.Media.VideoEncoder != "qsv" {
		t.Fatalf("unexpected normalized video encoder %q", cfg.Media.VideoEncoder)
	}
	if cfg.Media.InputPreset != "stable" {
		t.Fatalf("unexpected normalized input preset %q", cfg.Media.InputPreset)
	}

	if len(cfg.Media.WebRTCICEServers) != 1 {
		t.Fatalf("expected one normalized ice server, got %d", len(cfg.Media.WebRTCICEServers))
	}
	server := cfg.Media.WebRTCICEServers[0]
	if len(server.URLs) != 2 {
		t.Fatalf("expected 2 normalized ice urls, got %+v", server.URLs)
	}
	if server.URLs[0] != "stun:stun1.example.net:3478" {
		t.Fatalf("unexpected first ice url %q", server.URLs[0])
	}
	if server.Username != "user" || server.Credential != "pass" {
		t.Fatalf("unexpected normalized credentials %+v", server)
	}
}

func TestNormalizeMediaWebRTCUplinkTargets(t *testing.T) {
	cfg := defaultConfig()
	cfg.Media.WebRTCUplinkTargets = []string{
		" 127.0.0.1:5004 ",
		"udp://127.0.0.1:5006",
		"",
	}

	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}

	if len(cfg.Media.WebRTCUplinkTargets) != 2 {
		t.Fatalf("expected 2 normalized uplink targets, got %+v", cfg.Media.WebRTCUplinkTargets)
	}
	if cfg.Media.WebRTCUplinkTargets[0] != "udp://127.0.0.1:5004" {
		t.Fatalf("unexpected first uplink target %q", cfg.Media.WebRTCUplinkTargets[0])
	}
}

func TestValidateRejectsUnsupportedMediaVideoEncoder(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.Media.VideoEncoder = "broken"
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
	}}

	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "media.video_encoder") {
		t.Fatalf("expected media video encoder validation error, got %v", err)
	}
}

func TestValidateRejectsUnsupportedMediaInputPreset(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.Media.InputPreset = "broken"
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
	}}

	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "media.input_preset") {
		t.Fatalf("expected media input preset validation error, got %v", err)
	}
}

func TestNormalizeDeviceChannelAllowlist(t *testing.T) {
	cfg := DeviceConfig{
		ID:               "nvr",
		BaseURL:          "http://127.0.0.1",
		ChannelAllowlist: []int{6, 2, 6, -1, 0, 11},
	}

	if err := normalizeDevice(&cfg); err != nil {
		t.Fatalf("normalizeDevice returned error: %v", err)
	}

	want := []int{2, 6, 11}
	if len(cfg.ChannelAllowlist) != len(want) {
		t.Fatalf("unexpected normalized allowlist %+v", cfg.ChannelAllowlist)
	}
	for i, value := range want {
		if cfg.ChannelAllowlist[i] != value {
			t.Fatalf("unexpected normalized allowlist %+v", cfg.ChannelAllowlist)
		}
	}
}

func TestNormalizeImouConfigFromEnvAndOverrides(t *testing.T) {
	t.Setenv("DAHUABRIDGE_IMOU_APP_ID", " env-app ")
	t.Setenv("DAHUABRIDGE_IMOU_APP_SECRET", " env-secret ")
	t.Setenv("DAHUABRIDGE_IMOU_DATA_CENTER", " FK ")

	cfg := defaultConfig()
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
		ChannelImouOverrides: []ChannelImouOverride{
			{Channel: 6, DeviceID: " serial ", ChannelID: " 1 ", Features: []string{" warning_light ", "events", "siren", "bad"}},
			{Channel: 5, DeviceID: " serial ", ChannelID: " 0 ", Features: []string{"light"}},
		},
	}}

	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}

	if !cfg.Imou.Enabled {
		t.Fatal("expected imou to auto-enable when overrides and env credentials are present")
	}
	if cfg.Imou.AppID != "env-app" || cfg.Imou.AppSecret != "env-secret" || cfg.Imou.DataCenter != "fk" {
		t.Fatalf("unexpected normalized imou config %+v", cfg.Imou)
	}
	if len(cfg.Devices.NVR[0].ChannelImouOverrides) != 2 {
		t.Fatalf("unexpected imou overrides %+v", cfg.Devices.NVR[0].ChannelImouOverrides)
	}
	first := cfg.Devices.NVR[0].ChannelImouOverrides[0]
	if first.Channel != 5 || first.DeviceID != "serial" || first.ChannelID != "0" {
		t.Fatalf("unexpected first imou override %+v", first)
	}
	second := cfg.Devices.NVR[0].ChannelImouOverrides[1]
	if len(second.Features) != 3 || second.Features[0] != "events" || second.Features[1] != "siren" || second.Features[2] != "warning_light" {
		t.Fatalf("unexpected normalized second features %+v", second.Features)
	}
}

func TestValidateRequiresImouCredentialsWhenOverrideConfigured(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.Imou.Enabled = true
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
		ChannelImouOverrides: []ChannelImouOverride{
			{Channel: 5, DeviceID: "serial", ChannelID: "0", Features: []string{"events"}},
		},
	}}

	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "imou.app_id") {
		t.Fatalf("expected imou.app_id validation error, got %v", err)
	}
}

func TestValidateRejectsUnsupportedImouDataCenter(t *testing.T) {
	cfg := defaultConfig()
	cfg.MQTT.Enabled = false
	cfg.Imou.Enabled = true
	cfg.Imou.AppID = "app"
	cfg.Imou.AppSecret = "secret"
	cfg.Imou.DataCenter = "broken"
	cfg.Devices.NVR = []DeviceConfig{{
		ID:       "nvr",
		BaseURL:  "http://127.0.0.1",
		Username: "admin",
		Password: "secret",
		Enabled:  boolPtr(true),
	}}

	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "imou.data_center") {
		t.Fatalf("expected imou.data_center validation error, got %v", err)
	}
}

func TestNormalizeChannelPTZAndRecordingOverrides(t *testing.T) {
	cfg := DeviceConfig{
		ID:      "nvr",
		BaseURL: "http://127.0.0.1",
		ChannelPTZControlOverrides: []ChannelPTZControlOverride{
			{Channel: 9, Enabled: boolPtr(false)},
			{Channel: -1, Enabled: boolPtr(false)},
		},
		ChannelRecordingOverrides: []ChannelRecordingControlOverride{
			{Channel: 2, Supported: boolPtr(true), Active: boolPtr(true), Mode: " Auto "},
			{Channel: 0, Supported: boolPtr(true)},
		},
	}

	if err := normalizeDevice(&cfg); err != nil {
		t.Fatalf("normalizeDevice returned error: %v", err)
	}
	if len(cfg.ChannelPTZControlOverrides) != 1 || cfg.ChannelPTZControlOverrides[0].Channel != 9 || cfg.ChannelPTZControlOverrides[0].Enabled == nil || *cfg.ChannelPTZControlOverrides[0].Enabled {
		t.Fatalf("unexpected ptz overrides %+v", cfg.ChannelPTZControlOverrides)
	}
	if len(cfg.ChannelRecordingOverrides) != 1 || cfg.ChannelRecordingOverrides[0].Channel != 2 || cfg.ChannelRecordingOverrides[0].Mode != "auto" {
		t.Fatalf("unexpected recording overrides %+v", cfg.ChannelRecordingOverrides)
	}
}

func TestNormalizeDeviceDirectIPCCredentials(t *testing.T) {
	cfg := DeviceConfig{
		ID:      "nvr",
		BaseURL: "http://127.0.0.1",
		DirectIPCCredentials: []ChannelDirectIPCCredential{
			{NVRChannel: 8, DirectIPCIP: " 192.168.150.120 ", DirectIPCBaseURL: " https://192.168.150.120/ ", DirectIPCUser: " admin ", DirectIPCPassword: " secret "},
			{NVRChannel: 0, DirectIPCIP: "192.168.150.80", DirectIPCUser: "admin", DirectIPCPassword: "secret"},
			{NVRChannel: 8, DirectIPCIP: "192.168.150.121", DirectIPCUser: "operator", DirectIPCPassword: "updated"},
			{NVRChannel: 11, DirectIPCIP: "192.168.150.20", DirectIPCUser: "viewer", DirectIPCPassword: "pw"},
		},
	}

	if err := normalizeDevice(&cfg); err != nil {
		t.Fatalf("normalizeDevice returned error: %v", err)
	}
	if len(cfg.DirectIPCCredentials) != 2 {
		t.Fatalf("unexpected direct ipc credentials %+v", cfg.DirectIPCCredentials)
	}

	channel8, ok := cfg.DirectIPCCredential(8)
	if !ok {
		t.Fatal("expected channel 8 direct ipc credential")
	}
	if channel8.DirectIPCIP != "192.168.150.121" || channel8.DirectIPCUser != "operator" || channel8.DirectIPCPassword != "updated" || channel8.DirectIPCBaseURL != "" {
		t.Fatalf("unexpected channel 8 credential %+v", channel8)
	}

	channel11, ok := cfg.DirectIPCCredential(11)
	if !ok {
		t.Fatal("expected channel 11 direct ipc credential")
	}
	if channel11.DirectIPCIP != "192.168.150.20" || channel11.DirectIPCUser != "viewer" || channel11.DirectIPCPassword != "pw" {
		t.Fatalf("unexpected channel 11 credential %+v", channel11)
	}
}

func TestNormalizeDeviceDirectIPCCredentialBaseURL(t *testing.T) {
	cfg := DeviceConfig{
		ID:      "nvr",
		BaseURL: "http://127.0.0.1",
		DirectIPCCredentials: []ChannelDirectIPCCredential{
			{NVRChannel: 11, DirectIPCIP: "192.168.150.20", DirectIPCBaseURL: " https://192.168.150.20/ ", DirectIPCUser: "admin", DirectIPCPassword: "secret"},
		},
	}

	if err := normalizeDevice(&cfg); err != nil {
		t.Fatalf("normalizeDevice returned error: %v", err)
	}

	channel11, ok := cfg.DirectIPCCredential(11)
	if !ok {
		t.Fatal("expected channel 11 direct ipc credential")
	}
	if channel11.DirectIPCBaseURL != "https://192.168.150.20" {
		t.Fatalf("unexpected normalized direct ipc base url %q", channel11.DirectIPCBaseURL)
	}
}

func TestNormalizeDeviceChannelAuxControlOverrides(t *testing.T) {
	cfg := DeviceConfig{
		ID:      "nvr",
		BaseURL: "http://127.0.0.1",
		ChannelAuxControlOverrides: []ChannelAuxControlOverride{
			{Channel: 11, Features: []string{" Siren ", "wiper"}},
			{Channel: 9, Outputs: []string{" warning_light "}},
			{Channel: 11, Outputs: []string{"aux", "light"}},
			{Channel: -1, Outputs: []string{"aux"}},
		},
	}

	if err := normalizeDevice(&cfg); err != nil {
		t.Fatalf("normalizeDevice returned error: %v", err)
	}

	if len(cfg.ChannelAuxControlOverrides) != 2 {
		t.Fatalf("unexpected override count %+v", cfg.ChannelAuxControlOverrides)
	}

	override9, ok := cfg.AuxControlOverride(9)
	if !ok {
		t.Fatal("expected channel 9 override")
	}
	if len(override9.Outputs) != 1 || override9.Outputs[0] != "light" {
		t.Fatalf("unexpected channel 9 outputs %+v", override9.Outputs)
	}
	if len(override9.Features) != 1 || override9.Features[0] != "warning_light" {
		t.Fatalf("unexpected channel 9 features %+v", override9.Features)
	}

	override11, ok := cfg.AuxControlOverride(11)
	if !ok {
		t.Fatal("expected channel 11 override")
	}
	if len(override11.Outputs) != 3 || override11.Outputs[0] != "aux" || override11.Outputs[1] != "light" || override11.Outputs[2] != "wiper" {
		t.Fatalf("unexpected channel 11 outputs %+v", override11.Outputs)
	}
	if len(override11.Features) != 3 || override11.Features[0] != "siren" || override11.Features[1] != "warning_light" || override11.Features[2] != "wiper" {
		t.Fatalf("unexpected channel 11 features %+v", override11.Features)
	}
}

func TestDeviceConfigAllowsChannel(t *testing.T) {
	cfg := DeviceConfig{ChannelAllowlist: []int{2, 6, 11}}

	if !cfg.AllowsChannel(6) {
		t.Fatal("expected channel 6 to be allowed")
	}
	if cfg.AllowsChannel(7) {
		t.Fatal("expected channel 7 to be rejected")
	}
	if cfg.AllowsChannel(0) {
		t.Fatal("expected channel 0 to be rejected")
	}
}

func TestNormalizeDeviceAccessoryAllowlists(t *testing.T) {
	cfg := DeviceConfig{
		ID:             "vto",
		BaseURL:        "http://127.0.0.1",
		LockAllowlist:  []int{1, 1, 3, -1},
		AlarmAllowlist: []int{2, 0, 2, 5},
	}

	if err := normalizeDevice(&cfg); err != nil {
		t.Fatalf("normalizeDevice returned error: %v", err)
	}

	if len(cfg.LockAllowlist) != 2 || cfg.LockAllowlist[0] != 1 || cfg.LockAllowlist[1] != 3 {
		t.Fatalf("unexpected normalized lock allowlist %+v", cfg.LockAllowlist)
	}
	if len(cfg.AlarmAllowlist) != 2 || cfg.AlarmAllowlist[0] != 2 || cfg.AlarmAllowlist[1] != 5 {
		t.Fatalf("unexpected normalized alarm allowlist %+v", cfg.AlarmAllowlist)
	}
}

func TestDeviceConfigAllowsVTOAccessories(t *testing.T) {
	cfg := DeviceConfig{
		LockAllowlist:  []int{1},
		AlarmAllowlist: []int{2, 4},
	}

	if !cfg.AllowsLock(1) || cfg.AllowsLock(2) {
		t.Fatalf("unexpected lock allowlist behavior %+v", cfg.LockAllowlist)
	}
	if !cfg.AllowsAlarm(2) || cfg.AllowsAlarm(3) {
		t.Fatalf("unexpected alarm allowlist behavior %+v", cfg.AlarmAllowlist)
	}
}
