package archive

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"

	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

type stubSearcher struct {
	find func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)
}

type stubClipFinder struct {
	findClips func(mediaapi.ClipQuery) ([]mediaapi.ClipInfo, error)
	getClip   func(string) (mediaapi.ClipInfo, error)
}

type stubClipPrefetcher struct {
	ensure func(context.Context, string, dahua.NVRRecording) (mediaapi.ClipInfo, error)
}

func (s stubSearcher) NVRRecordings(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	return s.find(ctx, deviceID, query)
}

func (s stubClipFinder) FindClips(query mediaapi.ClipQuery) ([]mediaapi.ClipInfo, error) {
	if s.findClips != nil {
		return s.findClips(query)
	}
	return nil, nil
}

func (s stubClipFinder) GetClip(clipID string) (mediaapi.ClipInfo, error) {
	if s.getClip != nil {
		return s.getClip(clipID)
	}
	return mediaapi.ClipInfo{}, errors.New("clip not found")
}

func (s stubClipPrefetcher) EnsureNVRArchiveClip(ctx context.Context, deviceID string, item dahua.NVRRecording) (mediaapi.ClipInfo, error) {
	return s.ensure(ctx, deviceID, item)
}

func TestSQLiteStoreUpsertSMDIVSEventsDoesNotCreateChunks(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	archiveStore := NewSQLiteStore(db)
	if err := archiveStore.InitSchema(context.Background()); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	seenAt := time.Date(2026, 5, 2, 20, 0, 0, 0, time.UTC)
	items := []dahua.NVRRecording{{
		Source:      "nvr_event",
		Channel:     1,
		StartTime:   "2026-05-01 11:32:48",
		EndTime:     "2026-05-01 11:33:08",
		FilePath:    "/mnt/dvr/2026-05-01/0/dav/11/1/0/526862/11.30.00-12.00.00[R][0@0][0].dav",
		Type:        "Event.smdTypeHuman",
		VideoStream: "Main",
		Flags:       []string{"Event", "smdTypeHuman"},
	}}

	if err := archiveStore.UpsertArchiveEvents(context.Background(), "west20_nvr", items, seenAt); err != nil {
		t.Fatalf("upsert archive events: %v", err)
	}

	var eventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM smd_ivs_events`).Scan(&eventCount); err != nil {
		t.Fatalf("count smd_ivs_events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("smd_ivs_events count = %d, want 1", eventCount)
	}

	var chunkCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM nvr_recording_chunks`).Scan(&chunkCount); err != nil {
		t.Fatalf("count nvr_recording_chunks: %v", err)
	}
	if chunkCount != 0 {
		t.Fatalf("nvr_recording_chunks count = %d, want 0", chunkCount)
	}
}

