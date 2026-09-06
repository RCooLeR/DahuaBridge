package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/imou"
)

func TestProbeStoreReturnsClones(t *testing.T) {
	s := NewProbeStore()
	s.Set("nvr", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:         "nvr",
			Attributes: map[string]string{"firmware": "1"},
		},
		States: map[string]dahua.DeviceState{
			"nvr": {Available: true, Info: map[string]any{"count": 1}},
		},
	})

	result, ok := s.Get("nvr")
	if !ok {
		t.Fatal("expected stored result")
	}

	result.Root.Attributes["firmware"] = "2"
	result.States["nvr"] = dahua.DeviceState{Available: false}

	again, ok := s.Get("nvr")
	if !ok {
		t.Fatal("expected stored result on second read")
	}
	if again.Root.Attributes["firmware"] != "1" {
		t.Fatalf("unexpected mutated firmware %q", again.Root.Attributes["firmware"])
	}
	if !again.States["nvr"].Available {
		t.Fatal("expected original availability to remain true")
	}
}

func TestProbeStoreSaveAndLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := NewProbeStore()
	s.Set("ipc_30", &dahua.ProbeResult{
		Root: dahua.Device{ID: "ipc_30", Kind: dahua.DeviceKindIPC},
		States: map[string]dahua.DeviceState{
			"ipc_30": {Available: true, Info: map[string]any{"name": "IPC 30"}},
		},
	})

	if err := s.SaveFile(path); err != nil {
		t.Fatalf("SaveFile returned error: %v", err)
	}

	loaded := NewProbeStore()
	ok, err := loaded.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected snapshot to load")
	}

	result, exists := loaded.Get("ipc_30")
	if !exists {
		t.Fatal("expected loaded result")
	}
	if result.Root.ID != "ipc_30" {
		t.Fatalf("unexpected root id %q", result.Root.ID)
	}
}

func TestProbeStoreSaveFileWithMetadataSkipsUnchangedState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	auth := &imou.AuthState{
		AccessToken: "token-1",
		ExpiresAt:   time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
	}

	s := NewProbeStore()
	s.Set("ipc_30", &dahua.ProbeResult{
		Root: dahua.Device{ID: "ipc_30", Kind: dahua.DeviceKindIPC},
	})
	if err := s.SaveFileWithMetadata(path, auth); err != nil {
		t.Fatalf("SaveFileWithMetadata returned error: %v", err)
	}
	// A fixed timestamp detects unnecessary writes without sleeps or assuming
	// filesystem clocks advance monotonically across VM/container boundaries.
	sentinel := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, sentinel, sentinel); err != nil {
		t.Fatal(err)
	}
	firstInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}

	if err := s.SaveFileWithMetadata(path, auth); err != nil {
		t.Fatalf("second SaveFileWithMetadata returned error: %v", err)
	}
	secondInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state file after unchanged save: %v", err)
	}
	if !secondInfo.ModTime().Equal(firstInfo.ModTime()) {
		t.Fatalf("expected unchanged metadata save to skip write: before=%s after=%s", firstInfo.ModTime(), secondInfo.ModTime())
	}

	changedAuth := &imou.AuthState{
		AccessToken: "token-2",
		ExpiresAt:   auth.ExpiresAt,
	}
	if err := s.SaveFileWithMetadata(path, changedAuth); err != nil {
		t.Fatalf("changed SaveFileWithMetadata returned error: %v", err)
	}
	thirdInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state file after changed save: %v", err)
	}
	if thirdInfo.ModTime().Equal(secondInfo.ModTime()) {
		t.Fatalf("expected changed metadata save to write: before=%s after=%s", secondInfo.ModTime(), thirdInfo.ModTime())
	}
	_, restored, err := NewProbeStore().LoadFileWithMetadata(path)
	if err != nil || restored == nil || restored.AccessToken != changedAuth.AccessToken {
		t.Fatalf("changed authentication metadata was not persisted: %v", err)
	}
}
