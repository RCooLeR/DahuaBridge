package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/store"
	"RCooLeR/DahuaBridge/internal/streams"
)

func TestGlobalLiveSourceOnlyInvalidatesChangedEffectiveSources(t *testing.T) {
	runtime, deviceCfg := liveSourceRuntime(t)
	runtime.probes.Update("nvr", func(probe *dahua.ProbeResult) {
		for _, channel := range []int{6, 7, 8} {
			probe.Children = append(probe.Children, dahua.Device{ID: fmt.Sprintf("nvr_channel_%02d", channel), Kind: dahua.DeviceKindNVRChannel, Attributes: map[string]string{"channel_index": fmt.Sprint(channel)}})
		}
	})
	for _, channel := range []int{6, 8} {
		deviceCfg.DirectIPCCredentials = append(deviceCfg.DirectIPCCredentials, config.ChannelDirectIPCCredential{NVRChannel: channel, DirectIPCIP: fmt.Sprintf("camera%d.local", channel), DirectIPCUser: "camera-user", DirectIPCPassword: "camera-pass"})
	}
	runtime.UpdateDeviceConfig("nvr", deviceCfg)
	for id, source := range map[string]string{"nvr_channel_06": "nvr", "nvr_channel_08": "camera"} {
		if _, err := runtime.SetStreamLiveSource(context.Background(), id, source); err != nil {
			t.Fatal(err)
		}
	}
	mediaReader := &invalidatingRuntimeMedia{}
	runtime.AttachMedia(mediaReader)
	for _, channel := range []int{5, 6, 7, 8} {
		runtime.storeSnapshot(snapshotCacheKey("nvr", "nvr", channel), []byte("cached"), "image/jpeg")
	}
	oldFlight, _ := runtime.beginSnapshotFlight(snapshotCacheKey("nvr", "nvr", 5))
	settings, err := runtime.SetDefaultLiveSource(context.Background(), "camera")
	if err != nil || settings.Source != "camera" {
		t.Fatalf("set global source: %+v, %v", settings, err)
	}
	if len(mediaReader.invalidated) != 1 || mediaReader.invalidated[0] != "nvr_channel_05" {
		t.Fatalf("invalidated unchanged/overridden source: %v", mediaReader.invalidated)
	}
	for _, channel := range []int{5, 6, 7, 8} {
		_, _, cached := runtime.cachedSnapshot(snapshotCacheKey("nvr", "nvr", channel))
		if cached != (channel != 5) {
			t.Fatalf("incorrect snapshot invalidation for channel %d", channel)
		}
	}
	newFlight, owner := runtime.beginSnapshotFlight(snapshotCacheKey("nvr", "nvr", 5))
	if !owner || newFlight == oldFlight {
		t.Fatal("global change kept old source flight")
	}
	for _, entry := range runtime.ListStreams(false) {
		want := map[int]string{5: "camera", 6: "nvr", 7: "nvr", 8: "camera"}[entry.Channel]
		if entry.LiveSource.Source != want || entry.LiveSource.DefaultSource != "camera" {
			t.Fatalf("incorrect effective source: %+v", entry.LiveSource)
		}
		if entry.Channel == 7 && (entry.LiveSource.CameraUnavailableReason == "" || entry.Profiles["quality"].StreamURL == "") {
			t.Fatal("global camera default broke unconfigured channel")
		}
	}
	mediaReader.invalidated = nil
	entry, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_06", "default")
	if err != nil || entry.LiveSource.OverrideSource != "" || entry.LiveSource.Source != "camera" {
		t.Fatalf("restore inheritance: %+v, %v", entry, err)
	}
	if len(mediaReader.invalidated) != 1 || mediaReader.invalidated[0] != entry.ID {
		t.Fatal("restoring default did not reconnect changed source")
	}
	mediaReader.invalidated = nil
	if _, err := runtime.SetDefaultLiveSource(context.Background(), "camera"); err != nil {
		t.Fatal(err)
	}
	if len(mediaReader.invalidated) != 0 {
		t.Fatal("unchanged global preference interrupted workers")
	}
	loaded := store.NewProbeStore()
	if ok, err := loaded.LoadFile(runtime.cfg.StateStore.Path); !ok || err != nil {
		t.Fatalf("reload preferences: %v, %v", ok, err)
	}
	restarted := newRuntimeServices(runtime.cfg, loaded)
	restarted.RegisterNVR("nvr", nil, nil, deviceCfg)
	if restarted.GetDefaultLiveSource().Source != "camera" {
		t.Fatal("global source lost after restart")
	}
	for _, entry := range restarted.ListStreams(false) {
		if entry.Channel == 6 && (entry.LiveSource.Source != "camera" || entry.LiveSource.OverrideSource != "") {
			t.Fatal("removed override returned after restart")
		}
		if entry.Channel == 8 && entry.LiveSource.OverrideSource != "camera" {
			t.Fatal("global update removed explicit override")
		}
	}
}

func TestGlobalLiveSourceValidationAndMissingCredentials(t *testing.T) {
	runtime, deviceCfg := liveSourceRuntime(t)
	deviceCfg.DirectIPCCredentials = nil
	runtime.UpdateDeviceConfig("nvr", deviceCfg)
	if _, err := runtime.SetDefaultLiveSource(context.Background(), "camera"); err != nil {
		t.Fatalf("missing direct credentials must not prevent global default: %v", err)
	}
	if _, err := runtime.SetDefaultLiveSource(context.Background(), "default"); !errors.Is(err, streams.ErrInvalidLiveSource) {
		t.Fatalf("global default accepted invalid source: %v", err)
	}
	runtime.cfg.StateStore.Enabled = false
	if _, err := runtime.SetDefaultLiveSource(context.Background(), "nvr"); !errors.Is(err, streams.ErrLiveSourcePersistence) {
		t.Fatalf("global setting accepted absent persistence: %v", err)
	}
	if runtime.GetDefaultLiveSource().Source != "camera" {
		t.Fatal("failed update changed default")
	}
}
