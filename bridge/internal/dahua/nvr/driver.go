package nvr

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
	dahuarpc "RCooLeR/DahuaBridge/internal/dahua/rpc"
	dahuartsp "RCooLeR/DahuaBridge/internal/dahua/rtsp"
	"RCooLeR/DahuaBridge/internal/imou"
	"RCooLeR/DahuaBridge/internal/onvif"
	"github.com/rs/zerolog"
)

const (
	recordingTimeLayout    = "2006-01-02 15:04:05"
	recordingRPCTimeLayout = "2006-1-2 15:04:05"
)

var (
	recordingItemPattern = regexp.MustCompile(`^items\[(\d+)\]\.(Channel|Cluster|CutLength|Disk|EndTime|FilePath|Length|Partition|StartTime|Type|VideoStream)$`)
	recordingFlagPattern = regexp.MustCompile(`^items\[(\d+)\]\.Flags\[(\d+)\]$`)
)

type Driver struct {
	cfgMu   sync.RWMutex
	cfg     config.DeviceConfig
	client  *cgi.Client
	rpc     *dahuarpc.Client
	rtsp    *dahuartsp.Checker
	onvif   *onvif.Client
	imou    imou.Service
	imouCfg config.ImouConfig
	logger  zerolog.Logger

	mu               sync.RWMutex
	cachedInventory  *inventorySnapshot
	inventoryExpires time.Time
}

type inventorySnapshot struct {
	Channels []channelInventory
}

type channelInventory struct {
	Index          int
	Name           string
	MainResolution string
	MainCodec      string
	AudioCodec     string
	AudioEnabled   bool
	AudioKnown     bool
	SubResolution  string
	SubCodec       string
}

type diskInventory struct {
	Index      int
	Name       string
	State      string
	TotalBytes float64
	UsedBytes  float64
	IsError    bool
}

type diskSummary struct {
	DiskFault        bool
	DiskErrorCount   int
	DiskHealthyCount int
	TotalBytes       float64
	UsedBytes        float64
	FreeBytes        float64
	UsedPercent      float64
}

var (
	channelNamePattern            = regexp.MustCompile(`^table\.ChannelTitle\[(\d+)\]\.Name$`)
	encodeResolutionPattern       = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.(MainFormat|ExtraFormat)\[0\]\.Video\.resolution$`)
	encodeCompressionPattern      = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.(MainFormat|ExtraFormat)\[0\]\.Video\.Compression$`)
	encodeAudioCompressionPattern = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.MainFormat\[\d+\]\.Audio\.Compression$`)
	encodeAudioEnablePattern      = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.MainFormat\[\d+\]\.AudioEnable$`)
	storageNamePattern            = regexp.MustCompile(`^list\.info\[(\d+)\]\.Name$`)
	storageStatePattern           = regexp.MustCompile(`^list\.info\[(\d+)\]\.State$`)
	storageDetailTotal            = regexp.MustCompile(`^list\.info\[(\d+)\]\.Detail\[(\d+)\]\.TotalBytes$`)
	storageDetailUsed             = regexp.MustCompile(`^list\.info\[(\d+)\]\.Detail\[(\d+)\]\.UsedBytes$`)
	storageDetailError            = regexp.MustCompile(`^list\.info\[(\d+)\]\.Detail\[(\d+)\]\.IsError$`)
	placeholderChannelNamePattern = regexp.MustCompile(`(?i)^\s*(channel|канал)\s*0*(\d+)\s*$`)
)

func New(cfg config.DeviceConfig, imouCfg config.ImouConfig, imouClient imou.Service, logger zerolog.Logger, client *cgi.Client) *Driver {
	return &Driver{
		cfg:     cfg,
		client:  client,
		rpc:     dahuarpc.New(cfg),
		rtsp:    dahuartsp.NewChecker(cfg),
		onvif:   onvif.New(cfg),
		imou:    imouClient,
		imouCfg: imouCfg,
		logger:  logger.With().Str("device_id", cfg.ID).Str("device_type", string(dahua.DeviceKindNVR)).Logger(),
	}
}

func (d *Driver) ID() string {
	d.cfgMu.RLock()
	defer d.cfgMu.RUnlock()
	return d.cfg.ID
}

func (d *Driver) Kind() dahua.DeviceKind {
	return dahua.DeviceKindNVR
}

func (d *Driver) PollInterval() time.Duration {
	d.cfgMu.RLock()
	defer d.cfgMu.RUnlock()
	return d.cfg.PollInterval
}

