package ha

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"gopkg.in/yaml.v3"
)

type CameraPackageInput struct {
	Config       config.Config
	ProbeResults []*dahua.ProbeResult
	NVRConfigs   map[string]config.DeviceConfig
	VTOConfigs   map[string]config.DeviceConfig
	IPCConfigs   map[string]config.DeviceConfig
	Options      CameraPackageOptions
}

type CameraPackageOptions struct {
	IncludeCredentials       bool
	Profile                  CameraStreamProfile
	RTSPTransport            string
	FrameRate                int
	UseWallclockAsTimestamps *bool
}

type CameraStreamProfile string

const (
	CameraStreamProfileDefault   CameraStreamProfile = ""
	CameraStreamProfileStable    CameraStreamProfile = "stable"
	CameraStreamProfileQuality   CameraStreamProfile = "quality"
	CameraStreamProfileSubstream CameraStreamProfile = "substream"
)

func RenderCameraPackage(input CameraPackageInput) (string, error) {
	type genericCamera struct {
		Platform                 string `yaml:"platform"`
		Name                     string `yaml:"name"`
		StillImageURL            string `yaml:"still_image_url"`
		StreamSource             string `yaml:"stream_source"`
		Username                 string `yaml:"username,omitempty"`
		Password                 string `yaml:"password,omitempty"`
		Authentication           string `yaml:"authentication,omitempty"`
		VerifySSL                bool   `yaml:"verify_ssl,omitempty"`
		LimitRefetchToURLChange  bool   `yaml:"limit_refetch_to_url_change,omitempty"`
		FrameRate                int    `yaml:"frame_rate,omitempty"`
		RTSPTransport            string `yaml:"rtsp_transport,omitempty"`
		UseWallclockAsTimestamps bool   `yaml:"use_wallclock_as_timestamps,omitempty"`
	}

	type packageDoc struct {
		Camera []genericCamera `yaml:"camera"`
	}

	doc := packageDoc{Camera: []genericCamera{}}
	options := normalizeCameraPackageOptions(input.Options)
	results := append([]*dahua.ProbeResult(nil), input.ProbeResults...)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Root.ID < results[j].Root.ID
	})

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

				channelNumber, err := strconv.Atoi(child.Attributes["channel_index"])
				if err != nil || channelNumber <= 0 {
					continue
				}

				streamURL, err := buildRTSPURL(deviceCfg, channelNumber, streamSubtypeForProfile(options.Profile), options.IncludeCredentials)
				if err != nil {
					return "", err
				}

				camera := genericCamera{
					Platform:                 "generic",
					Name:                     child.Name,
					StillImageURL:            snapshotURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, channelNumber, cameraPathNVR),
					StreamSource:             streamURL,
					Authentication:           "basic",
					VerifySSL:                false,
					LimitRefetchToURLChange:  true,
					FrameRate:                options.FrameRate,
					RTSPTransport:            options.RTSPTransport,
					UseWallclockAsTimestamps: options.useWallclock(),
				}

				if options.IncludeCredentials {
					camera.Username = deviceCfg.Username
					camera.Password = deviceCfg.Password
				}

				doc.Camera = append(doc.Camera, camera)
			}

		case dahua.DeviceKindVTO:
			deviceCfg, ok := input.VTOConfigs[result.Root.ID]
			if !ok {
				continue
			}

			streamURL, err := buildRTSPURL(deviceCfg, 1, streamSubtypeForProfile(options.Profile), options.IncludeCredentials)
			if err != nil {
				return "", err
			}

			camera := genericCamera{
				Platform:                 "generic",
				Name:                     result.Root.Name,
				StillImageURL:            snapshotURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, 0, cameraPathVTO),
				StreamSource:             streamURL,
				Authentication:           "basic",
				VerifySSL:                false,
				LimitRefetchToURLChange:  true,
				FrameRate:                options.FrameRate,
				RTSPTransport:            options.RTSPTransport,
				UseWallclockAsTimestamps: options.useWallclock(),
			}

			if options.IncludeCredentials {
				camera.Username = deviceCfg.Username
				camera.Password = deviceCfg.Password
			}

			doc.Camera = append(doc.Camera, camera)

		case dahua.DeviceKindIPC:
			deviceCfg, ok := input.IPCConfigs[result.Root.ID]
			if !ok {
				continue
			}

			streamURL, err := buildRTSPURL(deviceCfg, 1, streamSubtypeForProfile(options.Profile), options.IncludeCredentials)
			if err != nil {
				return "", err
			}

			camera := genericCamera{
				Platform:                 "generic",
				Name:                     result.Root.Name,
				StillImageURL:            snapshotURL(input.Config.HomeAssistant.PublicBaseURL, result.Root.ID, 0, cameraPathIPC),
				StreamSource:             streamURL,
				Authentication:           "basic",
				VerifySSL:                false,
				LimitRefetchToURLChange:  true,
				FrameRate:                options.FrameRate,
				RTSPTransport:            options.RTSPTransport,
				UseWallclockAsTimestamps: options.useWallclock(),
			}

			if options.IncludeCredentials {
				camera.Username = deviceCfg.Username
				camera.Password = deviceCfg.Password
			}

			doc.Camera = append(doc.Camera, camera)
		}
	}

	output, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}

	header := "# Generated by DahuaBridge\n# Import this as a Home Assistant package or merge the camera entries into your configuration.\n"
	if options.Profile != CameraStreamProfileDefault {
		header += fmt.Sprintf("# Stream profile: %s\n", options.Profile)
	}

	return header + string(output), nil
}

