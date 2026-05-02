package app

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	archiveapi "RCooLeR/DahuaBridge/internal/archive"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"
	"RCooLeR/DahuaBridge/internal/streams"
)

type runtimeServices struct {
	mu               sync.RWMutex
	cfg              config.Config
	probes           *store.ProbeStore
	media            runtimeMediaReader
	archive          runtimeArchiveReader
	nvrSnapshots     map[string]dahua.SnapshotProvider
	nvrDownloads     map[string]dahua.NVRRecordingDownloader
	nvrClipDownloads map[string]dahua.NVRRecordingClipDownloader
	nvrRecordings    map[string]dahua.NVRRecordingSearcher
	playback         map[string]playbackSession
	nvrConfigs       map[string]config.DeviceConfig
	vtoSnapshots     map[string]dahua.SnapshotProvider
	vtoConfigs       map[string]config.DeviceConfig
	ipcSnapshots     map[string]dahua.SnapshotProvider
	ipcConfigs       map[string]config.DeviceConfig
	snapshotCache    map[string]cachedSnapshot
	recordingCache   map[string]cachedRecordingSearch
	snapshotFlight   map[string]*snapshotFlight
	recordingFlight  map[string]*recordingSearchFlight
}

type runtimeMediaReader interface {
	IntercomStatus(string) media.IntercomStatus
	CaptureFrame(context.Context, string, string, int) ([]byte, string, error)
	FindClips(media.ClipQuery) ([]media.ClipInfo, error)
	GetClip(string) (media.ClipInfo, error)
	ActiveClip(string) (media.ClipInfo, bool)
}

type runtimeArchiveReader interface {
	SearchRecordings(
		context.Context,
		string,
		dahua.NVRRecordingQuery,
		func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error),
	) (dahua.NVRRecordingSearchResult, error)
	EnrichRecordings(context.Context, string, *dahua.NVRRecordingSearchResult, archiveapi.ClipFinder) error
	TrackClipExport(context.Context, string, dahua.NVRPlaybackSessionRequest, media.ClipInfo) error
}

type cachedSnapshot struct {
	body        []byte
	contentType string
	expiresAt   time.Time
}

type cachedRecordingSearch struct {
	result    dahua.NVRRecordingSearchResult
	expiresAt time.Time
}

type snapshotFlight struct {
	body        []byte
	contentType string
	err         error
	done        chan struct{}
}

type recordingSearchFlight struct {
	result dahua.NVRRecordingSearchResult
	err    error
	done   chan struct{}
}

const snapshotCacheTTL = 2 * time.Second
const recordingSearchCacheTTL = 5 * time.Second
const bridgeRecordingTimeLayout = "2006-01-02 15:04:05"

func newRuntimeServices(cfg config.Config, probes *store.ProbeStore) *runtimeServices {
	return &runtimeServices{
		cfg:              cfg,
		probes:           probes,
		nvrSnapshots:     make(map[string]dahua.SnapshotProvider),
		nvrDownloads:     make(map[string]dahua.NVRRecordingDownloader),
		nvrClipDownloads: make(map[string]dahua.NVRRecordingClipDownloader),
		nvrRecordings:    make(map[string]dahua.NVRRecordingSearcher),
		playback:         make(map[string]playbackSession),
		nvrConfigs:       make(map[string]config.DeviceConfig),
		vtoSnapshots:     make(map[string]dahua.SnapshotProvider),
		vtoConfigs:       make(map[string]config.DeviceConfig),
		ipcSnapshots:     make(map[string]dahua.SnapshotProvider),
		ipcConfigs:       make(map[string]config.DeviceConfig),
		snapshotCache:    make(map[string]cachedSnapshot),
		recordingCache:   make(map[string]cachedRecordingSearch),
		snapshotFlight:   make(map[string]*snapshotFlight),
		recordingFlight:  make(map[string]*recordingSearchFlight),
	}
}

func (r *runtimeServices) AttachMedia(reader runtimeMediaReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.media = reader
}

func (r *runtimeServices) AttachArchive(reader runtimeArchiveReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.archive = reader
}

