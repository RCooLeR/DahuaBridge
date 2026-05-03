package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClipOutputSizeBytesReturnsFileSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	body := make([]byte, 8192)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write clip: %v", err)
	}

	size, err := clipOutputSizeBytes(path)
	if err != nil {
		t.Fatalf("clipOutputSizeBytes returned error: %v", err)
	}
	if size != int64(len(body)) {
		t.Fatalf("clipOutputSizeBytes size = %d, want %d", size, len(body))
	}
}

func TestClipOutputSizeBytesRejectsMissingFile(t *testing.T) {
	if _, err := clipOutputSizeBytes(filepath.Join(t.TempDir(), "missing.mp4")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
