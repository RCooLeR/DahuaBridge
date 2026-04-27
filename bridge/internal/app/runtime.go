package app

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"sync"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/ha"
	"RCooLeR/DahuaBridge/internal/haapi"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"
	"RCooLeR/DahuaBridge/internal/streams"
)

type runtimeServices struct {
	mu           sync.RWMutex
	cfg          config.Config
	probes       *store.ProbeStore
	media        intercomStatusReader
	nvrSnapshots map[string]dahua.SnapshotProvider
	nvrConfigs   map[string]config.DeviceConfig
	vtoSnapshots map[string]dahua.SnapshotProvider
	vtoConfigs   map[string]config.DeviceConfig
	ipcSnapshots map[string]dahua.SnapshotProvider
	ipcConfigs   map[string]config.DeviceConfig
}

type intercomStatusReader interface {
	IntercomStatus(string) media.IntercomStatus
}

func newRuntimeServices(cfg config.Config, probes *store.ProbeStore) *runtimeServices {
	return &runtimeServices{
		cfg:          cfg,
		probes:       probes,
		nvrSnapshots: make(map[string]dahua.SnapshotProvider),
		nvrConfigs:   make(map[string]config.DeviceConfig),
		vtoSnapshots: make(map[string]dahua.SnapshotProvider),
		vtoConfigs:   make(map[string]config.DeviceConfig),
		ipcSnapshots: make(map[string]dahua.SnapshotProvider),
		ipcConfigs:   make(map[string]config.DeviceConfig),
	}
}

func (r *runtimeServices) AttachMedia(reader intercomStatusReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.media = reader
}

func (r *runtimeServices) RegisterNVR(deviceID string, provider dahua.SnapshotProvider, cfg config.DeviceConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nvrSnapshots[deviceID] = provider
	r.nvrConfigs[deviceID] = cfg
}

func (r *runtimeServices) RegisterVTO(deviceID string, provider dahua.SnapshotProvider, cfg config.DeviceConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.vtoSnapshots[deviceID] = provider
	r.vtoConfigs[deviceID] = cfg
}

func (r *runtimeServices) RegisterIPC(deviceID string, provider dahua.SnapshotProvider, cfg config.DeviceConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ipcSnapshots[deviceID] = provider
	r.ipcConfigs[deviceID] = cfg
}

func (r *runtimeServices) GetDeviceConfig(deviceID string) (config.DeviceConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if cfg, ok := r.nvrConfigs[deviceID]; ok {
		return cfg, true
	}
	if cfg, ok := r.vtoConfigs[deviceID]; ok {
		return cfg, true
	}
	if cfg, ok := r.ipcConfigs[deviceID]; ok {
		return cfg, true
	}
	return config.DeviceConfig{}, false
}

func (r *runtimeServices) UpdateDeviceConfig(deviceID string, cfg config.DeviceConfig) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.nvrConfigs[deviceID]; ok {
		r.nvrConfigs[deviceID] = cfg
		return true
	}
	if _, ok := r.vtoConfigs[deviceID]; ok {
		r.vtoConfigs[deviceID] = cfg
		return true
	}
	if _, ok := r.ipcConfigs[deviceID]; ok {
		r.ipcConfigs[deviceID] = cfg
		return true
	}
	return false
}

func (r *runtimeServices) NVRSnapshot(ctx context.Context, deviceID string, channel int) ([]byte, string, error) {
	r.mu.RLock()
	provider, ok := r.nvrSnapshots[deviceID]
	r.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("snapshot provider not found for device %q", deviceID)
	}

	return provider.Snapshot(ctx, channel)
}

func (r *runtimeServices) VTOSnapshot(ctx context.Context, deviceID string) ([]byte, string, error) {
	r.mu.RLock()
	provider, ok := r.vtoSnapshots[deviceID]
	r.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("snapshot provider not found for vto %q", deviceID)
	}

	return provider.Snapshot(ctx, 0)
}

func (r *runtimeServices) IPCSnapshot(ctx context.Context, deviceID string) ([]byte, string, error) {
	r.mu.RLock()
	provider, ok := r.ipcSnapshots[deviceID]
	r.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("snapshot provider not found for ipc %q", deviceID)
	}

	return provider.Snapshot(ctx, 1)
}

