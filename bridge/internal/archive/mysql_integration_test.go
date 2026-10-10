package archive

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	mysql "github.com/go-sql-driver/mysql"
)

func TestMySQLArchiveSchemaAndSQLiteMigration(t *testing.T) {
	dsn := os.Getenv("DAHUABRIDGE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set DAHUABRIDGE_TEST_MYSQL_DSN to run MySQL integration tests")
	}
	ctx := t.Context()
	cfg := mysqlIntegrationConfig(t, dsn)
	target, err := openArchiveMySQLDB(ctx, cfg)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer target.Close()
	assertMySQLIntegrationTLS(t, ctx, target, cfg)
	targetStore := newMySQLStore(target)
	if err := targetStore.InitSchema(ctx); err != nil {
		t.Fatalf("init mysql schema: %v", err)
	}
	assertMySQLArchiveSchemaFeatures(t, ctx, target)
	clearMySQLArchiveTables(t, targetStore)

	upgradeFixture := dahua.NVRRecording{
		Channel: 9, StartTime: "2026-10-10 07:00:00", EndTime: "2026-10-10 07:00:20",
		FilePath: "/mnt/dvr/schema-upgrade.dav", Source: "smd_ivs", Type: "human",
	}
	if err := targetStore.UpsertArchiveEvents(ctx, "schema-upgrade", []dahua.NVRRecording{upgradeFixture}, time.Now()); err != nil {
		t.Fatalf("seed mysql schema upgrade fixture: %v", err)
	}
	upgradeEventID := archiveEventID("schema-upgrade", upgradeFixture)
	if _, err := target.ExecContext(ctx, `UPDATE smd_ivs_events SET mp4_status = ' COMPLETED ' WHERE event_id = ?`, upgradeEventID); err != nil {
		t.Fatalf("seed unnormalized mysql clip status: %v", err)
	}
	for table, indexes := range map[string][]string{
		"smd_ivs_events":       {"idx_smd_ivs_events_last_seen", "idx_smd_ivs_events_status", "idx_smd_ivs_events_pending_export"},
		"nvr_recording_chunks": {"idx_nvr_recording_chunks_last_seen"},
	} {
		for _, index := range indexes {
			if _, err := target.ExecContext(ctx, `DROP INDEX `+quoteMySQLIdentifier(index)+` ON `+quoteMySQLIdentifier(table)); err != nil {
				t.Fatalf("remove mysql v3 index %s for upgrade test: %v", index, err)
			}
		}
	}
	if _, err := target.ExecContext(ctx, `DELETE FROM archive_schema_migrations WHERE version = 3`); err != nil {
		t.Fatalf("restore mysql schema version 2 fixture: %v", err)
	}
	if err := targetStore.InitSchema(ctx); err != nil {
		t.Fatalf("upgrade mysql schema from v2: %v", err)
	}
	assertMySQLArchiveSchemaFeatures(t, ctx, target)
	var normalizedStatus string
	if err := target.QueryRowContext(ctx, `SELECT mp4_status FROM smd_ivs_events WHERE event_id = ?`, upgradeEventID).Scan(&normalizedStatus); err != nil {
		t.Fatalf("read normalized mysql clip status: %v", err)
	}
	if normalizedStatus != "completed" {
		t.Fatalf("mysql v3 migration left clip status %q", normalizedStatus)
	}
	clearMySQLArchiveTables(t, targetStore)

	sourcePath := filepath.Join(t.TempDir(), "archive.db")
	source, err := openArchiveSQLiteDB(ctx, sourcePath)
	if err != nil {
		t.Fatalf("open sqlite source: %v", err)
	}
	sourceStore := NewSQLiteStore(source)
	if err := sourceStore.InitSchema(ctx); err != nil {
		t.Fatalf("init sqlite source: %v", err)
	}
	// Recorder timestamps are stored and queried in the bridge's local time.
	// Keep the fixture location-independent so it behaves the same on UTC CI
	// runners and on deployments with a configured local timezone.
	seenAt := time.Date(2026, 10, 10, 10, 0, 0, 0, time.Local)
	chunk := dahua.NVRRecording{
		Channel: 3, StartTime: "2026-10-10 10:00:00", EndTime: "2026-10-10 10:05:00",
		FilePath: "/mnt/dvr/record 3.dav", VideoStream: "Main", Disk: 1, Partition: 2, Cluster: 3,
		LengthBytes: 1024, CutLengthBytes: 512, Flags: []string{"Timing"},
	}
	event := dahua.NVRRecording{
		Channel: 3, StartTime: "2026-10-10 10:01:00", EndTime: "2026-10-10 10:01:20",
		FilePath: "/mnt/dvr/event.dav", Source: "smd_ivs",
		Flags: []string{"smdTypeHuman", "quoted-value"},
	}
	if err := sourceStore.UpsertArchiveFiles(ctx, "nvr", []dahua.NVRRecording{chunk}, seenAt); err != nil {
		t.Fatalf("seed sqlite chunk: %v", err)
	}
	if err := sourceStore.UpsertArchiveEvents(ctx, "nvr", []dahua.NVRRecording{event}, seenAt); err != nil {
		t.Fatalf("seed sqlite event: %v", err)
	}
	eventID := archiveEventID("nvr", event)
	if err := sourceStore.UpsertClipAsset(ctx, "smd_ivs", eventID, "nvr", event.FilePath, mediaapi.ClipInfo{
		ID: "clip_event", StreamID: "nvr_export_event", Channel: 3, FileName: "/data/clips/event.mp4",
		Status: mediaapi.ClipStatusCompleted, SourceStartAt: seenAt, SourceEndAt: seenAt.Add(20 * time.Second),
	}); err != nil {
		t.Fatalf("seed sqlite clip: %v", err)
	}
	if err := sourceStore.saveEventScanProgress(ctx, "nvr", 3, "human", eventScanProgress{seenAt, seenAt.Add(-time.Hour)}); err != nil {
		t.Fatalf("seed sqlite checkpoint: %v", err)
	}
	for index, fixture := range []struct {
		device  string
		channel int
		code    string
	}{
		{device: "z-device", channel: 1, code: "vehicle"},
		{device: "A-device", channel: 10, code: "human"},
		{device: "пристрій", channel: 2, code: "animal"},
	} {
		shift := time.Duration(index+1) * time.Minute
		if err := sourceStore.saveEventScanProgress(ctx, fixture.device, fixture.channel, fixture.code,
			eventScanProgress{seenAt.Add(shift), seenAt.Add(-time.Hour - shift)}); err != nil {
			t.Fatalf("seed sqlite composite checkpoint %d: %v", index, err)
		}
	}
	if err := source.Close(); err != nil {
		t.Fatalf("close sqlite source: %v", err)
	}

	stats, err := MigrateSQLiteToMySQL(ctx, sourcePath, cfg)
	if err != nil {
		t.Fatalf("migrate sqlite to mysql: %v", err)
	}
	if stats.AlreadyApplied {
		t.Fatal("first migration unexpectedly reported an existing receipt")
	}
	for table, want := range map[string]int64{
		"archive_event_scan_progress": 4,
		"smd_ivs_events":              1,
		"nvr_recording_chunks":        1,
		"bridge_mp4_clips":            1,
		"archive_clip_retention":      1,
	} {
		if got := stats.Rows[table]; got != want {
			t.Fatalf("migration count for %s = %d, want %d", table, got, want)
		}
	}

	result, err := targetStore.SearchRecordings(ctx, "nvr", dahua.NVRRecordingQuery{
		Channel: 3, StartTime: seenAt.Add(-time.Hour), EndTime: seenAt.Add(time.Hour), Limit: 10,
		EventOnly: true, EventCode: "human",
	})
	if err != nil {
		t.Fatalf("search migrated mysql event: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].AssetClipID != "clip_event" {
		t.Fatalf("unexpected migrated event result: %+v", result.Items)
	}
	fileResult, err := targetStore.SearchRecordings(ctx, "nvr", dahua.NVRRecordingQuery{
		Channel: 3, StartTime: seenAt.Add(-time.Hour), EndTime: seenAt.Add(time.Hour), Limit: 10,
	})
	if err != nil {
		t.Fatalf("search migrated mysql chunk: %v", err)
	}
	if len(fileResult.Items) != 1 || fileResult.Items[0].Partition != 2 {
		t.Fatalf("unexpected migrated chunk result: %+v", fileResult.Items)
	}

	chunkID := archiveFileID("nvr", chunk)
	if _, err := target.ExecContext(ctx,
		`UPDATE nvr_recording_chunks SET video_stream = ? WHERE chunk_id = ?`, "newer-mysql-value", chunkID,
	); err != nil {
		t.Fatalf("advance mysql row after migration: %v", err)
	}
	repeated, err := MigrateSQLiteToMySQL(ctx, sourcePath, cfg)
	if err != nil {
		t.Fatalf("repeat matching migration: %v", err)
	}
	if !repeated.AlreadyApplied {
		t.Fatal("matching migration did not use its receipt")
	}
	var targetVideoStream string
	if err := target.QueryRowContext(ctx,
		`SELECT video_stream FROM nvr_recording_chunks WHERE chunk_id = ?`, chunkID,
	).Scan(&targetVideoStream); err != nil {
		t.Fatalf("read mysql row after matching migration: %v", err)
	}
	if targetVideoStream != "newer-mysql-value" {
		t.Fatalf("matching receipt rerun overwrote newer mysql data: %q", targetVideoStream)
	}

	mutableSource, err := openArchiveSQLiteDB(ctx, sourcePath)
	if err != nil {
		t.Fatalf("reopen sqlite source for changed-snapshot test: %v", err)
	}
	if _, err := mutableSource.ExecContext(ctx,
		`UPDATE nvr_recording_chunks SET video_stream = ? WHERE chunk_id = ?`, "changed-sqlite-value", chunkID,
	); err != nil {
		_ = mutableSource.Close()
		t.Fatalf("change sqlite source row: %v", err)
	}
	if err := mutableSource.Close(); err != nil {
		t.Fatalf("close changed sqlite source: %v", err)
	}
	if _, err := MigrateSQLiteToMySQL(ctx, sourcePath, cfg); err == nil || !strings.Contains(err.Error(), "different SQLite snapshot") {
		t.Fatalf("changed sqlite source was not rejected: %v", err)
	}
	if err := target.QueryRowContext(ctx,
		`SELECT video_stream FROM nvr_recording_chunks WHERE chunk_id = ?`, chunkID,
	).Scan(&targetVideoStream); err != nil {
		t.Fatalf("read mysql row after rejected migration: %v", err)
	}
	if targetVideoStream != "newer-mysql-value" {
		t.Fatalf("rejected changed snapshot overwrote mysql data: %q", targetVideoStream)
	}

	if _, err := target.ExecContext(ctx, `DELETE FROM archive_sqlite_import_receipt WHERE receipt_id = 1`); err != nil {
		t.Fatalf("remove receipt for nonempty-target test: %v", err)
	}
	if _, err := MigrateSQLiteToMySQL(ctx, sourcePath, cfg); err == nil || !strings.Contains(err.Error(), "target must be empty") {
		t.Fatalf("nonempty target without a receipt was not rejected: %v", err)
	}
}

