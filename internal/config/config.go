package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Log           LogConfig           `yaml:"log"`
	HTTP          HTTPConfig          `yaml:"http"`
	MQTT          MQTTConfig          `yaml:"mqtt"`
	Media         MediaConfig         `yaml:"media"`
	HomeAssistant HomeAssistantConfig `yaml:"home_assistant"`
	StateStore    StateStoreConfig    `yaml:"state_store"`
	Devices       DevicesConfig       `yaml:"devices"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Pretty bool   `yaml:"pretty"`
}

type HTTPConfig struct {
	ListenAddress              string        `yaml:"listen_address"`
	MetricsPath                string        `yaml:"metrics_path"`
	HealthPath                 string        `yaml:"health_path"`
	ReadTimeout                time.Duration `yaml:"read_timeout"`
	WriteTimeout               time.Duration `yaml:"write_timeout"`
	IdleTimeout                time.Duration `yaml:"idle_timeout"`
	AdminRateLimitPerMinute    int           `yaml:"admin_rate_limit_per_minute"`
	AdminRateLimitBurst        int           `yaml:"admin_rate_limit_burst"`
	SnapshotRateLimitPerMinute int           `yaml:"snapshot_rate_limit_per_minute"`
	SnapshotRateLimitBurst     int           `yaml:"snapshot_rate_limit_burst"`
	MediaRateLimitPerMinute    int           `yaml:"media_rate_limit_per_minute"`
	MediaRateLimitBurst        int           `yaml:"media_rate_limit_burst"`
}

type MQTTConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Broker          string        `yaml:"broker"`
	ClientID        string        `yaml:"client_id"`
	Username        string        `yaml:"username"`
	Password        string        `yaml:"password"`
	TopicPrefix     string        `yaml:"topic_prefix"`
	DiscoveryPrefix string        `yaml:"discovery_prefix"`
	QoS             byte          `yaml:"qos"`
	Retain          bool          `yaml:"retain"`
	CleanSession    bool          `yaml:"clean_session"`
	KeepAlive       time.Duration `yaml:"keep_alive"`
	ConnectTimeout  time.Duration `yaml:"connect_timeout"`
	PublishTimeout  time.Duration `yaml:"publish_timeout"`
}

type HomeAssistantConfig struct {
	Enabled        bool          `yaml:"enabled"`
	NodeID         string        `yaml:"node_id"`
	PublicBaseURL  string        `yaml:"public_base_url"`
	APIBaseURL     string        `yaml:"api_base_url"`
	AccessToken    string        `yaml:"access_token"`
	RequestTimeout time.Duration `yaml:"request_timeout"`
}

type MediaConfig struct {
	Enabled             bool                    `yaml:"enabled"`
	FFmpegPath          string                  `yaml:"ffmpeg_path"`
	IdleTimeout         time.Duration           `yaml:"idle_timeout"`
	StartTimeout        time.Duration           `yaml:"start_timeout"`
	MaxWorkers          int                     `yaml:"max_workers"`
	FrameRate           int                     `yaml:"frame_rate"`
	JPEGQuality         int                     `yaml:"jpeg_quality"`
	Threads             int                     `yaml:"threads"`
	ScaleWidth          int                     `yaml:"scale_width"`
	ScaleHeight         int                     `yaml:"scale_height"`
	ReadBufferSize      int                     `yaml:"read_buffer_size"`
	HLSSegmentTime      time.Duration           `yaml:"hls_segment_time"`
	HLSListSize         int                     `yaml:"hls_list_size"`
	HWAccelArgs         []string                `yaml:"hwaccel_args"`
	WebRTCICEServers    []WebRTCICEServerConfig `yaml:"webrtc_ice_servers"`
	WebRTCUplinkTargets []string                `yaml:"webrtc_uplink_targets"`
}

type WebRTCICEServerConfig struct {
	URLs       []string `yaml:"urls"`
	Username   string   `yaml:"username"`
	Credential string   `yaml:"credential"`
}

type StateStoreConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Path          string        `yaml:"path"`
	FlushInterval time.Duration `yaml:"flush_interval"`
}

type DevicesConfig struct {
	NVR []DeviceConfig `yaml:"nvr"`
	VTO []DeviceConfig `yaml:"vto"`
	IPC []DeviceConfig `yaml:"ipc"`
}

type DeviceConfig struct {
	ID              string        `yaml:"id"`
	Name            string        `yaml:"name"`
	Manufacturer    string        `yaml:"manufacturer"`
	Model           string        `yaml:"model"`
	BaseURL         string        `yaml:"base_url"`
	Username        string        `yaml:"username"`
	Password        string        `yaml:"password"`
	OnvifEnabled    *bool         `yaml:"onvif_enabled"`
	OnvifUsername   string        `yaml:"onvif_username"`
	OnvifPassword   string        `yaml:"onvif_password"`
	OnvifServiceURL string        `yaml:"onvif_service_url"`
	PollInterval    time.Duration `yaml:"poll_interval"`
	RequestTimeout  time.Duration `yaml:"request_timeout"`
	InsecureSkipTLS bool          `yaml:"insecure_skip_tls"`
	Enabled         *bool         `yaml:"enabled"`
}

func Load(path string) (Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func NormalizeDevice(dev DeviceConfig) (DeviceConfig, error) {
	normalized := dev
	if err := normalizeDevice(&normalized); err != nil {
		return DeviceConfig{}, err
	}
	return normalized, nil
}

func defaultConfig() Config {
	return Config{
		Log: LogConfig{
			Level: "info",
		},
		HTTP: HTTPConfig{
			ListenAddress:              ":8080",
			MetricsPath:                "/metrics",
			HealthPath:                 "/healthz",
			ReadTimeout:                5 * time.Second,
			WriteTimeout:               10 * time.Second,
			IdleTimeout:                60 * time.Second,
			AdminRateLimitPerMinute:    30,
			AdminRateLimitBurst:        10,
			SnapshotRateLimitPerMinute: 240,
			SnapshotRateLimitBurst:     40,
			MediaRateLimitPerMinute:    60,
			MediaRateLimitBurst:        12,
		},
		MQTT: MQTTConfig{
			Enabled:         true,
			ClientID:        "dahuabridge",
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
			CleanSession:    false,
			KeepAlive:       30 * time.Second,
			ConnectTimeout:  15 * time.Second,
			PublishTimeout:  10 * time.Second,
		},
		Media: MediaConfig{
			Enabled:        true,
			FFmpegPath:     "ffmpeg",
			IdleTimeout:    30 * time.Second,
			StartTimeout:   15 * time.Second,
			MaxWorkers:     14,
			FrameRate:      5,
			JPEGQuality:    7,
			Threads:        1,
			ScaleWidth:     960,
			ReadBufferSize: 1024 * 1024,
			HLSSegmentTime: 2 * time.Second,
			HLSListSize:    6,
		},
		HomeAssistant: HomeAssistantConfig{
			Enabled:        true,
			NodeID:         "dahuabridge",
			RequestTimeout: 15 * time.Second,
		},
		StateStore: StateStoreConfig{
			FlushInterval: 5 * time.Second,
		},
	}
}

func (c *Config) normalize() error {
	c.HTTP.MetricsPath = normalizePath(c.HTTP.MetricsPath, "/metrics")
	c.HTTP.HealthPath = normalizePath(c.HTTP.HealthPath, "/healthz")
	if c.HTTP.AdminRateLimitPerMinute <= 0 {
		c.HTTP.AdminRateLimitPerMinute = 30
	}
	if c.HTTP.AdminRateLimitBurst <= 0 {
		c.HTTP.AdminRateLimitBurst = 10
	}
	if c.HTTP.SnapshotRateLimitPerMinute <= 0 {
		c.HTTP.SnapshotRateLimitPerMinute = 240
	}
	if c.HTTP.SnapshotRateLimitBurst <= 0 {
		c.HTTP.SnapshotRateLimitBurst = 40
	}
	if c.HTTP.MediaRateLimitPerMinute <= 0 {
		c.HTTP.MediaRateLimitPerMinute = 60
	}
	if c.HTTP.MediaRateLimitBurst <= 0 {
		c.HTTP.MediaRateLimitBurst = 12
	}
	c.HomeAssistant.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.HomeAssistant.PublicBaseURL), "/")
	if c.HomeAssistant.APIBaseURL != "" {
		normalizedAPIBaseURL, err := normalizeBaseURL(c.HomeAssistant.APIBaseURL)
		if err != nil {
			return fmt.Errorf("normalize home_assistant.api_base_url: %w", err)
		}
		c.HomeAssistant.APIBaseURL = normalizedAPIBaseURL
	}
	c.HomeAssistant.AccessToken = strings.TrimSpace(c.HomeAssistant.AccessToken)
	if c.HomeAssistant.RequestTimeout <= 0 {
		c.HomeAssistant.RequestTimeout = 15 * time.Second
	}
	c.Media.FFmpegPath = strings.TrimSpace(c.Media.FFmpegPath)
	if c.Media.FFmpegPath == "" {
		c.Media.FFmpegPath = "ffmpeg"
	}
	if c.Media.IdleTimeout <= 0 {
		c.Media.IdleTimeout = 30 * time.Second
	}
	if c.Media.StartTimeout <= 0 {
		c.Media.StartTimeout = 15 * time.Second
	}
	if c.Media.MaxWorkers <= 0 {
		c.Media.MaxWorkers = 14
	}
	if c.Media.FrameRate <= 0 {
		c.Media.FrameRate = 5
	}
	if c.Media.JPEGQuality <= 0 {
		c.Media.JPEGQuality = 7
	}
	if c.Media.Threads <= 0 {
		c.Media.Threads = 1
	}
	if c.Media.ScaleWidth < 0 {
		c.Media.ScaleWidth = 0
	}
	if c.Media.ScaleHeight < 0 {
		c.Media.ScaleHeight = 0
	}
	if c.Media.ReadBufferSize <= 0 {
		c.Media.ReadBufferSize = 1024 * 1024
	}
	if c.Media.HLSSegmentTime <= 0 {
		c.Media.HLSSegmentTime = 2 * time.Second
	}
	if c.Media.HLSListSize <= 0 {
		c.Media.HLSListSize = 6
	}
	for index := range c.Media.WebRTCICEServers {
		server := &c.Media.WebRTCICEServers[index]
		server.Username = strings.TrimSpace(server.Username)
		server.Credential = strings.TrimSpace(server.Credential)
		urls := make([]string, 0, len(server.URLs))
		for _, rawURL := range server.URLs {
			trimmed := strings.TrimSpace(rawURL)
			if trimmed != "" {
				urls = append(urls, trimmed)
			}
		}
		server.URLs = urls
	}
	uplinkTargets := make([]string, 0, len(c.Media.WebRTCUplinkTargets))
	for _, rawTarget := range c.Media.WebRTCUplinkTargets {
		normalizedTarget, err := normalizeUDPTarget(rawTarget)
		if err != nil {
			return fmt.Errorf("normalize media.webrtc_uplink_targets: %w", err)
		}
		if normalizedTarget != "" {
			uplinkTargets = append(uplinkTargets, normalizedTarget)
		}
	}
	c.Media.WebRTCUplinkTargets = uplinkTargets
	c.StateStore.Path = strings.TrimSpace(c.StateStore.Path)
	if c.StateStore.FlushInterval <= 0 {
		c.StateStore.FlushInterval = 5 * time.Second
	}

	for i := range c.Devices.NVR {
		if err := normalizeDevice(&c.Devices.NVR[i]); err != nil {
			return err
		}
	}

	for i := range c.Devices.VTO {
		if err := normalizeDevice(&c.Devices.VTO[i]); err != nil {
			return err
		}
	}

	for i := range c.Devices.IPC {
		if err := normalizeDevice(&c.Devices.IPC[i]); err != nil {
			return err
		}
	}

	return nil
}

func (c Config) validate() error {
	if c.MQTT.Enabled && c.MQTT.Broker == "" {
		return errors.New("mqtt.broker is required when mqtt.enabled=true")
	}

	if c.StateStore.Enabled && c.StateStore.Path == "" {
		return errors.New("state_store.path is required when state_store.enabled=true")
	}
	if c.HomeAssistant.APIBaseURL != "" && c.HomeAssistant.AccessToken == "" {
		return errors.New("home_assistant.access_token is required when home_assistant.api_base_url is set")
	}
	if c.HomeAssistant.AccessToken != "" && c.HomeAssistant.APIBaseURL == "" {
		return errors.New("home_assistant.api_base_url is required when home_assistant.access_token is set")
	}

	if len(c.Devices.NVR) == 0 && len(c.Devices.VTO) == 0 && len(c.Devices.IPC) == 0 {
		return errors.New("at least one device must be configured")
	}

	devices := append([]DeviceConfig{}, c.Devices.NVR...)
	devices = append(devices, c.Devices.VTO...)
	devices = append(devices, c.Devices.IPC...)

	for _, dev := range devices {
		if !dev.EnabledValue() {
			continue
		}

		if dev.ID == "" {
			return fmt.Errorf("device with base_url %q must have an id", dev.BaseURL)
		}

		if dev.BaseURL == "" {
			return fmt.Errorf("device %q must have a base_url", dev.ID)
		}

		if dev.Username == "" {
			return fmt.Errorf("device %q must have a username", dev.ID)
		}

		if dev.Password == "" {
			return fmt.Errorf("device %q must have a password", dev.ID)
		}
	}

	return nil
}

func normalizeDevice(dev *DeviceConfig) error {
	if dev.Enabled != nil && !*dev.Enabled && dev.ID == "" && dev.BaseURL == "" {
		return nil
	}

	normalized, err := normalizeBaseURL(dev.BaseURL)
	if err != nil {
		return fmt.Errorf("normalize device %q base_url: %w", dev.ID, err)
	}

	dev.BaseURL = normalized

	if dev.PollInterval <= 0 {
		dev.PollInterval = 30 * time.Second
	}

	if dev.RequestTimeout <= 0 {
		dev.RequestTimeout = 10 * time.Second
	}

	if dev.Manufacturer == "" {
		dev.Manufacturer = "Dahua"
	}

	if dev.Name == "" {
		dev.Name = dev.ID
	}

	if dev.Enabled == nil {
		dev.Enabled = boolPtr(true)
	}
	dev.OnvifUsername = strings.TrimSpace(dev.OnvifUsername)
	dev.OnvifPassword = strings.TrimSpace(dev.OnvifPassword)
	if dev.OnvifServiceURL != "" {
		normalizedServiceURL, err := normalizeBaseURL(dev.OnvifServiceURL)
		if err != nil {
			return fmt.Errorf("normalize device %q onvif_service_url: %w", dev.ID, err)
		}
		dev.OnvifServiceURL = normalizedServiceURL
	}

	return nil
}

func (d DeviceConfig) EnabledValue() bool {
	if d.Enabled == nil {
		return true
	}

	return *d.Enabled
}

func boolPtr(value bool) *bool {
	return &value
}

func (d DeviceConfig) ONVIFEnabledValue() bool {
	if d.OnvifEnabled != nil {
		return *d.OnvifEnabled
	}
	return d.OnvifUsername != "" || d.OnvifPassword != "" || d.OnvifServiceURL != ""
}

func (d DeviceConfig) ONVIFUsernameValue() string {
	if d.OnvifUsername != "" {
		return d.OnvifUsername
	}
	return d.Username
}

func (d DeviceConfig) ONVIFPasswordValue() string {
	if d.OnvifPassword != "" {
		return d.OnvifPassword
	}
	return d.Password
}

func normalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}

	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("missing host in %q", raw)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func normalizePath(value string, fallback string) string {
	if value == "" {
		return fallback
	}

	if strings.HasPrefix(value, "/") {
		return value
	}

	return "/" + value
}

func normalizeUDPTarget(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "udp://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(parsed.Scheme, "udp") {
		return "", fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("missing host in %q", raw)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("unexpected path in %q", raw)
	}
	if _, _, err := net.SplitHostPort(parsed.Host); err != nil {
		return "", fmt.Errorf("invalid host:port in %q: %w", raw, err)
	}

	parsed.Scheme = "udp"
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
