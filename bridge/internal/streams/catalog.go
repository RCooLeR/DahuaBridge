package streams

import (
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

type CatalogInput struct {
	Config             config.Config
	ProbeResults       []*dahua.ProbeResult
	NVRConfigs         map[string]config.DeviceConfig
	VTOConfigs         map[string]config.DeviceConfig
	IPCConfigs         map[string]config.DeviceConfig
	IntercomStatuses   map[string]RuntimeIntercomStatus
	IncludeCredentials bool
}

type RuntimeIntercomStatus struct {
	Active                 bool
	SessionCount           int
	ExternalUplinkEnabled  bool
	UplinkActive           bool
	UplinkCodec            string
	UplinkPackets          uint64
	UplinkTargetCount      int
	UplinkForwardedPackets uint64
	UplinkForwardErrors    uint64
}

type Entry struct {
	ID                       string                 `json:"id"`
	RootDeviceID             string                 `json:"root_device_id"`
	SourceDeviceID           string                 `json:"source_device_id"`
	DeviceKind               dahua.DeviceKind       `json:"device_kind"`
	Name                     string                 `json:"name"`
	Channel                  int                    `json:"channel,omitempty"`
	LockCount                int                    `json:"lock_count,omitempty"`
	SnapshotURL              string                 `json:"snapshot_url"`
	LocalPreviewURL          string                 `json:"local_preview_url,omitempty"`
	LocalIntercomURL         string                 `json:"local_intercom_url,omitempty"`
	Intercom                 *IntercomSummary       `json:"intercom,omitempty"`
	Capture                  *CaptureSummary        `json:"capture,omitempty"`
	MainCodec                string                 `json:"main_codec,omitempty"`
	MainResolution           string                 `json:"main_resolution,omitempty"`
	SubCodec                 string                 `json:"sub_codec,omitempty"`
	SubResolution            string                 `json:"sub_resolution,omitempty"`
	AudioCodec               string                 `json:"audio_codec,omitempty"`
	Controls                 *ChannelControlSummary `json:"controls,omitempty"`
	Features                 []FeatureSummary       `json:"features,omitempty"`
	RecommendedProfile       string                 `json:"recommended_profile"`
	RecommendedHAIntegration string                 `json:"recommended_ha_integration"`
	RecommendedHAReason      string                 `json:"recommended_ha_reason,omitempty"`
	ONVIFH264Available       bool                   `json:"onvif_h264_available"`
	ONVIFProfileToken        string                 `json:"onvif_profile_token,omitempty"`
	ONVIFProfileName         string                 `json:"onvif_profile_name,omitempty"`
	ONVIFStreamURL           string                 `json:"onvif_stream_url,omitempty"`
	ONVIFSnapshotURL         string                 `json:"onvif_snapshot_url,omitempty"`
	Profiles                 map[string]Profile     `json:"profiles"`
}

type ChannelControlSummary struct {
	PTZ             *PTZControlSummary       `json:"ptz,omitempty"`
	Aux             *AuxControlSummary       `json:"aux,omitempty"`
	Audio           *AudioControlSummary     `json:"audio,omitempty"`
	Recording       *RecordingControlSummary `json:"recording,omitempty"`
	ValidationNotes []string                 `json:"validation_notes,omitempty"`
}

type PTZControlSummary struct {
	URL            string   `json:"url,omitempty"`
	Supported      bool     `json:"supported"`
	Pan            bool     `json:"pan"`
	Tilt           bool     `json:"tilt"`
	Zoom           bool     `json:"zoom"`
	Focus          bool     `json:"focus"`
	MoveRelatively bool     `json:"move_relatively"`
	AutoScan       bool     `json:"auto_scan"`
	Preset         bool     `json:"preset"`
	Pattern        bool     `json:"pattern"`
	Tour           bool     `json:"tour"`
	Commands       []string `json:"commands,omitempty"`
}

type AuxControlSummary struct {
	URL       string   `json:"url,omitempty"`
	Supported bool     `json:"supported"`
	Outputs   []string `json:"outputs,omitempty"`
	Features  []string `json:"features,omitempty"`
}

type AudioControlSummary struct {
	Supported              bool     `json:"supported"`
	Mute                   bool     `json:"mute"`
	Volume                 bool     `json:"volume"`
	VolumePermissionDenied bool     `json:"volume_permission_denied,omitempty"`
	Muted                  bool     `json:"muted,omitempty"`
	StreamAudioEnabled     bool     `json:"stream_audio_enabled,omitempty"`
	PlaybackSupported      bool     `json:"playback_supported"`
	PlaybackSiren          bool     `json:"playback_siren"`
	PlaybackQuickReply     bool     `json:"playback_quick_reply"`
	PlaybackFormats        []string `json:"playback_formats,omitempty"`
	PlaybackFileCount      int      `json:"playback_file_count,omitempty"`
}

type RecordingControlSummary struct {
	URL       string `json:"url,omitempty"`
	Supported bool   `json:"supported"`
	Active    bool   `json:"active"`
	Mode      string `json:"mode,omitempty"`
}

type FeatureSummary struct {
	Key            string          `json:"key"`
	Label          string          `json:"label"`
	Group          string          `json:"group,omitempty"`
	Kind           string          `json:"kind"`
	URL            string          `json:"url,omitempty"`
	Supported      bool            `json:"supported"`
	ParameterKey   string          `json:"parameter_key,omitempty"`
	ParameterValue string          `json:"parameter_value,omitempty"`
	Commands       []string        `json:"commands,omitempty"`
	Actions        []string        `json:"actions,omitempty"`
	Targets        []FeatureTarget `json:"targets,omitempty"`
	AllowedValues  []int           `json:"allowed_values,omitempty"`
	MinValue       *int            `json:"min_value,omitempty"`
	MaxValue       *int            `json:"max_value,omitempty"`
	StepValue      *int            `json:"step_value,omitempty"`
	CurrentValue   *int            `json:"current_value,omitempty"`
	Active         *bool           `json:"active,omitempty"`
	CurrentText    string          `json:"current_text,omitempty"`
}

type FeatureTarget struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	URL   string `json:"url,omitempty"`
}