func TestServiceSyncNowIndexesArchiveWindows(t *testing.T) {
	tempDir := t.TempDir()
	probes := store.NewProbeStore()
	probes.Set("west20_nvr", &dahua.ProbeResult{
		Children: []dahua.Device{{
			ID:   "west20_nvr_channel_01",
			Kind: dahua.DeviceKindNVRChannel,
			Attributes: map[string]string{
				"channel_index": "1",
			},
		}},
	})

	queries := 0
	service, err := New(config.ArchiveConfig{
		Enabled:      true,
		DBPath:       filepath.Join(tempDir, "archive.db"),
		CacheDir:     filepath.Join(tempDir, "cache"),
		TempDir:      filepath.Join(tempDir, "tmp"),
		PrefetchDays: 1,
		RetainDays:   7,
		PrefetchSMD:  true,
		PrefetchIVS:  true,
		Cron:         "5 * * * *",
	}, []config.DeviceConfig{{
		ID:      "west20_nvr",
		Enabled: boolPtr(true),
	}}, stubSearcher{
		find: func(_ context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			queries++
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device %q", deviceID)
			}
			if query.EventOnly {
				return dahua.NVRRecordingSearchResult{
					Items: []dahua.NVRRecording{{
						Source:      "nvr_event",
						Channel:     query.Channel,
						StartTime:   query.StartTime.In(time.Local).Format("2006-01-02 15:04:05"),
						EndTime:     query.StartTime.Add(20 * time.Second).In(time.Local).Format("2006-01-02 15:04:05"),
						FilePath:    "/mnt/dvr/2026-05-01/0/dav/event.dav",
						Type:        "Event." + query.EventCode,
						VideoStream: "Main",
					}},
				}, nil
			}
			return dahua.NVRRecordingSearchResult{
				Items: []dahua.NVRRecording{{
					Source:      "nvr",
					Channel:     query.Channel,
					StartTime:   query.StartTime.In(time.Local).Format("2006-01-02 15:04:05"),
					EndTime:     query.EndTime.In(time.Local).Format("2006-01-02 15:04:05"),
					FilePath:    "/mnt/dvr/2026-05-01/0/dav/archive.dav",
					VideoStream: "Main",
				}},
			}, nil
		},
	}, probes, zerolog.Nop())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	if err := service.SyncNow(context.Background()); err != nil {
		t.Fatalf("sync now: %v", err)
	}
	if err := service.SyncSMDIVSNow(context.Background()); err != nil {
		t.Fatalf("sync smd_ivs now: %v", err)
	}
	if queries == 0 {
		t.Fatal("expected archive sync queries")
	}

	var chunkCount int
	if err := service.db.QueryRow(`SELECT COUNT(*) FROM nvr_recording_chunks`).Scan(&chunkCount); err != nil {
		t.Fatalf("count nvr_recording_chunks: %v", err)
	}
	if chunkCount == 0 {
		t.Fatal("expected nvr_recording_chunks rows")
	}

	var eventCount int
	if err := service.db.QueryRow(`SELECT COUNT(*) FROM smd_ivs_events`).Scan(&eventCount); err != nil {
		t.Fatalf("count smd_ivs_events: %v", err)
	}
	if eventCount == 0 {
		t.Fatal("expected smd_ivs_events rows")
	}
}