func (r *runtimeServices) RenderHomeAssistantCameraPackage(options ha.CameraPackageOptions) (string, error) {
	r.mu.RLock()
	nvrConfigs := make(map[string]config.DeviceConfig, len(r.nvrConfigs))
	for key, value := range r.nvrConfigs {
		nvrConfigs[key] = value
	}
	vtoConfigs := make(map[string]config.DeviceConfig, len(r.vtoConfigs))
	for key, value := range r.vtoConfigs {
		vtoConfigs[key] = value
	}
	ipcConfigs := make(map[string]config.DeviceConfig, len(r.ipcConfigs))
	for key, value := range r.ipcConfigs {
		ipcConfigs[key] = value
	}
	r.mu.RUnlock()

	return ha.RenderCameraPackage(ha.CameraPackageInput{
		Config:       r.cfg,
		ProbeResults: r.probes.List(),
		NVRConfigs:   nvrConfigs,
		VTOConfigs:   vtoConfigs,
		IPCConfigs:   ipcConfigs,
		Options:      options,
	})
}

func (r *runtimeServices) RenderHomeAssistantDashboardPackage() (string, error) {
	return ha.RenderDashboardCameraPackage(r.ListStreams(false))
}

func (r *runtimeServices) RenderHomeAssistantLovelaceDashboard() (string, error) {
	return ha.RenderLovelaceDashboard(r.ListStreams(false))
}

func (r *runtimeServices) AdminSettings() map[string]any {
	r.mu.RLock()
	cfg := r.cfg
	r.mu.RUnlock()

	return map[string]any{
		"log": map[string]any{
			"level":  cfg.Log.Level,
			"pretty": cfg.Log.Pretty,
		},
		"http": map[string]any{
			"listen_address":                 cfg.HTTP.ListenAddress,
			"metrics_path":                   cfg.HTTP.MetricsPath,
			"health_path":                    cfg.HTTP.HealthPath,
			"read_timeout":                   cfg.HTTP.ReadTimeout.String(),
			"write_timeout":                  cfg.HTTP.WriteTimeout.String(),
			"idle_timeout":                   cfg.HTTP.IdleTimeout.String(),
			"admin_rate_limit_per_minute":    cfg.HTTP.AdminRateLimitPerMinute,
			"admin_rate_limit_burst":         cfg.HTTP.AdminRateLimitBurst,
			"snapshot_rate_limit_per_minute": cfg.HTTP.SnapshotRateLimitPerMinute,
			"snapshot_rate_limit_burst":      cfg.HTTP.SnapshotRateLimitBurst,
			"media_rate_limit_per_minute":    cfg.HTTP.MediaRateLimitPerMinute,
			"media_rate_limit_burst":         cfg.HTTP.MediaRateLimitBurst,
		},
		"mqtt": map[string]any{
			"enabled":          cfg.MQTT.Enabled,
			"broker":           cfg.MQTT.Broker,
			"client_id":        cfg.MQTT.ClientID,
			"username":         cfg.MQTT.Username,
			"password":         redactedSecret(cfg.MQTT.Password),
			"topic_prefix":     cfg.MQTT.TopicPrefix,
			"discovery_prefix": cfg.MQTT.DiscoveryPrefix,
			"qos":              cfg.MQTT.QoS,
			"retain":           cfg.MQTT.Retain,
			"clean_session":    cfg.MQTT.CleanSession,
			"keep_alive":       cfg.MQTT.KeepAlive.String(),
			"connect_timeout":  cfg.MQTT.ConnectTimeout.String(),
			"publish_timeout":  cfg.MQTT.PublishTimeout.String(),
		},
		"home_assistant": map[string]any{
			"enabled":         cfg.HomeAssistant.Enabled,
			"node_id":         cfg.HomeAssistant.NodeID,
			"entity_mode":     cfg.HomeAssistant.EntityMode,
			"public_base_url": cfg.HomeAssistant.PublicBaseURL,
			"api_base_url":    cfg.HomeAssistant.APIBaseURL,
			"access_token":    redactedSecret(cfg.HomeAssistant.AccessToken),
			"request_timeout": cfg.HomeAssistant.RequestTimeout.String(),
		},
		"media": map[string]any{
			"enabled":               cfg.Media.Enabled,
			"ffmpeg_path":           cfg.Media.FFmpegPath,
			"idle_timeout":          cfg.Media.IdleTimeout.String(),
			"start_timeout":         cfg.Media.StartTimeout.String(),
			"max_workers":           cfg.Media.MaxWorkers,
			"frame_rate":            cfg.Media.FrameRate,
			"jpeg_quality":          cfg.Media.JPEGQuality,
			"threads":               cfg.Media.Threads,
			"scale_width":           cfg.Media.ScaleWidth,
			"scale_height":          cfg.Media.ScaleHeight,
			"read_buffer_size":      cfg.Media.ReadBufferSize,
			"hls_segment_time":      cfg.Media.HLSSegmentTime.String(),
			"hls_list_size":         cfg.Media.HLSListSize,
			"hwaccel_args":          append([]string(nil), cfg.Media.HWAccelArgs...),
			"webrtc_ice_servers":    redactICEServers(cfg.Media.WebRTCICEServers),
			"webrtc_uplink_targets": append([]string(nil), cfg.Media.WebRTCUplinkTargets...),
		},
		"state_store": map[string]any{
			"enabled":        cfg.StateStore.Enabled,
			"path":           cfg.StateStore.Path,
			"flush_interval": cfg.StateStore.FlushInterval.String(),
		},
		"devices": map[string]any{
			"nvr": redactDeviceConfigs(cfg.Devices.NVR),
			"vto": redactDeviceConfigs(cfg.Devices.VTO),
			"ipc": redactDeviceConfigs(cfg.Devices.IPC),
		},
	}
}