type IntercomSummary struct {
	CallState                           string   `json:"call_state,omitempty"`
	LastRingAt                          string   `json:"last_ring_at,omitempty"`
	LastCallStartedAt                   string   `json:"last_call_started_at,omitempty"`
	LastCallEndedAt                     string   `json:"last_call_ended_at,omitempty"`
	LastCallSource                      string   `json:"last_call_source,omitempty"`
	LastCallDurationSeconds             int      `json:"last_call_duration_seconds,omitempty"`
	AnswerURL                           string   `json:"answer_url,omitempty"`
	HangupURL                           string   `json:"hangup_url,omitempty"`
	BridgeSessionResetURL               string   `json:"bridge_session_reset_url,omitempty"`
	LockURLs                            []string `json:"lock_urls,omitempty"`
	ExternalUplinkEnableURL             string   `json:"external_uplink_enable_url,omitempty"`
	ExternalUplinkDisableURL            string   `json:"external_uplink_disable_url,omitempty"`
	OutputVolumeURL                     string   `json:"output_volume_url,omitempty"`
	InputVolumeURL                      string   `json:"input_volume_url,omitempty"`
	MuteURL                             string   `json:"mute_url,omitempty"`
	RecordingURL                        string   `json:"recording_url,omitempty"`
	BridgeSessionActive                 bool     `json:"bridge_session_active"`
	BridgeSessionCount                  int      `json:"bridge_session_count,omitempty"`
	ExternalUplinkEnabled               bool     `json:"external_uplink_enabled"`
	BridgeUplinkActive                  bool     `json:"bridge_uplink_active"`
	BridgeUplinkCodec                   string   `json:"bridge_uplink_codec,omitempty"`
	BridgeUplinkPackets                 uint64   `json:"bridge_uplink_packets,omitempty"`
	BridgeForwardedPackets              uint64   `json:"bridge_forwarded_packets,omitempty"`
	BridgeForwardErrors                 uint64   `json:"bridge_forward_errors,omitempty"`
	SupportsHangup                      bool     `json:"supports_hangup"`
	SupportsBridgeSessionReset          bool     `json:"supports_bridge_session_reset"`
	SupportsUnlock                      bool     `json:"supports_unlock"`
	SupportsBrowserMicrophone           bool     `json:"supports_browser_microphone"`
	SupportsBridgeAudioUplink           bool     `json:"supports_bridge_audio_uplink"`
	SupportsBridgeAudioOutput           bool     `json:"supports_bridge_audio_output"`
	SupportsExternalAudioExport         bool     `json:"supports_external_audio_export"`
	ConfiguredExternalUplinkTargetCount int      `json:"configured_external_uplink_target_count,omitempty"`
	SupportsVTOCallAnswer               bool     `json:"supports_vto_call_answer"`
	SupportsVTOOutputVolumeControl      bool     `json:"supports_vto_output_volume_control"`
	SupportsVTOInputVolumeControl       bool     `json:"supports_vto_input_volume_control"`
	SupportsVTOMuteControl              bool     `json:"supports_vto_mute_control"`
	SupportsVTORecordingControl         bool     `json:"supports_vto_recording_control"`
	SupportsVTOTalkback                 bool     `json:"supports_vto_talkback"`
	SupportsFullCallAcceptance          bool     `json:"supports_full_call_acceptance"`
	OutputVolumeLevel                   int      `json:"output_volume_level,omitempty"`
	OutputVolumeLevels                  []int    `json:"output_volume_levels,omitempty"`
	InputVolumeLevel                    int      `json:"input_volume_level,omitempty"`
	InputVolumeLevels                   []int    `json:"input_volume_levels,omitempty"`
	Muted                               bool     `json:"muted,omitempty"`
	AutoRecordEnabled                   bool     `json:"auto_record_enabled,omitempty"`
	AutoRecordTimeSeconds               int      `json:"auto_record_time_seconds,omitempty"`
	StreamAudioEnabled                  bool     `json:"stream_audio_enabled,omitempty"`
	ValidationNotes                     []string `json:"validation_notes,omitempty"`
}

type CaptureSummary struct {
	SnapshotURL           string `json:"snapshot_url,omitempty"`
	StartRecordingURL     string `json:"start_recording_url,omitempty"`
	StopRecordingURL      string `json:"stop_recording_url,omitempty"`
	RecordingsURL         string `json:"recordings_url,omitempty"`
	Active                bool   `json:"active"`
	ActiveClipID          string `json:"active_clip_id,omitempty"`
	ActiveClipProfile     string `json:"active_clip_profile,omitempty"`
	ActiveClipStartedAt   string `json:"active_clip_started_at,omitempty"`
	ActiveClipDownloadURL string `json:"active_clip_download_url,omitempty"`
}

type Profile struct {
	Name                     string `json:"name"`
	StreamURL                string `json:"stream_url"`
	LocalMJPEGURL            string `json:"local_mjpeg_url,omitempty"`
	LocalHLSURL              string `json:"local_hls_url,omitempty"`
	LocalWebRTCURL           string `json:"local_webrtc_url,omitempty"`
	Subtype                  int    `json:"subtype"`
	RTSPTransport            string `json:"rtsp_transport,omitempty"`
	FrameRate                int    `json:"frame_rate,omitempty"`
	SourceWidth              int    `json:"source_width,omitempty"`
	SourceHeight             int    `json:"source_height,omitempty"`
	UseWallclockAsTimestamps bool   `json:"use_wallclock_as_timestamps,omitempty"`
	Recommended              bool   `json:"recommended,omitempty"`
}

