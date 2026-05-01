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
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/onvif"
	"github.com/rs/zerolog"
)

const (
	recordingTimeLayout    = "2006-01-02 15:04:05"
	recordingRPCTimeLayout = "2006-1-2 15:04:05"
	recordingEventPreroll  = 15 * time.Second
	recordingEventPostroll = 45 * time.Second
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
	ipcCfgs []config.DeviceConfig
	metrics *metrics.Registry
	logger  zerolog.Logger

	mu               sync.RWMutex
	cachedInventory  *inventorySnapshot
	inventoryExpires time.Time

	configWriteMu      sync.RWMutex
	configWriteChecked time.Time
	configWriteKnown   bool
	configWriteAllowed bool
	configWriteReason  string

	audioWriteMu     sync.RWMutex
	audioWriteStatus map[int]channelWriteStatus
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
	RemoteDevice   remoteDeviceInventory
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

type nvrLogStartFindResponse struct {
	Token int64 `json:"token"`
}

type nvrLogCountResponse struct {
	Count int `json:"count"`
}

type nvrLogSeekResponse struct {
	Found int          `json:"found"`
	Items []nvrLogItem `json:"items"`
}

type nvrLogItem struct {
	Channel  *int     `json:"Channel"`
	Channels []int    `json:"Channels"`
	Code     string   `json:"Code"`
	Codes    []string `json:"Codes"`
	Detail   string   `json:"Detail"`
	Time     string   `json:"Time"`
	Type     string   `json:"Type"`
	UTCTime  string   `json:"UTCTime"`
	User     string   `json:"User"`
}

type nvrSMDStartFindResponse struct {
	Count int   `json:"Count"`
	Token int64 `json:"Token"`
}

type nvrSMDFindResponse struct {
	SMDInfo []nvrSMDInfo `json:"SmdInfo"`
}

type nvrSMDInfo struct {
	Channel   int    `json:"Channel"`
	StartTime string `json:"StartTime"`
	EndTime   string `json:"EndTime"`
	Type      string `json:"Type"`
	SMDType   string `json:"SmdType"`
	Event     string `json:"Event"`
}

type nvrMediaFileCountResponse struct {
	Count int `json:"count"`
	Found int `json:"found"`
}

var (
	channelNamePattern            = regexp.MustCompile(`^table\.ChannelTitle\[(\d+)\]\.Name$`)
	encodeResolutionPattern       = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.(MainFormat|ExtraFormat)\[0\]\.Video\.resolution$`)
	encodeCompressionPattern      = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.(MainFormat|ExtraFormat)\[0\]\.Video\.Compression$`)
	encodeAudioCompressionPattern = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.MainFormat\[\d+\]\.Audio\.Compression$`)
	encodeAudioEnablePattern      = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.MainFormat\[\d+\]\.AudioEnable$`)
	encodeAnyAudioEnablePattern   = regexp.MustCompile(`^table\.Encode\[(\d+)\]\.(MainFormat|ExtraFormat)\[\d+\]\.AudioEnable$`)
	remoteDevicePattern           = regexp.MustCompile(`^table\.RemoteDevice\.[^.]*_(\d+)\.([A-Za-z0-9_.\[\]]+)$`)
	remoteDeviceLegacyPattern     = regexp.MustCompile(`^table\.RemoteDevice\[(\d+)\]\.([A-Za-z0-9_.\[\]]+)$`)
	storageNamePattern            = regexp.MustCompile(`^list\.info\[(\d+)\]\.Name$`)
	storageStatePattern           = regexp.MustCompile(`^list\.info\[(\d+)\]\.State$`)
	storageDetailTotal            = regexp.MustCompile(`^list\.info\[(\d+)\]\.Detail\[(\d+)\]\.TotalBytes$`)
	storageDetailUsed             = regexp.MustCompile(`^list\.info\[(\d+)\]\.Detail\[(\d+)\]\.UsedBytes$`)
	storageDetailError            = regexp.MustCompile(`^list\.info\[(\d+)\]\.Detail\[(\d+)\]\.IsError$`)
	placeholderChannelNamePattern = regexp.MustCompile(`(?i)^\s*(channel|канал)\s*0*(\d+)\s*$`)
)

