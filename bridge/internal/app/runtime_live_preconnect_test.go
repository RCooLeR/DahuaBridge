package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/streams"
)

type wakingLiveRelay struct{ wakes int }

func (*wakingLiveRelay) InvalidateStream(string) {}
func (r *wakingLiveRelay) WakePreconnect()       { r.wakes++ }

func TestPreconnectSettingValidatesPersistsAndWakesRelay(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	relay := &wakingLiveRelay{}
	runtime.AttachLiveRelay(relay)
	want := streams.LivePreconnectSettings{Mode: "always", Profile: "stable"}
	if got, err := runtime.SetLivePreconnect(context.Background(), want); err != nil || got != want || relay.wakes != 1 {
		t.Fatalf("save and wake: %+v, %v, wakes=%d", got, err, relay.wakes)
	}
	if _, err := runtime.SetLivePreconnect(context.Background(), want); err != nil || relay.wakes != 1 {
		t.Fatal("unchanged settings triggered another update")
	}
	for _, bad := range []streams.LivePreconnectSettings{{}, {Mode: "always", Profile: "bad"}, {Mode: "bad", Profile: "stable"}} {
		if _, err := runtime.SetLivePreconnect(context.Background(), bad); !errors.Is(err, streams.ErrInvalidLivePreconnect) {
			t.Fatalf("accepted invalid settings: %+v, %v", bad, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.SetLivePreconnect(ctx, want); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored canceled request")
	}
	runtime.cfg.StateStore.Enabled = false
	if _, err := runtime.SetLivePreconnect(context.Background(), streams.LivePreconnectSettings{Mode: "off", Profile: "auto"}); !errors.Is(err, streams.ErrLiveSourcePersistence) {
		t.Fatalf("accepted missing persistence: %v", err)
	}
	if runtime.GetLivePreconnect() != want || relay.wakes != 1 {
		t.Fatal("failed save changed active settings")
	}
}

func TestPreconnectTargetsFollowProfileAndCurrentSourceHealth(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	runtime.cfg.Media.Enabled = true
	for _, mode := range []string{"off", "recent"} {
		if _, err := runtime.SetLivePreconnect(context.Background(), streams.LivePreconnectSettings{Mode: mode, Profile: "stable"}); err != nil {
			t.Fatal(err)
		}
		if settings, targets := runtime.LivePreconnectTargets(); settings.Mode != mode || len(targets) != 0 {
			t.Fatalf("non-preconnect mode opened inputs: %+v, %+v", settings, targets)
		}
	}
	if _, err := runtime.SetLivePreconnect(context.Background(), streams.LivePreconnectSettings{Mode: "always", Profile: "stable"}); err != nil {
		t.Fatal(err)
	}
	_, targets := runtime.LivePreconnectTargets()
	if len(targets) != 1 || targets[0].StreamID != "nvr_channel_05" || targets[0].Profile != "stable" {
		t.Fatalf("wrong selected input: %+v", targets)
	}
	entry, profile, ok := runtime.GetStream(targets[0].StreamID, targets[0].Profile, true)
	if !ok || profile.StreamURL == "" || entry.LiveSource.Source != "nvr" {
		t.Fatal("preconnect must use the current bridge source")
	}
	runtime.liveHealth.states[entry.ID] = liveSourceHealthState{
		selection:   streams.RuntimeLiveSourceState{Source: "nvr", Signature: entry.LiveSourceSignature},
		failedUntil: map[string]time.Time{"nvr": time.Now().Add(time.Minute)},
	}
	if _, targets := runtime.LivePreconnectTargets(); len(targets) != 0 {
		t.Fatalf("warming retried current route during health cooldown: %+v", targets)
	}
	if _, err := runtime.SetDefaultLiveSource(context.Background(), "camera"); err != nil {
		t.Fatal(err)
	}
	if _, targets := runtime.LivePreconnectTargets(); len(targets) != 1 {
		t.Fatal("new preferred route inherited obsolete cooldown")
	}
	runtime.cfg.Media.Enabled = false
	if _, targets := runtime.LivePreconnectTargets(); len(targets) != 0 {
		t.Fatal("disabled media returned warm targets")
	}
}

func TestPreconnectProfileMatchesHAPreferencesAndRejectsArchiveInputs(t *testing.T) {
	entry := streams.Entry{RecommendedProfile: "stable", Profiles: map[string]streams.Profile{
		"stable":  {StreamURL: "rtsp://camera.local/sub"},
		"quality": {StreamURL: "rtsp://camera.local/main"},
	}}
	for preferred, want := range map[string]string{"auto": "stable", "stable": "stable", "quality": "quality"} {
		if got, ok := preconnectProfile(entry, preferred); !ok || got != want {
			t.Fatalf("%s selected %s, want %s", preferred, got, want)
		}
	}
	entry.Profiles["quality"] = streams.Profile{StreamURL: "rtsp://nvr.local/cam/playback"}
	if got, ok := preconnectProfile(entry, "quality"); !ok || got != "stable" {
		t.Fatal("archive playback was selected for background warm-up")
	}
	entry.Profiles["stable"] = streams.Profile{StreamURL: "rtsp://bridge.local/api/v1/rtsp/live/camera/stable"}
	if _, ok := preconnectProfile(entry, "auto"); ok {
		t.Fatal("relay URL was selected as its own upstream")
	}
}
