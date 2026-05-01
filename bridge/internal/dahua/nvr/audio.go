package nvr

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	dahuarpc "RCooLeR/DahuaBridge/internal/dahua/rpc"
)

const (
	rpcAuthorityDeniedCode = 285278249
)

type remoteSpeakCapsResponse struct {
	Caps []remoteSpeakCaps `json:"Caps"`
}

type remoteSpeakCaps struct {
	AudioPlayPath        []remoteSpeakPath   `json:"AudioPlayPath"`
	SupportAudioPlay     bool                `json:"SupportAudioPlay"`
	SupportQuickReply    bool                `json:"SupportQuickReply"`
	SupportSiren         bool                `json:"SupportSiren"`
	SupportedAudioFormat []remoteSpeakFormat `json:"SupportedAudioFormat"`
}

type remoteSpeakPath struct {
	Path          string `json:"Path"`
	SupportUpload bool   `json:"SupportUpload"`
}

type remoteSpeakFormat struct {
	Format string `json:"Format"`
}

type remoteFileListResponse struct {
	FileInfo []remoteFileInfo `json:"FileInfo"`
}

type remoteFileInfo struct {
	Path string `json:"Path"`
	Size int64  `json:"Size"`
}

func (d *Driver) audioCapabilities(ctx context.Context, channel int) (dahua.NVRChannelAudioCapabilities, []string) {
	capabilities := dahua.NVRChannelAudioCapabilities{}
	notes := make([]string, 0, 4)

	if enabled, known, codec := d.channelAudioStreamState(ctx, channel); known {
		capabilities.Supported = true
		capabilities.Muted = !enabled
		capabilities.StreamEnabled = enabled
		if strings.TrimSpace(codec) != "" {
			notes = append(notes, "channel_main_stream_audio_codec_detected")
		}
		notes = append(notes, "channel_audio_transcode_managed_by_bridge")
	}

	playback, playbackNotes := d.remoteSpeakCapabilities(ctx, channel)
	capabilities.Playback = playback
	capabilities.Supported = capabilities.Supported || capabilities.Playback.Supported
	notes = append(notes, playbackNotes...)

	if playback.Supported {
		volumePermissionDenied, volumeProbeFailed := d.remoteSpeakVolumeProbe(ctx, channel)
		capabilities.VolumePermissionDenied = volumePermissionDenied
		if volumePermissionDenied {
			notes = append(notes, "channel_audio_volume_control_permission_denied")
		} else if volumeProbeFailed {
			notes = append(notes, "channel_audio_volume_probe_failed")
		}
	}

	if capabilities.Supported && !capabilities.Mute && !capabilities.Volume {
		notes = append(notes, "channel_audio_device_mute_not_exposed")
	}

	return capabilities, uniqueSortedStrings(notes)
}

func (d *Driver) remoteSpeakCapabilities(ctx context.Context, channel int) (dahua.NVRChannelAudioPlaybackCapabilities, []string) {
	if d.rpc == nil {
		return dahua.NVRChannelAudioPlaybackCapabilities{}, nil
	}

	var response remoteSpeakCapsResponse
	if err := d.rpc.Call(ctx, "RemoteSpeak.getCaps", map[string]any{"Channels": []int{channel}}, &response); err != nil {
		if isUnknownRPCError(err) {
			return dahua.NVRChannelAudioPlaybackCapabilities{}, nil
		}
		if isAuthorityDeniedRPCError(err) {
			return dahua.NVRChannelAudioPlaybackCapabilities{}, []string{"channel_remote_speak_capability_permission_denied"}
		}
		return dahua.NVRChannelAudioPlaybackCapabilities{}, []string{"channel_remote_speak_capability_probe_failed"}
	}
	if len(response.Caps) == 0 {
		return dahua.NVRChannelAudioPlaybackCapabilities{}, nil
	}

	caps := response.Caps[0]
	playback := dahua.NVRChannelAudioPlaybackCapabilities{
		Supported:  caps.SupportAudioPlay || caps.SupportSiren || caps.SupportQuickReply || len(caps.AudioPlayPath) > 0 || len(caps.SupportedAudioFormat) > 0,
		Siren:      caps.SupportSiren,
		QuickReply: caps.SupportQuickReply,
		Formats:    uniqueSortedStrings(extractAudioFormats(caps.SupportedAudioFormat)),
	}
	if !playback.Supported {
		return playback, nil
	}

	notes := []string{"channel_remote_speak_supported"}
	if files, err := d.remoteSpeakFiles(ctx, channel); err == nil {
		playback.Files = files
		playback.FileCount = len(files)
	} else if isAuthorityDeniedRPCError(err) {
		notes = append(notes, "channel_remote_speak_file_list_permission_denied")
	} else if !isUnknownRPCError(err) {
		notes = append(notes, "channel_remote_speak_file_list_probe_failed")
	}

	return playback, uniqueSortedStrings(notes)
}

