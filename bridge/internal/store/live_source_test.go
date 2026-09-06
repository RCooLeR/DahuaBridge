package store

import (
	"os"
	"path/filepath"
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/imou"
)

func TestLiveSourcePersistsAcrossProbesAndRetainsAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := NewProbeStore()
	if err := s.SaveFileWithMetadata(path, &imou.AuthState{AccessToken: "keep-auth"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiveSourceAndSave(path, "nvr_channel_05", "camera"); err != nil {
		t.Fatal(err)
	}
	s.Set("nvr", &dahua.ProbeResult{Root: dahua.Device{ID: "nvr"}})
	if err := s.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	loaded := NewProbeStore()
	ok, auth, err := loaded.LoadFileWithMetadata(path)
	if !ok || err != nil || auth == nil || auth.AccessToken != "keep-auth" {
		t.Fatalf("lost snapshot/auth: %v, %+v, %v", ok, auth, err)
	}
	if loaded.LiveSources()["nvr_channel_05"] != "camera" {
		t.Fatal("lost preference after probe replacement")
	}
	clone := loaded.LiveSources()
	clone["nvr_channel_05"] = "nvr"
	if loaded.LiveSources()["nvr_channel_05"] != "camera" {
		t.Fatal("live sources map was not cloned")
	}
}

func TestLiveSourceWriteFailureKeepsCurrentPreference(t *testing.T) {
	s := NewProbeStore()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := s.SetLiveSourceAndSave(path, "nvr_channel_05", "nvr"); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("blocks directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiveSourceAndSave(filepath.Join(blocker, "state.json"), "nvr_channel_05", "camera"); err == nil {
		t.Fatal("expected persistence failure")
	}
	if s.LiveSources()["nvr_channel_05"] != "nvr" {
		t.Fatal("failed setting changed active preference")
	}
}

func TestDefaultLiveSourcePersistsAndOverrideCanBeRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := NewProbeStore()
	if source, overrides := s.LiveSourceSettings(); source != "nvr" || len(overrides) != 0 {
		t.Fatal("unexpected initial defaults")
	}
	if err := s.SaveFileWithMetadata(path, &imou.AuthState{AccessToken: "keep-auth"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLiveSourceAndSave(path, "cam5", "nvr"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDefaultLiveSourceAndSave(path, "camera"); err != nil {
		t.Fatal(err)
	}
	s.Set("nvr", &dahua.ProbeResult{Root: dahua.Device{ID: "nvr"}})
	if err := s.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	loaded := NewProbeStore()
	if ok, auth, err := loaded.LoadFileWithMetadata(path); !ok || err != nil || auth == nil || auth.AccessToken != "keep-auth" {
		t.Fatalf("load default/auth: %v, %v, %v", ok, auth, err)
	}
	if source, overrides := loaded.LiveSourceSettings(); source != "camera" || overrides["cam5"] != "nvr" {
		t.Fatalf("lost default or override: %s, %v", source, overrides)
	}
	if err := loaded.SetLiveSourceAndSave(path, "cam5", ""); err != nil {
		t.Fatal(err)
	}
	restarted := NewProbeStore()
	if _, err := restarted.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if source, overrides := restarted.LiveSourceSettings(); source != "camera" || len(overrides) != 0 {
		t.Fatalf("override was not removed durably: %s, %v", source, overrides)
	}
}

func TestDefaultLiveSourceWriteFailureKeepsPreferences(t *testing.T) {
	s := NewProbeStore()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := s.SetLiveSourceAndSave(path, "cam5", "camera"); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("blocks directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDefaultLiveSourceAndSave(filepath.Join(blocker, "state.json"), "camera"); err == nil {
		t.Fatal("expected persistence failure")
	}
	if source, overrides := s.LiveSourceSettings(); source != "nvr" || overrides["cam5"] != "camera" {
		t.Fatal("failed global save changed preferences")
	}
}