func TestServiceSMDIVSSyncIsNotBlockedByChunkSync(t *testing.T) {
	tempDir := t.TempDir()
	chunkStarted := make(chan struct{})
	releaseChunk := make(chan struct{})
	eventQueried := make(chan struct{}, 1)
	var chunkStartedOnce sync.Once

	service, err := New(config.ArchiveConfig{
		Enabled:      true,
		DBPath:       filepath.Join(tempDir, "archive.db"),
		CacheDir:     filepath.Join(tempDir, "cache"),
		TempDir:      filepath.Join(tempDir, "tmp"),
		PrefetchDays: 0,
		RetainDays:   7,
		PrefetchSMD:  true,
		PrefetchIVS:  false,
		Cron:         "5 * * * *",
	}, []config.DeviceConfig{{
		ID:               "west20_nvr",
		Enabled:          boolPtr(true),
		ChannelAllowlist: []int{1},
	}}, stubSearcher{
		find: func(ctx context.Context, _ string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			if !query.EventOnly {
				chunkStartedOnce.Do(func() {
					close(chunkStarted)
				})
				select {
				case <-releaseChunk:
				case <-ctx.Done():
					return dahua.NVRRecordingSearchResult{}, ctx.Err()
				}
				return dahua.NVRRecordingSearchResult{}, nil
			}

			select {
			case eventQueried <- struct{}{}:
			default:
			}
			return dahua.NVRRecordingSearchResult{
				Items: []dahua.NVRRecording{{
					Source:      "nvr_event",
					Channel:     query.Channel,
					StartTime:   query.StartTime.In(time.Local).Format("2006-01-02 15:04:05"),
					EndTime:     query.StartTime.Add(20 * time.Second).In(time.Local).Format("2006-01-02 15:04:05"),
					FilePath:    "/mnt/dvr/2026-05-01/0/dav/event.dav",
					Type:        "Event." + query.EventCode,
					VideoStream: "Main",
				}},
			}, nil
		},
	}, store.NewProbeStore(), zerolog.Nop())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	syncDone := make(chan error, 1)
	go func() {
		syncDone <- service.SyncNow(context.Background())
	}()

	select {
	case <-chunkStarted:
	case <-time.After(time.Second):
		t.Fatal("chunk sync did not start")
	}

	if err := service.SyncSMDIVSNow(context.Background()); err != nil {
		t.Fatalf("sync smd_ivs now: %v", err)
	}
	select {
	case <-eventQueried:
	case <-time.After(time.Second):
		t.Fatal("expected smd_ivs query while chunk sync was still running")
	}

	close(releaseChunk)
	select {
	case err := <-syncDone:
		if err != nil {
			t.Fatalf("chunk sync returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("chunk sync did not finish")
	}
}

func TestArchiveSyncWindowsRunNewestFirst(t *testing.T) {
	start := time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)
	end := time.Date(2026, 5, 1, 12, 15, 0, 0, time.Local)
	var windows [][2]time.Time

	err := forArchiveSyncWindows(start, end, func(from time.Time, to time.Time) error {
		windows = append(windows, [2]time.Time{from, to})
		return nil
	})
	if err != nil {
		t.Fatalf("sync windows: %v", err)
	}
	if len(windows) != 3 {
		t.Fatalf("windows = %d, want 3", len(windows))
	}
	if !windows[0][0].Equal(time.Date(2026, 5, 1, 11, 15, 0, 0, time.Local)) || !windows[0][1].Equal(end) {
		t.Fatalf("first window = %s - %s, want newest partial window", windows[0][0], windows[0][1])
	}
	if !windows[2][0].Equal(start) || !windows[2][1].Equal(time.Date(2026, 5, 1, 10, 15, 0, 0, time.Local)) {
		t.Fatalf("last window = %s - %s, want oldest partial window", windows[2][0], windows[2][1])
	}
}

func TestServicePrefetchPendingEventAssetsUsesDBRows(t *testing.T) {
	tempDir := t.TempDir()
	service, err := New(config.ArchiveConfig{
		Enabled:         true,
		DBPath:          filepath.Join(tempDir, "archive.db"),
		CacheDir:        filepath.Join(tempDir, "cache"),
		TempDir:         filepath.Join(tempDir, "tmp"),
		PrefetchDays:    7,
		RetainDays:      7,
		MaxParallelJobs: 1,
		PrefetchSMD:     true,
		PrefetchIVS:     true,
		Cron:            "5 * * * *",
	}, nil, stubSearcher{
		find: func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			return dahua.NVRRecordingSearchResult{}, nil
		},
	}, store.NewProbeStore(), zerolog.Nop())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	startTime := time.Now().In(time.Local).Add(-time.Hour)
	event := dahua.NVRRecording{
		Source:      "nvr_event",
		Channel:     1,
		StartTime:   startTime.Format(archiveTimeLayout),
		EndTime:     startTime.Add(20 * time.Second).Format(archiveTimeLayout),
		FilePath:    "/mnt/dvr/recent-event.dav",
		Type:        "Event.smdTypeHuman",
		VideoStream: "Main",
		Flags:       []string{"Event", "smdTypeHuman"},
	}
	if err := service.store.UpsertArchiveEvents(context.Background(), "west20_nvr", []dahua.NVRRecording{event}, time.Now().UTC()); err != nil {
		t.Fatalf("upsert event: %v", err)
	}

	var ensured dahua.NVRRecording
	service.clips = stubClipPrefetcher{
		ensure: func(_ context.Context, deviceID string, item dahua.NVRRecording) (mediaapi.ClipInfo, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device %q", deviceID)
			}
			ensured = item
			return mediaapi.ClipInfo{
				ID:            "clip_db_pending",
				StreamID:      "nvr_export_test",
				Channel:       item.Channel,
				Status:        mediaapi.ClipStatusRecording,
				SourceStartAt: startTime.UTC(),
				SourceEndAt:   startTime.Add(20 * time.Second).UTC(),
				FileName:      "clip_db_pending.mp4",
			}, nil
		},
	}

	if err := service.prefetchPendingEventAssets(context.Background()); err != nil {
		t.Fatalf("prefetch pending assets: %v", err)
	}
	if ensured.ID == "" {
		t.Fatal("expected pending DB event to be exported")
	}
	if ensured.RecordKind != "smd_ivs" || ensured.Source != "smd_ivs" {
		t.Fatalf("unexpected exported event identity: %+v", ensured)
	}

	var clipID, status string
	if err := service.db.QueryRow(`SELECT mp4_clip_id, mp4_status FROM smd_ivs_events`).Scan(&clipID, &status); err != nil {
		t.Fatalf("query mp4 asset: %v", err)
	}
	if clipID != "clip_db_pending" || status != string(mediaapi.ClipStatusRecording) {
		t.Fatalf("unexpected mp4 asset state clip=%q status=%q", clipID, status)
	}
}

