package ha

import (
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

func TestRenderCameraPackage(t *testing.T) {
	cfg := config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
	}

	result, err := RenderCameraPackage(CameraPackageInput{
		Config: cfg,
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "west20_nvr",
					Kind: dahua.DeviceKindNVR,
				},
				Children: []dahua.Device{
					{
						ID:       "west20_nvr_channel_01",
						Kind:     dahua.DeviceKindNVRChannel,
						Name:     "Front Gate",
						ParentID: "west20_nvr",
						Attributes: map[string]string{
							"channel_index": "1",
						},
					},
				},
			},
		},
		NVRConfigs: map[string]config.DeviceConfig{
			"west20_nvr": {
				BaseURL:  "http://nvr.example.local",
				Username: "admin",
				Password: "secret",
			},
		},
		Options: CameraPackageOptions{
			IncludeCredentials: false,
		},
	})
	if err != nil {
		t.Fatalf("RenderCameraPackage returned error: %v", err)
	}

	if !strings.Contains(result, "http://bridge.local:8080/api/v1/nvr/west20_nvr/channels/1/snapshot") {
		t.Fatalf("missing still image url in package:\n%s", result)
	}

	if !strings.Contains(result, "rtsp://nvr.example.local:554/cam/realmonitor?channel=1&subtype=0") {
		t.Fatalf("missing rtsp url in package:\n%s", result)
	}
}

func TestRenderCameraPackageForVTO(t *testing.T) {
	cfg := config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
	}

	result, err := RenderCameraPackage(CameraPackageInput{
		Config: cfg,
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "west20_vto",
					Name: "Front Door",
					Kind: dahua.DeviceKindVTO,
				},
			},
		},
		VTOConfigs: map[string]config.DeviceConfig{
			"west20_vto": {
				BaseURL:  "http://vto.example.local",
				Username: "admin",
				Password: "secret",
			},
		},
		Options: CameraPackageOptions{
			IncludeCredentials: false,
		},
	})
	if err != nil {
		t.Fatalf("RenderCameraPackage returned error: %v", err)
	}

	if !strings.Contains(result, "http://bridge.local:8080/api/v1/vto/west20_vto/snapshot") {
		t.Fatalf("missing vto snapshot url in package:\n%s", result)
	}

	if !strings.Contains(result, "rtsp://vto.example.local:554/cam/realmonitor?channel=1&subtype=0") {
		t.Fatalf("missing vto rtsp url in package:\n%s", result)
	}
}

func TestRenderCameraPackageForIPC(t *testing.T) {
	cfg := config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
	}

	result, err := RenderCameraPackage(CameraPackageInput{
		Config: cfg,
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard Camera",
					Kind: dahua.DeviceKindIPC,
				},
			},
		},
		IPCConfigs: map[string]config.DeviceConfig{
			"yard_ipc": {
				BaseURL:  "http://192.168.150.120",
				Username: "admin",
				Password: "secret",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderCameraPackage returned error: %v", err)
	}

	if !strings.Contains(result, "http://bridge.local:8080/api/v1/ipc/yard_ipc/snapshot") {
		t.Fatalf("missing ipc snapshot url in package:\n%s", result)
	}

	if !strings.Contains(result, "rtsp://192.168.150.120:554/cam/realmonitor?channel=1&subtype=0") {
		t.Fatalf("missing ipc rtsp url in package:\n%s", result)
	}
}

func TestRenderCameraPackageStableProfile(t *testing.T) {
	cfg := config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
	}

	result, err := RenderCameraPackage(CameraPackageInput{
		Config: cfg,
		ProbeResults: []*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard Camera",
					Kind: dahua.DeviceKindIPC,
				},
			},
		},
		IPCConfigs: map[string]config.DeviceConfig{
			"yard_ipc": {
				BaseURL:  "http://192.168.150.120",
				Username: "admin",
				Password: "secret",
			},
		},
		Options: CameraPackageOptions{
			Profile: CameraStreamProfileStable,
		},
	})
	if err != nil {
		t.Fatalf("RenderCameraPackage returned error: %v", err)
	}

	if !strings.Contains(result, "subtype=1") {
		t.Fatalf("missing stable substream url in package:\n%s", result)
	}
	if !strings.Contains(result, "frame_rate: 5") {
		t.Fatalf("missing stable frame_rate in package:\n%s", result)
	}
	if !strings.Contains(result, "rtsp_transport: tcp") {
		t.Fatalf("missing stable rtsp_transport in package:\n%s", result)
	}
	if !strings.Contains(result, "use_wallclock_as_timestamps: true") {
		t.Fatalf("missing stable wallclock setting in package:\n%s", result)
	}
}