func (r *runtimeServices) RegisterNVR(deviceID string, provider dahua.SnapshotProvider, recordings dahua.NVRRecordingSearcher, cfg config.DeviceConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nvrSnapshots[deviceID] = provider
	if downloader, ok := provider.(dahua.NVRRecordingDownloader); ok && downloader != nil {
		r.nvrDownloads[deviceID] = downloader
	}
	if downloader, ok := provider.(dahua.NVRRecordingClipDownloader); ok && downloader != nil {
		r.nvrClipDownloads[deviceID] = downloader
	}
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
	flight, owner := r.beginSnapshotFlight(cacheKey)
	if !owner {
		return waitForSnapshotFlight(ctx, flight)
	}

	r.mu.RLock()
	provider, ok := r.nvrSnapshots[deviceID]
	mediaReader := r.media
	r.mu.RUnlock()
	var (
		body        []byte
		contentType string
		err         error
	)
	if mediaReader != nil {
		if body, contentType, err = r.captureSnapshotFromStream(ctx, mediaReader, deviceID, channel, dahua.DeviceKindNVRChannel); err == nil {
			r.storeSnapshot(cacheKey, body, contentType)
			r.finishSnapshotFlight(cacheKey, flight, body, contentType, nil)
			return body, contentType, nil
		}
	}
	if !ok {
		err = fmt.Errorf("snapshot provider not found for device %q", deviceID)
		r.finishSnapshotFlight(cacheKey, flight, nil, "", err)
		return nil, "", err
	}

	body, contentType, err = provider.Snapshot(ctx, channel)
	if err != nil {
		r.finishSnapshotFlight(cacheKey, flight, nil, "", err)
		return nil, "", err
	}
	r.storeSnapshot(cacheKey, body, contentType)
	r.finishSnapshotFlight(cacheKey, flight, body, contentType, nil)
	return body, contentType, nil
}

func (r *runtimeServices) NVRRecordings(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	cacheKey := recordingSearchCacheKey(deviceID, query)
	if result, ok := r.cachedRecordingSearch(cacheKey); ok {
		return result, nil
	}
	flight, owner := r.beginRecordingSearchFlight(cacheKey)
	if !owner {
		return waitForRecordingSearchFlight(ctx, flight)
	}

	r.mu.RLock()
	searcher, ok := r.nvrRecordings[deviceID]
	archiveReader := r.archive
	mediaReader := r.media
	r.mu.RUnlock()
	if !ok {
		err := fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
		r.finishRecordingSearchFlight(cacheKey, flight, dahua.NVRRecordingSearchResult{}, err)
		return dahua.NVRRecordingSearchResult{}, err
	}

	searchLive := func(callCtx context.Context, _ string, callQuery dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
		return searcher.FindRecordings(callCtx, callQuery)
	}

	var (
		result dahua.NVRRecordingSearchResult
		err    error
	)
	if archiveReader != nil {
		result, err = archiveReader.SearchRecordings(ctx, deviceID, query, searchLive)
	} else {
		result, err = searchLive(ctx, deviceID, query)
	}
	if err != nil {
		r.finishRecordingSearchFlight(cacheKey, flight, dahua.NVRRecordingSearchResult{}, err)
		return dahua.NVRRecordingSearchResult{}, err
	}
	if result.DeviceID == "" {
		result.DeviceID = deviceID
	}
	if result.Items == nil {
		result.Items = []dahua.NVRRecording{}
	}
	if archiveReader != nil {
		if enrichErr := archiveReader.EnrichRecordings(ctx, deviceID, &result, mediaReader); enrichErr != nil {
			r.finishRecordingSearchFlight(cacheKey, flight, dahua.NVRRecordingSearchResult{}, enrichErr)
			return dahua.NVRRecordingSearchResult{}, enrichErr
		}
	}
	r.storeRecordingSearch(cacheKey, result)
	r.finishRecordingSearchFlight(cacheKey, flight, result, nil)
	return result, nil
}

func isAllRecordingEventFilter(eventCode string) bool {
	switch strings.ToLower(strings.TrimSpace(eventCode)) {
	case "", "*", "all", "any", "__all__":
		return true
	default:
		return false
	}
}

