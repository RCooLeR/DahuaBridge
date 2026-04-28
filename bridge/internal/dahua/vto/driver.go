package vto

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
	dahuartsp "RCooLeR/DahuaBridge/internal/dahua/rtsp"
	"RCooLeR/DahuaBridge/internal/onvif"
	"github.com/rs/zerolog"
)

type Driver struct {
	mu     sync.RWMutex
	cfg    config.DeviceConfig
	client *cgi.Client
	rpc    *rpcClient
	rtsp   *dahuartsp.Checker
	onvif  *onvif.Client
	logger zerolog.Logger

	probeCache       *cachedProbeMetadata
	probeCacheExpiry time.Time
}

type lockInventory struct {
	Index              int
	Name               string
	State              string
	SensorEnabled      bool
	LockMode           string
	UnlockHoldInterval string
}

type alarmInventory struct {
	Index       int
	Name        string
	SenseMethod string
	Enabled     bool
}

type cachedProbeMetadata struct {
	systemInfo      map[string]string
	machineName     map[string]string
	softwareVersion string
	accessKV        map[string]string
	commKV          map[string]string
	alarmKV         map[string]string
	encodeKV        map[string]string
	onvifDiscovery  *onvif.Discovery
}

var (
	vtoAccessNamePattern         = regexp.MustCompile(`^table\.AccessControl\[(\d+)\]\.Name$`)
	vtoAccessStatePattern        = regexp.MustCompile(`^table\.AccessControl\[(\d+)\]\.State$`)
	vtoAccessSensorEnablePattern = regexp.MustCompile(`^table\.AccessControl\[(\d+)\]\.SensorEnable$`)
	vtoAccessLockModePattern     = regexp.MustCompile(`^table\.AccessControl\[(\d+)\]\.LockMode$`)
	vtoAccessUnlockHoldPattern   = regexp.MustCompile(`^table\.AccessControl\[(\d+)\]\.UnlockHoldInterval$`)
	vtoAlarmNamePattern          = regexp.MustCompile(`^table\.Alarm\[(\d+)\]\.Name$`)
	vtoAlarmSensePattern         = regexp.MustCompile(`^table\.Alarm\[(\d+)\]\.SenseMethod$`)
	vtoAlarmEnablePattern        = regexp.MustCompile(`^table\.Alarm\[(\d+)\]\.Enable$`)
)

const (
	vtoProbeMetadataCacheTTL     = 15 * time.Minute
	vtoProbeMetadataRetryBackoff = 3 * time.Minute
)

func New(cfg config.DeviceConfig, logger zerolog.Logger, client *cgi.Client) *Driver {
	rpc, err := newRPCClient(cfg)
	if err != nil {
		logger.Warn().Err(err).Str("device_id", cfg.ID).Msg("vto rpc client initialization failed")
	}
	return &Driver{
		cfg:    cfg,
		client: client,
		rpc:    rpc,
		rtsp:   dahuartsp.NewChecker(cfg),
		onvif:  onvif.New(cfg),
		logger: logger.With().Str("device_id", cfg.ID).Str("device_type", string(dahua.DeviceKindVTO)).Logger(),
	}
}

func (d *Driver) ID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg.ID
}

func (d *Driver) Kind() dahua.DeviceKind {
	return dahua.DeviceKindVTO
}

func (d *Driver) PollInterval() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg.PollInterval
}

