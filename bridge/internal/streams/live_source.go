package streams

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

const (
	LiveSourceNVR     = "nvr"
	LiveSourceCamera  = "camera"
	LiveSourceDefault = "default"
)

var (
	ErrInvalidLiveSource         = errors.New("live source must be nvr or camera")
	ErrInvalidLiveSourceOverride = errors.New("live source must be default, nvr, or camera")
	ErrLiveSourceUnavailable     = errors.New("camera live source is unavailable")
	ErrLiveSourcePersistence     = errors.New("live source settings require writable persistent state storage")
	ErrLiveSourceUnsupported     = errors.New("live source selection is only supported for NVR channels")
)

type LiveSourceSummary struct {
	Source                  string `json:"source"`
	DefaultSource           string `json:"default_source"`
	OverrideSource          string `json:"override_source"`
	PreferredSource         string `json:"preferred_source"`
	FallbackReason          string `json:"fallback_reason,omitempty"`
	CameraAvailable         bool   `json:"camera_available"`
	CameraUnavailableReason string `json:"camera_unavailable_reason,omitempty"`
	URL                     string `json:"url"`
}

type LiveSourceSettings struct {
	Source string `json:"source"`
}

type RuntimeLiveSourceState struct {
	Source         string
	Signature      string
	FallbackReason string
}

func applyNVRLiveSource(entry *Entry, input CatalogInput, deviceCfg config.DeviceConfig, child dahua.Device, state dahua.DeviceState) {
	cameraCfg, reason := directCameraStreamConfig(deviceCfg, entry.Channel, child, state)
	cameraChannel := 1
	if credential, ok := deviceCfg.DirectIPCCredential(entry.Channel); ok && credential.DirectIPCChannel > 0 {
		cameraChannel = credential.DirectIPCChannel
	}
	defaultSource := LiveSourceNVR
	if input.DefaultLiveSource == LiveSourceCamera {
		defaultSource = LiveSourceCamera
	}
	overrideSource := input.LiveSources[entry.ID]
	if overrideSource != LiveSourceNVR && overrideSource != LiveSourceCamera {
		overrideSource = ""
	}
	preferredSource := overrideSource
	if preferredSource == "" {
		preferredSource = defaultSource
	}
	source, fallbackReason := preferredSource, ""
	if source == LiveSourceCamera && reason != "" {
		source, fallbackReason = LiveSourceNVR, reason
	}
	// The key is independent of catalog credential visibility and changes when
	// a preference, address, credential, input channel, or subtype changes.
	var routeKeys []string
	for name, profile := range entry.Profiles {
		cameraURL := ""
		if reason == "" {
			cameraURL = buildRTSPURL(cameraCfg, cameraChannel, profile.Subtype, true)
		}
		routeKeys = append(routeKeys, name+"\n"+buildRTSPURL(deviceCfg, entry.Channel, profile.Subtype, true)+"\n"+cameraURL)
	}
	sort.Strings(routeKeys)
	entry.LiveSourceSignature = fmt.Sprintf("%x", sha256.Sum256([]byte(preferredSource+"\n"+strings.Join(routeKeys, "\n"))))
	if selection, ok := input.LiveSourceStates[entry.ID]; ok && selection.Signature == entry.LiveSourceSignature {
		if selection.Source == LiveSourceNVR || (selection.Source == LiveSourceCamera && reason == "") {
			source, fallbackReason = selection.Source, selection.FallbackReason
		}
	}
	entry.LiveSource = &LiveSourceSummary{
		Source:                  source,
		DefaultSource:           defaultSource,
		OverrideSource:          overrideSource,
		PreferredSource:         preferredSource,
		FallbackReason:          fallbackReason,
		CameraAvailable:         reason == "",
		CameraUnavailableReason: reason,
		URL:                     strings.TrimRight(input.Config.HomeAssistant.PublicBaseURL, "/") + "/api/v1/streams/" + url.PathEscape(entry.ID) + "/live-source",
	}
	for name, profile := range entry.Profiles {
		profile.RecorderStreamURL = profile.StreamURL
		cameraURL := ""
		if reason == "" {
			cameraURL = buildRTSPURL(cameraCfg, cameraChannel, profile.Subtype, input.IncludeCredentials)
		}
		profile.AlternativeStreamURL = cameraURL
		if source == LiveSourceCamera {
			profile.AlternativeStreamURL = profile.RecorderStreamURL
			profile.StreamURL = cameraURL
		}
		entry.Profiles[name] = profile
	}
	if source == LiveSourceCamera {
		// These ONVIF profiles belong to the recorder, not the selected camera.
		entry.ONVIFH264Available = false
		entry.ONVIFProfileToken = ""
		entry.ONVIFProfileName = ""
		entry.ONVIFStreamURL = ""
		entry.ONVIFSnapshotURL = ""
		entry.RecommendedHAIntegration = "bridge_media"
		entry.RecommendedHAReason = "direct_camera_live_source"
	}
}

func directCameraStreamConfig(deviceCfg config.DeviceConfig, channel int, child dahua.Device, state dahua.DeviceState) (config.DeviceConfig, string) {
	credential, ok := deviceCfg.DirectIPCCredential(channel)
	if !ok || strings.TrimSpace(credential.DirectIPCUser) == "" || strings.TrimSpace(credential.DirectIPCPassword) == "" {
		return config.DeviceConfig{}, "Direct camera credentials are not configured for this channel."
	}
	if credential.DirectIPCChannel < 0 {
		return config.DeviceConfig{}, "Direct camera input channel is invalid."
	}
	address := strings.TrimSpace(credential.DirectIPCIP)
	if address == "" {
		return config.DeviceConfig{}, "Direct camera address is not configured for this channel."
	}
	// Addresses may be stored with an HTTP scheme or port for device controls.
	// Only the host is relevant here; RTSP uses its separately discovered port.
	parseAddress := address
	if !strings.Contains(parseAddress, "://") {
		if net.ParseIP(strings.Trim(parseAddress, "[]")) != nil && strings.Contains(parseAddress, ":") {
			parseAddress = "[" + strings.Trim(parseAddress, "[]") + "]"
		}
		parseAddress = "http://" + parseAddress
	}
	parsed, err := url.Parse(parseAddress)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return config.DeviceConfig{}, "Direct camera address is invalid."
	}
	port := intValueOrState(child.Attributes["direct_ipc_rtsp_port"], state, "direct_ipc_rtsp_port")
	if port <= 0 {
		port = 554
	}
	if port > 65535 {
		return config.DeviceConfig{}, "Direct camera RTSP port is invalid."
	}
	return config.DeviceConfig{
		BaseURL:  "rtsp://" + net.JoinHostPort(parsed.Hostname(), strconv.Itoa(port)),
		Username: credential.DirectIPCUser,
		Password: credential.DirectIPCPassword,
	}, ""
}
