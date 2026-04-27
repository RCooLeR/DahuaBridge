package ha

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

type HAMigrationPlan struct {
	GeneratedAt string                  `json:"generated_at"`
	Strategy    string                  `json:"strategy"`
	Summary     HAMigrationPlanSummary  `json:"summary"`
	GlobalSteps []string                `json:"global_steps"`
	Devices     []HAMigrationPlanDevice `json:"devices"`
}

type HAMigrationPlanSummary struct {
	DeviceCount              int `json:"device_count"`
	StreamableDeviceCount    int `json:"streamable_device_count"`
	NativePrimaryDeviceCount int `json:"native_primary_device_count"`
	MQTTDuplicateRiskCount   int `json:"mqtt_duplicate_risk_count"`
	GenericCameraRiskCount   int `json:"generic_camera_duplicate_risk_count"`
	ONVIFDuplicateRiskCount  int `json:"onvif_duplicate_risk_count"`
}

type HAMigrationPlanDevice struct {
	DeviceID                string           `json:"device_id"`
	ParentID                string           `json:"parent_id,omitempty"`
	Name                    string           `json:"name"`
	Kind                    dahua.DeviceKind `json:"kind"`
	Streamable              bool             `json:"streamable"`
	RecommendedPrimaryPath  string           `json:"recommended_primary_path"`
	Reason                  string           `json:"reason"`
	DuplicatePathsIfPresent []string         `json:"duplicate_paths_if_present,omitempty"`
	OptionalLegacyPaths     []string         `json:"optional_legacy_paths,omitempty"`
}

func BuildHAMigrationPlan(results []*dahua.ProbeResult, entries []streams.Entry) HAMigrationPlan {
	catalog := BuildNativeCatalog(results, entries)
	plan := HAMigrationPlan{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Strategy:    "native_integration_primary",
		GlobalSteps: []string{
			"Install and use the bridge-native Home Assistant integration as the primary Home Assistant path.",
			"For the cleanest Home Assistant device and entity list, set home_assistant.entity_mode to native in the bridge config.",
			"After switching to native mode, call POST /api/v1/home-assistant/mqtt/discovery/remove once to remove retained legacy MQTT discovery entries from Home Assistant.",
			"Remove old generic camera packages if you previously imported /api/v1/home-assistant/package/cameras*.yaml.",
			"Remove ONVIF config entries for devices now represented by the native bridge integration if you want one clean Home Assistant device per streamable thing.",
			"Keep raw MQTT topics only if you still rely on legacy automations or non-native consumers; native mode only suppresses Home Assistant MQTT discovery, not the bridge MQTT topic model itself.",
		},
		Devices: make([]HAMigrationPlanDevice, 0, len(catalog.Devices)),
	}

	for _, record := range catalog.Devices {
		device := record.Device
		streamable := record.Stream != nil
		item := HAMigrationPlanDevice{
			DeviceID:               device.ID,
			ParentID:               device.ParentID,
			Name:                   device.Name,
			Kind:                   device.Kind,
			Streamable:             streamable,
			RecommendedPrimaryPath: "native_integration",
			Reason:                 migrationReasonForDevice(record),
		}

		item.DuplicatePathsIfPresent = append(item.DuplicatePathsIfPresent, "mqtt_discovery")
		plan.Summary.MQTTDuplicateRiskCount++

		if streamable {
			item.DuplicatePathsIfPresent = append(item.DuplicatePathsIfPresent, "generic_camera_package")
			plan.Summary.GenericCameraRiskCount++
		}

		if streamable && streamHasONVIFDuplicateRisk(record.Stream) {
			item.DuplicatePathsIfPresent = append(item.DuplicatePathsIfPresent, "onvif_config_entry")
			plan.Summary.ONVIFDuplicateRiskCount++
		}

		if hasOptionalLegacyMQTT(item.Kind) {
			item.OptionalLegacyPaths = append(item.OptionalLegacyPaths, "raw_mqtt_topics")
		}

		sort.Strings(item.DuplicatePathsIfPresent)
		sort.Strings(item.OptionalLegacyPaths)
		plan.Devices = append(plan.Devices, item)

		plan.Summary.DeviceCount++
		plan.Summary.NativePrimaryDeviceCount++
		if streamable {
			plan.Summary.StreamableDeviceCount++
		}
	}

	sort.Slice(plan.Devices, func(i, j int) bool {
		return plan.Devices[i].DeviceID < plan.Devices[j].DeviceID
	})

	return plan
}