func (d *Driver) Probe(ctx context.Context) (*dahua.ProbeResult, error) {
	cfg := d.currentConfig()
	systemInfo, err := d.client.GetKeyValues(ctx, "/cgi-bin/magicBox.cgi", url.Values{
		"action": []string{"getSystemInfo"},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch system info: %w", err)
	}

	machineName, err := d.client.GetKeyValues(ctx, "/cgi-bin/magicBox.cgi", url.Values{
		"action": []string{"getMachineName"},
	})
	if err != nil {
		d.logger.Warn().Err(err).Msg("machine name probe failed")
	}

	softwareVersion, err := d.client.GetText(ctx, "/cgi-bin/magicBox.cgi", url.Values{
		"action": []string{"getSoftwareVersion"},
	})
	if err != nil {
		d.logger.Warn().Err(err).Msg("software version probe failed")
	}

	firmware, buildDate := parseSoftwareVersion(softwareVersion)
	name := firstNonEmpty(machineName["name"], cfg.Name, cfg.ID)
	channels, inventoryErr := d.loadInventory(ctx)
	if inventoryErr != nil {
		d.logger.Warn().Err(inventoryErr).Msg("inventory probe failed")
	}

	disks, diskErr := d.loadDisks(ctx)
	if diskErr != nil {
		d.logger.Warn().Err(diskErr).Msg("disk probe failed")
	}
	diskHealth := summarizeDisks(disks)

	raw := map[string]string{
		"deviceType":   systemInfo["deviceType"],
		"processor":    systemInfo["processor"],
		"serialNumber": systemInfo["serialNumber"],
		"updateSerial": systemInfo["updateSerial"],
		"name":         name,
		"version":      firmware,
		"build_date":   buildDate,
	}

	var onvifDiscovery *onvif.Discovery
	if d.onvif.Enabled() {
		discovery, err := d.onvif.Discover(ctx)
		if err != nil {
			d.logger.Warn().Err(err).Msg("onvif probe failed")
		} else {
			onvifDiscovery = discovery
			raw["onvif_device_service_url"] = discovery.DeviceServiceURL
			raw["onvif_media_service_url"] = discovery.MediaServiceURL
			raw["onvif_h264_profile_count"] = strconv.Itoa(discovery.H264ProfileCount())
		}
	}

	children := make([]dahua.Device, 0, len(channels)+len(disks))
	states := map[string]dahua.DeviceState{
		cfg.ID: {
			Available: true,
			Info: map[string]any{
				"name":               name,
				"serial":             systemInfo["serialNumber"],
				"firmware":           firmware,
				"build_date":         buildDate,
				"channel_count":      len(channels),
				"disk_count":         len(disks),
				"disk_fault":         diskHealth.DiskFault,
				"disk_error_count":   diskHealth.DiskErrorCount,
				"disk_healthy_count": diskHealth.DiskHealthyCount,
				"total_bytes":        diskHealth.TotalBytes,
				"used_bytes":         diskHealth.UsedBytes,
				"free_bytes":         diskHealth.FreeBytes,
				"used_percent":       diskHealth.UsedPercent,
			},
		},
	}

	if onvifDiscovery != nil {
		rootState := states[cfg.ID]
		rootState.Info["onvif_probed"] = true
		rootState.Info["onvif_device_service_url"] = onvifDiscovery.DeviceServiceURL
		rootState.Info["onvif_media_service_url"] = onvifDiscovery.MediaServiceURL
		rootState.Info["onvif_h264_profile_count"] = onvifDiscovery.H264ProfileCount()
		rootState.Info["onvif_profiles"] = onvifDiscovery.ProfileMaps()
		states[cfg.ID] = rootState
	}

	recordModes, recordModeErr := d.loadRecordModes(ctx)
	if recordModeErr != nil {
		d.logger.Debug().Err(recordModeErr).Msg("record mode probe failed")
	}

	for _, channel := range channels {
		if !channelInventoryWanted(cfg, channel) {
			continue
		}
		childID := channelDeviceID(cfg.ID, channel.Index)
		children = append(children, dahua.Device{
			ID:           childID,
			ParentID:     cfg.ID,
			Name:         channel.Name,
			Manufacturer: cfg.Manufacturer,
			Model:        "NVR Channel",
			BaseURL:      cfg.BaseURL,
			Kind:         dahua.DeviceKindNVRChannel,
			Attributes: map[string]string{
				"channel_index":    strconv.Itoa(channel.Index + 1),
				"main_resolution":  channel.MainResolution,
				"main_codec":       channel.MainCodec,
				"audio_codec":      channel.AudioCodec,
				"sub_resolution":   channel.SubResolution,
				"sub_codec":        channel.SubCodec,
				"snapshot_path":    fmt.Sprintf("/api/v1/nvr/%s/channels/%d/snapshot", cfg.ID, channel.Index+1),
				"rtsp_main_path":   fmt.Sprintf("/cam/realmonitor?channel=%d&subtype=0", channel.Index+1),
				"rtsp_sub_path":    fmt.Sprintf("/cam/realmonitor?channel=%d&subtype=1", channel.Index+1),
				"rtsp_main_url":    buildRTSPURL(cfg.BaseURL, channel.Index+1, 0),
				"rtsp_sub_url":     buildRTSPURL(cfg.BaseURL, channel.Index+1, 1),
				"device_category":  "channel",
				"inventory_source": "cgi",
			},
		})
		states[childID] = dahua.DeviceState{
			Available: true,
			Info: map[string]any{
				"channel":          channel.Index + 1,
				"name":             channel.Name,
				"main_resolution":  channel.MainResolution,
				"main_codec":       channel.MainCodec,
				"audio_codec":      channel.AudioCodec,
				"sub_resolution":   channel.SubResolution,
				"sub_codec":        channel.SubCodec,
				"rtsp_main_url":    buildRTSPURL(cfg.BaseURL, channel.Index+1, 0),
				"rtsp_sub_url":     buildRTSPURL(cfg.BaseURL, channel.Index+1, 1),
				"snapshot_rel_url": fmt.Sprintf("/api/v1/nvr/%s/channels/%d/snapshot", cfg.ID, channel.Index+1),
				"stream_available": d.streamAvailable(ctx, cfg, channel.Index+1),
			},
		}
		if channel.AudioKnown {
			states[childID].Info["control_audio_stream_enabled"] = channel.AudioEnabled
		}
		state := states[childID]
		recordingCapabilities := dahua.NVRRecordingCapabilities{}
		if recordModeErr == nil {
			recordingCapabilities = recordingCapabilitiesForChannel(channel.Index+1, recordModes)
		}
		recordingCapabilities = d.applyRecordingOverride(channel.Index+1, recordingCapabilities)
		if recordingCapabilities.Supported || recordingCapabilities.Active || recordingCapabilities.Mode != "" {
			attachChannelRecordingState(&state, recordingCapabilities)
		}
		ptzCapabilities, ptzErr := d.ptzCapabilities(ctx, channel.Index+1)
		ptzCapabilities = d.applyPTZOverride(channel.Index+1, ptzCapabilities)
		if ptzErr != nil && !isUnsupportedPTZSurfaceError(ptzErr) {
			d.logger.Debug().Err(ptzErr).Int("channel", channel.Index+1).Msg("channel ptz capability probe failed")
		}
		auxCapabilities, auxErr := d.auxCapabilities(ctx, channel.Index+1, ptzCapabilities)
		if auxErr != nil && !isUnsupportedPTZSurfaceError(auxErr) {
			d.logger.Debug().Err(auxErr).Int("channel", channel.Index+1).Msg("channel aux capability probe failed")
		}
		audioCapabilities, audioNotes := d.audioCapabilities(ctx, channel.Index+1)
		attachChannelControlState(&state, dahua.NVRChannelControlCapabilities{
			DeviceID:  cfg.ID,
			Channel:   channel.Index + 1,
			PTZ:       ptzCapabilities,
			Aux:       auxCapabilities,
			Audio:     audioCapabilities,
			Recording: recordingCapabilities,
		})
		notes := make([]string, 0, 4)
		if ptzErr != nil && auxCapabilities.Supported {
			notes = append(notes, "ptz_capability_query_failed_aux_fallback_used")
		}
		if !ptzCapabilities.Supported && auxCapabilities.Supported && len(auxCapabilities.Features) > 0 {
			notes = append(notes, "non_ptz_channel_feature_surface_detected")
		}
		notes = append(notes, audioNotes...)
		attachValidationNotes(&state, notes)
		d.attachImouChannelState(ctx, channel.Index+1, &state)
		states[childID] = state

		if onvifDiscovery != nil {
			child := children[len(children)-1]
			state := states[childID]
			attachONVIFChannelState(&child, &state, *onvifDiscovery, channel.Index+1)
			children[len(children)-1] = child
			states[childID] = state
		}
	}

	for _, disk := range disks {
		childID := diskDeviceID(cfg.ID, disk.Index)
		children = append(children, dahua.Device{
			ID:           childID,
			ParentID:     cfg.ID,
			Name:         disk.Name,
			Manufacturer: cfg.Manufacturer,
			Model:        "NVR Disk",
			BaseURL:      cfg.BaseURL,
			Kind:         dahua.DeviceKindNVRDisk,
			Attributes: map[string]string{
				"disk_index":       strconv.Itoa(disk.Index),
				"state":            disk.State,
				"device_category":  "disk",
				"inventory_source": "cgi",
			},
		})
		states[childID] = dahua.DeviceState{
			Available: !disk.IsError && strings.EqualFold(disk.State, "Success"),
			Info: map[string]any{
				"name":        disk.Name,
				"state":       disk.State,
				"total_bytes": disk.TotalBytes,
				"used_bytes":  disk.UsedBytes,
				"is_error":    disk.IsError,
			},
		}
	}

	return &dahua.ProbeResult{
		Root: dahua.Device{
			ID:           cfg.ID,
			Name:         name,
			Manufacturer: cfg.Manufacturer,
			Model:        firstNonEmpty(cfg.Model, systemInfo["updateSerial"]),
			Serial:       systemInfo["serialNumber"],
			Firmware:     firmware,
			BuildDate:    buildDate,
			BaseURL:      cfg.BaseURL,
			Kind:         dahua.DeviceKindNVR,
			Attributes: map[string]string{
				"device_type":        systemInfo["deviceType"],
				"processor":          systemInfo["processor"],
				"update_serial":      systemInfo["updateSerial"],
				"channel_count":      strconv.Itoa(len(channels)),
				"disk_count":         strconv.Itoa(len(disks)),
				"disk_fault":         strconv.FormatBool(diskHealth.DiskFault),
				"disk_error_count":   strconv.Itoa(diskHealth.DiskErrorCount),
				"disk_healthy_count": strconv.Itoa(diskHealth.DiskHealthyCount),
			},
		},
		Children: children,
		States:   states,
		Raw:      raw,
	}, nil
}

func attachONVIFChannelState(device *dahua.Device, state *dahua.DeviceState, discovery onvif.Discovery, channel int) {
	if device.Attributes == nil {
		device.Attributes = make(map[string]string)
	}
	if state.Info == nil {
		state.Info = make(map[string]any)
	}

	profileCount := discovery.H264ProfileCountForChannel(channel)
	device.Attributes["onvif_probed"] = "true"
	device.Attributes["onvif_h264_profile_count"] = strconv.Itoa(profileCount)
	state.Info["onvif_probed"] = true
	state.Info["onvif_h264_profile_count"] = profileCount

	best, ok := discovery.BestH264ProfileForChannel(channel)
	if !ok {
		device.Attributes["onvif_h264_available"] = "false"
		device.Attributes["recommended_ha_integration"] = "bridge_media"
		state.Info["onvif_h264_available"] = false
		state.Info["recommended_ha_integration"] = "bridge_media"
		return
	}

	device.Attributes["onvif_h264_available"] = "true"
	device.Attributes["onvif_profile_token"] = best.Token
	device.Attributes["onvif_profile_name"] = best.Name
	device.Attributes["onvif_stream_url"] = best.StreamURI
	if best.SnapshotURI != "" {
		device.Attributes["onvif_snapshot_url"] = best.SnapshotURI
	}
	device.Attributes["recommended_ha_integration"] = "onvif"

	state.Info["onvif_h264_available"] = true
	state.Info["onvif_profile_token"] = best.Token
	state.Info["onvif_profile_name"] = best.Name
	state.Info["onvif_stream_url"] = best.StreamURI
	if best.SnapshotURI != "" {
		state.Info["onvif_snapshot_url"] = best.SnapshotURI
	}
	state.Info["recommended_ha_integration"] = "onvif"
}

func (d *Driver) Snapshot(ctx context.Context, channel int) ([]byte, string, error) {
	if channel <= 0 {
		return nil, "", fmt.Errorf("channel must be >= 1")
	}

	return d.client.GetBinary(ctx, "/cgi-bin/snapshot.cgi", url.Values{
		"channel": []string{strconv.Itoa(channel)},
	})
}

func (d *Driver) FindRecordings(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	if query.Channel <= 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("channel must be >= 1")
	}
	if query.StartTime.IsZero() || query.EndTime.IsZero() {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start and end time are required")
	}
	if query.EndTime.Before(query.StartTime) {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("end time must not be before start time")
	}
	if query.Limit <= 0 {
		query.Limit = 25
	}

	if d.rpc != nil {
		result, err := d.findRecordingsViaRPC(ctx, query)
		if err == nil {
			return result, nil
		}
		d.logger.Debug().Err(err).Int("channel", query.Channel).Msg("rpc recording search failed, falling back to cgi")
	}

	return d.findRecordingsViaCGI(ctx, query)
}

