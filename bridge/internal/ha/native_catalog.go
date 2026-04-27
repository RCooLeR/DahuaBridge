package ha

import (
	"sort"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

type NativeCatalog struct {
	GeneratedAt string                `json:"generated_at"`
	Devices     []NativeCatalogDevice `json:"devices"`
}

type NativeCatalogDevice struct {
	Device dahua.Device      `json:"device"`
	State  dahua.DeviceState `json:"state,omitempty"`
	Stream *streams.Entry    `json:"stream,omitempty"`
}

func BuildNativeCatalog(results []*dahua.ProbeResult, entries []streams.Entry) NativeCatalog {
	streamsByDeviceID := make(map[string]streams.Entry, len(entries))
	for _, entry := range entries {
		streamsByDeviceID[entry.ID] = entry
	}

	devices := make([]NativeCatalogDevice, 0)
	for _, result := range results {
		if result == nil {
			continue
		}

		devices = append(devices, nativeCatalogDevice(result.Root, result.States[result.Root.ID], streamsByDeviceID))
		for _, child := range result.Children {
			devices = append(devices, nativeCatalogDevice(child, result.States[child.ID], streamsByDeviceID))
		}
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Device.ID < devices[j].Device.ID
	})

	return NativeCatalog{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Devices:     devices,
	}
}

func nativeCatalogDevice(device dahua.Device, state dahua.DeviceState, streamsByDeviceID map[string]streams.Entry) NativeCatalogDevice {
	record := NativeCatalogDevice{
		Device: device,
		State:  cloneNativeCatalogState(state),
	}
	if entry, ok := streamsByDeviceID[device.ID]; ok {
		entryCopy := entry
		record.Stream = &entryCopy
		mergeIntercomState(&record.State, entryCopy.Intercom)
	}
	return record
}

func cloneNativeCatalogState(state dahua.DeviceState) dahua.DeviceState {
	cloned := dahua.DeviceState{
		Available: state.Available,
	}
	if len(state.Info) == 0 {
		return cloned
	}

	cloned.Info = make(map[string]any, len(state.Info))
	for key, value := range state.Info {
		cloned.Info[key] = value
	}
	return cloned
}

func mergeIntercomState(state *dahua.DeviceState, intercom *streams.IntercomSummary) {
	if state == nil || intercom == nil {
		return
	}
	if state.Info == nil {
		state.Info = make(map[string]any)
	}

	assignIfNotEmpty(state.Info, "call_state", intercom.CallState)
	assignIfNotEmpty(state.Info, "last_ring_at", intercom.LastRingAt)
	assignIfNotEmpty(state.Info, "last_call_started_at", intercom.LastCallStartedAt)
	assignIfNotEmpty(state.Info, "last_call_ended_at", intercom.LastCallEndedAt)
	assignIfNotEmpty(state.Info, "last_call_source", intercom.LastCallSource)
	assignIfNonZero(state.Info, "last_call_duration_seconds", intercom.LastCallDurationSeconds)
	assignIfNonZero(state.Info, "bridge_session_count", intercom.BridgeSessionCount)
	assignIfNotEmpty(state.Info, "bridge_uplink_codec", intercom.BridgeUplinkCodec)
	assignIfNonZeroUint64(state.Info, "bridge_uplink_packets", intercom.BridgeUplinkPackets)
	assignIfNonZeroUint64(state.Info, "bridge_forwarded_packets", intercom.BridgeForwardedPackets)
	assignIfNonZeroUint64(state.Info, "bridge_forward_errors", intercom.BridgeForwardErrors)
	assignIfNonZero(state.Info, "configured_external_uplink_target_count", intercom.ConfiguredExternalUplinkTargetCount)

	state.Info["bridge_session_active"] = intercom.BridgeSessionActive
	state.Info["external_uplink_enabled"] = intercom.ExternalUplinkEnabled
	state.Info["bridge_uplink_active"] = intercom.BridgeUplinkActive
}

func assignIfNotEmpty(target map[string]any, key string, value string) {
	if value == "" {
		return
	}
	target[key] = value
}

func assignIfNonZero(target map[string]any, key string, value int) {
	if value == 0 {
		return
	}
	target[key] = value
}

func assignIfNonZeroUint64(target map[string]any, key string, value uint64) {
	if value == 0 {
		return
	}
	target[key] = value
}