func TestServiceRefreshActiveClipAssetsAllowsNextPendingMP4(t *testing.T) {
	tempDir := t.TempDir()
	service, err := New(config.ArchiveConfig{
		Enabled:         true,
		DBPath:          filepath.Join(tempDir, "archive.db"),
		CacheDir:        filepath.Join(tempDir, "cache"),
		TempDir:         filepath.Join(tempDir, "tmp"),
		PrefetchDays:    7,
		RetainDays:      7,
		MaxParallelJobs: 1,
		PrefetchSMD:     true,
		PrefetchIVS:     true,
		Cron:            "5 * * * *",
	}, nil, stubSearcher{
		find: func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			return dahua.NVRRecordingSearchResult{}, nil
		},
	}, store.NewProbeStore(), zerolog.Nop())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	base := time.Now().In(time.Local).Add(-2 * time.Hour)
	activeEvent := dahua.NVRRecording{
		Source:      "nvr_event",
		Channel:     1,
		StartTime:   base.Format(archiveTimeLayout),
		EndTime:     base.Add(20 * time.Second).Format(archiveTimeLayout),
		FilePath:    "/mnt/dvr/active-event.dav",
		Type:        "Event.smdTypeVehicle",
		VideoStream: "Main",
		Flags:       []string{"Event", "smdTypeVehicle"},
	}
	pendingEvent := dahua.NVRRecording{
		Source:      "nvr_event",
		Channel:     1,
		StartTime:   base.Add(time.Hour).Format(archiveTimeLayout),
		EndTime:     base.Add(time.Hour + 20*time.Second).Format(archiveTimeLayout),
		FilePath:    "/mnt/dvr/pending-event.dav",
		Type:        "Event.smdTypeHuman",
		VideoStream: "Main",
		Flags:       []string{"Event", "smdTypeHuman"},
	}
	if err := service.store.UpsertArchiveEvents(context.Background(), "west20_nvr", []dahua.NVRRecording{activeEvent, pendingEvent}, time.Now().UTC()); err != nil {
		t.Fatalf("upsert events: %v", err)
	}
	activeID, activeKind := archiveRecordID("west20_nvr", activeEvent)
	if err := service.store.UpsertClipAsset(context.Background(), activeKind, activeID, "west20_nvr", activeEvent.FilePath, mediaapi.ClipInfo{
		ID:            "clip_active",
		Status:        mediaapi.ClipStatusRecording,
		Channel:       1,
		SourceStartAt: base.UTC(),
		SourceEndAt:   base.Add(20 * time.Second).UTC(),
		FileName:      "clip_active.mp4",
	}); err != nil {
		t.Fatalf("upsert active clip: %v", err)
	}

	service.clipInfo = stubClipFinder{
		getClip: func(clipID string) (mediaapi.ClipInfo, error) {
			if clipID != "clip_active" {
				return mediaapi.ClipInfo{}, errors.New("clip not found")
			}
			return mediaapi.ClipInfo{
				ID:            "clip_active",
				Status:        mediaapi.ClipStatusCompleted,
				Channel:       1,
				SourceStartAt: base.UTC(),
				SourceEndAt:   base.Add(20 * time.Second).UTC(),
				FileName:      "clip_active.mp4",
			}, nil
		},
	}
	var exportedID string
	service.clips = stubClipPrefetcher{
		ensure: func(_ context.Context, _ string, item dahua.NVRRecording) (mediaapi.ClipInfo, error) {
			exportedID = item.ID
			return mediaapi.ClipInfo{
				ID:            "clip_next",
				Status:        mediaapi.ClipStatusRecording,
				Channel:       item.Channel,
				SourceStartAt: base.Add(time.Hour).UTC(),
				SourceEndAt:   base.Add(time.Hour + 20*time.Second).UTC(),
				FileName:      "clip_next.mp4",
			}, nil
		},
	}

	if err := service.prefetchPendingEventAssets(context.Background()); err != nil {
		t.Fatalf("prefetch pending assets: %v", err)
	}
	if exportedID == "" {
		t.Fatal("expected refreshed active clip to free a slot for pending export")
	}
	pendingID, _ := archiveRecordID("west20_nvr", pendingEvent)
	if exportedID != pendingID {
		t.Fatalf("exported record %q, want pending %q", exportedID, pendingID)
	}

	var activeStatus, pendingClipID string
	if err := service.db.QueryRow(`SELECT mp4_status FROM smd_ivs_events WHERE event_id = ?`, activeID).Scan(&activeStatus); err != nil {
		t.Fatalf("query active status: %v", err)
	}
	if activeStatus != string(mediaapi.ClipStatusCompleted) {
		t.Fatalf("active status = %q, want completed", activeStatus)
	}
	if err := service.db.QueryRow(`SELECT mp4_clip_id FROM smd_ivs_events WHERE event_id = ?`, pendingID).Scan(&pendingClipID); err != nil {
		t.Fatalf("query pending clip: %v", err)
	}
	if pendingClipID != "clip_next" {
		t.Fatalf("pending clip id = %q, want clip_next", pendingClipID)
	}
}