func BuildCatalog(input CatalogInput) []Entry {
	results := append([]*dahua.ProbeResult(nil), input.ProbeResults...)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Root.ID < results[j].Root.ID
	})

	entries := make([]Entry, 0)
	for _, result := range results {
		if result == nil {
			continue
		}

		switch result.Root.Kind {
		case dahua.DeviceKindNVR:
			deviceCfg, ok := input.NVRConfigs[result.Root.ID]
			if !ok {
				continue
			}
			for _, child := range result.Children {
				if child.Kind != dahua.DeviceKindNVRChannel {
					continue
				}
				channel, err := strconv.Atoi(child.Attributes["channel_index"])
				if err != nil || channel <= 0 {
					continue
				}

				mainCodec := valueOrState(child.Attributes["main_codec"], result.States[child.ID], "main_codec")
				mainResolution := valueOrState(child.Attributes["main_resolution"], result.States[child.ID], "main_resolution")
				subCodec := valueOrState(child.Attributes["sub_codec"], result.States[child.ID], "sub_codec")
				subResolution := valueOrState(child.Attributes["sub_resolution"], result.States[child.ID], "sub_resolution")
				recommended := recommendProfile(mainCodec, mainResolution, subCodec, subResolution)
				entry := Entry{
					ID:                 child.ID,
					RootDeviceID:       result.Root.ID,
					SourceDeviceID:     child.ID,
					DeviceKind:         child.Kind,
					Name:               child.Name,
					Channel:            channel,
					SnapshotURL:        snapshotURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, channel, cameraPathNVR),
					LocalPreviewURL:    buildLocalPreviewURL(input.Config.HomeAssistant.PublicBaseURL, child.ID, recommended),
					MainCodec:          mainCodec,
					MainResolution:     mainResolution,
					SubCodec:           subCodec,
					SubResolution:      subResolution,
					Controls:           buildNVRChannelControlSummary(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, channel, result.States[child.ID]),
					RecommendedProfile: recommended,
					Profiles:           buildProfiles(deviceCfg, channel, input.IncludeCredentials, recommended, input.Config.HomeAssistant.PublicBaseURL, child.ID, input.Config.Media, mainResolution, subCodec, subResolution),
				}
				entry.Features = buildNVRChannelFeatures(
					input.Config.HomeAssistant.PublicBaseURL,
					result.Root.ID,
					channel,
					entry.Controls,
					result.States[child.ID],
				)
				applyHASelection(&entry, result.States[child.ID])
				entries = append(entries, entry)
			}
		case dahua.DeviceKindVTO:
			deviceCfg, ok := input.VTOConfigs[result.Root.ID]
			if !ok {
				continue
			}

			state := result.States[result.Root.ID]
			mainCodec := valueOrState(result.Root.Attributes["main_codec"], state, "main_codec")
			mainResolution := valueOrState(result.Root.Attributes["main_resolution"], state, "main_resolution")
			subCodec := valueOrState(result.Root.Attributes["sub_codec"], state, "sub_codec")
			subResolution := valueOrState(result.Root.Attributes["sub_resolution"], state, "sub_resolution")
			audioCodec := valueOrState(result.Root.Attributes["audio_codec"], state, "audio_codec")
			recommended := recommendProfile(mainCodec, mainResolution, subCodec, subResolution)

			lockCount := intValueOrState(result.Root.Attributes["lock_count"], state, "lock_count")
			entry := Entry{
				ID:                 result.Root.ID,
				RootDeviceID:       result.Root.ID,
				SourceDeviceID:     result.Root.ID,
				DeviceKind:         result.Root.Kind,
				Name:               result.Root.Name,
				Channel:            1,
				LockCount:          lockCount,
				SnapshotURL:        snapshotURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, 0, cameraPathVTO),
				LocalPreviewURL:    buildLocalPreviewURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, recommended),
				LocalIntercomURL:   buildLocalIntercomURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, recommended),
				Intercom:           buildVTOIntercomSummary(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, state, lockCount, audioCodec, len(input.Config.Media.WebRTCUplinkTargets), input.IntercomStatuses[result.Root.ID]),
				MainCodec:          mainCodec,
				MainResolution:     mainResolution,
				SubCodec:           subCodec,
				SubResolution:      subResolution,
				AudioCodec:         audioCodec,
				RecommendedProfile: recommended,
				Profiles:           buildProfiles(deviceCfg, 1, input.IncludeCredentials, recommended, input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, input.Config.Media, mainResolution, subCodec, subResolution),
			}
			entry.Features = buildVTOFeatures(entry.Intercom)
			entries = append(entries, entry)
			applyHASelection(&entries[len(entries)-1], state)
		case dahua.DeviceKindIPC:
			deviceCfg, ok := input.IPCConfigs[result.Root.ID]
			if !ok {
				continue
			}

			state := result.States[result.Root.ID]
			mainCodec := valueOrState(result.Root.Attributes["main_codec"], state, "main_codec")
			mainResolution := valueOrState(result.Root.Attributes["main_resolution"], state, "main_resolution")
			subCodec := valueOrState(result.Root.Attributes["sub_codec"], state, "sub_codec")
			subResolution := valueOrState(result.Root.Attributes["sub_resolution"], state, "sub_resolution")
			audioCodec := valueOrState(result.Root.Attributes["audio_codec"], state, "audio_codec")
			recommended := recommendProfile(mainCodec, mainResolution, subCodec, subResolution)

			entries = append(entries, Entry{
				ID:                 result.Root.ID,
				RootDeviceID:       result.Root.ID,
				SourceDeviceID:     result.Root.ID,
				DeviceKind:         result.Root.Kind,
				Name:               result.Root.Name,
				Channel:            1,
				SnapshotURL:        snapshotURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, 0, cameraPathIPC),
				LocalPreviewURL:    buildLocalPreviewURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, recommended),
				MainCodec:          mainCodec,
				MainResolution:     mainResolution,
				SubCodec:           subCodec,
				SubResolution:      subResolution,
				AudioCodec:         audioCodec,
				RecommendedProfile: recommended,
				Profiles:           buildProfiles(deviceCfg, 1, input.IncludeCredentials, recommended, input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, input.Config.Media, mainResolution, subCodec, subResolution),
			})
			applyHASelection(&entries[len(entries)-1], state)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

