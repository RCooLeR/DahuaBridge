package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/rs/zerolog"
)

func TestDeleteClipPreservesMetadataForRetryAfterVideoRemovalFailure(t *testing.T) {
	root := t.TempDir()
	manager := New(config.MediaConfig{ClipPath: root}, testResolver{}, zerolog.Nop(), nil)
	t.Cleanup(manager.Close)
	info := ClipInfo{ID: "clip_delete_retry", FileName: "clip_delete_retry.mp4", Status: ClipStatusFailed}
	if err := manager.persistClip(info); err != nil {
		t.Fatal(err)
	}
	videoPath := filepath.Join(root, info.FileName)
	// A non-empty directory produces a removal failure on Unix and Windows.
	if err := os.Mkdir(videoPath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(videoPath, "blocker")
	if err := os.WriteFile(blocker, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteClip(context.Background(), info.ID); err == nil {
		t.Fatal("expected video removal to fail")
	}
	if stored, err := manager.GetClip(info.ID); err != nil || stored.FileName != info.FileName {
		t.Fatalf("metadata needed for retry was lost: %+v, %v", stored, err)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteClip(context.Background(), info.ID); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if _, err := manager.GetClip(info.ID); !errors.Is(err, ErrClipNotFound) {
		t.Fatalf("metadata was not removed after successful retry: %v", err)
	}
	if _, err := os.Stat(videoPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("video was not removed after successful retry: %v", err)
	}
}

func TestClipFilePathRejectsEscapingNames(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../escape.mp4", "..\\escape.mp4", filepath.Join("nested", "..", "..", "escape.mp4")} {
		if path, err := clipFilePath(root, name); err == nil {
			t.Fatalf("expected %q to be rejected, got path %q", name, path)
		}
	}

	path, err := clipFilePath(root, filepath.Join("nested", "clip.mp4"))
	if err != nil {
		t.Fatalf("expected nested clip path: %v", err)
	}
	if rel, err := filepath.Rel(root, path); err != nil || rel != filepath.Join("nested", "clip.mp4") {
		t.Fatalf("unexpected relative path %q err=%v", rel, err)
	}
}

func TestClipSourceWindowUsesPlaybackRangeAndDuration(t *testing.T) {
	start, end := clipSourceWindow(
		"rtsp://example.local/cam/realmonitor?channel=1&subtype=0&starttime=2026_05_01_20_12_05&endtime=2026_05_01_20_12_25",
		8*time.Second,
	)

	if want := time.Date(2026, 5, 1, 20, 12, 5, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("unexpected start time %s", start)
	}
	if want := time.Date(2026, 5, 1, 20, 12, 13, 0, time.UTC); !end.Equal(want) {
		t.Fatalf("unexpected end time %s", end)
	}
}

func TestMatchesClipQueryUsesSourceWindowWhenPresent(t *testing.T) {
	info := ClipInfo{
		ID:            "clip_test",
		StartedAt:     time.Date(2026, 5, 1, 19, 18, 2, 0, time.UTC),
		EndedAt:       time.Date(2026, 5, 1, 19, 18, 22, 0, time.UTC),
		SourceStartAt: time.Date(2026, 5, 1, 20, 12, 5, 0, time.UTC),
		SourceEndAt:   time.Date(2026, 5, 1, 20, 12, 25, 0, time.UTC),
	}

	if !matchesClipQuery(info, ClipQuery{
		StartTime: time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 1, 20, 30, 0, 0, time.UTC),
	}) {
		t.Fatal("expected query to match source window")
	}

	if matchesClipQuery(info, ClipQuery{
		StartTime: time.Date(2026, 5, 1, 21, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 1, 21, 30, 0, 0, time.UTC),
	}) {
		t.Fatal("expected query outside source window to miss")
	}
}

func TestBuildClipFFmpegArgsDisablesStdinForFiniteClips(t *testing.T) {
	args := buildClipFFmpegArgs(
		config.MediaConfig{InputPreset: "stable", ScaleWidth: 960},
		streams.Profile{StreamURL: "rtsp://example.local/live"},
		10*time.Second,
		"clip.mp4",
		false,
		true,
	)

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-nostdin") {
		t.Fatalf("expected finite clip args to disable stdin, got %q", joined)
	}
	if !strings.Contains(joined, "-t 10") {
		t.Fatalf("expected finite clip args to include duration, got %q", joined)
	}
	if strings.Contains(joined, "scale=") || strings.Contains(joined, "vpp_qsv=") {
		t.Fatalf("expected clip args to ignore live max-width scaling, got %q", joined)
	}
}

func TestShouldIncludeSourceAudioTrustsKnownAudioCodec(t *testing.T) {
	manager := New(config.MediaConfig{FFmpegPath: "missing-ffmpeg"}, testResolver{}, zerolog.Nop(), nil)

	if !manager.shouldIncludeSourceAudio(streams.Profile{
		StreamURL:   "rtsp://example.local/live",
		AudioCodec:  "AAC",
		VideoCodec:  "H.264",
		SourceWidth: 1920,
	}, zerolog.Nop()) {
		t.Fatal("expected known audio codec metadata to keep MP4 audio enabled")
	}
}

func TestBuildPrefixedClipFFmpegArgsKeepsSourceAudio(t *testing.T) {
	args := buildClipFFmpegArgs(
		config.MediaConfig{InputPreset: "stable"},
		streams.Profile{
			StreamURL:           "recording.dav",
			InputPrefixURL:      "iframe.dav",
			InputPrefixDuration: int64(500 * time.Millisecond),
			InputSeekOffset:     int64(2 * time.Second),
			AudioCodec:          "AAC",
		},
		10*time.Second,
		"clip.mp4",
		true,
		true,
	)

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-map [a]") || !strings.Contains(joined, "-c:a aac") {
		t.Fatalf("expected prefixed clip args to include AAC audio mapping, got %q", joined)
	}
	if !strings.Contains(joined, "atrim=start=2") {
		t.Fatalf("expected prefixed clip audio to honor source seek offset, got %q", joined)
	}
	if strings.Contains(joined, " -an ") {
		t.Fatalf("did not expect prefixed clip args to disable audio, got %q", joined)
	}
}

func TestPlaybackDurationFromStreamURLParsesClipFallbackDuration(t *testing.T) {
	duration, ok := playbackDurationFromStreamURL("rtsp://example.local/playback?starttime=2026_05_01_02_30_10&endtime=2026_05_01_02_30_20")
	if !ok {
		t.Fatal("expected playback duration to be parsed")
	}
	if duration != 10*time.Second {
		t.Fatalf("unexpected playback duration %s", duration)
	}
}