func (d *Driver) remoteSpeakFiles(ctx context.Context, channel int) ([]dahua.NVRChannelAudioFile, error) {
	if d.rpc == nil {
		return nil, nil
	}
	var response remoteFileListResponse
	if err := d.rpc.Call(ctx, "RemoteFileManager.listCache", map[string]any{"Channel": channel}, &response); err != nil {
		return nil, err
	}
	files := make([]dahua.NVRChannelAudioFile, 0, len(response.FileInfo))
	for _, info := range response.FileInfo {
		files = append(files, dahua.NVRChannelAudioFile{
			Name:      path.Base(strings.TrimSpace(info.Path)),
			Path:      strings.TrimSpace(info.Path),
			SizeBytes: info.Size,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
	return files, nil
}

func (d *Driver) remoteSpeakVolumeProbe(ctx context.Context, channel int) (bool, bool) {
	if d.rpc == nil {
		return false, false
	}
	err := d.rpc.Call(ctx, "RemoteFileManager.GetVolume", map[string]any{"Channel": channel + 1000}, nil)
	if err == nil {
		return false, false
	}
	if isAuthorityDeniedRPCError(err) {
		return true, false
	}
	if isUnknownRPCError(err) {
		return false, false
	}
	return false, true
}

func (d *Driver) SetAudioMute(ctx context.Context, request dahua.NVRAudioRequest) error {
	cfg := d.currentConfig()
	if request.Channel <= 0 {
		return fmt.Errorf("channel must be >= 1")
	}
	if !cfg.AllowsChannel(request.Channel) {
		return fmt.Errorf("%w: channel %d is not allowed", dahua.ErrUnsupportedOperation, request.Channel)
	}
	return fmt.Errorf("%w: channel %d stream-audio changes are disabled; bridge output audio is handled at transcode time", dahua.ErrUnsupportedOperation, request.Channel)
}

func (d *Driver) audioControlAuthority(ctx context.Context, channel int) string {
	_ = ctx
	_ = channel
	return "bridge_transcode"
}

func extractAudioFormats(values []remoteSpeakFormat) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value.Format); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func isAuthorityDeniedRPCError(err error) bool {
	var rpcErr *dahuarpc.Error
	return errors.As(err, &rpcErr) && rpcErr != nil && rpcErr.Code == rpcAuthorityDeniedCode
}

func isUnknownRPCError(err error) bool {
	var rpcErr *dahuarpc.Error
	return errors.As(err, &rpcErr) && rpcErr != nil && rpcErr.Code == 268959743
}

func (d *Driver) channelAudioStreamState(ctx context.Context, channel int) (bool, bool, string) {
	if d.client == nil {
		return false, false, ""
	}
	inventory, err := d.loadInventory(ctx)
	if err != nil {
		return false, false, ""
	}
	for _, item := range inventory {
		if item.Index+1 != channel {
			continue
		}
		return item.AudioEnabled, item.AudioKnown, strings.TrimSpace(item.AudioCodec)
	}
	return false, false, ""
}

func (d *Driver) setChannelMainAudioEnabled(ctx context.Context, channel int, enabled bool) error {
	if allowed, reason := d.nvrAudioWriteStatus(ctx, channel); !allowed {
		switch strings.TrimSpace(reason) {
		case "permission_denied":
			return fmt.Errorf("%w: stream-audio control requires nvr audio config-write permission on channel %d", dahua.ErrUnsupportedOperation, channel)
		case "disabled_by_config":
			return fmt.Errorf("%w: stream-audio control requires allow_config_writes=true on channel %d", dahua.ErrUnsupportedOperation, channel)
		default:
			return fmt.Errorf("%w: stream-audio control requires a writable nvr audio config surface on channel %d", dahua.ErrUnsupportedOperation, channel)
		}
	}
	if d.client == nil {
		return fmt.Errorf("%w: audio mute control is not supported on channel %d", dahua.ErrUnsupportedOperation, channel)
	}
	values, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"Encode"},
	})
	if err != nil {
		return err
	}

	keys := channelAudioEnableKeys(channel, values)
	if len(keys) == 0 {
		return fmt.Errorf("%w: audio mute control is not supported on channel %d", dahua.ErrUnsupportedOperation, channel)
	}

	for _, tablePrefix := range []bool{true, false} {
		body, err := d.client.GetText(ctx, "/cgi-bin/configManager.cgi", channelAudioEnableQuery(keys, enabled, tablePrefix))
		if err != nil {
			if isUnsupportedRecordConfigError(err) {
				continue
			}
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(body), "OK") {
			return fmt.Errorf("audio action returned %q", strings.TrimSpace(body))
		}
		d.InvalidateInventoryCache()
		return nil
	}
	return dahua.ErrUnsupportedOperation
}

