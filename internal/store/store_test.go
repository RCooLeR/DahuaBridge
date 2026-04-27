package store

import (
	"path/filepath"
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
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