func TestServiceRefreshActiveClipAssetsClearsMissingClip(t *testing.T) {
	tempDir := t.TempDir()
	service, err := New(config.ArchiveConfig{
		Enabled:         true,
		DBPath:          filepath.Join(tempDir, "archive.db"),
		CacheDir:        filepath.Join(tempDir, "cache"),
		TempDir:         filepath.Join(tempDir, "tmp"),
		PrefetchDays:    7,
		RetainDays:      7,
		MaxParallelJobs: 1,
		PrefetchSMD:     true,
		PrefetchIVS:     true,
		Cron:            "5 * * * *",
	}, nil, stubSearcher{
		find: func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			return dahua.NVRRecordingSearchResult{}, nil
		},
	}, store.NewProbeStore(), zerolog.Nop())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	startTime := time.Now().In(time.Local).Add(-time.Hour)
	event := dahua.NVRRecording{
		Source:      "nvr_event",
		Channel:     1,
		StartTime:   startTime.Format(archiveTimeLayout),
		EndTime:     startTime.Add(20 * time.Second).Format(archiveTimeLayout),
		FilePath:    "/mnt/dvr/stale-event.dav",
		Type:        "Event.smdTypeHuman",
		VideoStream: "Main",
		Flags:       []string{"Event", "smdTypeHuman"},
	}
	if err := service.store.UpsertArchiveEvents(context.Background(), "west20_nvr", []dahua.NVRRecording{event}, time.Now().UTC()); err != nil {
		t.Fatalf("upsert event: %v", err)
	}
	eventID, eventKind := archiveRecordID("west20_nvr", event)
	if err := service.store.UpsertClipAsset(context.Background(), eventKind, eventID, "west20_nvr", event.FilePath, mediaapi.ClipInfo{
		ID:            "clip_stale",
		Status:        mediaapi.ClipStatusRecording,
		Channel:       1,
		SourceStartAt: startTime.UTC(),
		SourceEndAt:   startTime.Add(20 * time.Second).UTC(),
		FileName:      "clip_stale.mp4",
	}); err != nil {
		t.Fatalf("upsert active clip: %v", err)
	}
	service.clipInfo = stubClipFinder{
		getClip: func(string) (mediaapi.ClipInfo, error) {
			return mediaapi.ClipInfo{}, errors.New("clip metadata missing")
		},
	}

	if err := service.refreshActiveClipAssets(context.Background()); err != nil {
		t.Fatalf("refresh active clips: %v", err)
	}

	var clipID, status string
	if err := service.db.QueryRow(`SELECT mp4_clip_id, mp4_status FROM smd_ivs_events WHERE event_id = ?`, eventID).Scan(&clipID, &status); err != nil {
		t.Fatalf("query stale clip state: %v", err)
	}
	if clipID != "" || status != "" {
		t.Fatalf("expected stale active clip to be cleared, got clip=%q status=%q", clipID, status)
	}
}