func (d *Driver) findRecordingsViaCGI(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	handleKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/mediaFileFind.cgi", url.Values{
		"action": []string{"factory.create"},
	})
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("create recording search handle: %w", err)
	}
	handle := strings.TrimSpace(handleKV["result"])
	if handle == "" {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("recording search handle is empty")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = d.client.GetText(closeCtx, "/cgi-bin/mediaFileFind.cgi", url.Values{
			"action": []string{"close"},
			"object": []string{handle},
		})
	}()

	findBody, err := d.client.GetText(ctx, "/cgi-bin/mediaFileFind.cgi", url.Values{
		"action":              []string{"findFile"},
		"object":              []string{handle},
		"condition.Channel":   []string{strconv.Itoa(query.Channel)},
		"condition.StartTime": []string{query.StartTime.Format(recordingTimeLayout)},
		"condition.EndTime":   []string{query.EndTime.Format(recordingTimeLayout)},
	})
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start recording search: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(findBody), "OK") {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start recording search returned %q", strings.TrimSpace(findBody))
	}

	itemsKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/mediaFileFind.cgi", url.Values{
		"action": []string{"findNextFile"},
		"object": []string{handle},
		"count":  []string{strconv.Itoa(query.Limit)},
	})
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch recording search results: %w", err)
	}

	result := parseRecordingSearchResult(itemsKV)
	result.DeviceID = d.ID()
	result.Channel = query.Channel
	result.StartTime = query.StartTime.In(time.Local).Format(recordingTimeLayout)
	result.EndTime = query.EndTime.In(time.Local).Format(recordingTimeLayout)
	result.Limit = query.Limit
	return result, nil
}