func New(cfg config.DeviceConfig, imouCfg config.ImouConfig, imouClient imou.Service, ipcCfgs []config.DeviceConfig, logger zerolog.Logger, metricsRegistry *metrics.Registry, client *cgi.Client) *Driver {
	return &Driver{
		cfg:     cfg,
		client:  client,
		rpc:     dahuarpc.New(cfg, logger),
		rtsp:    dahuartsp.NewChecker(cfg),
		onvif:   onvif.New(cfg),
		imou:    imouClient,
		imouCfg: imouCfg,
		ipcCfgs: append([]config.DeviceConfig(nil), ipcCfgs...),
		metrics: metricsRegistry,
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
	nvrConfigWritable, nvrConfigReason := d.nvrConfigWriteStatus(ctx)
	rootState := states[cfg.ID]
	rootState.Info["nvr_config_writable"] = nvrConfigWritable
	rootState.Info["nvr_config_reason"] = nvrConfigReason
	states[cfg.ID] = rootState

	for _, channel := range channels {
		if !channelInventoryWanted(cfg, channel) {
			continue
		}
		childID := channelDeviceID(cfg.ID, channel.Index)
		directIPCCredential, directIPCConfigured := cfg.DirectIPCCredential(channel.Index + 1)
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
				"direct_ipc_ip":    channel.RemoteDevice.Address,
				"direct_ipc_model": channel.RemoteDevice.DeviceType,
			},
		})
		if directIPCConfigured {
			children[len(children)-1].Attributes["direct_ipc_configured"] = "true"
			children[len(children)-1].Attributes["direct_ipc_configured_ip"] = directIPCCredential.DirectIPCIP
		}
		if channel.RemoteDevice.HTTPPort > 0 {
			children[len(children)-1].Attributes["direct_ipc_http_port"] = strconv.Itoa(channel.RemoteDevice.HTTPPort)
		}
		if channel.RemoteDevice.HTTPSPort > 0 {
			children[len(children)-1].Attributes["direct_ipc_https_port"] = strconv.Itoa(channel.RemoteDevice.HTTPSPort)
		}
		if channel.RemoteDevice.RTSPPort > 0 {
			children[len(children)-1].Attributes["direct_ipc_rtsp_port"] = strconv.Itoa(channel.RemoteDevice.RTSPPort)
		}
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
				"direct_ipc_ip":    channel.RemoteDevice.Address,
				"direct_ipc_model": channel.RemoteDevice.DeviceType,
			},
		}
		if channel.RemoteDevice.HTTPPort > 0 {
			states[childID].Info["direct_ipc_http_port"] = channel.RemoteDevice.HTTPPort
		}
		if channel.RemoteDevice.HTTPSPort > 0 {
			states[childID].Info["direct_ipc_https_port"] = channel.RemoteDevice.HTTPSPort
		}
		if channel.RemoteDevice.RTSPPort > 0 {
			states[childID].Info["direct_ipc_rtsp_port"] = channel.RemoteDevice.RTSPPort
		}
		if channel.RemoteDevice.UserName != "" {
			states[childID].Info["direct_ipc_inventory_username"] = channel.RemoteDevice.UserName
		}
		states[childID].Info["nvr_config_writable"] = nvrConfigWritable
		states[childID].Info["nvr_config_reason"] = nvrConfigReason
		if directIPCConfigured {
			states[childID].Info["direct_ipc_configured"] = true
			states[childID].Info["direct_ipc_configured_ip"] = directIPCCredential.DirectIPCIP
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
		if recordingCapabilities.Supported && !nvrConfigWritable {
			recordingCapabilities.Supported = false
		}
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
		state.Info["control_audio_authority"] = d.audioControlAuthority(ctx, channel.Index+1)
		state.Info["control_audio_semantic"] = "stream_audio_enable"
		notes := make([]string, 0, 4)
		if ptzErr != nil && auxCapabilities.Supported {
			notes = append(notes, "ptz_capability_query_failed_aux_fallback_used")
		}
		if !ptzCapabilities.Supported && auxCapabilities.Supported && len(auxCapabilities.Features) > 0 {
			notes = append(notes, "non_ptz_channel_feature_surface_detected")
		}
		if directIPCConfigured && channel.RemoteDevice.Address != "" && !strings.EqualFold(strings.TrimSpace(channel.RemoteDevice.Address), strings.TrimSpace(directIPCCredential.DirectIPCIP)) {
			notes = append(notes, "direct_ipc_configured_ip_differs_from_nvr_remote_device_ip")
		}
		if !nvrConfigWritable && nvrConfigReason == "permission_denied" {
			notes = append(notes, "nvr_config_write_permission_denied")
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

	if query.EventOnly {
		return d.findEventRecordings(ctx, query)
	}

	eventCode := recordingEventCondition(query.EventCode)
	if d.rpc != nil && eventCode != "*" {
		result, err := d.findEventRecordingsViaLogRPC(ctx, query)
		if err == nil {
			return result, nil
		}
		d.logger.Debug().Err(err).Int("channel", query.Channel).Str("event_code", eventCode).Msg("rpc event log recording search failed, falling back to media file search")
	}

	if d.rpc != nil {
		result, err := d.findRecordingsViaRPC(ctx, query)
		if err == nil {
			return filterRecordingSearchResultForEvent(result, query.EventCode), nil
		}
		d.logger.Debug().Err(err).Int("channel", query.Channel).Msg("rpc recording search failed, falling back to cgi")
	}

	result, err := d.findRecordingsViaCGI(ctx, query)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, err
	}
	return filterRecordingSearchResultForEvent(result, query.EventCode), nil
}