func buildProfiles(deviceCfg config.DeviceConfig, channel int, includeCredentials bool, recommended string, publicBaseURL string, streamID string, mediaCfg config.MediaConfig, mainResolution string, subCodec string, subResolution string) map[string]Profile {
	mainWidth, mainHeight, mainOK := parseResolution(mainResolution)
	subWidth, subHeight, subOK := parseResolution(subResolution)
	stableFrameRate := mediaCfg.StableFrameRate
	if stableFrameRate <= 0 {
		stableFrameRate = 5
	}
	substreamFrameRate := mediaCfg.SubstreamFrameRate
	if substreamFrameRate <= 0 {
		substreamFrameRate = stableFrameRate
	}
	if !mainOK {
		mainWidth, mainHeight = 0, 0
	}
	if !subOK {
		subWidth, subHeight = 0, 0
	}
	mainSource := profileSourceVariant{
		subtype: 0,
		width:   mainWidth,
		height:  mainHeight,
	}
	substreamAvailable := strings.TrimSpace(subCodec) != "" || strings.TrimSpace(subResolution) != ""
	stableSource := resolveProfileSource(mainSource, profileSourceVariant{
		subtype: 1,
		width:   subWidth,
		height:  subHeight,
	}, substreamAvailable)
	substreamSource := resolveProfileSource(mainSource, profileSourceVariant{
		subtype: 1,
		width:   subWidth,
		height:  subHeight,
	}, substreamAvailable)

	profiles := map[string]Profile{
		"default": {
			Name:           "default",
			StreamURL:      buildRTSPURL(deviceCfg, channel, mainSource.subtype, includeCredentials),
			LocalMJPEGURL:  buildLocalMJPEGURL(publicBaseURL, streamID, "default"),
			LocalHLSURL:    buildLocalHLSURL(publicBaseURL, streamID, "default"),
			LocalWebRTCURL: buildLocalWebRTCURL(publicBaseURL, streamID, "default"),
			Subtype:        mainSource.subtype,
			SourceWidth:    mainSource.width,
			SourceHeight:   mainSource.height,
			Recommended:    recommended == "default",
		},
		"quality": {
			Name:                     "quality",
			StreamURL:                buildRTSPURL(deviceCfg, channel, mainSource.subtype, includeCredentials),
			LocalMJPEGURL:            buildLocalMJPEGURL(publicBaseURL, streamID, "quality"),
			LocalHLSURL:              buildLocalHLSURL(publicBaseURL, streamID, "quality"),
			LocalWebRTCURL:           buildLocalWebRTCURL(publicBaseURL, streamID, "quality"),
			Subtype:                  mainSource.subtype,
			RTSPTransport:            "tcp",
			SourceWidth:              mainSource.width,
			SourceHeight:             mainSource.height,
			UseWallclockAsTimestamps: true,
			Recommended:              recommended == "quality",
		},
		"stable": {
			Name:                     "stable",
			StreamURL:                buildRTSPURL(deviceCfg, channel, stableSource.subtype, includeCredentials),
			LocalMJPEGURL:            buildLocalMJPEGURL(publicBaseURL, streamID, "stable"),
			LocalHLSURL:              buildLocalHLSURL(publicBaseURL, streamID, "stable"),
			LocalWebRTCURL:           buildLocalWebRTCURL(publicBaseURL, streamID, "stable"),
			Subtype:                  stableSource.subtype,
			RTSPTransport:            "tcp",
			FrameRate:                stableFrameRate,
			SourceWidth:              stableSource.width,
			SourceHeight:             stableSource.height,
			UseWallclockAsTimestamps: true,
			Recommended:              recommended == "stable",
		},
		"substream": {
			Name:           "substream",
			StreamURL:      buildRTSPURL(deviceCfg, channel, substreamSource.subtype, includeCredentials),
			LocalMJPEGURL:  buildLocalMJPEGURL(publicBaseURL, streamID, "substream"),
			LocalHLSURL:    buildLocalHLSURL(publicBaseURL, streamID, "substream"),
			LocalWebRTCURL: buildLocalWebRTCURL(publicBaseURL, streamID, "substream"),
			Subtype:        substreamSource.subtype,
			RTSPTransport:  "tcp",
			FrameRate:      substreamFrameRate,
			SourceWidth:    substreamSource.width,
			SourceHeight:   substreamSource.height,
			Recommended:    recommended == "substream",
		},
	}
	return profiles
}

type profileSourceVariant struct {
	subtype int
	width   int
	height  int
}

func resolveProfileSource(primary profileSourceVariant, secondary profileSourceVariant, secondaryAvailable bool) profileSourceVariant {
	if secondaryAvailable {
		return secondary
	}
	return primary
}

func applyHASelection(entry *Entry, state dahua.DeviceState) {
	if entry == nil {
		return
	}

	entry.ONVIFH264Available = anyBool(state.Info, "onvif_h264_available")
	entry.ONVIFProfileToken = anyString(state.Info, "onvif_profile_token")
	entry.ONVIFProfileName = anyString(state.Info, "onvif_profile_name")
	entry.ONVIFStreamURL = anyString(state.Info, "onvif_stream_url")
	entry.ONVIFSnapshotURL = anyString(state.Info, "onvif_snapshot_url")
	entry.RecommendedHAIntegration = anyString(state.Info, "recommended_ha_integration")

	switch entry.RecommendedHAIntegration {
	case "onvif":
		entry.RecommendedHAReason = "onvif_h264_profile_available"
	case "bridge_media":
		if anyBool(state.Info, "onvif_probed") {
			entry.RecommendedHAReason = "no_onvif_h264_profile"
		} else {
			entry.RecommendedHAReason = "onvif_not_configured"
		}
	default:
		if entry.ONVIFH264Available {
			entry.RecommendedHAIntegration = "onvif"
			entry.RecommendedHAReason = "onvif_h264_profile_available"
		} else {
			entry.RecommendedHAIntegration = "bridge_media"
			entry.RecommendedHAReason = "no_onvif_h264_profile"
		}
	}
}

func recommendProfile(mainCodec string, mainResolution string, subCodec string, subResolution string) string {
	switch codecFamily(mainCodec) {
	case "h265":
		return "stable"
	}

	width, height, ok := parseResolution(mainResolution)
	if ok && (width > 1920 || height > 1080) && (strings.TrimSpace(subCodec) != "" || strings.TrimSpace(subResolution) != "") {
		return "stable"
	}

	if strings.TrimSpace(mainResolution) == "" && (strings.TrimSpace(subCodec) != "" || strings.TrimSpace(subResolution) != "") {
		return "stable"
	}

	return "quality"
}

