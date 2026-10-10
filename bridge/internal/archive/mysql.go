package archive

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const mysqlArchiveSchemaVersion = 3

// initMySQLSchema applies forward-only, idempotent schema migrations. MySQL
// auto-commits DDL, so a named lock serializes startup across bridge replicas
// and every migration is safe to resume after an interrupted deployment.
func (s *SQLiteStore) initMySQLSchema(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var locked int
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('dahuabridge_archive_schema', 30)`).Scan(&locked); err != nil {
		return fmt.Errorf("acquire archive schema migration lock: %w", err)
	}
	if locked != 1 {
		return fmt.Errorf("acquire archive schema migration lock: timed out")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released sql.NullInt64
		_ = conn.QueryRowContext(releaseCtx, `SELECT RELEASE_LOCK('dahuabridge_archive_schema')`).Scan(&released)
	}()

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS archive_schema_migrations (
		version INT NOT NULL PRIMARY KEY,
		applied_at VARCHAR(35) NOT NULL
	) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin ROW_FORMAT=DYNAMIC`); err != nil {
		return fmt.Errorf("create archive schema migration table: %w", err)
	}

	var currentVersion int
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM archive_schema_migrations`).Scan(&currentVersion); err != nil {
		return fmt.Errorf("read archive schema version: %w", err)
	}
	if currentVersion > mysqlArchiveSchemaVersion {
		return fmt.Errorf("archive mysql schema version %d is newer than supported version %d", currentVersion, mysqlArchiveSchemaVersion)
	}
	if currentVersion < 1 {
		if err := applyMySQLArchiveSchemaV1(ctx, conn); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO archive_schema_migrations(version, applied_at) VALUES (?, ?)`,
			1, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record archive mysql schema version 1: %w", err)
		}
		currentVersion = 1
	}
	if currentVersion < 2 {
		if err := applyMySQLArchiveSchemaV2(ctx, conn); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO archive_schema_migrations(version, applied_at) VALUES (?, ?)`,
			2, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record archive mysql schema version 2: %w", err)
		}
		currentVersion = 2
	}
	if currentVersion < 3 {
		if err := applyMySQLArchiveSchemaV3(ctx, conn); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO archive_schema_migrations(version, applied_at) VALUES (?, ?)`,
			3, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record archive mysql schema version 3: %w", err)
		}
	}
	return nil
}

