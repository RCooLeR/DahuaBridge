package store

import (
	"os"
	"path/filepath"
	"testing"

	"RCooLeR/DahuaBridge/internal/imou"
	"RCooLeR/DahuaBridge/internal/streams"
)

func TestPreconnectPersistsAlongsideLiveSourcesAndAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := NewProbeStore()
	if got := s.LivePreconnect(); got.Mode != "off" || got.Profile != "auto" {
		t.Fatalf("unexpected initial settings: %+v", got)
	}
	if err := s.SaveFileWithMetadata(path, &imou.AuthState{AccessToken: "keep-auth"}); err != nil {
		t.Fatal(err)
	}
	want := streams.LivePreconnectSettings{Mode: "always", Profile: "stable"}
	if err := s.SetLivePreconnectAndSave(path, want); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDefaultLiveSourceAndSave(path, "camera"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiveSourceAndSave(path, "camera5", "nvr"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	loaded := NewProbeStore()
	if ok, auth, err := loaded.LoadFileWithMetadata(path); !ok || err != nil || auth == nil || auth.AccessToken != "keep-auth" {
		t.Fatalf("load metadata: %v, %+v, %v", ok, auth, err)
	}
	if got := loaded.LivePreconnect(); got != want {
		t.Fatalf("lost warm settings after other settings writes: %+v", got)
	}
	if source, overrides := loaded.LiveSourceSettings(); source != "camera" || overrides["camera5"] != "nvr" {
		t.Fatalf("lost source settings: %s, %v", source, overrides)
	}
	if err := loaded.SetLivePreconnectAndSave(path, streams.LivePreconnectSettings{Mode: "recent", Profile: "quality"}); err != nil {
		t.Fatal(err)
	}
	if source, overrides := loaded.LiveSourceSettings(); source != "camera" || overrides["camera5"] != "nvr" {
		t.Fatal("warm setting changed source preferences")
	}
}

func TestPreconnectFailedWriteKeepsCurrentSetting(t *testing.T) {
	s := NewProbeStore()
	want := streams.LivePreconnectSettings{Mode: "recent", Profile: "quality"}
	if err := s.SetLivePreconnectAndSave(filepath.Join(t.TempDir(), "state.json"), want); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLivePreconnectAndSave(filepath.Join(blocker, "state.json"), streams.LivePreconnectSettings{Mode: "always", Profile: "stable"}); err == nil {
		t.Fatal("expected persistence failure")
	}
	if got := s.LivePreconnect(); got != want {
		t.Fatalf("failed write changed settings: %+v", got)
	}
}

func TestOldOrInvalidPreconnectStateStaysOnDemand(t *testing.T) {
	for _, body := range []string{`{"version":4,"results":{}}`, `{"version":5,"results":{},"live_preconnect":{"mode":"unknown","profile":"stable"}}`} {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		s := NewProbeStore()
		if _, err := s.LoadFile(path); err != nil {
			t.Fatal(err)
		}
		if got := s.LivePreconnect(); got.Mode != "off" || got.Profile != "auto" {
			t.Fatalf("old or invalid state enabled warming: %+v", got)
		}
	}
}