func (r *runtimeServices) ListStreams(includeCredentials bool) []streams.Entry {
	r.mu.RLock()
	mediaReader := r.media
	nvrConfigs := make(map[string]config.DeviceConfig, len(r.nvrConfigs))
	for key, value := range r.nvrConfigs {
		nvrConfigs[key] = value
	}
	vtoConfigs := make(map[string]config.DeviceConfig, len(r.vtoConfigs))
	for key, value := range r.vtoConfigs {
		vtoConfigs[key] = value
	}
	ipcConfigs := make(map[string]config.DeviceConfig, len(r.ipcConfigs))
	for key, value := range r.ipcConfigs {
		ipcConfigs[key] = value
	}
	r.mu.RUnlock()

	intercomStatuses := make(map[string]streams.RuntimeIntercomStatus, len(vtoConfigs))
	if mediaReader != nil {
		for deviceID := range vtoConfigs {
			status := mediaReader.IntercomStatus(deviceID)
			intercomStatuses[deviceID] = streams.RuntimeIntercomStatus{
				Active:                 status.Active,
				SessionCount:           status.SessionCount,
				ExternalUplinkEnabled:  status.ExternalUplinkEnabled,
				UplinkActive:           status.UplinkActive,
				UplinkCodec:            status.UplinkCodec,
				UplinkPackets:          status.UplinkPackets,
				UplinkTargetCount:      status.UplinkTargetCount,
				UplinkForwardedPackets: status.UplinkForwardedPackets,
				UplinkForwardErrors:    status.UplinkForwardErrors,
			}
		}
	}

	return streams.BuildCatalog(streams.CatalogInput{
		Config:             r.cfg,
		ProbeResults:       r.probes.List(),
		NVRConfigs:         nvrConfigs,
		VTOConfigs:         vtoConfigs,
		IPCConfigs:         ipcConfigs,
		IntercomStatuses:   intercomStatuses,
		IncludeCredentials: includeCredentials,
	})
}

func (r *runtimeServices) GetStream(streamID string, profileName string, includeCredentials bool) (streams.Entry, streams.Profile, bool) {
	entries := r.ListStreams(includeCredentials)
	for _, entry := range entries {
		if entry.ID != streamID {
			continue
		}
		profile, ok := entry.Profiles[profileName]
		if ok {
			return entry, profile, true
		}
		if profileName == "" {
			profile, ok = entry.Profiles["stable"]
			return entry, profile, ok
		}
		return streams.Entry{}, streams.Profile{}, false
	}
	return streams.Entry{}, streams.Profile{}, false
}

