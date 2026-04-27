package config

import (
	"strings"
	"testing"
	"time"
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
	if cfg.Media.MaxWorkers != 14 {
		t.Fatalf("unexpected default media max_workers %d", cfg.Media.MaxWorkers)
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
	if len(cfg.Media.WebRTCICEServers) != 0 {
		t.Fatalf("expected no default webrtc ice servers, got %+v", cfg.Media.WebRTCICEServers)
	}
	if len(cfg.Media.WebRTCUplinkTargets) != 0 {
		t.Fatalf("expected no default webrtc uplink targets, got %+v", cfg.Media.WebRTCUplinkTargets)
	}
}

func TestHTTPRateLimitDefaults(t *testing.T) {
	cfg := defaultConfig()
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

	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	if cfg.HomeAssistant.APIBaseURL != "http://homeassistant.local:8123" {
		t.Fatalf("unexpected normalized api base url %q", cfg.HomeAssistant.APIBaseURL)
	}
}

func TestNormalizeMediaWebRTCICEServers(t *testing.T) {
	cfg := defaultConfig()
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
