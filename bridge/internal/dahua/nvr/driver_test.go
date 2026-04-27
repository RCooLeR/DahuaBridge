package nvr

import (
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/onvif"
)

func TestParseSoftwareVersion(t *testing.T) {
	firmware, buildDate := parseSoftwareVersion("version=5.001.0000000.2.R,build:2026-03-31\n")

	if firmware != "5.001.0000000.2.R" {
		t.Fatalf("firmware mismatch: got %q", firmware)
	}

	if buildDate != "2026-03-31" {
		t.Fatalf("buildDate mismatch: got %q", buildDate)
	}
}

func TestParseChannelTitles(t *testing.T) {
	values := map[string]string{
		"table.ChannelTitle[0].Name": "Front Gate",
		"table.ChannelTitle[1].Name": "Garage",
	}

	got := parseChannelTitles(values)

	if got[0] != "Front Gate" {
		t.Fatalf("channel 0 mismatch: got %q", got[0])
	}

	if got[1] != "Garage" {
		t.Fatalf("channel 1 mismatch: got %q", got[1])
	}
}

func TestParseChannelStreams(t *testing.T) {
	values := map[string]string{
		"table.Encode[0].MainFormat[0].Video.resolution":   "3840x2160",
		"table.Encode[0].MainFormat[0].Video.Compression":  "H.265",
		"table.Encode[0].ExtraFormat[0].Video.resolution":  "704x576",
		"table.Encode[0].ExtraFormat[0].Video.Compression": "H.264",
	}

	got := parseChannelStreams(values)
	item := got[0]

	if item.MainResolution != "3840x2160" {
		t.Fatalf("main resolution mismatch: got %q", item.MainResolution)
	}
	if item.MainCodec != "H.265" {
		t.Fatalf("main codec mismatch: got %q", item.MainCodec)
	}
	if item.SubResolution != "704x576" {
		t.Fatalf("sub resolution mismatch: got %q", item.SubResolution)
	}
	if item.SubCodec != "H.264" {
		t.Fatalf("sub codec mismatch: got %q", item.SubCodec)
	}
}

func TestParseDiskInventory(t *testing.T) {
	values := map[string]string{
		"list.info[0].Name":                 "/dev/sda",
		"list.info[0].State":                "Success",
		"list.info[0].Detail[0].TotalBytes": "100.0",
		"list.info[0].Detail[0].UsedBytes":  "40.0",
		"list.info[0].Detail[0].IsError":    "false",
		"list.info[0].Detail[1].TotalBytes": "50.0",
		"list.info[0].Detail[1].UsedBytes":  "20.0",
		"list.info[0].Detail[1].IsError":    "false",
	}

	got := parseDiskInventory(values)
	if len(got) != 1 {
		t.Fatalf("expected 1 disk, got %d", len(got))
	}

	if got[0].Name != "/dev/sda" {
		t.Fatalf("disk name mismatch: got %q", got[0].Name)
	}
	if got[0].State != "Success" {
		t.Fatalf("disk state mismatch: got %q", got[0].State)
	}
	if got[0].TotalBytes != 150 {
		t.Fatalf("disk total mismatch: got %v", got[0].TotalBytes)
	}
	if got[0].UsedBytes != 60 {
		t.Fatalf("disk used mismatch: got %v", got[0].UsedBytes)
	}
}

func TestSummarizeDisks(t *testing.T) {
	summary := summarizeDisks([]diskInventory{
		{
			Index:      0,
			State:      "Success",
			TotalBytes: 200,
			UsedBytes:  50,
		},
		{
			Index:      1,
			State:      "Error",
			TotalBytes: 100,
			UsedBytes:  80,
			IsError:    true,
		},
	})

	if !summary.DiskFault {
		t.Fatal("expected disk fault")
	}
	if summary.DiskErrorCount != 1 {
		t.Fatalf("unexpected disk error count %d", summary.DiskErrorCount)
	}
	if summary.DiskHealthyCount != 1 {
		t.Fatalf("unexpected healthy disk count %d", summary.DiskHealthyCount)
	}
	if summary.TotalBytes != 300 {
		t.Fatalf("unexpected total bytes %v", summary.TotalBytes)
	}
	if summary.UsedBytes != 130 {
		t.Fatalf("unexpected used bytes %v", summary.UsedBytes)
	}
	if summary.FreeBytes != 170 {
		t.Fatalf("unexpected free bytes %v", summary.FreeBytes)
	}
	if summary.UsedPercent < 43.3 || summary.UsedPercent > 43.4 {
		t.Fatalf("unexpected used percent %v", summary.UsedPercent)
	}
}