func recordingEventCondition(eventCode string) string {
	switch strings.ToLower(strings.TrimSpace(eventCode)) {
	case "", "*", "all", "any", "__all__", "com.all":
		return "*"
	default:
		code := canonicalRecordingEventCode(eventCode)
		if code != "" {
			return code
		}
		return normalizeRecordingEventCode(eventCode)
	}
}

func (d *Driver) findEventRecordings(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	result := emptyNVRRecordingSearchResult(d.ID(), query)
	if d.rpc == nil {
		return result, nil
	}

	var firstErr error
	appendResult := func(next dahua.NVRRecordingSearchResult, err error) {
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		result.Items = append(result.Items, next.Items...)
	}

	if smdTypes := recordingSMDTypesForEvent(query.EventCode); len(smdTypes) > 0 {
		next, err := d.findEventRecordingsViaSMDRPC(ctx, query, smdTypes)
		appendResult(next, err)
	}

	for _, ivsCode := range recordingIVSEventCodesForEvent(query.EventCode) {
		ivsQuery := query
		ivsQuery.EventCode = ivsCode
		next, err := d.findEventRecordingsViaIVSRPC(ctx, ivsQuery, ivsCode)
		appendResult(next, err)
	}

	if len(result.Items) == 0 {
		if firstErr != nil {
			return dahua.NVRRecordingSearchResult{}, firstErr
		}
		result.ReturnedCount = 0
		return result, nil
	}

	result.Items = dedupeNVRRecordings(result.Items)
	sort.SliceStable(result.Items, func(i, j int) bool {
		return nvrRecordingStartSortKey(result.Items[i]).After(nvrRecordingStartSortKey(result.Items[j]))
	})
	if query.Limit > 0 && len(result.Items) > query.Limit {
		result.Items = result.Items[:query.Limit]
	}
	result.ReturnedCount = len(result.Items)
	return result, nil
}

func emptyNVRRecordingSearchResult(deviceID string, query dahua.NVRRecordingQuery) dahua.NVRRecordingSearchResult {
	return dahua.NVRRecordingSearchResult{
		DeviceID:  deviceID,
		Channel:   query.Channel,
		StartTime: query.StartTime.In(time.Local).Format(recordingTimeLayout),
		EndTime:   query.EndTime.In(time.Local).Format(recordingTimeLayout),
		Limit:     query.Limit,
		Items:     []dahua.NVRRecording{},
	}
}

func (d *Driver) findEventRecordingsViaSMDRPC(ctx context.Context, query dahua.NVRRecordingQuery, smdTypes []string) (dahua.NVRRecordingSearchResult, error) {
	result := emptyNVRRecordingSearchResult(d.ID(), query)

	var startResp nvrSMDStartFindResponse
	if err := d.rpc.Call(ctx, "SmdDataFinder.startFind", map[string]any{
		"Condition": map[string]any{
			"StartTime": formatRecordingWallTime(query.StartTime),
			"EndTime":   formatRecordingWallTime(query.EndTime),
			"Orders": []map[string]any{{
				"Field": "SystemTime",
				"Type":  "Descent",
			}},
			"Channels": []int{query.Channel - 1},
			"Order":    "descOrder",
			"SmdType":  smdTypes,
		},
	}, &startResp); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start smd event search: %w", err)
	}
	if startResp.Token == 0 || startResp.Count == 0 {
		return result, nil
	}

	pageSize := recordingEventSearchPageSize(query.Limit)
	for offset := 0; offset < startResp.Count && len(result.Items) < query.Limit; offset += pageSize {
		count := pageSize
		if remaining := query.Limit - len(result.Items); remaining > 0 && remaining < count {
			count = remaining
		}
		if count <= 0 {
			break
		}

		var findResp nvrSMDFindResponse
		if err := d.rpc.Call(ctx, "SmdDataFinder.doFind", map[string]any{
			"Token":  startResp.Token,
			"Offset": offset,
			"Count":  count,
		}, &findResp); err != nil {
			return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch smd event results: %w", err)
		}
		if len(findResp.SMDInfo) == 0 {
			break
		}

		for _, info := range findResp.SMDInfo {
			recording, ok := smdInfoToRecording(info, query)
			if !ok {
				continue
			}
			result.Items = append(result.Items, recording)
			if len(result.Items) >= query.Limit {
				break
			}
		}
		if len(findResp.SMDInfo) < count {
			break
		}
	}

	result.ReturnedCount = len(result.Items)
	return result, nil
}