func TestServiceEnrichRecordingsAppliesStoredAssetStates(t *testing.T) {
	tempDir := t.TempDir()
	service, err := New(config.ArchiveConfig{
		Enabled:      true,
		DBPath:       filepath.Join(tempDir, "archive.db"),
		CacheDir:     filepath.Join(tempDir, "cache"),
		TempDir:      filepath.Join(tempDir, "tmp"),
		PrefetchDays: 1,
		RetainDays:   7,
		PrefetchSMD:  true,
		PrefetchIVS:  true,
		Cron:         "5,35 * * * *",
	}, nil, stubSearcher{
		find: func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			return dahua.NVRRecordingSearchResult{}, nil
		},
	}, store.NewProbeStore(), zerolog.Nop())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	readyItem := dahua.NVRRecording{
		Source:    "nvr_event",
		Channel:   1,
		StartTime: "2026-05-01 11:32:48",
		EndTime:   "2026-05-01 11:33:08",
		FilePath:  "/mnt/dvr/2026-05-01/1/11.32.48-11.33.08.dav",
		Type:      "Event.smdTypeVehicle",
		Flags:     []string{"Event", "smdTypeVehicle"},
	}
	failedItem := dahua.NVRRecording{
		Source:    "nvr_event",
		Channel:   1,
		StartTime: "2026-05-01 12:00:00",
		EndTime:   "2026-05-01 12:00:20",
		FilePath:  "/mnt/dvr/2026-05-01/1/12.00.00-12.00.20.dav",
		Type:      "Event.smdTypeHuman",
		Flags:     []string{"Event", "smdTypeHuman"},
	}
	if err := service.store.UpsertArchiveEvents(context.Background(), "west20_nvr", []dahua.NVRRecording{readyItem, failedItem}, time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("upsert smd_ivs events: %v", err)
	}
	readyID, readyKind := archiveRecordID("west20_nvr", readyItem)
	if err := service.store.UpsertClipAsset(context.Background(), readyKind, readyID, "west20_nvr", readyItem.FilePath, mediaapi.ClipInfo{
		ID:        "clip_ready",
		Status:    mediaapi.ClipStatusCompleted,
		StartedAt: time.Date(2026, 5, 1, 8, 32, 48, 0, time.UTC),
		EndedAt:   time.Date(2026, 5, 1, 8, 33, 8, 0, time.UTC),
		FileName:  "clip_ready.mp4",
	}); err != nil {
		t.Fatalf("upsert ready clip asset: %v", err)
	}

	failedID, failedKind := archiveRecordID("west20_nvr", failedItem)
	if err := service.store.UpsertClipAsset(context.Background(), failedKind, failedID, "west20_nvr", failedItem.FilePath, mediaapi.ClipInfo{
		ID:        "clip_failed",
		Status:    mediaapi.ClipStatusFailed,
		StartedAt: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 5, 1, 9, 0, 20, 0, time.UTC),
		FileName:  "clip_failed.mp4",
		Error:     "ffmpeg failed",
	}); err != nil {
		t.Fatalf("upsert failed clip asset: %v", err)
	}

	result := dahua.NVRRecordingSearchResult{
		DeviceID: "west20_nvr",
		Channel:  1,
		Items: []dahua.NVRRecording{
			readyItem,
			failedItem,
			{
				Source:    "nvr",
				Channel:   1,
				StartTime: "2026-05-01 12:30:00",
				EndTime:   "2026-05-01 12:40:00",
				FilePath:  "/mnt/dvr/2026-05-01/1/12.30.00-12.40.00.dav",
			},
		},
	}

	err = service.EnrichRecordings(context.Background(), "west20_nvr", &result, stubClipFinder{
		getClip: func(clipID string) (mediaapi.ClipInfo, error) {
			switch clipID {
			case "clip_ready":
				return mediaapi.ClipInfo{
					ID:        "clip_ready",
					Status:    mediaapi.ClipStatusCompleted,
					StartedAt: time.Date(2026, 5, 1, 8, 32, 48, 0, time.UTC),
					EndedAt:   time.Date(2026, 5, 1, 8, 33, 8, 0, time.UTC),
					FileName:  "clip_ready.mp4",
				}, nil
			case "clip_failed":
				return mediaapi.ClipInfo{}, errors.New("missing clip metadata")
			default:
				return mediaapi.ClipInfo{}, errors.New("clip not found")
			}
		},
	})
	if err != nil {
		t.Fatalf("enrich recordings: %v", err)
	}

	if got := result.Items[0].AssetStatus; got != archiveAssetStateReady {
		t.Fatalf("ready item asset_status = %q, want %q", got, archiveAssetStateReady)
	}
	if got := result.Items[0].AssetClipID; got != "clip_ready" {
		t.Fatalf("ready item asset_clip_id = %q, want clip_ready", got)
	}
	if got := result.Items[1].AssetStatus; got != archiveAssetStateFailed {
		t.Fatalf("failed item asset_status = %q, want %q", got, archiveAssetStateFailed)
	}
	if got := result.Items[1].AssetError; got != "ffmpeg failed" {
		t.Fatalf("failed item asset_error = %q, want ffmpeg failed", got)
	}
	if got := result.Items[2].AssetStatus; got != archiveAssetStateIndexed {
		t.Fatalf("indexed item asset_status = %q, want %q", got, archiveAssetStateIndexed)
	}
}

