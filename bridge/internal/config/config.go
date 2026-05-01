package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
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
	Imou          ImouConfig          `yaml:"imou"`
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
	Enabled              bool          `yaml:"enabled"`
	NodeID               string        `yaml:"node_id"`
	EntityMode           string        `yaml:"entity_mode"`
	CameraSnapshotSource string        `yaml:"camera_snapshot_source"`
	PublicBaseURL        string        `yaml:"public_base_url"`
	APIBaseURL           string        `yaml:"api_base_url"`
	AccessToken          string        `yaml:"access_token"`
	RequestTimeout       time.Duration `yaml:"request_timeout"`
}

type MediaConfig struct {
	Enabled             bool                    `yaml:"enabled"`
	FFmpegPath          string                  `yaml:"ffmpeg_path"`
	FFmpegLogLevel      string                  `yaml:"ffmpeg_log_level"`
	VideoEncoder        string                  `yaml:"video_encoder"`
	InputPreset         string                  `yaml:"input_preset"`
	ClipPath            string                  `yaml:"clip_path"`
	IdleTimeout         time.Duration           `yaml:"idle_timeout"`
	StartTimeout        time.Duration           `yaml:"start_timeout"`
	MaxWorkers          int                     `yaml:"max_workers"`
	FrameRate           int                     `yaml:"frame_rate"`
	StableFrameRate     int                     `yaml:"stable_frame_rate"`
	SubstreamFrameRate  int                     `yaml:"substream_frame_rate"`
	JPEGQuality         int                     `yaml:"jpeg_quality"`
	Threads             int                     `yaml:"threads"`
	ScaleWidth          int                     `yaml:"scale_width"`
	ReadBufferSize      int                     `yaml:"read_buffer_size"`
	HLSSegmentTime      time.Duration           `yaml:"hls_segment_time"`
	HLSListSize         int                     `yaml:"hls_list_size"`
	HWAccelArgs         []string                `yaml:"hwaccel_args"`
	WebRTCICEServers    []WebRTCICEServerConfig `yaml:"webrtc_ice_servers"`
	WebRTCUplinkTargets []string                `yaml:"webrtc_uplink_targets"`
	HLSTmpDir           string                  `yaml:"hls_tmp_dir"`
	HLSTempPath         string                  `yaml:"hls_temp_path"`
	HLSKeepAfterExit    time.Duration           `yaml:"hls_keep_after_exit"`
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

type ImouConfig struct {
	Enabled           bool          `yaml:"enabled"`
	AppID             string        `yaml:"app_id"`
	AppSecret         string        `yaml:"app_secret"`
	DataCenter        string        `yaml:"data_center"`
	Endpoint          string        `yaml:"endpoint"`
	RequestTimeout    time.Duration `yaml:"request_timeout"`
	AlarmPollInterval time.Duration `yaml:"alarm_poll_interval"`
	EventActiveWindow time.Duration `yaml:"event_active_window"`
}

type DevicesConfig struct {
	NVR []DeviceConfig `yaml:"nvr"`
	VTO []DeviceConfig `yaml:"vto"`
	IPC []DeviceConfig `yaml:"ipc"`
}

type ChannelAuxControlOverride struct {
	Channel  int      `yaml:"channel"`
	Outputs  []string `yaml:"outputs"`
	Features []string `yaml:"features"`
}

type ChannelPTZControlOverride struct {
	Channel int   `yaml:"channel"`
	Enabled *bool `yaml:"enabled"`
}

type ChannelRecordingControlOverride struct {
	Channel   int    `yaml:"channel"`
	Supported *bool  `yaml:"supported"`
	Active    *bool  `yaml:"active"`
	Mode      string `yaml:"mode"`
}

type ChannelImouOverride struct {
	Channel   int      `yaml:"channel"`
	DeviceID  string   `yaml:"device_id"`
	ChannelID string   `yaml:"channel_id"`
	Features  []string `yaml:"features"`
}

type ChannelDirectIPCCredential struct {
	NVRChannel        int    `yaml:"nvr_channel"`
	DirectIPCIP       string `yaml:"direct_ipc_ip"`
	DirectIPCBaseURL  string `yaml:"direct_ipc_base_url"`
	DirectIPCUser     string `yaml:"direct_ipc_user"`
	DirectIPCPassword string `yaml:"direct_ipc_password"`
}

type DeviceConfig struct {
	ID                         string                            `yaml:"id"`
	Name                       string                            `yaml:"name"`
	Manufacturer               string                            `yaml:"manufacturer"`
	Model                      string                            `yaml:"model"`
	BaseURL                    string                            `yaml:"base_url"`
	Username                   string                            `yaml:"username"`
	Password                   string                            `yaml:"password"`
	OnvifEnabled               *bool                             `yaml:"onvif_enabled"`
	OnvifUsername              string                            `yaml:"onvif_username"`
	OnvifPassword              string                            `yaml:"onvif_password"`
	OnvifServiceURL            string                            `yaml:"onvif_service_url"`
	ChannelAllowlist           []int                             `yaml:"channel_allowlist"`
	ChannelAuxControlOverrides []ChannelAuxControlOverride       `yaml:"channel_aux_control_overrides"`
	ChannelPTZControlOverrides []ChannelPTZControlOverride       `yaml:"channel_ptz_control_overrides"`
	ChannelRecordingOverrides  []ChannelRecordingControlOverride `yaml:"channel_recording_control_overrides"`
	ChannelImouOverrides       []ChannelImouOverride             `yaml:"channel_imou_overrides"`
	DirectIPCCredentials       []ChannelDirectIPCCredential      `yaml:"direct_ipc_credentials"`
	AllowConfigWrites          bool                              `yaml:"allow_config_writes"`
	LockAllowlist              []int                             `yaml:"lock_allowlist"`
	AlarmAllowlist             []int                             `yaml:"alarm_allowlist"`
	PollInterval               time.Duration                     `yaml:"poll_interval"`
	RequestTimeout             time.Duration                     `yaml:"request_timeout"`
	InsecureSkipTLS            bool                              `yaml:"insecure_skip_tls"`
	Enabled                    *bool                             `yaml:"enabled"`
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
			ListenAddress:              ":9205",
			MetricsPath:                "/metrics",
			HealthPath:                 "/healthz",
			ReadTimeout:                5 * time.Second,
			WriteTimeout:               60 * time.Second,
			IdleTimeout:                60 * time.Second,
			AdminRateLimitPerMinute:    30,
			AdminRateLimitBurst:        10,
			SnapshotRateLimitPerMinute: 240,
			SnapshotRateLimitBurst:     40,
			MediaRateLimitPerMinute:    60,
			MediaRateLimitBurst:        12,
		},
		MQTT: MQTTConfig{
			Enabled:         false,
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
			Enabled:            true,
			FFmpegPath:         "ffmpeg",
			FFmpegLogLevel:     "error",
			VideoEncoder:       "software",
			InputPreset:        "low_latency",
			ClipPath:           "/data/clips",
			IdleTimeout:        30 * time.Second,
			StartTimeout:       15 * time.Second,
			MaxWorkers:         32,
			FrameRate:          5,
			StableFrameRate:    5,
			SubstreamFrameRate: 5,
			JPEGQuality:        7,
			Threads:            1,
			ScaleWidth:         960,
			ReadBufferSize:     1024 * 1024,
			HLSSegmentTime:     2 * time.Second,
			HLSListSize:        6,
			HLSTmpDir:          "/data/tmp/dahuabridge/hls",
			HLSTempPath:        "/data/tmp/dahuabridge/hls",
			HLSKeepAfterExit:   6 * time.Hour,
		},
		HomeAssistant: HomeAssistantConfig{
			Enabled:              true,
			NodeID:               "dahuabridge",
			EntityMode:           "native",
			CameraSnapshotSource: "device",
			RequestTimeout:       15 * time.Second,
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
	c.HomeAssistant.EntityMode = strings.ToLower(strings.TrimSpace(c.HomeAssistant.EntityMode))
	if c.HomeAssistant.EntityMode == "" || c.HomeAssistant.EntityMode == "hybrid" {
		c.HomeAssistant.EntityMode = "native"
	}
	c.HomeAssistant.CameraSnapshotSource = strings.ToLower(strings.TrimSpace(c.HomeAssistant.CameraSnapshotSource))
	if c.HomeAssistant.CameraSnapshotSource == "" {
		c.HomeAssistant.CameraSnapshotSource = "device"
	}
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
	c.Imou.AppID = firstNonEmpty(strings.TrimSpace(c.Imou.AppID), strings.TrimSpace(os.Getenv("DAHUABRIDGE_IMOU_APP_ID")))
	c.Imou.AppSecret = firstNonEmpty(strings.TrimSpace(c.Imou.AppSecret), strings.TrimSpace(os.Getenv("DAHUABRIDGE_IMOU_APP_SECRET")))
	c.Imou.DataCenter = strings.ToLower(firstNonEmpty(strings.TrimSpace(c.Imou.DataCenter), strings.TrimSpace(os.Getenv("DAHUABRIDGE_IMOU_DATA_CENTER"))))
	c.Imou.Endpoint = strings.TrimRight(strings.TrimSpace(c.Imou.Endpoint), "/")
	if c.Imou.DataCenter == "" {
		c.Imou.DataCenter = "fk"
	}
	if c.Imou.Endpoint != "" {
		normalizedEndpoint, err := normalizeBaseURL(c.Imou.Endpoint)
		if err != nil {
			return fmt.Errorf("normalize imou.endpoint: %w", err)
		}
		c.Imou.Endpoint = normalizedEndpoint
	}
	if c.Imou.RequestTimeout <= 0 {
		c.Imou.RequestTimeout = 15 * time.Second
	}
	if c.Imou.AlarmPollInterval <= 0 {
		c.Imou.AlarmPollInterval = 15 * time.Second
	}
	if c.Imou.EventActiveWindow <= 0 {
		c.Imou.EventActiveWindow = 20 * time.Second
	}
	c.Media.FFmpegPath = strings.TrimSpace(c.Media.FFmpegPath)
	if c.Media.FFmpegPath == "" {
		c.Media.FFmpegPath = "ffmpeg"
	}
	c.Media.FFmpegLogLevel = strings.ToLower(strings.TrimSpace(c.Media.FFmpegLogLevel))
	if c.Media.FFmpegLogLevel == "" {
		c.Media.FFmpegLogLevel = "error"
	}
	c.Media.VideoEncoder = strings.ToLower(strings.TrimSpace(c.Media.VideoEncoder))
	if c.Media.VideoEncoder == "" {
		c.Media.VideoEncoder = "software"
	}
	c.Media.InputPreset = strings.ToLower(strings.TrimSpace(c.Media.InputPreset))
	if c.Media.InputPreset == "" {
		c.Media.InputPreset = "low_latency"
	}
	c.Media.ClipPath = strings.TrimSpace(c.Media.ClipPath)
	if c.Media.ClipPath == "" {
		c.Media.ClipPath = "/data/clips"
	}

	c.Media.HLSTmpDir = strings.TrimSpace(c.Media.HLSTmpDir)
	c.Media.HLSTempPath = strings.TrimSpace(c.Media.HLSTempPath)
	if c.Media.HLSTmpDir == "" {
		c.Media.HLSTmpDir = c.Media.HLSTempPath
	}
	if c.Media.HLSTmpDir == "" {
		c.Media.HLSTmpDir = "/data/tmp/dahuabridge/hls"
	}
	c.Media.HLSTempPath = c.Media.HLSTmpDir
	if c.Media.HLSKeepAfterExit < 0 {
		c.Media.HLSKeepAfterExit = 0
	}

	if c.Media.IdleTimeout <= 0 {
		c.Media.IdleTimeout = 30 * time.Second
	}
	if c.Media.StartTimeout <= 0 {
		c.Media.StartTimeout = 15 * time.Second
	}
	if c.Media.MaxWorkers <= 0 {
		c.Media.MaxWorkers = 32
	}
	if c.Media.FrameRate <= 0 {
		c.Media.FrameRate = 5
	}
	if c.Media.StableFrameRate <= 0 {
		c.Media.StableFrameRate = 5
	}
	if c.Media.SubstreamFrameRate <= 0 {
		c.Media.SubstreamFrameRate = c.Media.StableFrameRate
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

	if !c.Imou.Enabled && anyImouOverrides(c.Devices) && c.Imou.AppID != "" && c.Imou.AppSecret != "" {
		c.Imou.Enabled = true
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
	switch c.HomeAssistant.EntityMode {
	case "native":
	default:
		return fmt.Errorf("home_assistant.entity_mode must be native")
	}
	switch c.HomeAssistant.CameraSnapshotSource {
	case "device", "logo":
	default:
		return fmt.Errorf("home_assistant.camera_snapshot_source must be one of: device, logo")
	}
	switch c.Media.VideoEncoder {
	case "software", "qsv":
	default:
		return fmt.Errorf("media.video_encoder must be one of: software, qsv")
	}
	switch c.Media.InputPreset {
	case "low_latency", "stable":
	default:
		return fmt.Errorf("media.input_preset must be one of: low_latency, stable")
	}
	if c.Imou.Enabled {
		if c.Imou.AppID == "" {
			return errors.New("imou.app_id is required when imou.enabled=true")
		}
		if c.Imou.AppSecret == "" {
			return errors.New("imou.app_secret is required when imou.enabled=true")
		}
		if c.Imou.Endpoint == "" {
			switch c.Imou.DataCenter {
			case "fk", "sg", "or":
			default:
				return fmt.Errorf("imou.data_center must be one of: fk, sg, or")
			}
		}
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
		for _, override := range dev.ChannelImouOverrides {
			if !c.Imou.Enabled {
				return fmt.Errorf("device %q channel %d requires imou.enabled=true", dev.ID, override.Channel)
			}
			if override.DeviceID == "" {
				return fmt.Errorf("device %q channel %d imou override requires device_id", dev.ID, override.Channel)
			}
			if override.ChannelID == "" {
				return fmt.Errorf("device %q channel %d imou override requires channel_id", dev.ID, override.Channel)
			}
			if len(override.Features) == 0 {
				return fmt.Errorf("device %q channel %d imou override requires at least one feature", dev.ID, override.Channel)
			}
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
	dev.ChannelAllowlist = normalizePositiveIntList(dev.ChannelAllowlist)
	dev.ChannelAuxControlOverrides = normalizeChannelAuxControlOverrides(dev.ChannelAuxControlOverrides)
	dev.ChannelPTZControlOverrides = normalizeChannelPTZControlOverrides(dev.ChannelPTZControlOverrides)
	dev.ChannelRecordingOverrides = normalizeChannelRecordingControlOverrides(dev.ChannelRecordingOverrides)
	dev.ChannelImouOverrides = normalizeChannelImouOverrides(dev.ChannelImouOverrides)
	dev.DirectIPCCredentials = normalizeChannelDirectIPCCredentials(dev.DirectIPCCredentials)
	dev.LockAllowlist = normalizePositiveIntList(dev.LockAllowlist)
	dev.AlarmAllowlist = normalizePositiveIntList(dev.AlarmAllowlist)
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

func (d DeviceConfig) AllowsChannel(channel int) bool {
	if channel <= 0 {
		return false
	}
	if len(d.ChannelAllowlist) == 0 {
		return true
	}
	return slices.Contains(d.ChannelAllowlist, channel)
}

func (d DeviceConfig) AuxControlOverride(channel int) (ChannelAuxControlOverride, bool) {
	if channel <= 0 {
		return ChannelAuxControlOverride{}, false
	}
	for _, override := range d.ChannelAuxControlOverrides {
		if override.Channel == channel {
			return normalizeSingleChannelAuxControlOverride(override)
		}
	}
	return ChannelAuxControlOverride{}, false
}

func (d DeviceConfig) ImouOverride(channel int) (ChannelImouOverride, bool) {
	if channel <= 0 {
		return ChannelImouOverride{}, false
	}
	for _, override := range d.ChannelImouOverrides {
		if override.Channel == channel {
			return override, true
		}
	}
	return ChannelImouOverride{}, false
}

func (d DeviceConfig) DirectIPCCredential(channel int) (ChannelDirectIPCCredential, bool) {
	if channel <= 0 {
		return ChannelDirectIPCCredential{}, false
	}
	for _, credential := range d.DirectIPCCredentials {
		if credential.NVRChannel == channel {
			return credential, true
		}
	}
	return ChannelDirectIPCCredential{}, false
}

func (d DeviceConfig) PTZControlOverride(channel int) (ChannelPTZControlOverride, bool) {
	if channel <= 0 {
		return ChannelPTZControlOverride{}, false
	}
	for _, override := range d.ChannelPTZControlOverrides {
		if override.Channel == channel {
			return override, override.Enabled != nil
		}
	}
	return ChannelPTZControlOverride{}, false
}

func (d DeviceConfig) RecordingControlOverride(channel int) (ChannelRecordingControlOverride, bool) {
	if channel <= 0 {
		return ChannelRecordingControlOverride{}, false
	}
	for _, override := range d.ChannelRecordingOverrides {
		if override.Channel == channel {
			return override, override.Supported != nil || override.Active != nil || strings.TrimSpace(override.Mode) != ""
		}
	}
	return ChannelRecordingControlOverride{}, false
}

func (d DeviceConfig) AllowsLock(lock int) bool {
	if lock <= 0 {
		return false
	}
	if len(d.LockAllowlist) == 0 {
		return true
	}
	return slices.Contains(d.LockAllowlist, lock)
}

func (d DeviceConfig) AllowsAlarm(alarm int) bool {
	if alarm <= 0 {
		return false
	}
	if len(d.AlarmAllowlist) == 0 {
		return true
	}
	return slices.Contains(d.AlarmAllowlist, alarm)
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

func (c HomeAssistantConfig) NativeEntityMode() bool {
	return strings.EqualFold(strings.TrimSpace(c.EntityMode), "native")
}

func (c HomeAssistantConfig) LogoCameraSnapshots() bool {
	return strings.EqualFold(strings.TrimSpace(c.CameraSnapshotSource), "logo")
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

func normalizeChannelAuxControlOverrides(overrides []ChannelAuxControlOverride) []ChannelAuxControlOverride {
	if len(overrides) == 0 {
		return nil
	}

	byChannel := make(map[int]ChannelAuxControlOverride, len(overrides))
	channels := make([]int, 0, len(overrides))
	for _, override := range overrides {
		normalized, ok := normalizeSingleChannelAuxControlOverride(override)
		if !ok {
			continue
		}

		existing, ok := byChannel[normalized.Channel]
		if ok {
			existing.Outputs = uniqueLowerStrings(append(existing.Outputs, normalized.Outputs...))
			existing.Features = uniqueLowerStrings(append(existing.Features, normalized.Features...))
			byChannel[normalized.Channel] = existing
			continue
		}

		byChannel[normalized.Channel] = normalized
		channels = append(channels, normalized.Channel)
	}

	if len(channels) == 0 {
		return nil
	}
	slices.Sort(channels)
	normalized := make([]ChannelAuxControlOverride, 0, len(channels))
	for _, channel := range channels {
		normalized = append(normalized, byChannel[channel])
	}
	return normalized
}

func normalizeChannelImouOverrides(overrides []ChannelImouOverride) []ChannelImouOverride {
	if len(overrides) == 0 {
		return nil
	}

	byChannel := make(map[int]ChannelImouOverride, len(overrides))
	channels := make([]int, 0, len(overrides))
	for _, override := range overrides {
		normalized, ok := normalizeSingleChannelImouOverride(override)
		if !ok {
			continue
		}
		if _, exists := byChannel[normalized.Channel]; !exists {
			channels = append(channels, normalized.Channel)
		}
		byChannel[normalized.Channel] = normalized
	}
	if len(channels) == 0 {
		return nil
	}
	slices.Sort(channels)
	normalized := make([]ChannelImouOverride, 0, len(channels))
	for _, channel := range channels {
		normalized = append(normalized, byChannel[channel])
	}
	return normalized
}

func normalizeChannelDirectIPCCredentials(credentials []ChannelDirectIPCCredential) []ChannelDirectIPCCredential {
	if len(credentials) == 0 {
		return nil
	}

	byChannel := make(map[int]ChannelDirectIPCCredential, len(credentials))
	channels := make([]int, 0, len(credentials))
	for _, credential := range credentials {
		normalized, ok := normalizeSingleChannelDirectIPCCredential(credential)
		if !ok {
			continue
		}
		if _, exists := byChannel[normalized.NVRChannel]; !exists {
			channels = append(channels, normalized.NVRChannel)
		}
		byChannel[normalized.NVRChannel] = normalized
	}
	if len(channels) == 0 {
		return nil
	}
	slices.Sort(channels)
	normalized := make([]ChannelDirectIPCCredential, 0, len(channels))
	for _, channel := range channels {
		normalized = append(normalized, byChannel[channel])
	}
	return normalized
}

func normalizeChannelPTZControlOverrides(overrides []ChannelPTZControlOverride) []ChannelPTZControlOverride {
	if len(overrides) == 0 {
		return nil
	}
	byChannel := make(map[int]ChannelPTZControlOverride, len(overrides))
	channels := make([]int, 0, len(overrides))
	for _, override := range overrides {
		if override.Channel <= 0 || override.Enabled == nil {
			continue
		}
		if _, exists := byChannel[override.Channel]; !exists {
			channels = append(channels, override.Channel)
		}
		byChannel[override.Channel] = override
	}
	if len(channels) == 0 {
		return nil
	}
	slices.Sort(channels)
	normalized := make([]ChannelPTZControlOverride, 0, len(channels))
	for _, channel := range channels {
		normalized = append(normalized, byChannel[channel])
	}
	return normalized
}

func normalizeChannelRecordingControlOverrides(overrides []ChannelRecordingControlOverride) []ChannelRecordingControlOverride {
	if len(overrides) == 0 {
		return nil
	}
	byChannel := make(map[int]ChannelRecordingControlOverride, len(overrides))
	channels := make([]int, 0, len(overrides))
	for _, override := range overrides {
		if override.Channel <= 0 {
			continue
		}
		override.Mode = strings.ToLower(strings.TrimSpace(override.Mode))
		if override.Supported == nil && override.Active == nil && override.Mode == "" {
			continue
		}
		if _, exists := byChannel[override.Channel]; !exists {
			channels = append(channels, override.Channel)
		}
		byChannel[override.Channel] = override
	}
	if len(channels) == 0 {
		return nil
	}
	slices.Sort(channels)
	normalized := make([]ChannelRecordingControlOverride, 0, len(channels))
	for _, channel := range channels {
		normalized = append(normalized, byChannel[channel])
	}
	return normalized
}

func normalizeSingleChannelImouOverride(override ChannelImouOverride) (ChannelImouOverride, bool) {
	if override.Channel <= 0 {
		return ChannelImouOverride{}, false
	}
	normalized := ChannelImouOverride{
		Channel:   override.Channel,
		DeviceID:  strings.TrimSpace(override.DeviceID),
		ChannelID: strings.TrimSpace(override.ChannelID),
		Features:  normalizeImouOverrideFeatures(override.Features),
	}
	if normalized.DeviceID == "" || normalized.ChannelID == "" || len(normalized.Features) == 0 {
		return ChannelImouOverride{}, false
	}
	return normalized, true
}

func normalizeSingleChannelDirectIPCCredential(credential ChannelDirectIPCCredential) (ChannelDirectIPCCredential, bool) {
	normalized := ChannelDirectIPCCredential{
		NVRChannel:        credential.NVRChannel,
		DirectIPCIP:       strings.TrimSpace(credential.DirectIPCIP),
		DirectIPCBaseURL:  strings.TrimSpace(credential.DirectIPCBaseURL),
		DirectIPCUser:     strings.TrimSpace(credential.DirectIPCUser),
		DirectIPCPassword: strings.TrimSpace(credential.DirectIPCPassword),
	}
	if normalized.DirectIPCBaseURL != "" {
		baseURL, err := normalizeBaseURL(normalized.DirectIPCBaseURL)
		if err != nil {
			return ChannelDirectIPCCredential{}, false
		}
		normalized.DirectIPCBaseURL = baseURL
	}
	if normalized.NVRChannel <= 0 || normalized.DirectIPCIP == "" || normalized.DirectIPCUser == "" || normalized.DirectIPCPassword == "" {
		return ChannelDirectIPCCredential{}, false
	}
	return normalized, true
}

func normalizeImouOverrideFeatures(values []string) []string {
	features := make([]string, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "events":
			features = append(features, "events")
		case "light":
			features = append(features, "light")
		case "warning_light":
			features = append(features, "warning_light")
		case "siren", "aux":
			features = append(features, "siren")
		}
	}
	return uniqueLowerStrings(features)
}

func anyImouOverrides(devices DevicesConfig) bool {
	for _, dev := range devices.NVR {
		if len(dev.ChannelImouOverrides) > 0 {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeSingleChannelAuxControlOverride(override ChannelAuxControlOverride) (ChannelAuxControlOverride, bool) {
	if override.Channel <= 0 {
		return ChannelAuxControlOverride{}, false
	}
	normalized := ChannelAuxControlOverride{
		Channel: override.Channel,
		Outputs: normalizeAuxOverrideOutputs(override.Outputs),
	}
	normalized.Features = normalizeAuxOverrideFeatures(override.Features)
	normalized.Outputs = appendMissingString(normalized.Outputs, outputsForAuxFeatures(normalized.Features)...)
	normalized.Features = appendMissingString(normalized.Features, featuresForAuxOutputs(normalized.Outputs)...)
	normalized.Outputs = uniqueLowerStrings(normalized.Outputs)
	normalized.Features = uniqueLowerStrings(normalized.Features)
	if len(normalized.Outputs) == 0 && len(normalized.Features) == 0 {
		return ChannelAuxControlOverride{}, false
	}
	return normalized, true
}

func normalizeAuxOverrideOutputs(values []string) []string {
	outputs := make([]string, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "aux", "siren":
			outputs = append(outputs, "aux")
		case "light", "warning_light":
			outputs = append(outputs, "light")
		case "wiper":
			outputs = append(outputs, "wiper")
		}
	}
	return uniqueLowerStrings(outputs)
}

func normalizeAuxOverrideFeatures(values []string) []string {
	features := make([]string, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "aux", "siren":
			features = append(features, "siren")
		case "light":
			features = append(features, "light")
		case "warning_light":
			features = append(features, "warning_light")
		case "wiper":
			features = append(features, "wiper")
		}
	}
	return uniqueLowerStrings(features)
}

func outputsForAuxFeatures(features []string) []string {
	outputs := make([]string, 0, len(features))
	for _, feature := range features {
		switch strings.ToLower(strings.TrimSpace(feature)) {
		case "siren":
			outputs = append(outputs, "aux")
		case "light":
			outputs = append(outputs, "light")
		case "warning_light":
			outputs = append(outputs, "light")
		case "wiper":
			outputs = append(outputs, "wiper")
		}
	}
	return uniqueLowerStrings(outputs)
}

func featuresForAuxOutputs(outputs []string) []string {
	features := make([]string, 0, len(outputs))
	for _, output := range outputs {
		switch strings.ToLower(strings.TrimSpace(output)) {
		case "aux":
			features = append(features, "siren")
		case "light":
			features = append(features, "warning_light")
		case "wiper":
			features = append(features, "wiper")
		}
	}
	return uniqueLowerStrings(features)
}

func appendMissingString(values []string, additions ...string) []string {
	return append(values, additions...)
}

func uniqueLowerStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	slices.Sort(unique)
	return unique
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

func normalizePositiveIntList(values []int) []int {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[int]struct{}, len(values))
	normalized := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	slices.Sort(normalized)
	return normalized
}
