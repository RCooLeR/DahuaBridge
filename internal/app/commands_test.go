package app

import (
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/store"
)

func TestParseCommandTopic(t *testing.T) {
	deviceID, command, ok := parseCommandTopic("dahuabridge/devices/west20_vto_lock_00/command/press", "dahuabridge")
	if !ok {
		t.Fatal("expected command topic to parse")
	}
	if deviceID != "west20_vto_lock_00" {
		t.Fatalf("unexpected device id %q", deviceID)
	}
	if command != "press" {
		t.Fatalf("unexpected command %q", command)
	}
}

func TestParseCommandTopicRejectsUnexpectedTopic(t *testing.T) {
	if _, _, ok := parseCommandTopic("dahuabridge/devices/west20_vto_lock_00/state/press", "dahuabridge"); ok {
		t.Fatal("expected invalid topic to be rejected")
	}
}

func TestParseCommandTopicSupportsHangup(t *testing.T) {
	deviceID, command, ok := parseCommandTopic("dahuabridge/devices/front_vto/command/hangup", "dahuabridge")
	if !ok {
		t.Fatal("expected hangup topic to parse")
	}
	if deviceID != "front_vto" || command != "hangup" {
		t.Fatalf("unexpected parsed values %q %q", deviceID, command)
	}
}

func TestResolveVTOLockTarget(t *testing.T) {
	probes := store.NewProbeStore()
	probes.Set("west20_vto", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "west20_vto",
			Kind: dahua.DeviceKindVTO,
		},
		Children: []dahua.Device{
			{
				ID:   "west20_vto_lock_02",
				Kind: dahua.DeviceKindVTOLock,
				Attributes: map[string]string{
					"lock_index": "2",
				},
			},
		},
	})

	rootID, lockIndex, ok := resolveVTOLockTarget(probes, "west20_vto_lock_02")
	if !ok {
		t.Fatal("expected lock target resolution to succeed")
	}
	if rootID != "west20_vto" {
		t.Fatalf("unexpected root id %q", rootID)
	}
	if lockIndex != 2 {
		t.Fatalf("unexpected lock index %d", lockIndex)
	}
}