func (d *Driver) Probe(ctx context.Context) (*dahua.ProbeResult, error) {
	cfg := d.currentConfig()
	metadata, err := d.loadProbeMetadata(ctx)
	if err != nil {
		return nil, err
	}

	name := firstNonEmpty(metadata.machineName["name"], cfg.Name, cfg.ID)
	firmware, buildDate := parseSoftwareVersion(metadata.softwareVersion)
	mainResolution, mainCodec, subResolution, subCodec, audioCodec := parseVTOEncode(metadata.encodeKV)
	locks := parseVTOLocks(metadata.accessKV)
	alarms := parseVTOAlarms(metadata.alarmKV)
	locks = filterVTOLocks(cfg, locks)
	alarms = filterVTOAlarms(cfg, alarms)

	raw := map[string]string{
		"deviceType":      metadata.systemInfo["deviceType"],
		"processor":       metadata.systemInfo["processor"],
		"serialNumber":    metadata.systemInfo["serialNumber"],
		"updateSerial":    metadata.systemInfo["updateSerial"],
		"name":            name,
		"version":         firmware,
		"build_date":      buildDate,
		"current_profile": metadata.commKV["table.CommGlobal.CurrentProfile"],
		"alarm_enable":    metadata.commKV["table.CommGlobal.AlarmEnable"],
		"main_resolution": mainResolution,
		"main_codec":      mainCodec,
		"sub_resolution":  subResolution,
		"sub_codec":       subCodec,
		"audio_codec":     audioCodec,
	}

	root := dahua.Device{
		ID:           cfg.ID,
		Name:         name,
		Manufacturer: cfg.Manufacturer,
		Model:        firstNonEmpty(cfg.Model, metadata.systemInfo["updateSerial"]),
		Serial:       metadata.systemInfo["serialNumber"],
		Firmware:     firmware,
		BuildDate:    buildDate,
		BaseURL:      cfg.BaseURL,
		Kind:         dahua.DeviceKindVTO,
		Attributes: map[string]string{
			"device_type":       metadata.systemInfo["deviceType"],
			"processor":         metadata.systemInfo["processor"],
			"update_serial":     metadata.systemInfo["updateSerial"],
			"current_profile":   metadata.commKV["table.CommGlobal.CurrentProfile"],
			"alarm_enable":      metadata.commKV["table.CommGlobal.AlarmEnable"],
			"main_resolution":   mainResolution,
			"main_codec":        mainCodec,
			"sub_resolution":    subResolution,
			"sub_codec":         subCodec,
			"audio_codec":       audioCodec,
			"snapshot_path":     fmt.Sprintf("/api/v1/vto/%s/snapshot", cfg.ID),
			"rtsp_main_url":     buildVTOStreamURL(cfg.BaseURL, 0),
			"rtsp_sub_url":      buildVTOStreamURL(cfg.BaseURL, 1),
			"lock_count":        strconv.Itoa(len(locks)),
			"alarm_input_count": strconv.Itoa(len(alarms)),
		},
	}

	children := make([]dahua.Device, 0, len(locks)+len(alarms))
	states := map[string]dahua.DeviceState{
		cfg.ID: {
			Available: true,
			Info: map[string]any{
				"name":              name,
				"serial":            metadata.systemInfo["serialNumber"],
				"firmware":          firmware,
				"build_date":        buildDate,
				"current_profile":   metadata.commKV["table.CommGlobal.CurrentProfile"],
				"alarm_enable":      parseBool(metadata.commKV["table.CommGlobal.AlarmEnable"]),
				"main_resolution":   mainResolution,
				"main_codec":        mainCodec,
				"sub_resolution":    subResolution,
				"sub_codec":         subCodec,
				"audio_codec":       audioCodec,
				"rtsp_main_url":     buildVTOStreamURL(cfg.BaseURL, 0),
				"rtsp_sub_url":      buildVTOStreamURL(cfg.BaseURL, 1),
				"snapshot_rel_url":  fmt.Sprintf("/api/v1/vto/%s/snapshot", cfg.ID),
				"lock_count":        len(locks),
				"alarm_input_count": len(alarms),
				"call_state":        "idle",
				"stream_available":  d.streamAvailable(ctx, cfg),
			},
		},
	}

	if metadata.onvifDiscovery != nil {
		discovery := metadata.onvifDiscovery
		root.Attributes["onvif_h264_profile_count"] = strconv.Itoa(discovery.H264ProfileCount())
		root.Attributes["onvif_device_service_url"] = discovery.DeviceServiceURL
		root.Attributes["onvif_media_service_url"] = discovery.MediaServiceURL
		raw["onvif_device_service_url"] = discovery.DeviceServiceURL
		raw["onvif_media_service_url"] = discovery.MediaServiceURL
		raw["onvif_h264_profile_count"] = strconv.Itoa(discovery.H264ProfileCount())

		state := states[cfg.ID]
		state.Info["onvif_probed"] = true
		state.Info["onvif_device_service_url"] = discovery.DeviceServiceURL
		state.Info["onvif_media_service_url"] = discovery.MediaServiceURL
		state.Info["onvif_h264_profile_count"] = discovery.H264ProfileCount()
		state.Info["onvif_profiles"] = discovery.ProfileMaps()

		best, ok := discovery.BestH264ProfileForChannel(1)
		if ok {
			root.Attributes["onvif_h264_available"] = "true"
			root.Attributes["onvif_profile_token"] = best.Token
			root.Attributes["onvif_profile_name"] = best.Name
			root.Attributes["onvif_stream_url"] = best.StreamURI
			if best.SnapshotURI != "" {
				root.Attributes["onvif_snapshot_url"] = best.SnapshotURI
			}
			root.Attributes["recommended_ha_integration"] = "onvif"

			state.Info["onvif_h264_available"] = true
			state.Info["onvif_profile_token"] = best.Token
			state.Info["onvif_profile_name"] = best.Name
			state.Info["onvif_stream_url"] = best.StreamURI
			if best.SnapshotURI != "" {
				state.Info["onvif_snapshot_url"] = best.SnapshotURI
			}
			state.Info["recommended_ha_integration"] = "onvif"
		} else {
			root.Attributes["onvif_h264_available"] = "false"
			root.Attributes["recommended_ha_integration"] = "bridge_media"
			state.Info["onvif_h264_available"] = false
			state.Info["recommended_ha_integration"] = "bridge_media"
		}

		states[cfg.ID] = state
	}

	for _, lock := range locks {
		childID := lockDeviceID(cfg.ID, lock.Index)
		children = append(children, dahua.Device{
			ID:           childID,
			ParentID:     cfg.ID,
			Name:         firstNonEmpty(lock.Name, fmt.Sprintf("Door %d", lock.Index+1)),
			Manufacturer: cfg.Manufacturer,
			Model:        "VTO Lock",
			BaseURL:      cfg.BaseURL,
			Kind:         dahua.DeviceKindVTOLock,
			Attributes: map[string]string{
				"lock_index":           strconv.Itoa(lock.Index),
				"state":                lock.State,
				"sensor_enabled":       strconv.FormatBool(lock.SensorEnabled),
				"lock_mode":            lock.LockMode,
				"unlock_hold_interval": lock.UnlockHoldInterval,
				"device_category":      "lock",
				"inventory_source":     "cgi",
			},
		})
		states[childID] = dahua.DeviceState{
			Available: true,
			Info: map[string]any{
				"name":                 firstNonEmpty(lock.Name, fmt.Sprintf("Door %d", lock.Index+1)),
				"state":                lock.State,
				"sensor_enabled":       lock.SensorEnabled,
				"lock_mode":            lock.LockMode,
				"unlock_hold_interval": lock.UnlockHoldInterval,
			},
		}
	}

	for _, alarm := range alarms {
		childID := alarmDeviceID(cfg.ID, alarm.Index)
		children = append(children, dahua.Device{
			ID:           childID,
			ParentID:     cfg.ID,
			Name:         fmt.Sprintf("Alarm %d", alarm.Index+1),
			Manufacturer: cfg.Manufacturer,
			Model:        "VTO Alarm Input",
			BaseURL:      cfg.BaseURL,
			Kind:         dahua.DeviceKindVTOAlarm,
			Attributes: map[string]string{
				"alarm_index":      strconv.Itoa(alarm.Index),
				"alarm_enabled":    strconv.FormatBool(alarm.Enabled),
				"sense_method":     alarm.SenseMethod,
				"device_category":  "alarm_input",
				"inventory_source": "cgi",
			},
		})
		states[childID] = dahua.DeviceState{
			Available: true,
			Info: map[string]any{
				"name":         firstNonEmpty(alarm.Name, fmt.Sprintf("Alarm %d", alarm.Index+1)),
				"sense_method": alarm.SenseMethod,
				"enabled":      alarm.Enabled,
			},
		}
	}

	return &dahua.ProbeResult{
		Root:     root,
		Children: children,
		States:   states,
		Raw:      raw,
	}, nil
}

