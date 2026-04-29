package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"
	"RCooLeR/DahuaBridge/internal/streams"
)

type runtimeServices struct {
	mu            sync.RWMutex
	cfg           config.Config
	probes        *store.ProbeStore
	media         intercomStatusReader
	nvrSnapshots  map[string]dahua.SnapshotProvider
	nvrRecordings map[string]dahua.NVRRecordingSearcher
	playback      map[string]playbackSession
	nvrConfigs    map[string]config.DeviceConfig
	vtoSnapshots  map[string]dahua.SnapshotProvider
	vtoConfigs    map[string]config.DeviceConfig
	ipcSnapshots  map[string]dahua.SnapshotProvider
	ipcConfigs    map[string]config.DeviceConfig
	snapshotCache map[string]cachedSnapshot
}

type intercomStatusReader interface {
	IntercomStatus(string) media.IntercomStatus
}

type cachedSnapshot struct {
	body        []byte
	contentType string
	expiresAt   time.Time
}

const snapshotCacheTTL = 2 * time.Second

func newRuntimeServices(cfg config.Config, probes *store.ProbeStore) *runtimeServices {
	return &runtimeServices{
		cfg:           cfg,
		probes:        probes,
		nvrSnapshots:  make(map[string]dahua.SnapshotProvider),
		nvrRecordings: make(map[string]dahua.NVRRecordingSearcher),
		playback:      make(map[string]playbackSession),
		nvrConfigs:    make(map[string]config.DeviceConfig),
		vtoSnapshots:  make(map[string]dahua.SnapshotProvider),
		vtoConfigs:    make(map[string]config.DeviceConfig),
		ipcSnapshots:  make(map[string]dahua.SnapshotProvider),
		ipcConfigs:    make(map[string]config.DeviceConfig),
		snapshotCache: make(map[string]cachedSnapshot),
	}
}

func (r *runtimeServices) AttachMedia(reader intercomStatusReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.media = reader
}

func (r *runtimeServices) RegisterNVR(deviceID string, provider dahua.SnapshotProvider, recordings dahua.NVRRecordingSearcher, cfg config.DeviceConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nvrSnapshots[deviceID] = provider
	if recordings != nil {
		r.nvrRecordings[deviceID] = recordings
	}
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
	cacheKey := snapshotCacheKey("nvr", deviceID, channel)
	if body, contentType, ok := r.cachedSnapshot(cacheKey); ok {
		return body, contentType, nil
	}

	r.mu.RLock()
	provider, ok := r.nvrSnapshots[deviceID]
	r.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("snapshot provider not found for device %q", deviceID)
	}

	body, contentType, err := provider.Snapshot(ctx, channel)
	if err != nil {
		return nil, "", err
	}
	r.storeSnapshot(cacheKey, body, contentType)
	return body, contentType, nil
}

func (r *runtimeServices) NVRRecordings(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	r.mu.RLock()
	searcher, ok := r.nvrRecordings[deviceID]
	r.mu.RUnlock()
	if !ok {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}

	result, err := searcher.FindRecordings(ctx, query)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, err
	}
	if result.DeviceID == "" {
		result.DeviceID = deviceID
	}
	return result, nil
}

func (r *runtimeServices) VTOSnapshot(ctx context.Context, deviceID string) ([]byte, string, error) {
	cacheKey := snapshotCacheKey("vto", deviceID, 0)
	if body, contentType, ok := r.cachedSnapshot(cacheKey); ok {
		return body, contentType, nil
	}

	r.mu.RLock()
	provider, ok := r.vtoSnapshots[deviceID]
	r.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("snapshot provider not found for vto %q", deviceID)
	}

	body, contentType, err := provider.Snapshot(ctx, 0)
	if err != nil {
		return nil, "", err
	}
	r.storeSnapshot(cacheKey, body, contentType)
	return body, contentType, nil
}

func (r *runtimeServices) IPCSnapshot(ctx context.Context, deviceID string) ([]byte, string, error) {
	cacheKey := snapshotCacheKey("ipc", deviceID, 1)
	if body, contentType, ok := r.cachedSnapshot(cacheKey); ok {
		return body, contentType, nil
	}

	r.mu.RLock()
	provider, ok := r.ipcSnapshots[deviceID]
	r.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("snapshot provider not found for ipc %q", deviceID)
	}

	body, contentType, err := provider.Snapshot(ctx, 1)
	if err != nil {
		return nil, "", err
	}
	r.storeSnapshot(cacheKey, body, contentType)
	return body, contentType, nil
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
		"home_assistant": map[string]any{
			"enabled":         cfg.HomeAssistant.Enabled,
			"node_id":         cfg.HomeAssistant.NodeID,
			"public_base_url": cfg.HomeAssistant.PublicBaseURL,
		},
		"media": map[string]any{
			"enabled":               cfg.Media.Enabled,
			"ffmpeg_path":           cfg.Media.FFmpegPath,
			"video_encoder":         cfg.Media.VideoEncoder,
			"input_preset":          cfg.Media.InputPreset,
			"idle_timeout":          cfg.Media.IdleTimeout.String(),
			"start_timeout":         cfg.Media.StartTimeout.String(),
			"max_workers":           cfg.Media.MaxWorkers,
			"frame_rate":            cfg.Media.FrameRate,
			"stable_frame_rate":     cfg.Media.StableFrameRate,
			"substream_frame_rate":  cfg.Media.SubstreamFrameRate,
			"jpeg_quality":          cfg.Media.JPEGQuality,
			"threads":               cfg.Media.Threads,
			"scale_width":           cfg.Media.ScaleWidth,
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
	if entry, profile, ok := r.getPlaybackStream(streamID, profileName, includeCredentials); ok {
		return entry, profile, true
	}

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

func snapshotCacheKey(kind string, deviceID string, channel int) string {
	return fmt.Sprintf("%s:%s:%d", kind, deviceID, channel)
}

func (r *runtimeServices) cachedSnapshot(cacheKey string) ([]byte, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.snapshotCache[cacheKey]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, "", false
	}

	return append([]byte(nil), entry.body...), entry.contentType, true
}

func (r *runtimeServices) storeSnapshot(cacheKey string, body []byte, contentType string) {
	if len(body) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshotCache[cacheKey] = cachedSnapshot{
		body:        append([]byte(nil), body...),
		contentType: contentType,
		expiresAt:   time.Now().Add(snapshotCacheTTL),
	}
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
