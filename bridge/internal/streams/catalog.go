package streams

import (
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

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
	ID                       string             `json:"id"`
	RootDeviceID             string             `json:"root_device_id"`
	SourceDeviceID           string             `json:"source_device_id"`
	DeviceKind               dahua.DeviceKind   `json:"device_kind"`
	Name                     string             `json:"name"`
	Channel                  int                `json:"channel,omitempty"`
	LockCount                int                `json:"lock_count,omitempty"`
	SnapshotURL              string             `json:"snapshot_url"`
	LocalPreviewURL          string             `json:"local_preview_url,omitempty"`
	LocalIntercomURL         string             `json:"local_intercom_url,omitempty"`
	Intercom                 *IntercomSummary   `json:"intercom,omitempty"`
	MainCodec                string             `json:"main_codec,omitempty"`
	MainResolution           string             `json:"main_resolution,omitempty"`
	SubCodec                 string             `json:"sub_codec,omitempty"`
	SubResolution            string             `json:"sub_resolution,omitempty"`
	AudioCodec               string             `json:"audio_codec,omitempty"`
	RecommendedProfile       string             `json:"recommended_profile"`
	RecommendedHAIntegration string             `json:"recommended_ha_integration"`
	RecommendedHAReason      string             `json:"recommended_ha_reason,omitempty"`
	ONVIFH264Available       bool               `json:"onvif_h264_available"`
	ONVIFProfileToken        string             `json:"onvif_profile_token,omitempty"`
	ONVIFProfileName         string             `json:"onvif_profile_name,omitempty"`
	ONVIFStreamURL           string             `json:"onvif_stream_url,omitempty"`
	ONVIFSnapshotURL         string             `json:"onvif_snapshot_url,omitempty"`
	Profiles                 map[string]Profile `json:"profiles"`
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
	SupportsVTOTalkback                 bool     `json:"supports_vto_talkback"`
	SupportsFullCallAcceptance          bool     `json:"supports_full_call_acceptance"`
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
					RecommendedProfile: recommended,
					Profiles:           buildProfiles(deviceCfg, channel, input.IncludeCredentials, recommended, input.Config.HomeAssistant.PublicBaseURL, child.ID, input.Config.Media, mainResolution, subResolution),
				}
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
			entries = append(entries, Entry{
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
				Profiles:           buildProfiles(deviceCfg, 1, input.IncludeCredentials, recommended, input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, input.Config.Media, mainResolution, subResolution),
			})
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
				Profiles:           buildProfiles(deviceCfg, 1, input.IncludeCredentials, recommended, input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, input.Config.Media, mainResolution, subResolution),
			})
			applyHASelection(&entries[len(entries)-1], state)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

func buildProfiles(deviceCfg config.DeviceConfig, channel int, includeCredentials bool, recommended string, publicBaseURL string, streamID string, mediaCfg config.MediaConfig, mainResolution string, subResolution string) map[string]Profile {
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

	profiles := map[string]Profile{
		"default": {
			Name:           "default",
			StreamURL:      buildRTSPURL(deviceCfg, channel, 0, includeCredentials),
			LocalMJPEGURL:  buildLocalMJPEGURL(publicBaseURL, streamID, "default"),
			LocalHLSURL:    buildLocalHLSURL(publicBaseURL, streamID, "default"),
			LocalWebRTCURL: buildLocalWebRTCURL(publicBaseURL, streamID, "default"),
			Subtype:        0,
			SourceWidth:    mainWidth,
			SourceHeight:   mainHeight,
			Recommended:    recommended == "default",
		},
		"quality": {
			Name:                     "quality",
			StreamURL:                buildRTSPURL(deviceCfg, channel, 0, includeCredentials),
			LocalMJPEGURL:            buildLocalMJPEGURL(publicBaseURL, streamID, "quality"),
			LocalHLSURL:              buildLocalHLSURL(publicBaseURL, streamID, "quality"),
			LocalWebRTCURL:           buildLocalWebRTCURL(publicBaseURL, streamID, "quality"),
			Subtype:                  0,
			RTSPTransport:            "tcp",
			SourceWidth:              mainWidth,
			SourceHeight:             mainHeight,
			UseWallclockAsTimestamps: true,
			Recommended:              recommended == "quality",
		},
		"stable": {
			Name:                     "stable",
			StreamURL:                buildRTSPURL(deviceCfg, channel, 1, includeCredentials),
			LocalMJPEGURL:            buildLocalMJPEGURL(publicBaseURL, streamID, "stable"),
			LocalHLSURL:              buildLocalHLSURL(publicBaseURL, streamID, "stable"),
			LocalWebRTCURL:           buildLocalWebRTCURL(publicBaseURL, streamID, "stable"),
			Subtype:                  1,
			RTSPTransport:            "tcp",
			FrameRate:                stableFrameRate,
			SourceWidth:              subWidth,
			SourceHeight:             subHeight,
			UseWallclockAsTimestamps: true,
			Recommended:              recommended == "stable",
		},
		"substream": {
			Name:           "substream",
			StreamURL:      buildRTSPURL(deviceCfg, channel, 1, includeCredentials),
			LocalMJPEGURL:  buildLocalMJPEGURL(publicBaseURL, streamID, "substream"),
			LocalHLSURL:    buildLocalHLSURL(publicBaseURL, streamID, "substream"),
			LocalWebRTCURL: buildLocalWebRTCURL(publicBaseURL, streamID, "substream"),
			Subtype:        1,
			RTSPTransport:  "tcp",
			FrameRate:      substreamFrameRate,
			SourceWidth:    subWidth,
			SourceHeight:   subHeight,
			Recommended:    recommended == "substream",
		},
	}
	return profiles
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
		BridgeSessionActive:                 runtimeStatus.Active,
		BridgeSessionCount:                  runtimeStatus.SessionCount,
		ExternalUplinkEnabled:               runtimeStatus.ExternalUplinkEnabled,
		BridgeUplinkActive:                  runtimeStatus.UplinkActive,
		BridgeUplinkCodec:                   strings.TrimSpace(runtimeStatus.UplinkCodec),
		BridgeUplinkPackets:                 runtimeStatus.UplinkPackets,
		BridgeForwardedPackets:              runtimeStatus.UplinkForwardedPackets,
		BridgeForwardErrors:                 runtimeStatus.UplinkForwardErrors,
		SupportsVTOCallAnswer:               true,
		SupportsHangup:                      true,
		SupportsBridgeSessionReset:          true,
		SupportsUnlock:                      lockCount > 0,
		SupportsBrowserMicrophone:           true,
		SupportsBridgeAudioUplink:           true,
		SupportsBridgeAudioOutput:           strings.TrimSpace(audioCodec) != "",
		SupportsExternalAudioExport:         uplinkTargetCount > 0,
		ConfiguredExternalUplinkTargetCount: uplinkTargetCount,
		SupportsVTOTalkback:                 false,
		SupportsFullCallAcceptance:          false,
	}
}

func buildLocalWebRTCURL(publicBaseURL string, streamID string, profile string) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := "/api/v1/media/webrtc/" + url.PathEscape(streamID) + "/" + url.PathEscape(profile)
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