func TestServiceEventSummaryUsesIndexedSMDIVSRows(t *testing.T) {
	tempDir := t.TempDir()
	service, err := New(config.ArchiveConfig{
		Enabled:      true,
		DBPath:       filepath.Join(tempDir, "archive.db"),
		CacheDir:     filepath.Join(tempDir, "cache"),
		TempDir:      filepath.Join(tempDir, "tmp"),
		PrefetchDays: 1,
		RetainDays:   7,
		Cron:         "0 * * * *",
	}, []config.DeviceConfig{{
		ID:               "west20_nvr",
		Enabled:          boolPtr(true),
		ChannelAllowlist: []int{1, 2},
	}}, stubSearcher{
		find: func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			return dahua.NVRRecordingSearchResult{}, nil
		},
	}, store.NewProbeStore(), zerolog.Nop())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	seenAt := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	items := []dahua.NVRRecording{
		{
			Source:      "nvr_event",
			Channel:     1,
			StartTime:   "2026-05-01 11:00:00",
			EndTime:     "2026-05-01 11:00:20",
			FilePath:    "/mnt/dvr/2026-05-01/0/11.00.00-11.30.00.dav",
			Type:        "Event.smdTypeHuman",
			VideoStream: "Main",
			Flags:       []string{"Event", "smdTypeHuman"},
		},
		{
			Source:      "nvr_event",
			Channel:     1,
			StartTime:   "2026-05-01 11:10:00",
			EndTime:     "2026-05-01 11:10:20",
			FilePath:    "/mnt/dvr/2026-05-01/0/11.00.00-11.30.00.dav",
			Type:        "Event.CrossLineDetection",
			VideoStream: "Main",
			Flags:       []string{"Event", "CrossLineDetection"},
		},
		{
			Source:      "nvr_event",
			Channel:     2,
			StartTime:   "2026-05-01 11:20:00",
			EndTime:     "2026-05-01 11:20:20",
			FilePath:    "/mnt/dvr/2026-05-01/0/11.00.00-11.30.00.dav",
			Type:        "Event.smdTypeVehicle",
			VideoStream: "Main",
			Flags:       []string{"Event", "smdTypeVehicle"},
		},
		{
			Source:      "nvr_event",
			Channel:     2,
			StartTime:   "2026-05-01 11:25:00",
			EndTime:     "2026-05-01 11:25:20",
			FilePath:    "/mnt/dvr/2026-05-01/0/11.00.00-11.30.00.dav",
			Type:        "Event.MotionDetect",
			VideoStream: "Main",
			Flags:       []string{"Event", "MotionDetect"},
		},
	}
	if err := service.store.UpsertArchiveEvents(context.Background(), "west20_nvr", items, seenAt); err != nil {
		t.Fatalf("upsert events: %v", err)
	}

	summary, err := service.EventSummary(
		context.Background(),
		"west20_nvr",
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		"all",
	)
	if err != nil {
		t.Fatalf("event summary: %v", err)
	}
	if summary.TotalCount != 3 {
		t.Fatalf("summary total = %d, want 3", summary.TotalCount)
	}
	if len(summary.Channels) != 2 {
		t.Fatalf("summary channels = %d, want 2", len(summary.Channels))
	}
}