func (d *Driver) findRecordingsViaRPC(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	var handle int64
	if err := d.rpc.Call(ctx, "mediaFileFind.factory.create", nil, &handle); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("create recording search handle: %w", err)
	}
	if handle == 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("recording search handle is empty")
	}

	defer d.closeRecordingSearchHandle(handle)

	findParams := map[string]any{
		"condition": map[string]any{
			"StartTime":   formatRecordingRPCTime(query.StartTime),
			"EndTime":     formatRecordingRPCTime(query.EndTime),
			"Events":      []string{"*"},
			"Flags":       nil,
			"Types":       []string{"dav"},
			"Channel":     query.Channel - 1,
			"VideoStream": "Main",
		},
	}
	if err := d.rpc.CallObject(ctx, "mediaFileFind.findFile", findParams, handle, nil); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start recording search: %w", err)
	}

	var rawResult map[string]any
	if err := d.rpc.CallObject(ctx, "mediaFileFind.findNextFile", map[string]any{
		"count": query.Limit,
	}, handle, &rawResult); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch recording search results: %w", err)
	}

	result := parseRPCRecordingSearchResult(rawResult)
	result.DeviceID = d.ID()
	result.Channel = query.Channel
	result.StartTime = query.StartTime.In(time.Local).Format(recordingTimeLayout)
	result.EndTime = query.EndTime.In(time.Local).Format(recordingTimeLayout)
	result.Limit = query.Limit
	return result, nil
}