func TestMySQLArchiveRuntimeOperations(t *testing.T) {
	dsn := os.Getenv("DAHUABRIDGE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set DAHUABRIDGE_TEST_MYSQL_DSN to run MySQL integration tests")
	}
	ctx := t.Context()
	cfg := mysqlIntegrationConfig(t, dsn)
	target, err := openArchiveMySQLDB(ctx, cfg)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer target.Close()
	store := newMySQLStore(target)
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("init mysql schema: %v", err)
	}
	clearMySQLArchiveTables(t, store)

	seenAt := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	chunk := dahua.NVRRecording{
		Channel: 4, StartTime: "2026-10-10 08:00:00", EndTime: "2026-10-10 08:05:00",
		FilePath: "/mnt/dvr/runtime-chunk.dav", VideoStream: "Main", Disk: 1, Partition: 2, Cluster: 3,
		LengthBytes: 2048, CutLengthBytes: 1024, Flags: []string{"Timing"},
	}
	event := dahua.NVRRecording{
		Channel: 4, StartTime: "2026-10-10 08:01:00", EndTime: "2026-10-10 08:01:20",
		FilePath: "/mnt/dvr/runtime-event.dav", Source: "smd_ivs", Type: "human",
		Flags: []string{"smdTypeHuman"},
	}
	if err := store.UpsertArchiveFiles(ctx, "nvr", []dahua.NVRRecording{chunk}, seenAt); err != nil {
		t.Fatalf("upsert mysql archive file: %v", err)
	}
	if err := store.UpsertArchiveEvents(ctx, "nvr", []dahua.NVRRecording{event}, seenAt); err != nil {
		t.Fatalf("upsert mysql archive event: %v", err)
	}
	progress := eventScanProgress{recentEnd: seenAt, backfillBefore: seenAt.Add(-time.Hour)}
	if err := store.saveEventScanProgress(ctx, "nvr", 4, "human", progress); err != nil {
		t.Fatalf("save mysql event checkpoint: %v", err)
	}
	loadedProgress, err := store.loadEventScanProgress(ctx, "nvr", 4, "human")
	if err != nil {
		t.Fatalf("load mysql event checkpoint: %v", err)
	}
	if !loadedProgress.recentEnd.Equal(progress.recentEnd) || !loadedProgress.backfillBefore.Equal(progress.backfillBefore) {
		t.Fatalf("unexpected mysql checkpoint: %+v", loadedProgress)
	}

	eventID := archiveEventID("nvr", event)
	clip := mediaapi.ClipInfo{
		ID: "clip_runtime", StreamID: "nvr_export_runtime", Channel: 4,
		FileName: "/data/clips/runtime.mp4", Status: mediaapi.ClipStatus(" COMPLETED "),
		SourceStartAt: seenAt, SourceEndAt: seenAt.Add(20 * time.Second),
	}
	if err := store.UpsertClipAsset(ctx, "smd_ivs", eventID, "nvr", event.FilePath, clip); err != nil {
		t.Fatalf("upsert mysql clip: %v", err)
	}
	event.ID = eventID
	event.RecordKind = "smd_ivs"
	assets, err := store.LoadClipAssets(ctx, "nvr", []dahua.NVRRecording{event})
	if err != nil {
		t.Fatalf("load mysql clip asset: %v", err)
	}
	asset, ok := assets[archiveRecordKey("smd_ivs", eventID)]
	if !ok || asset.ClipID != clip.ID || asset.Status != "completed" {
		t.Fatalf("unexpected mysql clip asset: %+v found=%t", asset, ok)
	}

	clipIDs, err := store.PruneOlderThan(ctx, seenAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("prune mysql archive: %v", err)
	}
	if len(clipIDs) != 1 || clipIDs[0] != clip.ID {
		t.Fatalf("unexpected mysql retention queue: %v", clipIDs)
	}
	cleanupClip, err := store.clipForCleanup(ctx, clip.ID)
	if err != nil {
		t.Fatalf("load mysql clip queued for cleanup: %v", err)
	}
	if cleanupClip.ID != clip.ID || cleanupClip.FileName != clip.FileName {
		t.Fatalf("unexpected mysql cleanup clip: %+v", cleanupClip)
	}
	if err := store.finishClipCleanup(ctx, clip.ID); err != nil {
		t.Fatalf("finish mysql clip cleanup: %v", err)
	}
	for _, table := range []string{"smd_ivs_events", "nvr_recording_chunks", "bridge_mp4_clips", "archive_clip_retention"} {
		var count int
		if err := target.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+quoteMySQLIdentifier(table)).Scan(&count); err != nil {
			t.Fatalf("count mysql table %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("mysql prune left %d rows in %s", count, table)
		}
	}
}