func codecFamily(codec string) string {
	normalized := strings.ToLower(strings.TrimSpace(codec))
	normalized = strings.ReplaceAll(normalized, "smart", "")
	normalized = strings.ReplaceAll(normalized, "+", "")
	normalized = strings.ReplaceAll(normalized, ".", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, " ", "")

	switch {
	case strings.Contains(normalized, "265"), strings.Contains(normalized, "hevc"):
		return "h265"
	case strings.Contains(normalized, "264"), strings.Contains(normalized, "avc"):
		return "h264"
	case strings.Contains(normalized, "mjpeg"), strings.Contains(normalized, "mjpg"):
		return "mjpeg"
	case strings.Contains(normalized, "mpeg4"):
		return "mpeg4"
	case strings.Contains(normalized, "svac"):
		return "svac"
	default:
		return normalized
	}
}

func valueOrState(value string, state dahua.DeviceState, key string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if state.Info == nil {
		return ""
	}
	switch typed := state.Info[key].(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func intValueOrState(value string, state dahua.DeviceState, key string) int {
	if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed >= 0 {
		return parsed
	}
	if state.Info == nil {
		return 0
	}

	switch typed := state.Info[key].(type) {
	case int:
		if typed >= 0 {
			return typed
		}
	case int32:
		if typed >= 0 {
			return int(typed)
		}
	case int64:
		if typed >= 0 {
			return int(typed)
		}
	case float64:
		if typed >= 0 {
			return int(typed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed >= 0 {
			return parsed
		}
	}

	return 0
}

func anyString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch typed := values[key].(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func anyBool(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	switch typed := values[key].(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func anyInt(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	switch typed := values[key].(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func anyStringSlice(values map[string]any, key string) []string {
	if values == nil {
		return nil
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}

	switch typed := raw.(type) {
	case []string:
		cloned := make([]string, 0, len(typed))
		for _, value := range typed {
			value = strings.TrimSpace(value)
			if value != "" {
				cloned = append(cloned, value)
			}
		}
		return cloned
	case []any:
		cloned := make([]string, 0, len(typed))
		for _, value := range typed {
			if text, ok := value.(string); ok {
				text = strings.TrimSpace(text)
				if text != "" {
					cloned = append(cloned, text)
				}
			}
		}
		return cloned
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		return []string{text}
	default:
		return nil
	}
}

func anyIntSlice(values map[string]any, key string) []int {
	if values == nil {
		return nil
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}

	switch typed := raw.(type) {
	case []int:
		return append([]int(nil), typed...)
	case []any:
		items := make([]int, 0, len(typed))
		for _, item := range typed {
			switch value := item.(type) {
			case int:
				items = append(items, value)
			case int32:
				items = append(items, int(value))
			case int64:
				items = append(items, int(value))
			case float64:
				items = append(items, int(value))
			case string:
				parsed, err := strconv.Atoi(strings.TrimSpace(value))
				if err == nil {
					items = append(items, parsed)
				}
			}
		}
		if len(items) == 0 {
			return nil
		}
		return items
	case string:
		parts := strings.Split(typed, ",")
		items := make([]int, 0, len(parts))
		for _, part := range parts {
			parsed, err := strconv.Atoi(strings.TrimSpace(part))
			if err == nil {
				items = append(items, parsed)
			}
		}
		if len(items) == 0 {
			return nil
		}
		return items
	default:
		return nil
	}
}

func parseResolution(value string) (int, int, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, 0, false
	}

	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == 'x' || r == '*' || r == ',' || r == ' '
	})
	if len(parts) < 2 {
		return 0, 0, false
	}

	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return width, height, true
}

func buildLocalMJPEGURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/mjpeg/" + streamID + "?profile=" + url.QueryEscape(profile)
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildLocalHLSURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/hls/" + streamID + "/" + url.PathEscape(profile) + "/index.m3u8"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildLocalPreviewURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/preview/" + url.PathEscape(streamID)
	if strings.TrimSpace(profile) != "" {
		path += "?profile=" + url.QueryEscape(profile)
	}
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildLocalIntercomURL(publicBaseURL string, deviceID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/intercom"
	if strings.TrimSpace(profile) != "" {
		path += "?profile=" + url.QueryEscape(profile)
	}
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOIntercomSummary(publicBaseURL string, deviceID string, state dahua.DeviceState, lockCount int, audioCodec string, uplinkTargetCount int, runtimeStatus RuntimeIntercomStatus) *IntercomSummary {
	lockURLs := make([]string, 0, lockCount)
	for index := 0; index < lockCount; index++ {
		lockURLs = append(lockURLs, buildVTOLockUnlockURL(publicBaseURL, deviceID, index))
	}

	return &IntercomSummary{
		CallState:                           anyString(state.Info, "call_state"),
		LastRingAt:                          anyString(state.Info, "last_ring_at"),
		LastCallStartedAt:                   anyString(state.Info, "last_call_started_at"),
		LastCallEndedAt:                     anyString(state.Info, "last_call_ended_at"),
		LastCallSource:                      anyString(state.Info, "last_call_source"),
		LastCallDurationSeconds:             anyInt(state.Info, "last_call_duration_seconds"),
		AnswerURL:                           buildVTOAnswerURL(publicBaseURL, deviceID),
		HangupURL:                           buildVTOHangupURL(publicBaseURL, deviceID),
		BridgeSessionResetURL:               buildVTOIntercomResetURL(publicBaseURL, deviceID),
		LockURLs:                            lockURLs,
		ExternalUplinkEnableURL:             buildVTOIntercomUplinkURL(publicBaseURL, deviceID, "enable"),
		ExternalUplinkDisableURL:            buildVTOIntercomUplinkURL(publicBaseURL, deviceID, "disable"),
		OutputVolumeURL:                     buildVTOAudioOutputVolumeURL(publicBaseURL, deviceID),
		InputVolumeURL:                      buildVTOAudioInputVolumeURL(publicBaseURL, deviceID),
		MuteURL:                             buildVTOMuteURL(publicBaseURL, deviceID),
		RecordingURL:                        buildVTORecordingURL(publicBaseURL, deviceID),
		BridgeSessionActive:                 runtimeStatus.Active,
		BridgeSessionCount:                  runtimeStatus.SessionCount,
		ExternalUplinkEnabled:               runtimeStatus.ExternalUplinkEnabled,
		BridgeUplinkActive:                  runtimeStatus.UplinkActive,
		BridgeUplinkCodec:                   strings.TrimSpace(runtimeStatus.UplinkCodec),
		BridgeUplinkPackets:                 runtimeStatus.UplinkPackets,
		BridgeForwardedPackets:              runtimeStatus.UplinkForwardedPackets,
		BridgeForwardErrors:                 runtimeStatus.UplinkForwardErrors,
		SupportsVTOCallAnswer:               true,
		SupportsVTOOutputVolumeControl:      anyBool(state.Info, "control_audio_output_volume_supported"),
		SupportsVTOInputVolumeControl:       anyBool(state.Info, "control_audio_input_volume_supported"),
		SupportsVTOMuteControl:              anyBool(state.Info, "control_audio_mute_supported"),
		SupportsVTORecordingControl:         anyBool(state.Info, "control_recording_supported"),
		SupportsHangup:                      true,
		SupportsBridgeSessionReset:          true,
		SupportsUnlock:                      lockCount > 0,
		SupportsBrowserMicrophone:           true,
		SupportsBridgeAudioUplink:           true,
		SupportsBridgeAudioOutput:           strings.TrimSpace(audioCodec) != "",
		SupportsExternalAudioExport:         uplinkTargetCount > 0,
		ConfiguredExternalUplinkTargetCount: uplinkTargetCount,
		SupportsVTOTalkback:                 anyBool(state.Info, "control_direct_talkback_supported"),
		SupportsFullCallAcceptance:          anyBool(state.Info, "control_full_call_acceptance_supported"),
		OutputVolumeLevel:                   anyInt(state.Info, "control_audio_output_volume"),
		OutputVolumeLevels:                  anyIntSlice(state.Info, "control_audio_output_volume_levels"),
		InputVolumeLevel:                    anyInt(state.Info, "control_audio_input_volume"),
		InputVolumeLevels:                   anyIntSlice(state.Info, "control_audio_input_volume_levels"),
		Muted:                               anyBool(state.Info, "control_audio_muted"),
		AutoRecordEnabled:                   anyBool(state.Info, "control_recording_auto_enabled"),
		AutoRecordTimeSeconds:               anyInt(state.Info, "control_recording_auto_time_seconds"),
		StreamAudioEnabled:                  anyBool(state.Info, "control_stream_audio_enabled"),
		ValidationNotes:                     anyStringSlice(state.Info, "validation_notes"),
	}
}

func buildNVRChannelFeatures(publicBaseURL string, deviceID string, channel int, controls *ChannelControlSummary, state dahua.DeviceState) []FeatureSummary {
	features := []FeatureSummary{
		{
			Key:       "archive_search",
			Label:     "Recordings",
			Group:     "archive",
			Kind:      "query",
			URL:       buildNVRRecordingsCollectionURL(publicBaseURL, deviceID),
			Supported: true,
		},
		{
			Key:       "archive_playback",
			Label:     "Playback",
			Group:     "archive",
			Kind:      "session",
			URL:       buildNVRPlaybackSessionsCollectionURL(publicBaseURL, deviceID),
			Supported: true,
		},
	}
	if controls == nil {
		return features
	}

	if controls.PTZ != nil && controls.PTZ.Supported {
		features = append(features, FeatureSummary{
			Key:       "ptz",
			Label:     "PTZ",
			Group:     "movement",
			Kind:      "command_set",
			URL:       controls.PTZ.URL,
			Supported: true,
			Commands:  append([]string(nil), controls.PTZ.Commands...),
			Actions:   []string{"start", "stop", "pulse"},
		})
	}
	if controls.Aux != nil && controls.Aux.Supported {
		for _, descriptor := range auxFeatureDescriptors(controls.Aux) {
			if !allowDeterrenceFeature(descriptor.Key, state) {
				continue
			}
			kind := "action"
			if descriptor.Toggle {
				kind = "toggle"
			}
			features = append(features, FeatureSummary{
				Key:            descriptor.Key,
				Label:          descriptor.Label,
				Group:          "deterrence",
				Kind:           kind,
				URL:            controls.Aux.URL,
				Supported:      true,
				ParameterKey:   "output",
				ParameterValue: descriptor.ParameterValue,
				Actions:        append([]string(nil), descriptor.Actions...),
				Active:         auxFeatureActiveState(state, descriptor.Key),
				CurrentText:    auxFeatureCurrentText(state, descriptor.Key),
			})
		}
	}
	return features
}

func allowDeterrenceFeature(key string, state dahua.DeviceState) bool {
	switch strings.TrimSpace(key) {
	case "siren", "warning_light":
		return anyBool(state.Info, "control_imou_configured")
	default:
		return true
	}
}

func buildVTOFeatures(intercom *IntercomSummary) []FeatureSummary {
	if intercom == nil {
		return nil
	}

	features := make([]FeatureSummary, 0, 8)
	if intercom.SupportsVTOCallAnswer && strings.TrimSpace(intercom.AnswerURL) != "" {
		features = append(features, FeatureSummary{
			Key:       "call_answer",
			Label:     "Answer",
			Group:     "call",
			Kind:      "action",
			URL:       intercom.AnswerURL,
			Supported: true,
		})
	}
	if intercom.SupportsHangup && strings.TrimSpace(intercom.HangupURL) != "" {
		features = append(features, FeatureSummary{
			Key:       "call_hangup",
			Label:     "Hang Up",
			Group:     "call",
			Kind:      "action",
			URL:       intercom.HangupURL,
			Supported: true,
		})
	}
	if intercom.SupportsUnlock && len(intercom.LockURLs) > 0 {
		targets := make([]FeatureTarget, 0, len(intercom.LockURLs))
		for index, lockURL := range intercom.LockURLs {
			targets = append(targets, FeatureTarget{
				Key:   "lock_" + strconv.Itoa(index),
				Label: "Door " + strconv.Itoa(index+1),
				URL:   lockURL,
			})
		}
		features = append(features, FeatureSummary{
			Key:       "unlock",
			Label:     "Unlock",
			Group:     "door",
			Kind:      "targeted_action",
			Supported: true,
			Targets:   targets,
		})
	}
	if intercom.SupportsVTOOutputVolumeControl && strings.TrimSpace(intercom.OutputVolumeURL) != "" {
		features = append(features, buildVTONumberFeature("output_volume", "Output Volume", intercom.OutputVolumeURL, intercom.OutputVolumeLevel, intercom.OutputVolumeLevels))
	}
	if intercom.SupportsVTOInputVolumeControl && strings.TrimSpace(intercom.InputVolumeURL) != "" {
		features = append(features, buildVTONumberFeature("input_volume", "Input Volume", intercom.InputVolumeURL, intercom.InputVolumeLevel, intercom.InputVolumeLevels))
	}
	if intercom.SupportsVTOMuteControl && strings.TrimSpace(intercom.MuteURL) != "" {
		features = append(features, FeatureSummary{
			Key:       "mute",
			Label:     "Mute",
			Group:     "audio",
			Kind:      "toggle",
			URL:       intercom.MuteURL,
			Supported: true,
			Active:    boolPtr(intercom.Muted),
		})
	}
	if intercom.SupportsVTORecordingControl && strings.TrimSpace(intercom.RecordingURL) != "" {
		features = append(features, FeatureSummary{
			Key:       "auto_record",
			Label:     "Auto Record",
			Group:     "recording",
			Kind:      "toggle",
			URL:       intercom.RecordingURL,
			Supported: true,
			Active:    boolPtr(intercom.AutoRecordEnabled),
		})
	}
	if intercom.SupportsBridgeSessionReset && strings.TrimSpace(intercom.BridgeSessionResetURL) != "" {
		features = append(features, FeatureSummary{
			Key:       "intercom_reset",
			Label:     "Reset Session",
			Group:     "session",
			Kind:      "action",
			URL:       intercom.BridgeSessionResetURL,
			Supported: true,
		})
	}

	return features
}

type auxFeatureDescriptor struct {
	Key            string
	Label          string
	ParameterValue string
	Actions        []string
	Toggle         bool
}

func auxFeatureDescriptors(aux *AuxControlSummary) []auxFeatureDescriptor {
	if aux == nil {
		return nil
	}

	descriptors := make([]auxFeatureDescriptor, 0, len(aux.Features)+len(aux.Outputs))
	representedOutputs := make(map[string]struct{})
	appendDescriptor := func(key string, label string, parameterValue string, rawOutput string, actions []string, toggle bool) {
		if key == "" || parameterValue == "" {
			return
		}
		for _, existing := range descriptors {
			if existing.Key == key {
				return
			}
		}
		descriptors = append(descriptors, auxFeatureDescriptor{
			Key:            key,
			Label:          label,
			ParameterValue: parameterValue,
			Actions:        append([]string(nil), actions...),
			Toggle:         toggle,
		})
		if rawOutput != "" {
			representedOutputs[rawOutput] = struct{}{}
		}
	}

	for _, feature := range aux.Features {
		switch strings.TrimSpace(feature) {
		case "siren":
			appendDescriptor("siren", "Siren", "siren", "aux", []string{"start", "stop"}, true)
		case "light":
			appendDescriptor("light", "White Light", "light", "", []string{"start", "stop"}, true)
		case "warning_light":
			appendDescriptor("warning_light", "Warning Light", "warning_light", "", []string{"start", "stop"}, true)
		case "wiper":
			appendDescriptor("wiper", "Wiper", "wiper", "wiper", []string{"start", "stop", "pulse"}, true)
		default:
			appendDescriptor(feature, humanizeFeatureLabel(feature), feature, "", []string{"start", "stop", "pulse"}, false)
		}
	}
	for _, output := range aux.Outputs {
		rawOutput := strings.TrimSpace(output)
		if rawOutput == "" {
			continue
		}
		if _, ok := representedOutputs[rawOutput]; ok {
			continue
		}
		switch rawOutput {
		case "aux":
			appendDescriptor("aux", "Aux Output", "aux", "aux", []string{"start", "stop", "pulse"}, true)
		case "light":
			appendDescriptor("light", "White Light", "light", "light", []string{"start", "stop"}, true)
		case "wiper":
			appendDescriptor("wiper", "Wiper", "wiper", "wiper", []string{"start", "stop", "pulse"}, true)
		default:
			appendDescriptor(rawOutput, humanizeFeatureLabel(rawOutput), rawOutput, rawOutput, []string{"start", "stop", "pulse"}, false)
		}
	}

	return descriptors
}

func auxFeatureActiveState(state dahua.DeviceState, key string) *bool {
	rawKey := strings.TrimSpace(key)
	if rawKey == "" {
		return nil
	}

	until := anyString(state.Info, "control_aux_active_until_"+rawKey)
	if until != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, until); err == nil {
			active := parsed.After(time.Now().UTC())
			return boolPtr(active)
		}
	}

	if _, ok := state.Info["control_aux_active_"+rawKey]; !ok {
		return nil
	}
	return boolPtr(anyBool(state.Info, "control_aux_active_"+rawKey))
}

func auxFeatureCurrentText(state dahua.DeviceState, key string) string {
	return anyString(state.Info, "control_aux_current_text_"+strings.TrimSpace(key))
}

func buildVTONumberFeature(key string, label string, url string, level int, allowed []int) FeatureSummary {
	return FeatureSummary{
		Key:           key,
		Label:         label,
		Group:         "audio",
		Kind:          "level",
		URL:           url,
		Supported:     true,
		AllowedValues: append([]int(nil), allowed...),
		MinValue:      intPtr(0),
		MaxValue:      intPtr(100),
		StepValue:     intPtr(1),
		CurrentValue:  intPtr(level),
	}
}

func buildNVRChannelControlSummary(publicBaseURL string, deviceID string, channel int, state dahua.DeviceState) *ChannelControlSummary {
	ptzSupported := anyBool(state.Info, "control_ptz_supported")
	auxSupported := anyBool(state.Info, "control_aux_supported")
	_, audioKnown := state.Info["control_audio_supported"]
	recordingSupported := anyBool(state.Info, "control_recording_supported")

	if !ptzSupported && !auxSupported && !audioKnown && !recordingSupported {
		return nil
	}

	summary := &ChannelControlSummary{}
	if ptzSupported {
		summary.PTZ = &PTZControlSummary{
			URL:            buildNVRChannelPTZURL(publicBaseURL, deviceID, channel),
			Supported:      true,
			Pan:            anyBool(state.Info, "control_ptz_pan"),
			Tilt:           anyBool(state.Info, "control_ptz_tilt"),
			Zoom:           anyBool(state.Info, "control_ptz_zoom"),
			Focus:          anyBool(state.Info, "control_ptz_focus"),
			MoveRelatively: anyBool(state.Info, "control_ptz_move_relatively"),
			AutoScan:       anyBool(state.Info, "control_ptz_auto_scan"),
			Preset:         anyBool(state.Info, "control_ptz_preset"),
			Pattern:        anyBool(state.Info, "control_ptz_pattern"),
			Tour:           anyBool(state.Info, "control_ptz_tour"),
			Commands:       anyStringSlice(state.Info, "control_ptz_commands"),
		}
	}
	if auxSupported {
		auxOutputs := anyStringSlice(state.Info, "control_aux_outputs")
		auxFeatures := anyStringSlice(state.Info, "control_aux_features")
		if !anyBool(state.Info, "control_imou_configured") {
			auxOutputs = filterNVRDeterrenceValues(auxOutputs)
			auxFeatures = filterNVRDeterrenceValues(auxFeatures)
		}
		summary.Aux = &AuxControlSummary{
			URL:       buildNVRChannelAuxURL(publicBaseURL, deviceID, channel),
			Supported: true,
			Outputs:   auxOutputs,
			Features:  auxFeatures,
		}
	}
	if audioKnown {
		summary.Audio = &AudioControlSummary{
			Supported:              anyBool(state.Info, "control_audio_supported"),
			Mute:                   anyBool(state.Info, "control_audio_mute_supported"),
			Volume:                 anyBool(state.Info, "control_audio_volume_supported"),
			VolumePermissionDenied: anyBool(state.Info, "control_audio_volume_permission_denied"),
			Muted:                  anyBool(state.Info, "control_audio_muted"),
			StreamAudioEnabled:     anyBool(state.Info, "control_audio_stream_enabled"),
			PlaybackSupported:      anyBool(state.Info, "control_audio_playback_supported"),
			PlaybackSiren:          anyBool(state.Info, "control_audio_playback_siren"),
			PlaybackQuickReply:     anyBool(state.Info, "control_audio_playback_quick_reply"),
			PlaybackFormats:        anyStringSlice(state.Info, "control_audio_playback_formats"),
			PlaybackFileCount:      anyInt(state.Info, "control_audio_playback_file_count"),
		}
	}
	if recordingSupported {
		recordingMode := anyString(state.Info, "control_recording_mode")
		exposeRecordingControl := !strings.EqualFold(recordingMode, "manual")
		summary.Recording = &RecordingControlSummary{
			URL:       conditionalString(exposeRecordingControl, buildNVRChannelRecordingURL(publicBaseURL, deviceID, channel)),
			Supported: exposeRecordingControl,
			Active:    anyBool(state.Info, "control_recording_active"),
			Mode:      recordingMode,
		}
	}
	summary.ValidationNotes = anyStringSlice(state.Info, "validation_notes")
	return summary
}

func filterNVRDeterrenceValues(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "aux", "siren", "warning_light":
			continue
		default:
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func conditionalString(enabled bool, value string) string {
	if enabled {
		return value
	}
	return ""
}

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func buildLocalWebRTCURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/webrtc/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile)
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildNVRChannelPTZURL(publicBaseURL string, deviceID string, channel int) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/nvr/" + url.PathEscape(deviceID) + "/channels/" + strconv.Itoa(channel) + "/ptz"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildNVRChannelAuxURL(publicBaseURL string, deviceID string, channel int) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/nvr/" + url.PathEscape(deviceID) + "/channels/" + strconv.Itoa(channel) + "/aux"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildNVRChannelRecordingURL(publicBaseURL string, deviceID string, channel int) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/nvr/" + url.PathEscape(deviceID) + "/channels/" + strconv.Itoa(channel) + "/recording"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildNVRRecordingsCollectionURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/nvr/" + url.PathEscape(deviceID) + "/recordings"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildNVRPlaybackSessionsCollectionURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/nvr/" + url.PathEscape(deviceID) + "/playback/sessions"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOAnswerURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/call/answer"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOHangupURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/call/hangup"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOAudioOutputVolumeURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/audio/output-volume"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOAudioInputVolumeURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/audio/input-volume"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOMuteURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/audio/mute"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTORecordingURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/recording"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOIntercomUplinkURL(publicBaseURL string, deviceID string, action string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/intercom/uplink/" + url.PathEscape(action)
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOIntercomResetURL(publicBaseURL string, deviceID string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/intercom/reset"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildVTOLockUnlockURL(publicBaseURL string, deviceID string, lockIndex int) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/vto/" + url.PathEscape(deviceID) + "/locks/" + strconv.Itoa(lockIndex) + "/unlock"
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func buildRTSPURL(deviceCfg config.DeviceConfig, channel int, subtype int, includeCredentials bool) string {
	base, err := url.Parse(deviceCfg.BaseURL)
	if err != nil || base.Hostname() == "" {
		return ""
	}

	host := base.Hostname()
	if port := base.Port(); port != "" && port != "80" && port != "443" {
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

	if includeCredentials {
		rtspURL.User = url.UserPassword(deviceCfg.Username, deviceCfg.Password)
	}

	return rtspURL.String()
}

type cameraPathKind string

const (
	cameraPathNVR cameraPathKind = "nvr"
	cameraPathVTO cameraPathKind = "vto"
	cameraPathIPC cameraPathKind = "ipc"
)

func snapshotURL(publicBaseURL string, deviceID string, channel int, kind cameraPathKind) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/ipc/" + deviceID + "/snapshot"
	switch kind {
	case cameraPathNVR:
		path = "/api/v1/nvr/" + deviceID + "/channels/" + strconv.Itoa(channel) + "/snapshot"
	case cameraPathVTO:
		path = "/api/v1/vto/" + deviceID + "/snapshot"
	}
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}

func humanizeFeatureLabel(value string) string {
	parts := strings.Fields(strings.ReplaceAll(strings.TrimSpace(value), "_", " "))
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}
