package archive

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

func newReliabilityArchive(t *testing.T, path string, find func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)) *Service {
	t.Helper()
	s, err := New(config.ArchiveConfig{Enabled: true, DBPath: path, TempDir: filepath.Dir(path), PrefetchDays: 7, RetainDays: 7, PrefetchSMD: true, Cron: "5 * * * *", ExportEventMP4: boolPtr(false)},
		[]config.DeviceConfig{{ID: "nvr", ChannelAllowlist: []int{1}}}, stubSearcher{find: find}, store.NewProbeStore(), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestEventCheckpointsPersistAndBoundBackfill(t *testing.T) {
	var queries []dahua.NVRRecordingQuery
	find := func(_ context.Context, _ string, q dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
		queries = append(queries, q)
		if !q.ScanAll {
			t.Error("archive query can truncate")
		}
		return dahua.NVRRecordingSearchResult{}, nil
	}
	path := filepath.Join(t.TempDir(), "archive.db")
	s := newReliabilityArchive(t, path, find)
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	if err := s.syncIncrementalEventCode(t.Context(), "nvr", 1, "human", now); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 || !queries[0].StartTime.Equal(now.Add(-archiveEventOverlap)) {
		t.Fatalf("unexpected initial windows: %+v", queries)
	}
	last := queries[2].StartTime
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = newReliabilityArchive(t, path, find)
	queries = nil
	if err := s.syncIncrementalEventCode(t.Context(), "nvr", 1, "human", now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 || !queries[0].StartTime.Equal(now.Add(-archiveEventOverlap)) || !queries[1].EndTime.Equal(last) {
		t.Fatalf("restart lost recent cursor or backfill cursor: %+v", queries)
	}
	for _, q := range queries {
		if q.EndTime.Sub(q.StartTime) > time.Hour {
			t.Fatal("unbounded scan window")
		}
	}
	// A long downtime moves recent work forward and queues the gap as backfill.
	queries = nil
	if err := s.syncIncrementalEventCode(t.Context(), "nvr", 1, "human", now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 || !queries[0].StartTime.Equal(now.Add(23*time.Hour)) || !queries[1].EndTime.Equal(queries[0].StartTime) {
		t.Fatal("downtime gap was skipped")
	}
}

func TestFailedScanDoesNotAdvanceCheckpoint(t *testing.T) {
	var fail bool
	s := newReliabilityArchive(t, filepath.Join(t.TempDir(), "archive.db"), func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
		if fail {
			return dahua.NVRRecordingSearchResult{}, errors.New("recorder unavailable")
		}
		return dahua.NVRRecordingSearchResult{}, nil
	})
	now := time.Now()
	if err := s.syncIncrementalEventCode(t.Context(), "nvr", 1, "human", now); err != nil {
		t.Fatal(err)
	}
	before, _ := s.store.loadEventScanProgress(t.Context(), "nvr", 1, "human")
	fail = true
	if err := s.syncIncrementalEventCode(t.Context(), "nvr", 1, "human", now.Add(time.Minute)); err == nil {
		t.Fatal("failed scan succeeded")
	}
	after, _ := s.store.loadEventScanProgress(t.Context(), "nvr", 1, "human")
	if !before.recentEnd.Equal(after.recentEnd) || !before.backfillBefore.Equal(after.backfillBefore) {
		t.Fatal("failed scan advanced checkpoint")
	}
}

func TestArchiveStoresMoreThanPageWithTimestampTies(t *testing.T) {
	s := newReliabilityArchive(t, filepath.Join(t.TempDir(), "archive.db"), func(_ context.Context, _ string, q dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
		if !q.ScanAll {
			t.Fatal("archive did not request complete scan")
		}
		items := make([]dahua.NVRRecording, 300)
		for i := range items {
			items[i] = dahua.NVRRecording{Channel: 1, Source: "nvr_event", StartTime: "2026-09-06 12:00:00", EndTime: "2026-09-06 12:00:20", Type: "Event.smdTypeHuman", FilePath: fmt.Sprintf("/event/%d.dav", i)}
		}
		return dahua.NVRRecordingSearchResult{Items: items}, nil
	})
	if _, err := s.syncEventWindow(t.Context(), "nvr", 1, "human", time.Now().Add(-time.Hour), time.Now()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM smd_ivs_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 300 {
		t.Fatalf("stored %d records, want 300", count)
	}
}

func TestArchiveCloseCancelsAndJoinsRunningSearches(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	s := newReliabilityArchive(t, filepath.Join(t.TempDir(), "archive.db"), func(ctx context.Context, _ string, _ dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return dahua.NVRRecordingSearchResult{}, ctx.Err()
	})
	go func() { _ = s.SyncNow(context.Background()); close(finished) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("search did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel running search")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Close left sync running")
	}
	if err := s.SyncNow(t.Context()); err == nil {
		t.Fatal("closed service accepted sync")
	}
	if err := s.Start(t.Context()); err == nil {
		t.Fatal("closed service restarted")
	}
}

func TestArchiveCloseStopsScheduledWorkersWithUncanceledParent(t *testing.T) {
	started := make(chan struct{}, 2)
	s := newReliabilityArchive(t, filepath.Join(t.TempDir(), "archive.db"), func(ctx context.Context, _ string, _ dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return dahua.NVRRecordingSearchResult{}, ctx.Err()
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not start")
	}
	finished := make(chan error, 2)
	for range 2 {
		go func() { finished <- s.Close() }()
	}
	for range 2 {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent Close left scheduled workers running")
		}
	}
}

func TestCompletedClipReconciliationDetectsMissingFileWithoutExtendingRetention(t *testing.T) {
	s := newReliabilityArchive(t, filepath.Join(t.TempDir(), "archive.db"), func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
		return dahua.NVRRecordingSearchResult{}, nil
	})
	item := dahua.NVRRecording{Channel: 1, Source: "nvr_event", Type: "Event.smdTypeHuman", StartTime: "2026-09-06 12:00:00", EndTime: "2026-09-06 12:00:20"}
	seenAt := time.Now().Add(-48 * time.Hour).UTC()
	if err := s.store.UpsertArchiveEvents(t.Context(), "nvr", []dahua.NVRRecording{item}, seenAt); err != nil {
		t.Fatal(err)
	}
	id, kind := archiveRecordID("nvr", item)
	clip := media.ClipInfo{ID: "output", Status: media.ClipStatusCompleted}
	if err := s.store.UpsertClipAsset(t.Context(), kind, id, "nvr", "", clip); err != nil {
		t.Fatal(err)
	}
	s.clipInfo = stubClipFinder{getClip: func(string) (media.ClipInfo, error) { return clip, nil }}
	if err := s.reconcileClipAssets(t.Context()); err != nil {
		t.Fatal(err)
	}
	var seen string
	if err := s.db.QueryRow(`SELECT last_seen_at FROM smd_ivs_events WHERE event_id=?`, id).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen != seenAt.Format(time.RFC3339Nano) {
		t.Fatal("asset reconciliation extended event retention")
	}
	s.clipInfo = stubClipFinder{getClip: func(string) (media.ClipInfo, error) { return media.ClipInfo{}, media.ErrClipNotFound }}
	if err := s.reconcileClipAssets(t.Context()); err != nil {
		t.Fatal(err)
	}
	candidates, err := s.store.LoadPendingEventClipCandidates(t.Context(), time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local), time.Date(2026, 9, 7, 0, 0, 0, 0, time.Local), 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatal("missing completed output did not become retryable")
	}
}