func (d *Driver) nvrAudioWriteStatus(ctx context.Context, channel int) (bool, string) {
	if channel <= 0 {
		return false, "invalid_channel"
	}

	d.audioWriteMu.RLock()
	if d.audioWriteStatus != nil {
		if cached, ok := d.audioWriteStatus[channel]; ok && time.Since(cached.Checked) < nvrConfigWriteCacheTTL {
			d.audioWriteMu.RUnlock()
			return cached.Allowed, cached.Reason
		}
	}
	d.audioWriteMu.RUnlock()

	allowed, reason := d.probeNVRAudioWriteStatus(ctx, channel)

	d.audioWriteMu.Lock()
	if d.audioWriteStatus == nil {
		d.audioWriteStatus = make(map[int]channelWriteStatus)
	}
	d.audioWriteStatus[channel] = channelWriteStatus{
		Checked: time.Now(),
		Allowed: allowed,
		Reason:  reason,
	}
	d.audioWriteMu.Unlock()
	return allowed, reason
}

func (d *Driver) probeNVRAudioWriteStatus(ctx context.Context, channel int) (bool, string) {
	if d.client == nil {
		return false, "client_unavailable"
	}
	if allowed, reason := d.nvrConfigWriteStatus(ctx); !allowed {
		return false, reason
	}
	values, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"Encode"},
	})
	if err != nil {
		return false, "probe_failed"
	}

	keys := channelAudioEnableKeys(channel, values)
	if _, ok := firstKnownAudioEnabled(keys, values); !ok {
		return false, "audio_key_unavailable"
	}

	for _, tablePrefix := range []bool{true, false} {
		query, ok := channelAudioCurrentValueQuery(keys, values, tablePrefix)
		if !ok {
			continue
		}
		body, err := d.client.GetText(ctx, "/cgi-bin/configManager.cgi", query)
		if err != nil {
			if isUnsupportedRecordConfigError(err) {
				continue
			}
			if isAuthorityDeniedConfigError(err) {
				return false, "permission_denied"
			}
			return false, "probe_failed"
		}
		if !strings.EqualFold(strings.TrimSpace(body), "OK") {
			return false, "unexpected_response"
		}
		return true, "enabled_by_config"
	}
	return false, "audio_key_unavailable"
}

func firstKnownAudioEnabled(keys []string, values map[string]string) (bool, bool) {
	for _, key := range keys {
		if enabled, ok := parseOptionalBool(values[key]); ok {
			return enabled, true
		}
	}
	return false, false
}

func parseOptionalBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on", "yes":
		return true, true
	case "0", "false", "off", "no":
		return false, true
	default:
		return false, false
	}
}

func channelAudioEnableKeys(channel int, values map[string]string) []string {
	channelIndex := channel - 1
	keys := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)
	for key := range values {
		matches := encodeAnyAudioEnablePattern.FindStringSubmatch(key)
		if len(matches) != 3 {
			continue
		}
		index, ok := parseInt(matches[1])
		if !ok || index != channelIndex {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		keys = append(keys, fmt.Sprintf("table.Encode[%d].MainFormat[0].AudioEnable", channelIndex))
	}
	sort.Strings(keys)
	return keys
}

func channelAudioCurrentValueQuery(keys []string, values map[string]string, tablePrefix bool) (url.Values, bool) {
	query := url.Values{
		"action": []string{"setConfig"},
	}
	for _, key := range keys {
		enabled, ok := parseOptionalBool(values[key])
		if !ok {
			continue
		}
		normalizedKey := key
		if !tablePrefix {
			normalizedKey = strings.TrimPrefix(normalizedKey, "table.")
		}
		if enabled {
			query[normalizedKey] = []string{"true"}
		} else {
			query[normalizedKey] = []string{"false"}
		}
	}
	return query, len(query) > 1
}

func channelAudioEnableQuery(keys []string, enabled bool, tablePrefix bool) url.Values {
	query := url.Values{
		"action": []string{"setConfig"},
	}
	value := "false"
	if enabled {
		value = "true"
	}
	for _, key := range keys {
		normalizedKey := key
		if !tablePrefix {
			normalizedKey = strings.TrimPrefix(normalizedKey, "table.")
		}
		query[normalizedKey] = []string{value}
	}
	return query
}