func TestSQLiteStoreSearchRecordingsUsesIndexedAllEventScope(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	archiveStore := NewSQLiteStore(db)
	if err := archiveStore.InitSchema(context.Background()); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	seenAt := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	items := []dahua.NVRRecording{
		{
			Source:      "nvr_event",
			Channel:     1,
			StartTime:   "2026-05-01 11:00:00",
			EndTime:     "2026-05-01 11:00:20",
			FilePath:    "/mnt/dvr/2026-05-01/0/11.00.00-11.30.00.dav",
			Type:        "Event.smdTypeHuman",
			VideoStream: "Main",
			Flags:       []string{"Event", "smdTypeHuman"},
		},
		{
			Source:      "nvr_event",
			Channel:     1,
			StartTime:   "2026-05-01 12:00:00",
			EndTime:     "2026-05-01 12:00:20",
			FilePath:    "/mnt/dvr/2026-05-01/0/12.00.00-12.30.00.dav",
			Type:        "Event.CrossRegionDetection",
			VideoStream: "Main",
			Flags:       []string{"Event", "CrossRegionDetection"},
		},
	}
	if err := archiveStore.UpsertArchiveEvents(context.Background(), "west20_nvr", items, seenAt); err != nil {
		t.Fatalf("upsert archive events: %v", err)
	}

	result, err := archiveStore.SearchRecordings(context.Background(), "west20_nvr", dahua.NVRRecordingQuery{
		Channel:   1,
		StartTime: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Limit:     10,
		EventOnly: true,
		EventCode: "all",
	})
	if err != nil {
		t.Fatalf("search recordings: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 indexed event rows, got %+v", result.Items)
	}
}

func TestServiceArchiveCoverageUsesIndexedFileChunks(t *testing.T) {
	tempDir := t.TempDir()
	service, err := New(config.ArchiveConfig{
		Enabled:      true,
		DBPath:       filepath.Join(tempDir, "archive.db"),
		CacheDir:     filepath.Join(tempDir, "cache"),
		TempDir:      filepath.Join(tempDir, "tmp"),
		PrefetchDays: 1,
		RetainDays:   7,
		Cron:         "0 * * * *",
	}, nil, stubSearcher{
		find: func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			return dahua.NVRRecordingSearchResult{}, nil
		},
	}, store.NewProbeStore(), zerolog.Nop())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	seenAt := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	if err := service.store.UpsertArchiveFiles(context.Background(), "west20_nvr", []dahua.NVRRecording{
		{
			Source:      "nvr",
			Channel:     1,
			StartTime:   "2026-05-01 11:00:00",
			EndTime:     "2026-05-01 11:30:00",
			FilePath:    "/mnt/dvr/2026-05-01/0/11.00.00-11.30.00.dav",
			VideoStream: "Main",
		},
		{
			Source:      "nvr",
			Channel:     1,
			StartTime:   "2026-05-03 09:30:00",
			EndTime:     "2026-05-03 10:00:00",
			FilePath:    "/mnt/dvr/2026-05-03/0/09.30.00-10.00.00.dav",
			VideoStream: "Main",
		},
	}, seenAt); err != nil {
		t.Fatalf("upsert files: %v", err)
	}

	coverage, err := service.ArchiveCoverage(context.Background(), "west20_nvr", 1)
	if err != nil {
		t.Fatalf("archive coverage: %v", err)
	}
	if coverage.ChunkCount != 2 || len(coverage.Chunks) != 2 {
		t.Fatalf("unexpected coverage %+v", coverage)
	}
	if coverage.StartTime != "2026-05-01T08:00:00Z" {
		t.Fatalf("unexpected coverage start %q", coverage.StartTime)
	}
	if coverage.EndTime != "2026-05-03T07:00:00Z" {
		t.Fatalf("unexpected coverage end %q", coverage.EndTime)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