func RenderHAMigrationGuideMarkdown(plan HAMigrationPlan) string {
	var b strings.Builder

	b.WriteString("# Home Assistant Migration Guide\n\n")
	b.WriteString("Recommended strategy: `native_integration_primary`\n\n")
	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("- Devices in plan: %d\n", plan.Summary.DeviceCount))
	b.WriteString(fmt.Sprintf("- Streamable devices: %d\n", plan.Summary.StreamableDeviceCount))
	b.WriteString(fmt.Sprintf("- MQTT discovery duplicate risks: %d\n", plan.Summary.MQTTDuplicateRiskCount))
	b.WriteString(fmt.Sprintf("- Generic camera duplicate risks: %d\n", plan.Summary.GenericCameraRiskCount))
	b.WriteString(fmt.Sprintf("- ONVIF duplicate risks: %d\n", plan.Summary.ONVIFDuplicateRiskCount))
	b.WriteString("\n## Steps\n\n")
	for _, step := range plan.GlobalSteps {
		b.WriteString("- ")
		b.WriteString(step)
		b.WriteString("\n")
	}
	b.WriteString("\n## Per Device\n\n")
	for _, device := range plan.Devices {
		b.WriteString(fmt.Sprintf("### %s\n\n", device.DeviceID))
		b.WriteString(fmt.Sprintf("- Name: %s\n", device.Name))
		b.WriteString(fmt.Sprintf("- Kind: `%s`\n", device.Kind))
		b.WriteString(fmt.Sprintf("- Primary path: `%s`\n", device.RecommendedPrimaryPath))
		b.WriteString(fmt.Sprintf("- Streamable: `%t`\n", device.Streamable))
		b.WriteString(fmt.Sprintf("- Reason: %s\n", device.Reason))
		if len(device.DuplicatePathsIfPresent) > 0 {
			b.WriteString(fmt.Sprintf("- Remove if present: `%s`\n", strings.Join(device.DuplicatePathsIfPresent, "`, `")))
		}
		if len(device.OptionalLegacyPaths) > 0 {
			b.WriteString(fmt.Sprintf("- Optional legacy paths: `%s`\n", strings.Join(device.OptionalLegacyPaths, "`, `")))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func migrationReasonForDevice(record NativeCatalogDevice) string {
	switch record.Device.Kind {
	case dahua.DeviceKindNVRChannel:
		return "keeps the camera plus channel motion and AI sensors under one Home Assistant device"
	case dahua.DeviceKindIPC:
		return "keeps the camera plus IPC motion and AI sensors under one Home Assistant device"
	case dahua.DeviceKindVTO:
		return "keeps the camera, call state, and bridge-native VTO controls under one Home Assistant device"
	case dahua.DeviceKindNVR:
		return "keeps recorder diagnostics and root actions in the native integration instead of a parallel MQTT device"
	case dahua.DeviceKindNVRDisk:
		return "keeps disk health and storage diagnostics attached to the bridge-native device tree"
	case dahua.DeviceKindVTOLock:
		return "keeps VTO accessory devices inside the same bridge-native hierarchy"
	case dahua.DeviceKindVTOAlarm:
		return "keeps VTO alarm input devices inside the same bridge-native hierarchy"
	default:
		return "keeps Home Assistant entities aligned with the bridge-native device model"
	}
}

func streamHasONVIFDuplicateRisk(entry *streams.Entry) bool {
	if entry == nil {
		return false
	}
	return entry.ONVIFH264Available || strings.TrimSpace(entry.ONVIFProfileToken) != "" || strings.TrimSpace(entry.ONVIFProfileName) != ""
}

func hasOptionalLegacyMQTT(kind dahua.DeviceKind) bool {
	switch kind {
	case dahua.DeviceKindNVRChannel, dahua.DeviceKindIPC, dahua.DeviceKindVTO:
		return true
	default:
		return false
	}
}