func (d *Driver) closeRecordingSearchHandle(handle int64) {
	if d.rpc == nil || handle == 0 {
		return
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = d.rpc.CallObject(closeCtx, "mediaFileFind.close", nil, handle, nil)
	_ = d.rpc.CallObject(closeCtx, "mediaFileFind.destroy", nil, handle, nil)
}

func (d *Driver) DownloadRecording(ctx context.Context, filePath string) (dahua.NVRRecordingDownload, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return dahua.NVRRecordingDownload{}, fmt.Errorf("file path is required")
	}

	resp, err := d.client.OpenStream(ctx, "/RPC_Loadfile"+escapeRecordingFilePath(filePath), nil)
	if err != nil {
		return dahua.NVRRecordingDownload{}, fmt.Errorf("download recording %q: %w", filePath, err)
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	contentLength, _ := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("Content-Length")), 10, 64)

	return dahua.NVRRecordingDownload{
		Body:          resp.Body,
		ContentType:   contentType,
		FileName:      path.Base(filePath),
		ContentLength: contentLength,
	}, nil
}

func (d *Driver) UpdateConfig(cfg config.DeviceConfig) error {
	d.cfgMu.Lock()
	d.cfg = cfg
	d.cfgMu.Unlock()
	d.client.UpdateConfig(cfg)
	if d.rpc != nil {
		d.rpc.UpdateConfig(cfg)
	}
	d.rtsp.UpdateConfig(cfg)
	d.onvif.UpdateConfig(cfg)
	d.InvalidateInventoryCache()
	return nil
}

