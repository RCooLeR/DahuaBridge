package media

import (
	"testing"
	"time"
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