func (d *Driver) Snapshot(ctx context.Context, _ int) ([]byte, string, error) {
	return d.client.GetBinary(ctx, "/cgi-bin/snapshot.cgi", nil)
}

func (d *Driver) Unlock(ctx context.Context, index int) error {
	if d.rpc == nil {
		return fmt.Errorf("vto rpc client is not initialized")
	}
	if index < 0 {
		return fmt.Errorf("invalid lock index %d", index)
	}

	params := map[string]any{
		"Type": "Remote",
	}
	if index > 0 {
		params["DoorIndex"] = index
	}

	if err := d.rpc.Call(ctx, "accessControl.openDoor", params, nil); err != nil {
		return fmt.Errorf("unlock door %d: %w", index, err)
	}

	return nil
}

func (d *Driver) HangupCall(ctx context.Context) error {
	if d.rpc == nil {
		return fmt.Errorf("vto rpc client is not initialized")
	}

	objectID, err := d.videoTalkPhoneInstance(ctx)
	if err == nil {
		callState, stateErr := d.videoTalkPhoneCallState(ctx, objectID)
		if stateErr == nil && strings.TrimSpace(callState) != "" && !strings.EqualFold(callState, "idle") {
			if err := d.rpc.CallObject(ctx, "VideoTalkPhone.disconnect", nil, objectID, nil); err == nil {
				return nil
			}
			if err := d.rpc.CallObject(ctx, "VideoTalkPhone.endCall", nil, objectID, nil); err == nil {
				return nil
			}
		}
	}

	if err := d.rpc.Call(ctx, "console.runCmd", map[string]any{"command": "hc"}, nil); err != nil {
		return fmt.Errorf("hangup call: %w", err)
	}

	return nil
}