func (d *Driver) findEventRecordingsViaIVSRPC(ctx context.Context, query dahua.NVRRecordingQuery, eventCode string) (dahua.NVRRecordingSearchResult, error) {
	result := emptyNVRRecordingSearchResult(d.ID(), query)

	var handle int64
	if err := d.rpc.Call(ctx, "mediaFileFind.factory.create", nil, &handle); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("create ivs event search handle: %w", err)
	}
	if handle == 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("ivs event search handle is empty")
	}
	defer d.closeRecordingSearchHandle(handle)

	findParams := map[string]any{
		"condition": map[string]any{
			"StartTime": formatRecordingWallTime(query.StartTime),
			"EndTime":   formatRecordingWallTime(query.EndTime),
			"Channel":   []int{query.Channel - 1},
			"DB": map[string]any{
				"IVS": map[string]any{
					"Order":      "Descent",
					"Events":     []string{eventCode},
					"Rule":       eventCode,
					"ObjectType": []string{"Vehicle", "Human"},
				},
			},
			"Events": []string{eventCode},
		},
	}
	if err := d.rpc.CallObject(ctx, "mediaFileFind.findFile", findParams, handle, nil); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start ivs event search: %w", err)
	}

	var countResp nvrMediaFileCountResponse
	if err := d.rpc.CallObject(ctx, "mediaFileFind.getCount", nil, handle, &countResp); err != nil {
		d.logger.Debug().Err(err).Int64("handle", handle).Msg("ivs event count failed")
	} else if countResp.Count == 0 && countResp.Found == 0 {
		return result, nil
	}

	if err := d.rpc.CallObject(ctx, "mediaFileFind.setQueryResultOptions", map[string]any{
		"options": map[string]any{"offset": 0},
	}, handle, nil); err != nil {
		d.logger.Debug().Err(err).Int64("handle", handle).Msg("ivs event result option setup failed")
	}

	var rawResult map[string]any
	if err := d.rpc.CallObject(ctx, "mediaFileFind.findNextFile", map[string]any{
		"count": query.Limit,
	}, handle, &rawResult); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch ivs event results: %w", err)
	}

	result = parseRPCRecordingSearchResult(rawResult)
	result.DeviceID = d.ID()
	result.Channel = query.Channel
	result.StartTime = query.StartTime.In(time.Local).Format(recordingTimeLayout)
	result.EndTime = query.EndTime.In(time.Local).Format(recordingTimeLayout)
	result.Limit = query.Limit
	for i := range result.Items {
		result.Items[i] = normalizeIVSEventRecording(result.Items[i], query, eventCode)
	}
	result.ReturnedCount = len(result.Items)
	return result, nil
}

func recordingSMDTypesForEvent(eventCode string) []string {
	switch strings.ToLower(recordingEventCondition(eventCode)) {
	case "*":
		return []string{"smdTypeHuman", "smdTypeVehicle", "smdTypeAnimal"}
	case "smartmotionhuman":
		return []string{"smdTypeHuman"}
	case "smartmotionvehicle":
		return []string{"smdTypeVehicle"}
	case "animaldetection":
		return []string{"smdTypeAnimal"}
	default:
		return nil
	}
}

func recordingIVSEventCodesForEvent(eventCode string) []string {
	switch strings.ToLower(recordingEventCondition(eventCode)) {
	case "*":
		return []string{"CrossLineDetection", "CrossRegionDetection", "LeftDetection", "MoveDetection"}
	case "crosslinedetection":
		return []string{"CrossLineDetection"}
	case "crossregiondetection":
		return []string{"CrossRegionDetection"}
	case "leftdetection":
		return []string{"LeftDetection"}
	case "movedetection":
		return []string{"MoveDetection"}
	default:
		return nil
	}
}

func recordingEventSearchPageSize(limit int) int {
	pageSize := limit
	if pageSize < 25 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return pageSize
}

func smdInfoToRecording(info nvrSMDInfo, query dahua.NVRRecordingQuery) (dahua.NVRRecording, bool) {
	channel := info.Channel + 1
	if channel != query.Channel {
		return dahua.NVRRecording{}, false
	}

	startTime, ok := parseNVRRecordingLocalTime(info.StartTime)
	if !ok {
		return dahua.NVRRecording{}, false
	}
	endTime, ok := parseNVRRecordingLocalTime(info.EndTime)
	if !ok || !endTime.After(startTime) {
		endTime = startTime.Add(time.Minute)
	}
	startTime, endTime = clampNVRRecordingWindow(startTime, endTime, query.StartTime, query.EndTime)
	if !endTime.After(startTime) {
		return dahua.NVRRecording{}, false
	}

	eventType := firstNonEmpty(info.Type, info.SMDType, info.Event)
	if eventType == "" {
		eventType = "smd"
	}

	return dahua.NVRRecording{
		Source:      "nvr_event",
		Channel:     channel,
		StartTime:   startTime.In(time.Local).Format(recordingTimeLayout),
		EndTime:     endTime.In(time.Local).Format(recordingTimeLayout),
		Type:        "Event." + eventType,
		VideoStream: "Main",
		Flags:       []string{"Event", eventType},
	}, true
}