func normalizeCameraPackageOptions(options CameraPackageOptions) CameraPackageOptions {
	switch options.Profile {
	case CameraStreamProfileStable:
		if options.FrameRate <= 0 {
			options.FrameRate = 5
		}
		if strings.TrimSpace(options.RTSPTransport) == "" {
			options.RTSPTransport = "tcp"
		}
		if options.UseWallclockAsTimestamps == nil {
			value := true
			options.UseWallclockAsTimestamps = &value
		}
	case CameraStreamProfileQuality:
		if strings.TrimSpace(options.RTSPTransport) == "" {
			options.RTSPTransport = "tcp"
		}
		if options.UseWallclockAsTimestamps == nil {
			value := true
			options.UseWallclockAsTimestamps = &value
		}
	case CameraStreamProfileSubstream:
		if options.FrameRate <= 0 {
			options.FrameRate = 5
		}
		if strings.TrimSpace(options.RTSPTransport) == "" {
			options.RTSPTransport = "tcp"
		}
	}

	return options
}

func buildRTSPURL(deviceCfg config.DeviceConfig, channel int, subtype int, includeCredentials bool) (string, error) {
	base, err := url.Parse(deviceCfg.BaseURL)
	if err != nil {
		return "", err
	}
	if base.Hostname() == "" {
		return "", fmt.Errorf("missing host in base url %q", deviceCfg.BaseURL)
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

	return rtspURL.String(), nil
}

func streamSubtypeForProfile(profile CameraStreamProfile) int {
	switch profile {
	case CameraStreamProfileStable, CameraStreamProfileSubstream:
		return 1
	default:
		return 0
	}
}

func (o CameraPackageOptions) useWallclock() bool {
	if o.UseWallclockAsTimestamps == nil {
		return false
	}
	return *o.UseWallclockAsTimestamps
}

type cameraPathKind string

const (
	cameraPathNVR cameraPathKind = "nvr"
	cameraPathVTO cameraPathKind = "vto"
	cameraPathIPC cameraPathKind = "ipc"
)

func snapshotURL(publicBaseURL string, deviceID string, channel int, kind cameraPathKind) string {
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	path := fmt.Sprintf("/api/v1/nvr/%s/channels/%d/snapshot", deviceID, channel)
	switch kind {
	case cameraPathVTO:
		path = fmt.Sprintf("/api/v1/vto/%s/snapshot", deviceID)
	case cameraPathIPC:
		path = fmt.Sprintf("/api/v1/ipc/%s/snapshot", deviceID)
	}
	if publicBaseURL == "" {
		return path
	}
	return publicBaseURL + path
}