func TestMySQLClipReattachSerializesWithRetentionQueueing(t *testing.T) {
	dsn := os.Getenv("DAHUABRIDGE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set DAHUABRIDGE_TEST_MYSQL_DSN to run MySQL integration tests")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cfg := mysqlIntegrationConfig(t, dsn)
	target, err := openArchiveMySQLDB(ctx, cfg)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer target.Close()
	store := newMySQLStore(target)
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("init mysql schema: %v", err)
	}
	clearMySQLArchiveTables(t, store)

	seenAt := time.Date(2026, 10, 10, 8, 2, 0, 0, time.Local)
	pruneCutoff := time.Date(2026, 10, 10, 11, 0, 0, 0, time.Local)
	event := dahua.NVRRecording{
		Channel: 3, StartTime: "2026-10-10 08:00:00", EndTime: "2026-10-10 08:01:00",
		FilePath: "/mnt/dvr/retention-race.dav", Source: "smd_ivs", Flags: []string{"smdTypeHuman"},
	}
	if err := store.UpsertArchiveEvents(ctx, "nvr", []dahua.NVRRecording{event}, seenAt); err != nil {
		t.Fatalf("seed mysql event: %v", err)
	}
	eventID := archiveEventID("nvr", event)
	clip := mediaapi.ClipInfo{
		ID: "clip_retention_race", StreamID: "nvr_export_retention_race", Channel: 3,
		FileName: "/data/clips/retention-race.mp4", Status: mediaapi.ClipStatusCompleted,
		SourceStartAt: seenAt.Add(-2 * time.Minute), SourceEndAt: seenAt.Add(-time.Minute),
	}
	if err := store.UpsertClipAsset(ctx, "smd_ivs", eventID, "nvr", event.FilePath, clip); err != nil {
		t.Fatalf("seed mysql clip ownership: %v", err)
	}
	if err := store.DeleteClipAsset(ctx, "smd_ivs", eventID, "nvr"); err != nil {
		t.Fatalf("detach mysql clip: %v", err)
	}

	// Stop reattachment immediately after its retention locking read. This makes
	// the old unsafe interleaving deterministic: without the locking read the
	// probe below never observes a retention lock, because reattachment blocks
	// first on this clip row and prune can queue the ownership concurrently.
	blocker, err := target.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin clip blocker: %v", err)
	}
	defer blocker.Rollback()
	var blockedClipID string
	if err := blocker.QueryRowContext(ctx,
		`SELECT clip_id FROM bridge_mp4_clips WHERE clip_id = ? FOR UPDATE`, clip.ID,
	).Scan(&blockedClipID); err != nil {
		t.Fatalf("lock clip row: %v", err)
	}

	reattachDone := make(chan error, 1)
	go func() {
		reattachDone <- store.UpsertClipAsset(ctx, "smd_ivs", eventID, "nvr", event.FilePath, clip)
	}()
	waitForMySQLRetentionRowLock(t, ctx, target, clip.ID)

	pruneDone := make(chan struct {
		clipIDs []string
		err     error
	}, 1)
	go func() {
		clipIDs, pruneErr := store.PruneOlderThan(ctx, pruneCutoff)
		pruneDone <- struct {
			clipIDs []string
			err     error
		}{clipIDs: clipIDs, err: pruneErr}
	}()
	select {
	case result := <-pruneDone:
		t.Fatalf("prune completed before the reattachment locks were released: clip_ids=%v err=%v", result.clipIDs, result.err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := blocker.Commit(); err != nil {
		t.Fatalf("release clip blocker: %v", err)
	}
	if err := <-reattachDone; err != nil {
		t.Fatalf("reattach mysql clip: %v", err)
	}
	pruned := <-pruneDone
	if pruned.err != nil {
		t.Fatalf("prune mysql archive: %v", pruned.err)
	}
	if len(pruned.clipIDs) != 1 || pruned.clipIDs[0] != clip.ID {
		t.Fatalf("unexpected prune result after serialized reattachment: %v", pruned.clipIDs)
	}

	var queued int
	if err := target.QueryRowContext(ctx,
		`SELECT queued FROM archive_clip_retention WHERE clip_id = ?`, clip.ID,
	).Scan(&queued); err != nil {
		t.Fatalf("read retention race outcome: %v", err)
	}
	var eventCount int
	if err := target.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM smd_ivs_events WHERE event_id = ?`, eventID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("read pruned event outcome: %v", err)
	}
	if queued != 1 || eventCount != 0 {
		t.Fatalf("invalid serialized retention outcome: queued=%d event_count=%d", queued, eventCount)
	}
}

func mysqlIntegrationConfig(t *testing.T, dsn string) config.ArchiveConfig {
	t.Helper()
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse DAHUABRIDGE_TEST_MYSQL_DSN: %v", err)
	}
	if strings.TrimSpace(parsed.TLSConfig) == "" {
		t.Fatal("DAHUABRIDGE_TEST_MYSQL_DSN must include an explicit tls mode")
	}
	return config.ArchiveConfig{
		DBDriver:          "mysql",
		DBDSN:             dsn,
		DBTLS:             parsed.TLSConfig,
		DBConnectTimeout:  10 * time.Second,
		DBMaxOpenConns:    4,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: time.Minute,
		DBConnMaxIdleTime: 30 * time.Second,
	}
}

func assertMySQLIntegrationTLS(t *testing.T, ctx context.Context, db *sql.DB, cfg config.ArchiveConfig) {
	t.Helper()
	prepared, err := prepareArchiveMySQLConfig(cfg)
	if err != nil {
		t.Fatalf("prepare mysql integration config: %v", err)
	}
	if prepared.TLSConfig == "" {
		return
	}
	var variableName, cipher string
	if err := db.QueryRowContext(ctx, `SHOW SESSION STATUS LIKE 'Ssl_cipher'`).Scan(&variableName, &cipher); err != nil {
		t.Fatalf("read mysql TLS session status: %v", err)
	}
	if !strings.EqualFold(variableName, "Ssl_cipher") || strings.TrimSpace(cipher) == "" {
		t.Fatalf("mysql integration connection is not encrypted: %s=%q", variableName, cipher)
	}
}

func assertMySQLArchiveSchemaFeatures(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var schemaVersion int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM archive_schema_migrations`).Scan(&schemaVersion); err != nil {
		t.Fatalf("read mysql archive schema version: %v", err)
	}
	if schemaVersion != mysqlArchiveSchemaVersion {
		t.Fatalf("mysql archive schema version = %d, want %d", schemaVersion, mysqlArchiveSchemaVersion)
	}
	for table, indexes := range map[string][]string{
		"smd_ivs_events":       {"idx_smd_ivs_events_last_seen", "idx_smd_ivs_events_status", "idx_smd_ivs_events_pending_export"},
		"nvr_recording_chunks": {"idx_nvr_recording_chunks_last_seen"},
	} {
		for _, index := range indexes {
			var count int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
				WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, table, index).Scan(&count); err != nil {
				t.Fatalf("inspect mysql index %s: %v", index, err)
			}
			if count == 0 {
				t.Fatalf("mysql archive index %s is missing", index)
			}
		}
	}
	var collation string
	if err := db.QueryRowContext(ctx, `SELECT table_collation FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'smd_ivs_events'`).Scan(&collation); err != nil {
		t.Fatalf("inspect mysql archive table collation: %v", err)
	}
	if collation != "utf8mb4_0900_bin" {
		t.Fatalf("unexpected mysql archive table collation %q", collation)
	}
}

func waitForMySQLRetentionRowLock(t *testing.T, ctx context.Context, db interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}, clipID string) {
	t.Helper()
	for {
		probe, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin retention lock probe: %v", err)
		}
		var queued int
		err = probe.QueryRowContext(ctx,
			`SELECT queued FROM archive_clip_retention WHERE clip_id = ? FOR UPDATE NOWAIT`, clipID,
		).Scan(&queued)
		_ = probe.Rollback()
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 3572 {
			return
		}
		if err != nil {
			t.Fatalf("probe retention row lock: %v", err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for retention row lock: %v", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func clearMySQLArchiveTables(t *testing.T, store *SQLiteStore) {
	t.Helper()
	for _, table := range []string{
		"archive_sqlite_import_receipt", "archive_clip_retention", "bridge_mp4_clips", "smd_ivs_events", "nvr_recording_chunks", "archive_event_scan_progress",
	} {
		if _, err := store.db.ExecContext(t.Context(), `DELETE FROM `+quoteMySQLIdentifier(table)); err != nil {
			t.Fatalf("clear mysql table %s: %v", table, err)
		}
	}
}
