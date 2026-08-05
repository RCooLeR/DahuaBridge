package app

import (
	"testing"
	"time"

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

	got := buildMediaRecordingsURL("http://bridge.local:9020", entry)
	want := "http://bridge.local:9020/api/v1/media/recordings?channel=1&root_device_id=west20_nvr"
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
		BaseURL:  "http://192.0.2.10",
		Username: "assistant",
		Password: "example-password",
	}, "/mnt/dvr/2026-05-02/0/dav/01/1/0/627506/01.30.00-02.00.00[R][0@0][0].dav", true)
	want := "http://example-user:example-password@192.0.2.10/cgi-bin/RPC_Loadfile/mnt/dvr/2026-05-02/0/dav/01/1/0/627506/01.30.00-02.00.00%5BR%5D%5B0%400%5D%5B0%5D.dav"
	if got != want {
		t.Fatalf("unexpected playback download url %q", got)
	}
}

func TestBuildPlaybackRTSPURLPreservesDahuaQueryOrder(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() {
		time.Local = previousLocal
	})

	got := buildPlaybackRTSPURL(config.DeviceConfig{
		BaseURL:  "http://192.0.2.10",
		Username: "assistant",
		Password: "secret",
	}, 5, 0,
		time.Date(2026, 5, 3, 13, 48, 31, 0, time.UTC),
		time.Date(2026, 5, 3, 13, 48, 51, 0, time.UTC),
		true,
	)
	want := "rtsp://example-user:example-password@192.0.2.10:554/cam/playback?channel=5&subtype=0&starttime=2026_05_03_13_48_31&endtime=2026_05_03_13_48_51"
	if got != want {
		t.Fatalf("unexpected playback rtsp url %q", got)
	}
}