func (d *Driver) loadInventory(ctx context.Context) ([]channelInventory, error) {
	d.mu.RLock()
	if d.cachedInventory != nil && time.Now().Before(d.inventoryExpires) {
		channels := append([]channelInventory(nil), d.cachedInventory.Channels...)
		d.mu.RUnlock()
		return channels, nil
	}
	d.mu.RUnlock()

	titlesKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"ChannelTitle"},
	})
	if err != nil {
		return nil, err
	}

	encodeKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"Encode"},
	})
	if err != nil {
		return nil, err
	}

	channels := mergeChannelInventory(parseChannelTitles(titlesKV), parseChannelStreams(encodeKV))

	d.mu.Lock()
	d.cachedInventory = &inventorySnapshot{Channels: channels}
	d.inventoryExpires = time.Now().Add(15 * time.Minute)
	d.mu.Unlock()

	return append([]channelInventory(nil), channels...), nil
}

func (d *Driver) InvalidateInventoryCache() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cachedInventory = nil
	d.inventoryExpires = time.Time{}
}

func (d *Driver) loadDisks(ctx context.Context) ([]diskInventory, error) {
	kv, err := d.client.GetKeyValues(ctx, "/cgi-bin/storageDevice.cgi", url.Values{
		"action": []string{"getDeviceAllInfo"},
	})
	if err != nil {
		return nil, err
	}

	return parseDiskInventory(kv), nil
}

func parseSoftwareVersion(body string) (string, string) {
	line := strings.TrimSpace(body)
	if line == "" {
		return "", ""
	}

	version := ""
	build := ""

	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "version="):
			version = strings.TrimPrefix(part, "version=")
		case strings.HasPrefix(part, "build:"):
			build = strings.TrimPrefix(part, "build:")
		}
	}

	return version, build
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

var _ dahua.Driver = (*Driver)(nil)
var _ dahua.SnapshotProvider = (*Driver)(nil)
var _ dahua.NVRRecordingSearcher = (*Driver)(nil)
var _ dahua.NVRRecordingDownloader = (*Driver)(nil)
var _ dahua.NVRInventoryRefresher = (*Driver)(nil)
var _ dahua.ConfigurableDriver = (*Driver)(nil)

func (d *Driver) currentConfig() config.DeviceConfig {
	d.cfgMu.RLock()
	defer d.cfgMu.RUnlock()
	return d.cfg
}

func parseChannelTitles(values map[string]string) map[int]string {
	result := make(map[int]string)
	for key, value := range values {
		matches := channelNamePattern.FindStringSubmatch(key)
		if len(matches) != 2 {
			continue
		}
		index, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		result[index] = value
	}
	return result
}

func parseChannelStreams(values map[string]string) map[int]channelInventory {
	result := make(map[int]channelInventory)
	for key, value := range values {
		if matches := encodeResolutionPattern.FindStringSubmatch(key); len(matches) == 3 {
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			item := result[index]
			item.Index = index
			switch matches[2] {
			case "MainFormat":
				item.MainResolution = value
			case "ExtraFormat":
				item.SubResolution = value
			}
			result[index] = item
			continue
		}

		if matches := encodeCompressionPattern.FindStringSubmatch(key); len(matches) == 3 {
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			item := result[index]
			item.Index = index
			switch matches[2] {
			case "MainFormat":
				item.MainCodec = value
			case "ExtraFormat":
				item.SubCodec = value
			}
			result[index] = item
			continue
		}

		if matches := encodeAudioCompressionPattern.FindStringSubmatch(key); len(matches) == 2 {
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			item := result[index]
			item.Index = index
			if strings.TrimSpace(item.AudioCodec) == "" {
				item.AudioCodec = value
			}
			result[index] = item
			continue
		}

		if matches := encodeAudioEnablePattern.FindStringSubmatch(key); len(matches) == 2 {
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			item := result[index]
			item.Index = index
			if !item.AudioKnown {
				item.AudioEnabled = parseBool(value)
				item.AudioKnown = true
			}
			result[index] = item
		}
	}
	return result
}

func mergeChannelInventory(titles map[int]string, streams map[int]channelInventory) []channelInventory {
	seen := make(map[int]struct{})
	indexes := make([]int, 0, len(titles)+len(streams))

	for index := range titles {
		seen[index] = struct{}{}
		indexes = append(indexes, index)
	}
	for index := range streams {
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		indexes = append(indexes, index)
	}

	sort.Ints(indexes)

	items := make([]channelInventory, 0, len(indexes))
	for _, index := range indexes {
		item := streams[index]
		item.Index = index
		item.Name = firstNonEmpty(titles[index], fmt.Sprintf("Channel %d", index+1))
		items = append(items, item)
	}

	return items
}

func channelInventoryUsable(item channelInventory) bool {
	return strings.TrimSpace(item.MainResolution) != "" ||
		strings.TrimSpace(item.MainCodec) != "" ||
		strings.TrimSpace(item.SubResolution) != "" ||
		strings.TrimSpace(item.SubCodec) != ""
}

