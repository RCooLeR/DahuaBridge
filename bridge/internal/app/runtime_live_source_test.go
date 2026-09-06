package app

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/store"
	"RCooLeR/DahuaBridge/internal/streams"
)

type invalidatingRuntimeMedia struct {
	stubRuntimeMedia
	invalidated []string
}

func (m *invalidatingRuntimeMedia) InvalidateStream(streamID string) {
	m.invalidated = append(m.invalidated, streamID)
}

func liveSourceRuntime(t *testing.T) (*runtimeServices, config.DeviceConfig) {
	t.Helper()
	probes := store.NewProbeStore()
	probes.Set("nvr", &dahua.ProbeResult{
		Root:     dahua.Device{ID: "nvr", Kind: dahua.DeviceKindNVR},
		Children: []dahua.Device{{ID: "nvr_channel_05", Kind: dahua.DeviceKindNVRChannel, Attributes: map[string]string{"channel_index": "5", "sub_codec": "H264"}}},
	})
	cfg := config.DeviceConfig{
		ID: "nvr", BaseURL: "http://nvr.local", Username: "nvr-user", Password: "nvr-pass",
		DirectIPCCredentials: []config.ChannelDirectIPCCredential{{NVRChannel: 5, DirectIPCIP: "camera.local", DirectIPCUser: "camera-user", DirectIPCPassword: "camera-pass"}},
	}
	runtime := newRuntimeServices(config.Config{StateStore: config.StateStoreConfig{Enabled: true, Path: filepath.Join(t.TempDir(), "state.json")}}, probes)
	runtime.RegisterNVR("nvr", nil, nil, cfg)
	return runtime, cfg
}

func TestSetStreamLiveSourcePersistsRestartsLiveAndKeepsNVRPlayback(t *testing.T) {
	runtime, deviceCfg := liveSourceRuntime(t)
	mediaReader := &invalidatingRuntimeMedia{}
	runtime.AttachMedia(mediaReader)
	runtime.storeSnapshot(snapshotCacheKey("nvr", "nvr", 5), []byte("old"), "image/jpeg")
	entry, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_05", "camera")
	if err != nil {
		t.Fatal(err)
	}
	if entry.LiveSource.Source != "camera" || strings.Contains(entry.Profiles["quality"].StreamURL, "@") || strings.Contains(entry.Profiles["quality"].RecorderStreamURL, "@") {
		t.Fatalf("invalid public response: %+v", entry)
	}
	if len(mediaReader.invalidated) != 1 || mediaReader.invalidated[0] != entry.ID {
		t.Fatalf("live workers not invalidated: %+v", mediaReader.invalidated)
	}
	if _, _, ok := runtime.cachedSnapshot(snapshotCacheKey("nvr", "nvr", 5)); ok {
		t.Fatal("old snapshot cache survived source change")
	}
	loaded := store.NewProbeStore()
	if ok, err := loaded.LoadFile(runtime.cfg.StateStore.Path); !ok || err != nil {
		t.Fatalf("load persisted setting: %v, %v", ok, err)
	}
	restarted := newRuntimeServices(runtime.cfg, loaded)
	restarted.RegisterNVR("nvr", nil, nil, deviceCfg)
	_, live, ok := restarted.GetStream(entry.ID, "quality", true)
	if !ok || !strings.Contains(live.StreamURL, "camera-user:camera-pass@camera.local:554") {
		t.Fatalf("restart lost camera source: %+v", live)
	}
	start := time.Now().Add(-time.Hour)
	session, err := restarted.CreateNVRPlaybackSession(context.Background(), "nvr", dahua.NVRPlaybackSessionRequest{Channel: 5, StartTime: start, EndTime: start.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	_, playback, ok := restarted.GetStream(session.StreamID, "quality", true)
	parsed, err := url.Parse(playback.StreamURL)
	if !ok || err != nil || parsed.Hostname() != "nvr.local" || parsed.Path != "/cam/playback" || parsed.Query().Get("channel") != "5" {
		t.Fatalf("playback moved off recorder: %+v, %v", playback, err)
	}
	if _, err := runtime.SetStreamLiveSource(context.Background(), entry.ID, "nvr"); err != nil {
		t.Fatal(err)
	}
	if len(mediaReader.invalidated) != 2 {
		t.Fatal("switch back did not invalidate workers")
	}
}

func TestSetStreamLiveSourceRejectsUnusableSettings(t *testing.T) {
	for _, item := range []struct {
		name, source string
		mutate       func(*runtimeServices)
		want         error
	}{
		{"invalid", "auto", nil, streams.ErrInvalidLiveSourceOverride},
		{"disabled storage", "camera", func(r *runtimeServices) { r.cfg.StateStore.Enabled = false }, streams.ErrLiveSourcePersistence},
		{"empty storage path", "camera", func(r *runtimeServices) { r.cfg.StateStore.Path = "" }, streams.ErrLiveSourcePersistence},
	} {
		t.Run(item.name, func(t *testing.T) {
			runtime, _ := liveSourceRuntime(t)
			if item.mutate != nil {
				item.mutate(runtime)
			}
			_, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_05", item.source)
			if !errors.Is(err, item.want) {
				t.Fatalf("got %v, want %v", err, item.want)
			}
			if len(runtime.probes.LiveSources()) != 0 {
				t.Fatal("rejected setting changed preferences")
			}
		})
	}
}

func TestSourceSwitchDiscardsOldSnapshotFlight(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	key := snapshotCacheKey("nvr", "nvr", 5)
	old, _ := runtime.beginSnapshotFlight(key)
	if _, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_05", "camera"); err != nil {
		t.Fatal(err)
	}
	current, owner := runtime.beginSnapshotFlight(key)
	if !owner || current == old {
		t.Fatal("snapshot still joined old upstream flight")
	}
	runtime.storeSnapshotForFlight(key, old, []byte("old"), "image/jpeg")
	runtime.finishSnapshotFlight(key, old, []byte("old"), "image/jpeg", nil)
	if _, _, ok := runtime.cachedSnapshot(key); ok {
		t.Fatal("old snapshot repopulated cache")
	}
	if same, owner := runtime.beginSnapshotFlight(key); owner || same != current {
		t.Fatal("old completion removed new flight")
	}
	runtime.storeSnapshotForFlight(key, current, []byte("new"), "image/jpeg")
	runtime.finishSnapshotFlight(key, current, []byte("new"), "image/jpeg", nil)
	if body, _, ok := runtime.cachedSnapshot(key); !ok || string(body) != "new" {
		t.Fatal("new snapshot was not cached")
	}
}