func (d *Driver) AnswerCall(ctx context.Context) error {
	if d.rpc == nil {
		return fmt.Errorf("vto rpc client is not initialized")
	}

	objectID, err := d.videoTalkPhoneInstance(ctx)
	if err != nil {
		return err
	}
	if err := d.rpc.CallObject(ctx, "VideoTalkPhone.answer", nil, objectID, nil); err != nil {
		return fmt.Errorf("answer call: %w", err)
	}

	return nil
}

func (d *Driver) videoTalkPhoneInstance(ctx context.Context) (int64, error) {
	var objectID int64
	if err := d.rpc.Call(ctx, "VideoTalkPhone.factory.instance", nil, &objectID); err != nil {
		return 0, fmt.Errorf("create video talk phone instance: %w", err)
	}
	return objectID, nil
}

func (d *Driver) videoTalkPhoneCallState(ctx context.Context, objectID int64) (string, error) {
	var response struct {
		CallState string `json:"callState"`
	}
	if err := d.rpc.CallObject(ctx, "VideoTalkPhone.getCallState", nil, objectID, &response); err != nil {
		return "", fmt.Errorf("get call state: %w", err)
	}
	return response.CallState, nil
}

func (d *Driver) UpdateConfig(cfg config.DeviceConfig) error {
	d.mu.Lock()
	d.cfg = cfg
	d.probeCache = nil
	d.probeCacheExpiry = time.Time{}
	d.mu.Unlock()
	d.client.UpdateConfig(cfg)
	if d.rpc != nil {
		if err := d.rpc.UpdateConfig(cfg); err != nil {
			return err
		}
	}
	d.rtsp.UpdateConfig(cfg)
	d.onvif.UpdateConfig(cfg)
	return nil
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

func parseVTOLocks(values map[string]string) []lockInventory {
	items := map[int]*lockInventory{}
	get := func(index int) *lockInventory {
		item, ok := items[index]
		if !ok {
			item = &lockInventory{Index: index}
			items[index] = item
		}
		return item
	}

	for key, value := range values {
		switch {
		case vtoAccessNamePattern.MatchString(key):
			idx := mustSubmatchInt(vtoAccessNamePattern, key)
			get(idx).Name = value
		case vtoAccessStatePattern.MatchString(key):
			idx := mustSubmatchInt(vtoAccessStatePattern, key)
			get(idx).State = value
		case vtoAccessSensorEnablePattern.MatchString(key):
			idx := mustSubmatchInt(vtoAccessSensorEnablePattern, key)
			get(idx).SensorEnabled = parseBool(value)
		case vtoAccessLockModePattern.MatchString(key):
			idx := mustSubmatchInt(vtoAccessLockModePattern, key)
			get(idx).LockMode = value
		case vtoAccessUnlockHoldPattern.MatchString(key):
			idx := mustSubmatchInt(vtoAccessUnlockHoldPattern, key)
			get(idx).UnlockHoldInterval = value
		}
	}

	indexes := make([]int, 0, len(items))
	for idx := range items {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	result := make([]lockInventory, 0, len(indexes))
	for _, idx := range indexes {
		item := items[idx]
		if item.Name == "" && item.State == "" && item.UnlockHoldInterval == "" {
			continue
		}
		result = append(result, *item)
	}
	return result
}

func parseVTOAlarms(values map[string]string) []alarmInventory {
	items := map[int]*alarmInventory{}
	get := func(index int) *alarmInventory {
		item, ok := items[index]
		if !ok {
			item = &alarmInventory{Index: index}
			items[index] = item
		}
		return item
	}

	for key, value := range values {
		switch {
		case vtoAlarmNamePattern.MatchString(key):
			idx := mustSubmatchInt(vtoAlarmNamePattern, key)
			get(idx).Name = value
		case vtoAlarmSensePattern.MatchString(key):
			idx := mustSubmatchInt(vtoAlarmSensePattern, key)
			get(idx).SenseMethod = value
		case vtoAlarmEnablePattern.MatchString(key):
			idx := mustSubmatchInt(vtoAlarmEnablePattern, key)
			get(idx).Enabled = parseBool(value)
		}
	}

	indexes := make([]int, 0, len(items))
	for idx := range items {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	result := make([]alarmInventory, 0, len(indexes))
	for _, idx := range indexes {
		result = append(result, *items[idx])
	}
	return result
}

func filterVTOLocks(cfg config.DeviceConfig, items []lockInventory) []lockInventory {
	if len(items) == 0 {
		return nil
	}

	filtered := make([]lockInventory, 0, len(items))
	for _, item := range items {
		if !cfg.AllowsLock(item.Index + 1) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func filterVTOAlarms(cfg config.DeviceConfig, items []alarmInventory) []alarmInventory {
	if len(items) == 0 {
		return nil
	}

	filtered := make([]alarmInventory, 0, len(items))
	for _, item := range items {
		if !cfg.AllowsAlarm(item.Index + 1) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func parseVTOEncode(values map[string]string) (mainResolution string, mainCodec string, subResolution string, subCodec string, audioCodec string) {
	mainResolution = values["table.Encode[0].MainFormat[0].Video.resolution"]
	mainCodec = values["table.Encode[0].MainFormat[0].Video.Compression"]
	subResolution = values["table.Encode[0].ExtraFormat[0].Video.resolution"]
	subCodec = values["table.Encode[0].ExtraFormat[0].Video.Compression"]
	audioCodec = firstNonEmpty(
		values["table.Encode[0].MainFormat[0].Audio.Compression"],
		values["table.Encode[0].ExtraFormat[0].Audio.Compression"],
	)
	return
}

func parseBool(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func mustSubmatchInt(pattern *regexp.Regexp, value string) int {
	matches := pattern.FindStringSubmatch(value)
	if len(matches) < 2 {
		return -1
	}
	parsed, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1
	}
	return parsed
}

func lockDeviceID(rootID string, index int) string {
	return fmt.Sprintf("%s_lock_%02d", rootID, index)
}

func alarmDeviceID(rootID string, index int) string {
	return fmt.Sprintf("%s_alarm_%02d", rootID, index)
}

func buildVTOStreamURL(baseURL string, subtype int) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}

	host := net.JoinHostPort(parsed.Hostname(), "554")
	return (&url.URL{
		Scheme:   "rtsp",
		Host:     host,
		Path:     "/cam/realmonitor",
		RawQuery: url.Values{"channel": []string{"1"}, "subtype": []string{strconv.Itoa(subtype)}}.Encode(),
	}).String()
}

func (d *Driver) streamAvailable(ctx context.Context, cfg config.DeviceConfig) bool {
	available, err := d.rtsp.StreamAvailable(
		ctx,
		buildVTOStreamURL(cfg.BaseURL, 1),
		buildVTOStreamURL(cfg.BaseURL, 0),
	)
	if err != nil {
		d.logger.Debug().Err(err).Msg("rtsp availability probe failed")
	}
	return available
}

func (d *Driver) loadProbeMetadata(ctx context.Context) (*cachedProbeMetadata, error) {
	if cached, ok := d.cachedProbeMetadata(); ok {
		return cached, nil
	}

	stale := d.cachedProbeMetadataStale()
	metadata := cloneCachedProbeMetadata(stale)
	if metadata == nil {
		metadata = &cachedProbeMetadata{}
	}
	refreshed := false

	systemInfo, err := d.client.GetKeyValues(ctx, "/cgi-bin/magicBox.cgi", url.Values{
		"action": []string{"getSystemInfo"},
	})
	if err != nil {
		if len(metadata.systemInfo) == 0 {
			return nil, fmt.Errorf("fetch system info: %w", err)
		}
		d.logger.Warn().Err(err).Msg("system info probe failed, using cached value")
	} else {
		metadata.systemInfo = cloneStringMap(systemInfo)
		refreshed = true
	}

	if machineName, err := d.client.GetKeyValues(ctx, "/cgi-bin/magicBox.cgi", url.Values{
		"action": []string{"getMachineName"},
	}); err != nil {
		d.logCachedProbeFailure("machine name probe failed", err, len(metadata.machineName) > 0)
	} else {
		metadata.machineName = cloneStringMap(machineName)
		refreshed = true
	}

	if softwareVersion, err := d.client.GetText(ctx, "/cgi-bin/magicBox.cgi", url.Values{
		"action": []string{"getSoftwareVersion"},
	}); err != nil {
		d.logCachedProbeFailure("software version probe failed", err, metadata.softwareVersion != "")
	} else {
		metadata.softwareVersion = softwareVersion
		refreshed = true
	}

	if accessKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"AccessControl"},
	}); err != nil {
		d.logCachedProbeFailure("access control probe failed", err, len(metadata.accessKV) > 0)
	} else {
		metadata.accessKV = cloneStringMap(accessKV)
		refreshed = true
	}

	if commKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"CommGlobal"},
	}); err != nil {
		d.logCachedProbeFailure("comm global probe failed", err, len(metadata.commKV) > 0)
	} else {
		metadata.commKV = cloneStringMap(commKV)
		refreshed = true
	}

	if alarmKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"Alarm"},
	}); err != nil {
		d.logCachedProbeFailure("alarm probe failed", err, len(metadata.alarmKV) > 0)
	} else {
		metadata.alarmKV = cloneStringMap(alarmKV)
		refreshed = true
	}

	if encodeKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"Encode"},
	}); err != nil {
		d.logCachedProbeFailure("encode probe failed", err, len(metadata.encodeKV) > 0)
	} else {
		metadata.encodeKV = cloneStringMap(encodeKV)
		refreshed = true
	}

	if d.onvif.Enabled() {
		if discovery, err := d.onvif.Discover(ctx); err != nil {
			d.logCachedProbeFailure("onvif probe failed", err, metadata.onvifDiscovery != nil)
		} else {
			metadata.onvifDiscovery = cloneONVIFDiscovery(discovery)
			refreshed = true
		}
	}

	d.storeProbeMetadata(metadata, d.nextProbeMetadataTTL(refreshed, stale != nil))
	return metadata, nil
}