func channelInventoryWanted(cfg config.DeviceConfig, item channelInventory) bool {
	channelNumber := item.Index + 1
	if !cfg.AllowsChannel(channelNumber) {
		return false
	}
	if !channelInventoryUsable(item) {
		return false
	}
	return !channelInventoryLooksLikePlaceholder(item)
}

func channelInventoryLooksLikePlaceholder(item channelInventory) bool {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return false
	}

	matches := placeholderChannelNamePattern.FindStringSubmatch(name)
	if len(matches) != 3 {
		return false
	}

	index, err := strconv.Atoi(matches[2])
	if err != nil {
		return false
	}
	return index == item.Index+1
}

func parseDiskInventory(values map[string]string) []diskInventory {
	type aggregate struct {
		diskInventory
	}

	items := make(map[int]*aggregate)

	get := func(index int) *aggregate {
		item, ok := items[index]
		if !ok {
			item = &aggregate{
				diskInventory: diskInventory{Index: index},
			}
			items[index] = item
		}
		return item
	}

	for key, value := range values {
		switch {
		case storageNamePattern.MatchString(key):
			matches := storageNamePattern.FindStringSubmatch(key)
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			get(index).Name = value
		case storageStatePattern.MatchString(key):
			matches := storageStatePattern.FindStringSubmatch(key)
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			get(index).State = value
		case storageDetailTotal.MatchString(key):
			matches := storageDetailTotal.FindStringSubmatch(key)
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			get(index).TotalBytes += parsed
		case storageDetailUsed.MatchString(key):
			matches := storageDetailUsed.FindStringSubmatch(key)
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			get(index).UsedBytes += parsed
		case storageDetailError.MatchString(key):
			matches := storageDetailError.FindStringSubmatch(key)
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			if strings.EqualFold(value, "true") {
				get(index).IsError = true
			}
		}
	}

	indexes := make([]int, 0, len(items))
	for index := range items {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	result := make([]diskInventory, 0, len(indexes))
	for _, index := range indexes {
		result = append(result, items[index].diskInventory)
	}
	return result
}

func summarizeDisks(disks []diskInventory) diskSummary {
	summary := diskSummary{}
	for _, disk := range disks {
		if disk.IsError || !strings.EqualFold(disk.State, "Success") {
			summary.DiskFault = true
			summary.DiskErrorCount++
		} else {
			summary.DiskHealthyCount++
		}
		summary.TotalBytes += disk.TotalBytes
		summary.UsedBytes += disk.UsedBytes
	}

	summary.FreeBytes = summary.TotalBytes - summary.UsedBytes
	if summary.FreeBytes < 0 {
		summary.FreeBytes = 0
	}
	if summary.TotalBytes > 0 {
		summary.UsedPercent = (summary.UsedBytes / summary.TotalBytes) * 100
	}
	return summary
}

func parseRecordingSearchResult(values map[string]string) dahua.NVRRecordingSearchResult {
	result := dahua.NVRRecordingSearchResult{}
	if count, err := strconv.Atoi(strings.TrimSpace(values["found"])); err == nil && count >= 0 {
		result.ReturnedCount = count
	}

	items := make(map[int]*dahua.NVRRecording)
	get := func(index int) *dahua.NVRRecording {
		item, ok := items[index]
		if !ok {
			item = &dahua.NVRRecording{}
			items[index] = item
		}
		return item
	}

	for key, value := range values {
		if matches := recordingItemPattern.FindStringSubmatch(key); len(matches) == 3 {
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			item := get(index)
			switch matches[2] {
			case "Channel":
				if parsed, ok := parseInt(value); ok {
					item.Channel = parsed + 1
				}
			case "Cluster":
				item.Cluster, _ = parseInt(value)
			case "CutLength":
				item.CutLengthBytes, _ = parseInt64(value)
			case "Disk":
				item.Disk, _ = parseInt(value)
			case "EndTime":
				item.EndTime = strings.TrimSpace(value)
			case "FilePath":
				item.FilePath = strings.TrimSpace(value)
			case "Length":
				item.LengthBytes, _ = parseInt64(value)
			case "Partition":
				item.Partition, _ = parseInt(value)
			case "StartTime":
				item.StartTime = strings.TrimSpace(value)
			case "Type":
				item.Type = strings.TrimSpace(value)
			case "VideoStream":
				item.VideoStream = strings.TrimSpace(value)
			}
			continue
		}

		if matches := recordingFlagPattern.FindStringSubmatch(key); len(matches) == 3 {
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			item := get(index)
			item.Flags = append(item.Flags, strings.TrimSpace(value))
		}
	}

	indexes := make([]int, 0, len(items))
	for index := range items {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	result.Items = make([]dahua.NVRRecording, 0, len(indexes))
	for _, index := range indexes {
		result.Items = append(result.Items, *items[index])
	}
	if result.ReturnedCount == 0 {
		result.ReturnedCount = len(result.Items)
	}
	return result
}

func parseRPCRecordingSearchResult(values map[string]any) dahua.NVRRecordingSearchResult {
	result := dahua.NVRRecordingSearchResult{}
	if count, ok := numberFromAny(values["found"]); ok && count >= 0 {
		result.ReturnedCount = count
	}

	rawItems, _ := values["items"].([]any)
	result.Items = make([]dahua.NVRRecording, 0, len(rawItems))
	for _, rawItem := range rawItems {
		itemMap, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		item := dahua.NVRRecording{
			StartTime:   stringFromAny(itemMap["StartTime"]),
			EndTime:     stringFromAny(itemMap["EndTime"]),
			FilePath:    stringFromAny(itemMap["FilePath"]),
			Type:        stringFromAny(itemMap["Type"]),
			VideoStream: stringFromAny(itemMap["VideoStream"]),
		}
		if channel, ok := numberFromAny(itemMap["Channel"]); ok {
			item.Channel = channel + 1
		}
		if disk, ok := numberFromAny(itemMap["Disk"]); ok {
			item.Disk = disk
		}
		if partition, ok := numberFromAny(itemMap["Partition"]); ok {
			item.Partition = partition
		}
		if cluster, ok := numberFromAny(itemMap["Cluster"]); ok {
			item.Cluster = cluster
		}
		if lengthBytes, ok := int64FromAny(itemMap["Length"]); ok {
			item.LengthBytes = lengthBytes
		}
		if cutLengthBytes, ok := int64FromAny(itemMap["CutLength"]); ok {
			item.CutLengthBytes = cutLengthBytes
		}
		if flags, ok := itemMap["Flags"].([]any); ok {
			item.Flags = make([]string, 0, len(flags))
			for _, rawFlag := range flags {
				flag := strings.TrimSpace(stringFromAny(rawFlag))
				if flag != "" {
					item.Flags = append(item.Flags, flag)
				}
			}
		}
		result.Items = append(result.Items, item)
	}
	if result.ReturnedCount == 0 {
		result.ReturnedCount = len(result.Items)
	}
	return result
}

func channelDeviceID(rootID string, index int) string {
	return fmt.Sprintf("%s_channel_%02d", rootID, index+1)
}

func diskDeviceID(rootID string, index int) string {
	return fmt.Sprintf("%s_disk_%02d", rootID, index)
}

func parseInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func parseInt64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err == nil {
		return parsed, true
	}
	floatValue, floatErr := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if floatErr != nil {
		return 0, false
	}
	return int64(floatValue), true
}

func escapeRecordingFilePath(filePath string) string {
	if filePath == "" {
		return ""
	}
	segments := strings.Split(filePath, "/")
	for index, segment := range segments {
		if index == 0 && segment == "" {
			continue
		}
		segments[index] = url.PathEscape(segment)
	}
	escaped := strings.Join(segments, "/")
	if strings.HasPrefix(filePath, "/") && !strings.HasPrefix(escaped, "/") {
		return "/" + escaped
	}
	return escaped
}

func formatRecordingRPCTime(value time.Time) string {
	return value.In(time.Local).Format(recordingRPCTimeLayout)
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func numberFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		return parseInt(typed)
	default:
		return 0, false
	}
}

func int64FromAny(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case float32:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	case string:
		return parseInt64(typed)
	default:
		return 0, false
	}
}

func buildRTSPURL(baseURL string, channel int, subtype int) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}

	host := parsed.Hostname()
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		host = net.JoinHostPort(host, port)
	} else {
		host = net.JoinHostPort(host, "554")
	}

	rtspURL := &url.URL{
		Scheme:   "rtsp",
		Host:     host,
		Path:     "/cam/realmonitor",
		RawQuery: url.Values{"channel": []string{strconv.Itoa(channel)}, "subtype": []string{strconv.Itoa(subtype)}}.Encode(),
	}
	return rtspURL.String()
}

func (d *Driver) streamAvailable(ctx context.Context, cfg config.DeviceConfig, channel int) bool {
	available, err := d.rtsp.StreamAvailable(
		ctx,
		buildRTSPURL(cfg.BaseURL, channel, 1),
		buildRTSPURL(cfg.BaseURL, channel, 0),
	)
	if err != nil {
		d.logger.Debug().Err(err).Int("channel", channel).Msg("rtsp availability probe failed")
	}
	return available
}
