package archive

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
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

func TestSQLiteStoreUpsertArchiveEventsAndFiles(t *testing.T) {
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM archive_events`).Scan(&eventCount); err != nil {
		t.Fatalf("count archive_events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("archive_events count = %d, want 1", eventCount)
	}

	var fileCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM archive_files`).Scan(&fileCount); err != nil {
		t.Fatalf("count archive_files: %v", err)
	}
	if fileCount != 1 {
		t.Fatalf("archive_files count = %d, want 1", fileCount)
	}

	var linkCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM archive_event_files`).Scan(&linkCount); err != nil {
		t.Fatalf("count archive_event_files: %v", err)
	}
	if linkCount != 1 {
		t.Fatalf("archive_event_files count = %d, want 1", linkCount)
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
	if queries == 0 {
		t.Fatal("expected archive sync queries")
	}

	var fileCount int
	if err := service.db.QueryRow(`SELECT COUNT(*) FROM archive_files`).Scan(&fileCount); err != nil {
		t.Fatalf("count archive_files: %v", err)
	}
	if fileCount == 0 {
		t.Fatal("expected archive_files rows")
	}

	var eventCount int
	if err := service.db.QueryRow(`SELECT COUNT(*) FROM archive_events`).Scan(&eventCount); err != nil {
		t.Fatalf("count archive_events: %v", err)
	}
	if eventCount == 0 {
		t.Fatal("expected archive_events rows")
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
		Source:    "nvr",
		Channel:   1,
		StartTime: "2026-05-01 11:32:48",
		EndTime:   "2026-05-01 11:33:08",
		FilePath:  "/mnt/dvr/2026-05-01/1/11.32.48-11.33.08.dav",
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

	failedItem := dahua.NVRRecording{
		Source:    "nvr_event",
		Channel:   1,
		StartTime: "2026-05-01 12:00:00",
		EndTime:   "2026-05-01 12:00:20",
		FilePath:  "/mnt/dvr/2026-05-01/1/12.00.00-12.00.20.dav",
		Type:      "Event.smdTypeHuman",
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