func (r *runtimeServices) NVRDownloadRecording(ctx context.Context, deviceID string, filePath string) (dahua.NVRRecordingDownload, error) {
	r.mu.RLock()
	downloader, ok := r.nvrDownloads[deviceID]
	r.mu.RUnlock()
	if !ok {
		return dahua.NVRRecordingDownload{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return downloader.DownloadRecording(ctx, filePath)
}

func (r *runtimeServices) NVRDownloadRecordingClip(ctx context.Context, deviceID string, request dahua.NVRRecordingClipRequest) (dahua.NVRRecordingDownload, error) {
	r.mu.RLock()
	downloader, ok := r.nvrClipDownloads[deviceID]
	r.mu.RUnlock()
	if !ok {
		return dahua.NVRRecordingDownload{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}
	return downloader.DownloadRecordingClip(ctx, request)
}

func (r *runtimeServices) TrackNVRArchiveClip(ctx context.Context, deviceID string, request dahua.NVRPlaybackSessionRequest, clip media.ClipInfo) error {
	r.mu.RLock()
	archiveReader := r.archive
	r.mu.RUnlock()
	if archiveReader == nil {
		return nil
	}
	return archiveReader.TrackClipExport(ctx, deviceID, request, clip)
}

func (r *runtimeServices) VTOSnapshot(ctx context.Context, deviceID string) ([]byte, string, error) {
	cacheKey := snapshotCacheKey("vto", deviceID, 0)
	if body, contentType, ok := r.cachedSnapshot(cacheKey); ok {
		return body, contentType, nil
	}
	flight, owner := r.beginSnapshotFlight(cacheKey)
	if !owner {
		return waitForSnapshotFlight(ctx, flight)
	}

	r.mu.RLock()
	provider, ok := r.vtoSnapshots[deviceID]
	mediaReader := r.media
	r.mu.RUnlock()
	var (
		body        []byte
		contentType string
		err         error
	)
	if mediaReader != nil {
		if body, contentType, err = r.captureSnapshotFromStream(ctx, mediaReader, deviceID, 1, dahua.DeviceKindVTO); err == nil {
			r.storeSnapshot(cacheKey, body, contentType)
			r.finishSnapshotFlight(cacheKey, flight, body, contentType, nil)
			return body, contentType, nil
		}
	}
	if !ok {
		err = fmt.Errorf("snapshot provider not found for vto %q", deviceID)
		r.finishSnapshotFlight(cacheKey, flight, nil, "", err)
		return nil, "", err
	}

	body, contentType, err = provider.Snapshot(ctx, 0)
	if err != nil {
		r.finishSnapshotFlight(cacheKey, flight, nil, "", err)
		return nil, "", err
	}
	r.storeSnapshot(cacheKey, body, contentType)
	r.finishSnapshotFlight(cacheKey, flight, body, contentType, nil)
	return body, contentType, nil
}

func (r *runtimeServices) IPCSnapshot(ctx context.Context, deviceID string) ([]byte, string, error) {
	cacheKey := snapshotCacheKey("ipc", deviceID, 1)
	if body, contentType, ok := r.cachedSnapshot(cacheKey); ok {
		return body, contentType, nil
	}
	flight, owner := r.beginSnapshotFlight(cacheKey)
	if !owner {
		return waitForSnapshotFlight(ctx, flight)
	}

	r.mu.RLock()
	provider, ok := r.ipcSnapshots[deviceID]
	mediaReader := r.media
	r.mu.RUnlock()
	var (
		body        []byte
		contentType string
		err         error
	)
	if mediaReader != nil {
		if body, contentType, err = r.captureSnapshotFromStream(ctx, mediaReader, deviceID, 1, dahua.DeviceKindIPC); err == nil {
			r.storeSnapshot(cacheKey, body, contentType)
			r.finishSnapshotFlight(cacheKey, flight, body, contentType, nil)
			return body, contentType, nil
		}
	}
	if !ok {
		err = fmt.Errorf("snapshot provider not found for ipc %q", deviceID)
		r.finishSnapshotFlight(cacheKey, flight, nil, "", err)
		return nil, "", err
	}

	body, contentType, err = provider.Snapshot(ctx, 1)
	if err != nil {
		r.finishSnapshotFlight(cacheKey, flight, nil, "", err)
		return nil, "", err
	}
	r.storeSnapshot(cacheKey, body, contentType)
	r.finishSnapshotFlight(cacheKey, flight, body, contentType, nil)
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
			"clip_path":             cfg.Media.ClipPath,
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
			"hls_tmp_dir":           cfg.Media.HLSTmpDir,
			"hls_temp_path":         cfg.Media.HLSTempPath,
			"hls_keep_after_exit":   cfg.Media.HLSKeepAfterExit.String(),
			"hwaccel_args":          append([]string(nil), cfg.Media.HWAccelArgs...),
			"webrtc_ice_servers":    redactICEServers(cfg.Media.WebRTCICEServers),
			"webrtc_uplink_targets": append([]string(nil), cfg.Media.WebRTCUplinkTargets...),
		},
		"archive": map[string]any{
			"enabled":           cfg.Archive.Enabled,
			"db_path":           cfg.Archive.DBPath,
			"cache_dir":         cfg.Archive.CacheDir,
			"temp_dir":          cfg.Archive.TempDir,
			"prefetch_days":     cfg.Archive.PrefetchDays,
			"retain_days":       cfg.Archive.RetainDays,
			"max_parallel_jobs": cfg.Archive.MaxParallelJobs,
			"prefetch_smd":      cfg.Archive.PrefetchSMD,
			"prefetch_ivs":      cfg.Archive.PrefetchIVS,
			"cron":              cfg.Archive.Cron,
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

	entries := streams.BuildCatalog(streams.CatalogInput{
		Config:             r.cfg,
		ProbeResults:       r.probes.List(),
		NVRConfigs:         nvrConfigs,
		VTOConfigs:         vtoConfigs,
		IPCConfigs:         ipcConfigs,
		IntercomStatuses:   intercomStatuses,
		IncludeCredentials: includeCredentials,
	})
	if mediaReader != nil {
		for index := range entries {
			entries[index].Capture = buildCaptureSummary(r.cfg.HomeAssistant.PublicBaseURL, entries[index], mediaReader)
		}
	}
	return entries
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

func (r *runtimeServices) captureSnapshotFromStream(ctx context.Context, mediaReader runtimeMediaReader, deviceID string, channel int, kind dahua.DeviceKind) ([]byte, string, error) {
	streamID, profileName, ok := r.streamSnapshotTarget(deviceID, channel, kind)
	if !ok {
		return nil, "", fmt.Errorf("stream snapshot target not found")
	}
	return mediaReader.CaptureFrame(ctx, streamID, profileName, 0)
}

func (r *runtimeServices) streamSnapshotTarget(deviceID string, channel int, kind dahua.DeviceKind) (string, string, bool) {
	for _, entry := range r.ListStreams(false) {
		switch kind {
		case dahua.DeviceKindNVRChannel:
			if entry.DeviceKind == dahua.DeviceKindNVRChannel && entry.RootDeviceID == deviceID && entry.Channel == channel {
				return entry.ID, bestCaptureProfile(entry), true
			}
		case dahua.DeviceKindVTO:
			if entry.DeviceKind == dahua.DeviceKindVTO && entry.ID == deviceID {
				return entry.ID, bestCaptureProfile(entry), true
			}
		case dahua.DeviceKindIPC:
			if entry.DeviceKind == dahua.DeviceKindIPC && entry.ID == deviceID {
				return entry.ID, bestCaptureProfile(entry), true
			}
		}
	}
	return "", "", false
}

func bridgeClipsToRecordings(publicBaseURL string, clips []media.ClipInfo) []dahua.NVRRecording {
	items := make([]dahua.NVRRecording, 0, len(clips))
	for _, clip := range clips {
		items = append(items, dahua.NVRRecording{
			Source:         "bridge",
			Status:         string(clip.Status),
			ClipID:         clip.ID,
			StreamID:       clip.StreamID,
			DownloadURL:    buildClipDownloadURL(publicBaseURL, clip.ID),
			Channel:        clip.Channel,
			StartTime:      clip.StartedAt.Format(bridgeRecordingTimeLayout),
			EndTime:        firstNonEmptyTime(clip.EndedAt, clip.StartedAt.Add(clip.Duration), clip.StartedAt).Format(bridgeRecordingTimeLayout),
			FilePath:       clip.FileName,
			Type:           "bridge_mp4",
			VideoStream:    clip.Profile,
			LengthBytes:    clip.Bytes,
			CutLengthBytes: clip.Bytes,
		})
	}
	return items
}

func buildClipDownloadURL(publicBaseURL string, clipID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/recordings/" + url.PathEscape(clipID) + "/download"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildCaptureSummary(publicBaseURL string, entry streams.Entry, mediaReader runtimeMediaReader) *streams.CaptureSummary {
	captureProfile := bestCaptureProfile(entry)
	summary := &streams.CaptureSummary{
		SnapshotURL:       buildMediaSnapshotURL(publicBaseURL, entry.ID, captureProfile),
		StartRecordingURL: buildMediaStreamRecordingStartURL(publicBaseURL, entry.ID, captureProfile),
		RecordingsURL:     buildMediaRecordingsURL(publicBaseURL, entry),
	}
	if clip, ok := mediaReader.ActiveClip(entry.ID); ok {
		summary.Active = true
		summary.ActiveClipID = clip.ID
		summary.ActiveClipProfile = clip.Profile
		summary.ActiveClipStartedAt = clip.StartedAt.Format(time.RFC3339)
		summary.ActiveClipDownloadURL = buildClipDownloadURL(publicBaseURL, clip.ID)
		summary.StopRecordingURL = buildMediaRecordingStopURL(publicBaseURL, clip.ID)
	}
	return summary
}

func buildMediaSnapshotURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/snapshot/" + url.PathEscape(streamID)
	if strings.TrimSpace(profile) != "" {
		path += "?profile=" + url.QueryEscape(profile)
	}
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildMediaStreamRecordingStartURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/streams/" + url.PathEscape(streamID) + "/recordings"
	if strings.TrimSpace(profile) != "" {
		path += "?profile=" + url.QueryEscape(profile)
	}
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildMediaRecordingsURL(publicBaseURL string, entry streams.Entry) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/recordings"
	query := url.Values{}
	if strings.TrimSpace(entry.RootDeviceID) != "" && entry.Channel > 0 {
		query.Set("root_device_id", entry.RootDeviceID)
		query.Set("channel", fmt.Sprintf("%d", entry.Channel))
	} else {
		query.Set("stream_id", entry.ID)
	}
	path += "?" + query.Encode()
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildMediaRecordingStopURL(publicBaseURL string, clipID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/recordings/" + url.PathEscape(clipID) + "/stop"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func firstNonEmptyProfile(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "stable"
}

func bestCaptureProfile(entry streams.Entry) string {
	if len(entry.Profiles) == 0 {
		return firstNonEmptyProfile(entry.RecommendedProfile, "quality", "default", "stable", "substream")
	}

	bestName := ""
	bestArea := -1
	bestRank := 100

	consider := func(name string, profile streams.Profile) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		area := profileArea(profile)
		rank := captureProfileRank(name, entry.RecommendedProfile)
		if bestName == "" || area > bestArea || (area == bestArea && rank < bestRank) {
			bestName = name
			bestArea = area
			bestRank = rank
		}
	}

	for _, name := range []string{"quality", "default", "stable", "substream"} {
		if profile, ok := entry.Profiles[name]; ok {
			consider(name, profile)
		}
	}

	extraNames := make([]string, 0, len(entry.Profiles))
	for name := range entry.Profiles {
		switch name {
		case "quality", "default", "stable", "substream":
			continue
		default:
			extraNames = append(extraNames, name)
		}
	}
	sort.Strings(extraNames)
	for _, name := range extraNames {
		consider(name, entry.Profiles[name])
	}

	return firstNonEmptyProfile(bestName, entry.RecommendedProfile, "quality", "default", "stable", "substream")
}

func profileArea(profile streams.Profile) int {
	if profile.SourceWidth <= 0 || profile.SourceHeight <= 0 {
		return 0
	}
	return profile.SourceWidth * profile.SourceHeight
}

func captureProfileRank(name string, recommended string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "quality":
		return 0
	case "default":
		return 1
	case "stable":
		return 2
	case "substream":
		return 3
	}
	if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(recommended)) {
		return 4
	}
	return 5
}

func firstNonEmptyTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
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

func recordingSearchCacheKey(deviceID string, query dahua.NVRRecordingQuery) string {
	return strings.Join([]string{
		strings.TrimSpace(deviceID),
		strconv.Itoa(query.Channel),
		query.StartTime.UTC().Format(time.RFC3339Nano),
		query.EndTime.UTC().Format(time.RFC3339Nano),
		strconv.Itoa(query.Limit),
		strings.TrimSpace(query.EventCode),
		strconv.FormatBool(query.EventOnly),
	}, "|")
}

func cloneRecordingSearchResult(result dahua.NVRRecordingSearchResult) dahua.NVRRecordingSearchResult {
	cloned := result
	if result.Items != nil {
		cloned.Items = append([]dahua.NVRRecording(nil), result.Items...)
	}
	return cloned
}

func (r *runtimeServices) cachedRecordingSearch(cacheKey string) (dahua.NVRRecordingSearchResult, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.recordingCache[cacheKey]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(r.recordingCache, cacheKey)
		return dahua.NVRRecordingSearchResult{}, false
	}
	return cloneRecordingSearchResult(entry.result), true
}