func (r *runtimeServices) ListONVIFProvisionTargets(deviceIDs []string, force bool) []haapi.ONVIFProvisionTarget {
	allowedIDs := make(map[string]struct{}, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		if deviceID == "" {
			continue
		}
		allowedIDs[deviceID] = struct{}{}
	}

	recommendations := make(map[string]bool)
	reasons := make(map[string]string)
	for _, entry := range r.ListStreams(false) {
		rootID := entry.RootDeviceID
		if rootID == "" {
			rootID = entry.SourceDeviceID
		}
		if rootID == "" {
			rootID = entry.ID
		}
		if rootID == "" {
			continue
		}
		if _, ok := reasons[rootID]; !ok && entry.RecommendedHAReason != "" {
			reasons[rootID] = entry.RecommendedHAReason
		}
		if entry.RecommendedHAIntegration == "onvif" || entry.ONVIFH264Available {
			recommendations[rootID] = true
			reasons[rootID] = "onvif_h264_profile_available"
		}
	}

	r.mu.RLock()
	nvrConfigs := cloneDeviceConfigMap(r.nvrConfigs)
	vtoConfigs := cloneDeviceConfigMap(r.vtoConfigs)
	ipcConfigs := cloneDeviceConfigMap(r.ipcConfigs)
	r.mu.RUnlock()

	targets := make([]haapi.ONVIFProvisionTarget, 0, len(nvrConfigs)+len(vtoConfigs)+len(ipcConfigs))
	targets = append(targets, buildONVIFTargets(nvrConfigs, dahua.DeviceKindNVR, allowedIDs, force, recommendations, reasons)...)
	targets = append(targets, buildONVIFTargets(vtoConfigs, dahua.DeviceKindVTO, allowedIDs, force, recommendations, reasons)...)
	targets = append(targets, buildONVIFTargets(ipcConfigs, dahua.DeviceKindIPC, allowedIDs, force, recommendations, reasons)...)
	return targets
}

func cloneDeviceConfigMap(src map[string]config.DeviceConfig) map[string]config.DeviceConfig {
	cloned := make(map[string]config.DeviceConfig, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

func buildONVIFTargets(
	items map[string]config.DeviceConfig,
	kind dahua.DeviceKind,
	allowedIDs map[string]struct{},
	force bool,
	recommendations map[string]bool,
	reasons map[string]string,
) []haapi.ONVIFProvisionTarget {
	targets := make([]haapi.ONVIFProvisionTarget, 0, len(items))
	for deviceID, deviceCfg := range items {
		if len(allowedIDs) > 0 {
			if _, ok := allowedIDs[deviceID]; !ok {
				continue
			}
		}
		if !deviceCfg.ONVIFEnabledValue() {
			continue
		}
		if !force && !recommendations[deviceID] {
			continue
		}

		host, port, err := onvifHostPort(deviceCfg)
		if err != nil {
			continue
		}

		target := haapi.ONVIFProvisionTarget{
			DeviceID:   deviceID,
			DeviceKind: kind,
			Name:       deviceCfg.Name,
			Host:       host,
			Port:       port,
			Username:   deviceCfg.ONVIFUsernameValue(),
			Password:   deviceCfg.ONVIFPasswordValue(),
			Reason:     reasons[deviceID],
		}
		targets = append(targets, target)
	}
	return targets
}

func onvifHostPort(deviceCfg config.DeviceConfig) (string, int, error) {
	raw := deviceCfg.OnvifServiceURL
	if raw == "" {
		raw = deviceCfg.BaseURL
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", 0, err
	}
	if parsed.Hostname() == "" {
		return "", 0, fmt.Errorf("missing host in %q", raw)
	}

	port := 0
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil {
			return "", 0, err
		}
	} else {
		switch parsed.Scheme {
		case "https":
			port = 443
		default:
			port = 80
		}
	}

	return parsed.Hostname(), port, nil
}

func redactDeviceConfigs(items []config.DeviceConfig) []map[string]any {
	redacted := make([]map[string]any, 0, len(items))
	for _, item := range items {
		redacted = append(redacted, map[string]any{
			"id":                item.ID,
			"name":              item.Name,
			"manufacturer":      item.Manufacturer,
			"model":             item.Model,
			"base_url":          item.BaseURL,
			"username":          item.Username,
			"password":          redactedSecret(item.Password),
			"onvif_enabled":     item.ONVIFEnabledValue(),
			"onvif_username":    item.ONVIFUsernameValue(),
			"onvif_password":    redactedSecret(item.ONVIFPasswordValue()),
			"onvif_service_url": item.OnvifServiceURL,
			"poll_interval":     item.PollInterval.String(),
			"request_timeout":   item.RequestTimeout.String(),
			"insecure_skip_tls": item.InsecureSkipTLS,
			"enabled":           item.EnabledValue(),
		})
	}
	return redacted
}

func redactICEServers(items []config.WebRTCICEServerConfig) []map[string]any {
	redacted := make([]map[string]any, 0, len(items))
	for _, item := range items {
		redacted = append(redacted, map[string]any{
			"urls":       append([]string(nil), item.URLs...),
			"username":   item.Username,
			"credential": redactedSecret(item.Credential),
		})
	}
	return redacted
}

func redactedSecret(value string) string {
	if value == "" {
		return ""
	}
	return "[redacted]"
}
