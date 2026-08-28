package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"
)

func TestNewClipIDUsesUUIDv7(t *testing.T) {
	first := newClipID()
	second := newClipID()
	if first == second {
		t.Fatalf("newClipID returned duplicate IDs %q", first)
	}

	parsed, err := uuid.Parse(strings.TrimPrefix(first, "clip_"))
	if err != nil {
		t.Fatalf("newClipID returned invalid UUID: %v", err)
	}
	if parsed[6]&0xf0 != 0x70 || parsed[8]&0xc0 != 0x80 {
		t.Fatalf("newClipID UUID = %q, want RFC 9562 version 7", parsed)
	}
}

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
