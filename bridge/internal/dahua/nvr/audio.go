package nvr

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"

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

	playback, playbackNotes := d.remoteSpeakCapabilities(ctx, channel)
	capabilities.Playback = playback
	capabilities.Supported = capabilities.Playback.Supported
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

	if !capabilities.Mute && !capabilities.Volume {
		notes = append(notes, "channel_audio_mute_volume_not_exposed")
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
