package app

import (
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
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

func TestBuildPlaybackRecordingDownloadURLUsesCGIBinPathAndEscapesAtSign(t *testing.T) {
	got := buildPlaybackRecordingDownloadURL(config.DeviceConfig{
		BaseURL:  "http://192.168.150.10",
		Username: "assistant",
		Password: "veTsiaDa",
	}, "/mnt/dvr/2026-05-02/0/dav/01/1/0/627506/01.30.00-02.00.00[R][0@0][0].dav", true)
	want := "http://assistant:veTsiaDa@192.168.150.10/cgi-bin/RPC_Loadfile/mnt/dvr/2026-05-02/0/dav/01/1/0/627506/01.30.00-02.00.00%5BR%5D%5B0%400%5D%5B0%5D.dav"
	if got != want {
		t.Fatalf("unexpected playback download url %q", got)
	}
}
