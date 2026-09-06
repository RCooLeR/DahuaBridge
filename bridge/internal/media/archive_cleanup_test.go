package media

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"github.com/rs/zerolog"
)

func TestArchiveCleanupMissingMetadataAndOwnershipGuards(t *testing.T) {
	dir := t.TempDir()
	m := New(config.MediaConfig{ClipPath: dir}, testResolver{}, zerolog.Nop(), nil)
	defer m.Close()
	clip := ClipInfo{ID: "clip_orphan", StreamID: "nvr_export_test", FileName: "clip_orphan.mp4"}
	path := filepath.Join(dir, clip.FileName)
	if err := os.WriteFile(path, []byte("failed partial output"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []ClipInfo{
		{ID: clip.ID, StreamID: "camera_live", FileName: clip.FileName},
		{ID: clip.ID, StreamID: clip.StreamID, FileName: "manual.mp4"},
		{ID: "../outside", StreamID: clip.StreamID, FileName: "../outside.mp4"},
	} {
		if err := m.DeleteArchiveClip(t.Context(), invalid); err == nil {
			t.Fatal("invalid ownership accepted")
		}
	}
	m.clipJobs[clip.ID] = &clipJob{}
	if err := m.DeleteArchiveClip(t.Context(), clip); !errors.Is(err, ErrClipAlreadyActive) {
		t.Fatalf("active deletion: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("active output removed")
	}
	delete(m.clipJobs, clip.ID)
	if err := m.DeleteArchiveClip(t.Context(), clip); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan output remains: %v", err)
	}
	if err := m.DeleteArchiveClip(t.Context(), clip); err != nil {
		t.Fatalf("cleanup not idempotent: %v", err)
	}
}

func TestArchiveCleanupRejectsManualMetadata(t *testing.T) {
	m := New(config.MediaConfig{ClipPath: t.TempDir()}, testResolver{}, zerolog.Nop(), nil)
	defer m.Close()
	manual := ClipInfo{ID: "clip_manual", StreamID: "camera_live", FileName: "clip_manual.mp4"}
	if err := m.persistClip(manual); err != nil {
		t.Fatal(err)
	}
	claim := manual
	claim.StreamID = "nvr_export_forged"
	if err := m.DeleteArchiveClip(t.Context(), claim); err == nil {
		t.Fatal("manual metadata ownership overwritten")
	}
	if _, err := m.loadClip(manual.ID); err != nil {
		t.Fatal("manual metadata removed")
	}
}