func TestParseEventPayload(t *testing.T) {
	values := parseEventPayload("Code=VideoMotion;action=Start;index=0")

	if values["Code"] != "VideoMotion" {
		t.Fatalf("code mismatch: got %q", values["Code"])
	}
	if values["action"] != "Start" {
		t.Fatalf("action mismatch: got %q", values["action"])
	}
	if values["index"] != "0" {
		t.Fatalf("index mismatch: got %q", values["index"])
	}
}

func TestNormalizeEvent(t *testing.T) {
	event, ok := normalizeEvent("west20_nvr", map[string]string{
		"Code":   "VideoMotion",
		"action": "Start",
		"index":  "1",
	})
	if !ok {
		t.Fatal("expected event to normalize")
	}

	if event.ChildID != "west20_nvr_channel_02" {
		t.Fatalf("child id mismatch: got %q", event.ChildID)
	}
	if event.Channel != 2 {
		t.Fatalf("channel mismatch: got %d", event.Channel)
	}
	if event.Action != "start" {
		t.Fatalf("action mismatch: got %q", event.Action)
	}
}

func TestParseBoundary(t *testing.T) {
	got := parseBoundary("multipart/x-mixed-replace; boundary=myboundary")
	if got != "myboundary" {
		t.Fatalf("boundary mismatch: got %q", got)
	}
}

func TestBuildRTSPURL(t *testing.T) {
	got := buildRTSPURL("http://nvr.example.local", 3, 1)
	want := "rtsp://nvr.example.local:554/cam/realmonitor?channel=3&subtype=1"
	if got != want {
		t.Fatalf("rtsp url mismatch:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestInvalidateInventoryCache(t *testing.T) {
	driver := &Driver{
		cachedInventory: &inventorySnapshot{
			Channels: []channelInventory{{Index: 0, Name: "Front Gate"}},
		},
		inventoryExpires: time.Now().Add(time.Minute),
	}

	driver.InvalidateInventoryCache()

	if driver.cachedInventory != nil {
		t.Fatalf("expected cached inventory to be cleared: %+v", driver.cachedInventory)
	}
	if !driver.inventoryExpires.IsZero() {
		t.Fatalf("expected inventory expiry to be cleared: %v", driver.inventoryExpires)
	}
}

func TestAttachONVIFChannelStateIncludesSnapshotURI(t *testing.T) {
	device := dahua.Device{Attributes: map[string]string{}}
	state := dahua.DeviceState{Info: map[string]any{}}
	discovery := onvif.Discovery{
		Profiles: []onvif.Profile{
			{
				Token:       "Profile_1",
				Name:        "MainStream-H264",
				Encoding:    "H264",
				Width:       1920,
				Height:      1080,
				Channel:     1,
				Subtype:     0,
				StreamURI:   "rtsp://nvr.example.local:554/cam/realmonitor?channel=1&subtype=0",
				SnapshotURI: "http://nvr.example.local/onvif/snapshot1.jpg",
				IsH264:      true,
			},
		},
	}

	attachONVIFChannelState(&device, &state, discovery, 1)

	if device.Attributes["onvif_snapshot_url"] != "http://nvr.example.local/onvif/snapshot1.jpg" {
		t.Fatalf("unexpected device snapshot url %q", device.Attributes["onvif_snapshot_url"])
	}
	if state.Info["onvif_snapshot_url"] != "http://nvr.example.local/onvif/snapshot1.jpg" {
		t.Fatalf("unexpected state snapshot url %#v", state.Info["onvif_snapshot_url"])
	}
}