func (d *Driver) logCachedProbeFailure(message string, err error, cached bool) {
	event := d.logger.Warn().Err(err)
	if cached {
		event.Msg(message + ", using cached value")
		return
	}
	event.Msg(message)
}

func (d *Driver) cachedProbeMetadata() (*cachedProbeMetadata, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.probeCache == nil || time.Now().After(d.probeCacheExpiry) {
		return nil, false
	}
	return cloneCachedProbeMetadata(d.probeCache), true
}

func (d *Driver) cachedProbeMetadataStale() *cachedProbeMetadata {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return cloneCachedProbeMetadata(d.probeCache)
}

func (d *Driver) storeProbeMetadata(metadata *cachedProbeMetadata, ttl time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.probeCache = cloneCachedProbeMetadata(metadata)
	d.probeCacheExpiry = time.Now().Add(ttl)
}

func (d *Driver) nextProbeMetadataTTL(refreshed bool, hadStale bool) time.Duration {
	if !refreshed && hadStale {
		return vtoProbeMetadataRetryBackoff
	}
	return vtoProbeMetadataCacheTTL
}

func cloneCachedProbeMetadata(value *cachedProbeMetadata) *cachedProbeMetadata {
	if value == nil {
		return nil
	}
	return &cachedProbeMetadata{
		systemInfo:      cloneStringMap(value.systemInfo),
		machineName:     cloneStringMap(value.machineName),
		softwareVersion: value.softwareVersion,
		accessKV:        cloneStringMap(value.accessKV),
		commKV:          cloneStringMap(value.commKV),
		alarmKV:         cloneStringMap(value.alarmKV),
		encodeKV:        cloneStringMap(value.encodeKV),
		onvifDiscovery:  cloneONVIFDiscovery(value.onvifDiscovery),
	}
}

func cloneStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneONVIFDiscovery(value *onvif.Discovery) *onvif.Discovery {
	if value == nil {
		return nil
	}
	cloned := *value
	if len(value.Profiles) > 0 {
		cloned.Profiles = append([]onvif.Profile(nil), value.Profiles...)
	}
	return &cloned
}

var _ dahua.Driver = (*Driver)(nil)
var _ dahua.SnapshotProvider = (*Driver)(nil)
var _ dahua.VTOLockController = (*Driver)(nil)
var _ dahua.VTOCallController = (*Driver)(nil)
var _ dahua.ConfigurableDriver = (*Driver)(nil)

func (d *Driver) currentConfig() config.DeviceConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}
