package ha

import (
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

func TestBuildHAMigrationPlanPrefersNativeIntegrationAndFlagsDuplicates(t *testing.T) {
	plan := BuildHAMigrationPlan(
		[]*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "west20_nvr",
					Name: "West 20 NVR",
					Kind: dahua.DeviceKindNVR,
				},
				Children: []dahua.Device{
					{
						ID:       "west20_nvr_channel_01",
						ParentID: "west20_nvr",
						Name:     "Front Gate",
						Kind:     dahua.DeviceKindNVRChannel,
					},
				},
				States: map[string]dahua.DeviceState{
					"west20_nvr":            {Available: true},
					"west20_nvr_channel_01": {Available: true},
				},
			},
			{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard IPC",
					Kind: dahua.DeviceKindIPC,
				},
				States: map[string]dahua.DeviceState{
					"yard_ipc": {Available: true},
				},
			},
		},
		[]streams.Entry{
			{
				ID:                       "west20_nvr_channel_01",
				RootDeviceID:             "west20_nvr",
				Name:                     "Front Gate",
				DeviceKind:               dahua.DeviceKindNVRChannel,
				RecommendedHAIntegration: "onvif",
				ONVIFH264Available:       true,
			},
			{
				ID:                 "yard_ipc",
				RootDeviceID:       "yard_ipc",
				Name:               "Yard IPC",
				DeviceKind:         dahua.DeviceKindIPC,
				RecommendedProfile: "quality",
			},
		},
	)

	if plan.Strategy != "native_integration_primary" {
		t.Fatalf("unexpected strategy %q", plan.Strategy)
	}
	if len(plan.Devices) != 3 {
		t.Fatalf("expected 3 plan devices, got %d", len(plan.Devices))
	}
	if plan.Summary.StreamableDeviceCount != 2 {
		t.Fatalf("expected 2 streamable devices, got %+v", plan.Summary)
	}
	if plan.Summary.ONVIFDuplicateRiskCount != 1 {
		t.Fatalf("expected 1 onvif duplicate risk, got %+v", plan.Summary)
	}

	channel := findMigrationDevice(plan.Devices, "west20_nvr_channel_01")
	if channel == nil {
		t.Fatal("missing nvr channel migration device")
	}
	if channel.RecommendedPrimaryPath != "native_integration" {
		t.Fatalf("unexpected primary path %+v", channel)
	}
	if !containsString(channel.DuplicatePathsIfPresent, "mqtt_discovery") {
		t.Fatalf("expected mqtt discovery duplicate risk, got %+v", channel)
	}
	if !containsString(channel.DuplicatePathsIfPresent, "generic_camera_package") {
		t.Fatalf("expected generic camera duplicate risk, got %+v", channel)
	}
	if !containsString(channel.DuplicatePathsIfPresent, "onvif_config_entry") {
		t.Fatalf("expected onvif duplicate risk, got %+v", channel)
	}
}

func TestRenderHAMigrationGuideMarkdownIncludesCleanupStep(t *testing.T) {
	plan := BuildHAMigrationPlan(
		[]*dahua.ProbeResult{
			{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard IPC",
					Kind: dahua.DeviceKindIPC,
				},
				States: map[string]dahua.DeviceState{
					"yard_ipc": {Available: true},
				},
			},
		},
		[]streams.Entry{
			{
				ID:           "yard_ipc",
				RootDeviceID: "yard_ipc",
				Name:         "Yard IPC",
				DeviceKind:   dahua.DeviceKindIPC,
			},
		},
	)

	body := RenderHAMigrationGuideMarkdown(plan)
	if !strings.Contains(body, "POST /api/v1/home-assistant/mqtt/discovery/remove") {
		t.Fatalf("expected cleanup endpoint in markdown guide:\n%s", body)
	}
	if !strings.Contains(body, "yard_ipc") {
		t.Fatalf("expected device id in markdown guide:\n%s", body)
	}
}

func findMigrationDevice(devices []HAMigrationPlanDevice, deviceID string) *HAMigrationPlanDevice {
	for i := range devices {
		if devices[i].DeviceID == deviceID {
			return &devices[i]
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
