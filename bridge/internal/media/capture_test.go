package media

import (
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
)

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
		config.MediaConfig{InputPreset: "stable"},
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
