package ha

import (
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

func TestRenderLovelaceDashboard(t *testing.T) {
	body, err := RenderLovelaceDashboard([]streams.Entry{
		{
			ID:         "west20_nvr_channel_01",
			Name:       "Front Gate",
			DeviceKind: dahua.DeviceKindNVRChannel,
		},
		{
			ID:         "front_vto",
			Name:       "Front Door",
			DeviceKind: dahua.DeviceKindVTO,
			LockCount:  1,
			Intercom: &streams.IntercomSummary{
				SupportsBridgeSessionReset:  true,
				SupportsExternalAudioExport: true,
				SupportsVTOCallAnswer:       true,
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderLovelaceDashboard returned error: %v", err)
	}

	if !strings.Contains(body, "camera_image: camera.west20_nvr_channel_01_camera") {
		t.Fatalf("missing nvr camera entity id:\n%s", body)
	}
	if !strings.Contains(body, "- binary_sensor.west20_nvr_channel_01_motion") {
		t.Fatalf("missing nvr motion entity id:\n%s", body)
	}
	if !strings.Contains(body, "camera_image: camera.front_vto_camera") {
		t.Fatalf("missing vto camera entity id:\n%s", body)
	}
	if !strings.Contains(body, "- binary_sensor.front_vto_doorbell") {
		t.Fatalf("missing vto doorbell entity id:\n%s", body)
	}
	if !strings.Contains(body, "title: Intercom") {
		t.Fatalf("missing intercom view:\n%s", body)
	}
	if !strings.Contains(body, "sensor.front_vto_call_state") {
		t.Fatalf("missing vto call state entity id:\n%s", body)
	}
	if !strings.Contains(body, "button.front_vto_lock_00_open") {
		t.Fatalf("missing vto lock button entity id:\n%s", body)
	}
	if !strings.Contains(body, "button.front_vto_answer") {
		t.Fatalf("missing vto answer button entity id:\n%s", body)
	}
	if !strings.Contains(body, "button.front_vto_hangup") {
		t.Fatalf("missing vto hangup button entity id:\n%s", body)
	}
	if !strings.Contains(body, "button.front_vto_intercom_reset") {
		t.Fatalf("missing vto bridge session reset button entity id:\n%s", body)
	}
	if !strings.Contains(body, "button.front_vto_uplink_enable") || !strings.Contains(body, "button.front_vto_uplink_disable") {
		t.Fatalf("missing vto uplink control button entity ids:\n%s", body)
	}
}