func applyMySQLArchiveSchemaV2(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS archive_sqlite_import_receipt (
		receipt_id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
		source_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		source_schema_version INT UNSIGNED NOT NULL,
		source_rows_json JSON NOT NULL,
		source_digests_json JSON NOT NULL,
		imported_at VARCHAR(35) NOT NULL,
		CONSTRAINT chk_archive_sqlite_import_receipt_id CHECK (receipt_id = 1)
	) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin ROW_FORMAT=DYNAMIC`); err != nil {
		return fmt.Errorf("apply archive mysql schema v2: %w", err)
	}
	return nil
}

func applyMySQLArchiveSchemaV3(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, `UPDATE smd_ivs_events SET mp4_status = LOWER(TRIM(mp4_status))
		WHERE mp4_status <> LOWER(TRIM(mp4_status))`); err != nil {
		return fmt.Errorf("apply archive mysql schema v3 status normalization: %w", err)
	}
	indexes := []struct {
		table   string
		name    string
		columns []string
	}{
		{table: "smd_ivs_events", name: "idx_smd_ivs_events_last_seen", columns: []string{"last_seen_at"}},
		{table: "smd_ivs_events", name: "idx_smd_ivs_events_status", columns: []string{"mp4_status"}},
		{table: "smd_ivs_events", name: "idx_smd_ivs_events_pending_export", columns: []string{"start_time", "end_time", "mp4_status", "mp4_clip_id"}},
		{table: "nvr_recording_chunks", name: "idx_nvr_recording_chunks_last_seen", columns: []string{"last_seen_at"}},
	}
	for _, index := range indexes {
		if err := ensureMySQLArchiveIndex(ctx, conn, index.table, index.name, index.columns); err != nil {
			return err
		}
	}
	return nil
}

func ensureMySQLArchiveIndex(ctx context.Context, conn *sql.Conn, table, name string, columns []string) error {
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, table, name).Scan(&count); err != nil {
		return fmt.Errorf("inspect archive mysql index %s: %w", name, err)
	}
	if count != 0 {
		return nil
	}
	quotedColumns := make([]string, len(columns))
	for index, column := range columns {
		quotedColumns[index] = quoteMySQLIdentifier(column)
	}
	statement := `CREATE INDEX ` + quoteMySQLIdentifier(name) + ` ON ` + quoteMySQLIdentifier(table) + ` (` + strings.Join(quotedColumns, ",") + `)`
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("create archive mysql index %s: %w", name, err)
	}
	return nil
}

func applyMySQLArchiveSchemaV1(ctx context.Context, conn *sql.Conn) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS archive_event_scan_progress (
			device_id VARCHAR(191) NOT NULL,
			channel INT NOT NULL,
			event_code VARCHAR(64) NOT NULL,
			recent_end VARCHAR(35) NOT NULL,
			backfill_before VARCHAR(35) NOT NULL,
			PRIMARY KEY(device_id, channel, event_code)
		) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin ROW_FORMAT=DYNAMIC`,
		`CREATE TABLE IF NOT EXISTS smd_ivs_events (
			event_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
			device_id VARCHAR(191) NOT NULL,
			channel INT NOT NULL,
			event_type VARCHAR(191) NOT NULL,
			start_time VARCHAR(35) NOT NULL,
			end_time VARCHAR(35) NOT NULL,
			rtsp_main_url TEXT NOT NULL DEFAULT (''),
			rtsp_sub_url TEXT NOT NULL DEFAULT (''),
			mp4_clip_id VARCHAR(191) NOT NULL DEFAULT '',
			mp4_file_path TEXT NOT NULL DEFAULT (''),
			mp4_status VARCHAR(32) NOT NULL DEFAULT '',
			mp4_error TEXT NOT NULL DEFAULT (''),
			source_file_path TEXT NOT NULL DEFAULT (''),
			video_stream VARCHAR(191) NOT NULL DEFAULT '',
			flags_json JSON NOT NULL DEFAULT (JSON_ARRAY()),
			first_seen_at VARCHAR(35) NOT NULL,
			last_seen_at VARCHAR(35) NOT NULL,
			KEY idx_smd_ivs_events_device_channel_time(device_id, channel, start_time, end_time),
			KEY idx_smd_ivs_events_type(device_id, event_type, start_time),
			KEY idx_smd_ivs_events_mp4_clip(mp4_clip_id)
		) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin ROW_FORMAT=DYNAMIC`,
		`CREATE TABLE IF NOT EXISTS nvr_recording_chunks (
			chunk_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
			device_id VARCHAR(191) NOT NULL,
			channel INT NOT NULL,
			start_time VARCHAR(35) NOT NULL,
			end_time VARCHAR(35) NOT NULL,
			file_path TEXT NOT NULL,
			video_stream VARCHAR(191) NOT NULL DEFAULT '',
			disk INT NOT NULL DEFAULT 0,
			partition_number INT NOT NULL DEFAULT 0,
			cluster INT NOT NULL DEFAULT 0,
			length_bytes BIGINT NOT NULL DEFAULT 0,
			cut_length_bytes BIGINT NOT NULL DEFAULT 0,
			flags_json JSON NOT NULL DEFAULT (JSON_ARRAY()),
			first_seen_at VARCHAR(35) NOT NULL,
			last_seen_at VARCHAR(35) NOT NULL,
			KEY idx_nvr_recording_chunks_device_channel_time(device_id, channel, start_time, end_time)
		) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin ROW_FORMAT=DYNAMIC`,
		`CREATE TABLE IF NOT EXISTS bridge_mp4_clips (
			clip_id VARCHAR(191) NOT NULL PRIMARY KEY,
			device_id VARCHAR(191) NOT NULL,
			channel INT NOT NULL DEFAULT 0,
			stream_id VARCHAR(255) NOT NULL DEFAULT '',
			start_time VARCHAR(35) NOT NULL DEFAULT '',
			end_time VARCHAR(35) NOT NULL DEFAULT '',
			file_path TEXT NOT NULL DEFAULT (''),
			status VARCHAR(32) NOT NULL,
			error_text TEXT NOT NULL DEFAULT (''),
			created_at VARCHAR(35) NOT NULL,
			updated_at VARCHAR(35) NOT NULL,
			KEY idx_bridge_mp4_clips_device_channel_time(device_id, channel, start_time, end_time)
		) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin ROW_FORMAT=DYNAMIC`,
		`CREATE TABLE IF NOT EXISTS archive_clip_retention (
			clip_id VARCHAR(191) NOT NULL PRIMARY KEY,
			retain_from VARCHAR(35) NOT NULL,
			queued TINYINT UNSIGNED NOT NULL DEFAULT 0,
			last_attempt_at VARCHAR(35) NOT NULL DEFAULT '',
			KEY idx_archive_clip_retention_queue(queued, last_attempt_at, retain_from)
		) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin ROW_FORMAT=DYNAMIC`,
		`INSERT INTO archive_clip_retention(clip_id, retain_from)
			SELECT clip_id, COALESCE(NULLIF(end_time, ''), NULLIF(start_time, ''), created_at)
			FROM bridge_mp4_clips WHERE LEFT(stream_id, 11) = 'nvr_export_'
			ON DUPLICATE KEY UPDATE clip_id = archive_clip_retention.clip_id`,
	}
	for index, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply archive mysql schema v1 statement %d: %w", index+1, err)
		}
	}
	return nil
}