func normalizeIVSEventRecording(recording dahua.NVRRecording, query dahua.NVRRecordingQuery, eventCode string) dahua.NVRRecording {
	recording.Source = "nvr_event"
	if recording.Channel <= 0 {
		recording.Channel = query.Channel
	}
	if strings.TrimSpace(recording.Type) == "" || strings.EqualFold(recording.Type, "dav") {
		recording.Type = "Event." + eventCode
	}
	recording.Flags = appendRecordingFlags(recording.Flags, "Event", eventCode)
	if recording.VideoStream == "" {
		recording.VideoStream = "Main"
	}
	return recording
}

func appendRecordingFlags(flags []string, values ...string) []string {
	seen := make(map[string]struct{}, len(flags)+len(values))
	result := make([]string, 0, len(flags)+len(values))
	for _, flag := range append(flags, values...) {
		trimmed := strings.TrimSpace(flag)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func parseNVRRecordingLocalTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{recordingTimeLayout, recordingRPCTimeLayout, time.RFC3339} {
		var (
			parsed time.Time
			err    error
		)
		if layout == time.RFC3339 {
			parsed, err = time.Parse(layout, raw)
		} else {
			parsed, err = time.ParseInLocation(layout, raw, time.Local)
		}
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func clampNVRRecordingWindow(startTime time.Time, endTime time.Time, queryStart time.Time, queryEnd time.Time) (time.Time, time.Time) {
	if !queryStart.IsZero() && startTime.Before(queryStart) {
		startTime = queryStart
	}
	if !queryEnd.IsZero() && endTime.After(queryEnd) {
		endTime = queryEnd
	}
	return startTime, endTime
}

func dedupeNVRRecordings(items []dahua.NVRRecording) []dahua.NVRRecording {
	seen := make(map[string]struct{}, len(items))
	result := make([]dahua.NVRRecording, 0, len(items))
	for _, item := range items {
		key := strings.Join([]string{
			strconv.Itoa(item.Channel),
			strings.ToLower(strings.TrimSpace(item.Type)),
			strings.TrimSpace(item.StartTime),
			strings.TrimSpace(item.EndTime),
			strings.TrimSpace(item.FilePath),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func nvrRecordingStartSortKey(item dahua.NVRRecording) time.Time {
	if parsed, ok := parseNVRRecordingLocalTime(item.StartTime); ok {
		return parsed
	}
	return time.Time{}
}

func (d *Driver) findEventRecordingsViaLogRPC(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	eventCode := recordingEventCondition(query.EventCode)
	if eventCode == "*" {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("event log search requires a specific event filter")
	}

	logTypes := recordingLogTypesForEvent(eventCode)
	if len(logTypes) == 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("event log type list is empty")
	}

	result := dahua.NVRRecordingSearchResult{
		DeviceID:  d.ID(),
		Channel:   query.Channel,
		StartTime: query.StartTime.In(time.Local).Format(recordingTimeLayout),
		EndTime:   query.EndTime.In(time.Local).Format(recordingTimeLayout),
		Limit:     query.Limit,
	}

	var startResp nvrLogStartFindResponse
	err := d.rpc.Call(ctx, "log.startFind", map[string]any{
		"condition": map[string]any{
			"Types":     logTypes,
			"StartTime": query.StartTime.In(time.Local).Format(recordingTimeLayout),
			"EndTime":   query.EndTime.In(time.Local).Format(recordingTimeLayout),
			"Translate": true,
			"Order":     "Descent",
		},
	}, &startResp)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start event log search: %w", err)
	}
	if startResp.Token == 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("event log search token is empty")
	}

	countKnown := false
	totalCount := 0
	var countResp nvrLogCountResponse
	if err := d.rpc.Call(ctx, "log.getCount", map[string]any{"token": startResp.Token}, &countResp); err != nil {
		d.logger.Debug().Err(err).Int64("token", startResp.Token).Msg("event log count failed")
	} else {
		countKnown = true
		totalCount = countResp.Count
	}
	if countKnown && totalCount == 0 {
		return result, nil
	}

	pageSize := recordingEventLogPageSize(query.Limit)
	maxPages := 10
	if countKnown {
		maxPages = (totalCount + pageSize - 1) / pageSize
		if maxPages > 10 {
			maxPages = 10
		}
	}
	seen := make(map[string]struct{})
	for page := 0; page < maxPages && len(result.Items) < query.Limit; page++ {
		offset := page * pageSize
		var seekResp nvrLogSeekResponse
		err := d.rpc.Call(ctx, "log.doSeekFind", map[string]any{
			"token":  startResp.Token,
			"offset": offset,
			"count":  pageSize,
		}, &seekResp)
		if err != nil {
			if page == 0 {
				return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch event log results: %w", err)
			}
			d.logger.Debug().Err(err).Int("offset", offset).Msg("event log page failed")
			break
		}
		if len(seekResp.Items) == 0 {
			break
		}

		for _, logItem := range seekResp.Items {
			recording, eventTime, ok := nvrLogItemToRecording(logItem, query, eventCode)
			if !ok {
				continue
			}
			key := nvrLogRecordingDedupeKey(recording, eventCode, eventTime)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result.Items = append(result.Items, recording)
			if len(result.Items) >= query.Limit {
				break
			}
		}
		if countKnown && offset+pageSize >= totalCount {
			break
		}
		if len(seekResp.Items) < pageSize {
			break
		}
	}

	result.ReturnedCount = len(result.Items)
	return result, nil
}

func recordingLogTypesForEvent(eventCode string) []string {
	eventCode = recordingEventCondition(eventCode)
	switch strings.ToLower(eventCode) {
	case "*":
		return nil
	case "videomotion":
		return recordingLogTypeVariants("VideoMotion", "AlarmPIR")
	case "smartmotionhuman":
		return append(recordingLogTypeVariants("SmartMotionHuman"), recordingLogTypeVariants("Intelligence")...)
	case "smartmotionvehicle":
		return append(recordingLogTypeVariants("SmartMotionVehicle"), recordingLogTypeVariants("Intelligence")...)
	case "animaldetection":
		return append(recordingLogTypeVariants("AnimalDetection"), recordingLogTypeVariants("Intelligence")...)
	case "crosslinedetection":
		return append(recordingLogTypeVariants("CrossLineDetection"), recordingLogTypeVariants("Intelligence")...)
	case "crossregiondetection":
		return append(recordingLogTypeVariants("CrossRegionDetection"), recordingLogTypeVariants("Intelligence")...)
	case "leftdetection":
		return append(recordingLogTypeVariants("LeftDetection"), recordingLogTypeVariants("Intelligence")...)
	case "movedetection":
		return append(recordingLogTypeVariants("MoveDetection"), recordingLogTypeVariants("Intelligence")...)
	case "accesscontrol":
		return recordingLogTypeVariants("AccessControl", "AccessCtl")
	default:
		return recordingLogTypeVariants(eventCode)
	}
}

func recordingLogTypeVariants(codes ...string) []string {
	actions := []string{"Start", "Stop", "Pulse", "Pluse"}
	seen := make(map[string]struct{}, len(codes)*len(actions))
	values := make([]string, 0, len(codes)*len(actions))
	for _, code := range codes {
		code = normalizeRecordingEventCode(code)
		if code == "" {
			continue
		}
		for _, action := range actions {
			value := "Event." + code + "." + action
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values
}

func recordingEventMatchCodes(eventCode string) map[string]struct{} {
	eventCode = normalizeRecordingEventCode(recordingEventCondition(eventCode))
	codes := []string{eventCode}
	switch strings.ToLower(eventCode) {
	case "videomotion":
		codes = append(codes, "AlarmPIR")
	case "smartmotionhuman":
		codes = append(codes, "HumanDetection", "IntelliFrameHuman", "Human", "smdTypeHuman")
	case "smartmotionvehicle":
		codes = append(codes, "VehicleDetection", "MotorVehicle", "smdTypeVehicle")
	case "animaldetection":
		codes = append(codes, "Animal", "smdTypeAnimal")
	case "crosslinedetection":
		codes = append(codes, "Tripwire")
	case "crossregiondetection":
		codes = append(codes, "Intrusion")
	case "leftdetection":
		codes = append(codes, "LeftDetection")
	case "movedetection":
		codes = append(codes, "MoveDetection")
	case "accesscontrol":
		codes = append(codes, "AccessCtl")
	}

	values := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.ToLower(normalizeRecordingEventCode(code))
		if code != "" && code != "*" {
			values[code] = struct{}{}
		}
	}
	return values
}

func normalizeRecordingEventCode(code string) string {
	code = strings.TrimSpace(code)
	if strings.EqualFold(code, "event") {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(code), "event.") {
		code = code[len("event."):]
	}
	for {
		lower := strings.ToLower(code)
		trimmed := false
		for _, suffix := range []string{".start", ".stop", ".pulse", ".pluse"} {
			if strings.HasSuffix(lower, suffix) {
				code = code[:len(code)-len(suffix)]
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	for {
		lower := strings.ToLower(code)
		trimmed := false
		for _, prefix := range []string{"com.", "ivs."} {
			if strings.HasPrefix(lower, prefix) {
				code = code[len(prefix):]
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	return strings.TrimSpace(code)
}

func canonicalRecordingEventCode(code string) string {
	switch strings.ToLower(normalizeRecordingEventCode(code)) {
	case "motion", "videomotion":
		return "VideoMotion"
	case "human", "smartmotionhuman", "humandetection", "intelliframehuman", "smdtypehuman":
		return "SmartMotionHuman"
	case "vehicle", "smartmotionvehicle", "vehicledetection", "motorvehicle", "smdtypevehicle":
		return "SmartMotionVehicle"
	case "animal", "animaldetection", "smdtypeanimal":
		return "AnimalDetection"
	case "tripwire", "crosslinedetection":
		return "CrossLineDetection"
	case "intrusion", "crossregiondetection":
		return "CrossRegionDetection"
	case "leftdetection":
		return "LeftDetection"
	case "movedetection":
		return "MoveDetection"
	case "access", "accesscontrol", "accessctl":
		return "AccessControl"
	default:
		return normalizeRecordingEventCode(code)
	}
}

func recordingEventLogPageSize(limit int) int {
	pageSize := limit * 4
	if pageSize < 100 {
		pageSize = 100
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return pageSize
}

func nvrLogItemToRecording(item nvrLogItem, query dahua.NVRRecordingQuery, eventCode string) (dahua.NVRRecording, time.Time, bool) {
	if !nvrLogItemMatchesChannel(item, query.Channel) {
		return dahua.NVRRecording{}, time.Time{}, false
	}
	if !nvrLogItemMatchesRecordingEvent(item, eventCode) {
		return dahua.NVRRecording{}, time.Time{}, false
	}

	eventTime, ok := parseNVRLogItemTime(item)
	if !ok {
		return dahua.NVRRecording{}, time.Time{}, false
	}
	startTime, endTime := nvrEventRecordingWindow(eventTime, query.StartTime, query.EndTime)
	code := firstNonEmpty(canonicalRecordingEventCode(item.Code), canonicalRecordingEventCode(eventCode))
	channel := nvrLogItemChannel(item, query.Channel)
	recordingType := "Event"
	if code != "" {
		recordingType = "Event." + code
	}
	flags := []string{"Event"}
	if code != "" {
		flags = append(flags, code)
	}

	return dahua.NVRRecording{
		Source:      "nvr_event",
		Channel:     channel,
		StartTime:   startTime.In(time.Local).Format(recordingTimeLayout),
		EndTime:     endTime.In(time.Local).Format(recordingTimeLayout),
		Type:        recordingType,
		VideoStream: "Main",
		Flags:       flags,
	}, eventTime, true
}

func nvrLogItemMatchesChannel(item nvrLogItem, queryChannel int) bool {
	channels := nvrLogItemChannels(item)
	if len(channels) == 0 {
		return true
	}
	for _, channel := range channels {
		if channel == queryChannel {
			return true
		}
		if channel == 0 && queryChannel == 1 {
			return true
		}
	}
	return false
}

func nvrLogItemChannel(item nvrLogItem, queryChannel int) int {
	for _, channel := range nvrLogItemChannels(item) {
		if channel > 0 {
			return channel
		}
	}
	return queryChannel
}

func nvrLogItemChannels(item nvrLogItem) []int {
	channels := make([]int, 0, 1+len(item.Channels))
	if item.Channel != nil {
		channels = append(channels, *item.Channel)
	}
	channels = append(channels, item.Channels...)
	return channels
}

func nvrLogItemMatchesRecordingEvent(item nvrLogItem, eventCode string) bool {
	accepted := recordingEventMatchCodes(eventCode)
	if len(accepted) == 0 {
		return true
	}

	codes := make([]string, 0, 1+len(item.Codes))
	if strings.TrimSpace(item.Code) != "" {
		codes = append(codes, item.Code)
	}
	codes = append(codes, item.Codes...)
	if len(codes) == 0 {
		return true
	}

	for _, code := range codes {
		normalized := strings.ToLower(normalizeRecordingEventCode(code))
		if _, ok := accepted[normalized]; ok {
			return true
		}
	}
	return false
}

func parseNVRLogItemTime(item nvrLogItem) (time.Time, bool) {
	for _, raw := range []string{item.Time, item.UTCTime} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		for _, layout := range []string{recordingTimeLayout, recordingRPCTimeLayout, time.RFC3339} {
			var (
				parsed time.Time
				err    error
			)
			if layout == time.RFC3339 {
				parsed, err = time.Parse(layout, raw)
			} else {
				parsed, err = time.ParseInLocation(layout, raw, time.Local)
			}
			if err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func nvrEventRecordingWindow(eventTime time.Time, queryStart time.Time, queryEnd time.Time) (time.Time, time.Time) {
	startTime := eventTime.Add(-recordingEventPreroll)
	endTime := eventTime.Add(recordingEventPostroll)
	if !queryStart.IsZero() && startTime.Before(queryStart) {
		startTime = queryStart
	}
	if !queryEnd.IsZero() && endTime.After(queryEnd) {
		endTime = queryEnd
	}
	if !endTime.After(startTime) {
		endTime = startTime.Add(time.Second)
		if !queryEnd.IsZero() && endTime.After(queryEnd) {
			endTime = queryEnd
			startTime = endTime.Add(-time.Second)
			if !queryStart.IsZero() && startTime.Before(queryStart) {
				startTime = queryStart
			}
		}
	}
	return startTime, endTime
}

func nvrLogRecordingDedupeKey(recording dahua.NVRRecording, eventCode string, eventTime time.Time) string {
	code := canonicalRecordingEventCode(recording.Type)
	if code == "" {
		code = canonicalRecordingEventCode(eventCode)
	}
	return fmt.Sprintf("%d|%s|%d", recording.Channel, strings.ToLower(code), eventTime.Unix()/30)
}

func filterRecordingSearchResultForEvent(result dahua.NVRRecordingSearchResult, eventCode string) dahua.NVRRecordingSearchResult {
	if recordingEventCondition(eventCode) == "*" {
		return result
	}
	filtered := result.Items[:0]
	for _, item := range result.Items {
		if recordingLooksEventBacked(item) {
			filtered = append(filtered, item)
		}
	}
	result.Items = filtered
	result.ReturnedCount = len(filtered)
	return result
}

func recordingLooksEventBacked(item dahua.NVRRecording) bool {
	hasTimingFlag := false
	for _, flag := range item.Flags {
		switch strings.ToLower(strings.TrimSpace(flag)) {
		case "event", "alarm", "motion", "md", "ivsvideo", "ivs":
			return true
		case "timing":
			hasTimingFlag = true
		}
	}
	path := strings.ToUpper(item.FilePath)
	if strings.Contains(path, "[M]") || strings.Contains(path, "[A]") || strings.Contains(path, "[I]") || strings.Contains(path, "[E]") {
		return true
	}
	if hasTimingFlag || strings.Contains(path, "[R]") {
		return false
	}
	return true
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

	findQuery := url.Values{
		"action":                []string{"findFile"},
		"object":                []string{handle},
		"condition.Channel":     []string{strconv.Itoa(query.Channel)},
		"condition.StartTime":   []string{query.StartTime.Format(recordingTimeLayout)},
		"condition.EndTime":     []string{query.EndTime.Format(recordingTimeLayout)},
		"condition.Types[0]":    []string{"dav"},
		"condition.VideoStream": []string{"Main"},
	}
	if eventCode := recordingEventCondition(query.EventCode); eventCode != "*" {
		findQuery.Set("condition.Flag[0]", "Event")
		findQuery.Set("condition.Events[0]", eventCode)
	}
	findBody, err := d.client.GetText(ctx, "/cgi-bin/mediaFileFind.cgi", findQuery)
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

	eventCode := recordingEventCondition(query.EventCode)
	flags := any(nil)
	if eventCode != "*" {
		flags = []string{"Event"}
	}
	findParams := map[string]any{
		"condition": map[string]any{
			"StartTime":   formatRecordingRPCTime(query.StartTime),
			"EndTime":     formatRecordingRPCTime(query.EndTime),
			"Events":      []string{eventCode},
			"Flags":       flags,
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
	d.resetConfigWriteStatus()
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

	channels := make([]channelInventory, 0)
	remoteDevicesKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"RemoteDevice"},
	})
	if err == nil {
		channels = mergeChannelInventory(parseChannelTitles(titlesKV), parseChannelStreams(encodeKV), parseRemoteDevices(remoteDevicesKV))
	} else {
		channels = mergeChannelInventory(parseChannelTitles(titlesKV), parseChannelStreams(encodeKV), nil)
	}

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

func mergeChannelInventory(titles map[int]string, streams map[int]channelInventory, remoteDevices map[int]remoteDeviceInventory) []channelInventory {
	seen := make(map[int]struct{})
	indexes := make([]int, 0, len(titles)+len(streams)+len(remoteDevices))

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
	for index := range remoteDevices {
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
		item.RemoteDevice = remoteDevices[index]
		items = append(items, item)
	}

	return items
}

func channelInventoryUsable(item channelInventory) bool {
	return strings.TrimSpace(item.MainResolution) != "" ||
		strings.TrimSpace(item.MainCodec) != "" ||
		strings.TrimSpace(item.SubResolution) != "" ||
		strings.TrimSpace(item.SubCodec) != "" ||
		strings.TrimSpace(item.RemoteDevice.Address) != ""
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
			item = &dahua.NVRRecording{Source: "nvr"}
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

	rawItems, _ := values["infos"].([]any)
	if len(rawItems) == 0 {
		rawItems, _ = values["items"].([]any)
	}
	result.Items = make([]dahua.NVRRecording, 0, len(rawItems))
	for _, rawItem := range rawItems {
		itemMap, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		item := dahua.NVRRecording{
			Source:      "nvr",
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

func formatRecordingWallTime(value time.Time) string {
	return value.In(time.Local).Format(recordingTimeLayout)
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
