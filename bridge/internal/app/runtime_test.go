package app

import (
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

func TestBuildMediaRecordingsURLUsesRootDeviceAndChannelForNVRChannels(t *testing.T) {
	entry := streams.Entry{
		ID:           "west20_nvr_channel_01",
		RootDeviceID: "west20_nvr",
		Channel:      1,
		DeviceKind:   dahua.DeviceKindNVRChannel,
	}

	got := buildMediaRecordingsURL("http://bridge.local:9205", entry)
	want := "http://bridge.local:9205/api/v1/media/recordings?channel=1&root_device_id=west20_nvr"
	if got != want {
		t.Fatalf("unexpected recordings url %q", got)
	}
}

func TestBuildMediaRecordingsURLUsesStreamIDForStandaloneStreams(t *testing.T) {
	entry := streams.Entry{
		ID:         "front_vto",
		DeviceKind: dahua.DeviceKindVTO,
	}

	got := buildMediaRecordingsURL("", entry)
	want := "/api/v1/media/recordings?stream_id=front_vto"
	if got != want {
		t.Fatalf("unexpected recordings url %q", got)
	}
}