func (r *runtimeServices) storeRecordingSearch(cacheKey string, result dahua.NVRRecordingSearchResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.recordingCache[cacheKey] = cachedRecordingSearch{
		result:    cloneRecordingSearchResult(result),
		expiresAt: time.Now().Add(recordingSearchCacheTTL),
	}
}

func (r *runtimeServices) beginSnapshotFlight(cacheKey string) (*snapshotFlight, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if flight, ok := r.snapshotFlight[cacheKey]; ok {
		return flight, false
	}
	flight := &snapshotFlight{done: make(chan struct{})}
	r.snapshotFlight[cacheKey] = flight
	return flight, true
}

func (r *runtimeServices) finishSnapshotFlight(cacheKey string, flight *snapshotFlight, body []byte, contentType string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	flight.body = append([]byte(nil), body...)
	flight.contentType = contentType
	flight.err = err
	delete(r.snapshotFlight, cacheKey)
	close(flight.done)
}

func waitForSnapshotFlight(ctx context.Context, flight *snapshotFlight) ([]byte, string, error) {
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-flight.done:
		if flight.err != nil {
			return nil, "", flight.err
		}
		return append([]byte(nil), flight.body...), flight.contentType, nil
	}
}

func (r *runtimeServices) beginRecordingSearchFlight(cacheKey string) (*recordingSearchFlight, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if flight, ok := r.recordingFlight[cacheKey]; ok {
		return flight, false
	}
	flight := &recordingSearchFlight{done: make(chan struct{})}
	r.recordingFlight[cacheKey] = flight
	return flight, true
}

func (r *runtimeServices) finishRecordingSearchFlight(cacheKey string, flight *recordingSearchFlight, result dahua.NVRRecordingSearchResult, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	flight.result = cloneRecordingSearchResult(result)
	flight.err = err
	delete(r.recordingFlight, cacheKey)
	close(flight.done)
}

func waitForRecordingSearchFlight(ctx context.Context, flight *recordingSearchFlight) (dahua.NVRRecordingSearchResult, error) {
	select {
	case <-ctx.Done():
		return dahua.NVRRecordingSearchResult{}, ctx.Err()
	case <-flight.done:
		if flight.err != nil {
			return dahua.NVRRecordingSearchResult{}, flight.err
		}
		return cloneRecordingSearchResult(flight.result), nil
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
