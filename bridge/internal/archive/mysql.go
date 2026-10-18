package archive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"

	mysql "github.com/go-sql-driver/mysql"
)

const mysqlArchiveSchemaVersion = 3

func openArchiveMySQLDB(ctx context.Context, cfg config.ArchiveConfig) (*sql.DB, error) {
	dsn, err := prepareArchiveMySQLConfig(cfg)
	if err != nil {
		return nil, err
	}
	connector, err := mysql.NewConnector(dsn)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	maxOpen := cfg.DBMaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 8
	}
	maxIdle := cfg.DBMaxIdleConns
	if maxIdle < 0 {
		maxIdle = 0
	}
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	if cfg.DBConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	}
	if cfg.DBConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)
	}
	connectTimeout := cfg.DBConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = time.Minute
	}
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	var lastErr error
	for delay := 250 * time.Millisecond; ; delay = min(delay*2, 5*time.Second) {
		if err := db.PingContext(pingCtx); err == nil {
			break
		} else {
			lastErr = err
			if !isTransientMySQLConnectError(err) {
				_ = db.Close()
				return nil, err
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-pingCtx.Done():
			timer.Stop()
			_ = db.Close()
			return nil, fmt.Errorf("connect to mysql within %s: %w (last connection error: %v)", connectTimeout, pingCtx.Err(), lastErr)
		case <-timer.C:
		}
	}
	return db, nil
}

func prepareArchiveMySQLConfig(cfg config.ArchiveConfig) (*mysql.Config, error) {
	dsn, err := archiveMySQLConfig(cfg)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(dsn.DBName) == "" {
		return nil, errors.New("mysql DSN must select a database")
	}
	// MySQL 9.x supports row aliases used by our upserts. Parameter
	// interpolation avoids an extra prepare round-trip per archive batch while
	// the driver still performs type-aware escaping.
	hardenArchiveMySQLConfig(dsn)
	if dsn.Collation == "" {
		// The protocol handshake only has one byte for the collation ID, so
		// MySQL 8+/9 collations such as utf8mb4_0900_bin cannot be selected
		// here (their IDs are greater than 255). Use the compatible binary
		// utf8mb4 handshake collation; the schema itself still uses
		// utf8mb4_0900_bin for stored identifiers.
		dsn.Collation = "utf8mb4_bin"
	}
	if dsn.Timeout <= 0 {
		dsn.Timeout = 5 * time.Second
	}
	if dsn.ReadTimeout <= 0 {
		dsn.ReadTimeout = 30 * time.Second
	}
	if dsn.WriteTimeout <= 0 {
		dsn.WriteTimeout = 30 * time.Second
	}
	if strings.TrimSpace(dsn.ConnectionAttributes) == "" {
		dsn.ConnectionAttributes = "program_name:dahuabridge"
	}
	return dsn, nil
}

func isTransientMySQLConnectError(err error) bool {
	if serverErr, ok := errors.AsType[*mysql.MySQLError](err); ok {
		switch serverErr.Number {
		case 1040, // Too many connections.
			1203: // Per-user connection limit reached.
			return true
		default:
			return false
		}
	}
	_, ok := errors.AsType[net.Error](err)
	return ok
}

func hardenArchiveMySQLConfig(dsn *mysql.Config) {
	dsn.InterpolateParams = true
	dsn.MultiStatements = false
	dsn.AllowAllFiles = false
	dsn.AllowCleartextPasswords = false
	dsn.AllowFallbackToPlaintext = false
	dsn.AllowOldPasswords = false
	dsn.CheckConnLiveness = true
	dsn.RejectReadOnly = true
}

func archiveMySQLConfig(cfg config.ArchiveConfig) (*mysql.Config, error) {
	tlsMode, err := normalizeArchiveMySQLTLSMode(cfg.DBTLS)
	if err != nil {
		return nil, err
	}
	var dsn *mysql.Config
	rawDSN := strings.TrimSpace(cfg.DBDSN)
	if rawDSN != "" {
		dsn, err = mysql.ParseDSN(rawDSN)
		if err != nil {
			return nil, fmt.Errorf("parse mysql DSN: %w", err)
		}
		// ParseDSN deliberately finds the database/query boundary after the last
		// slash, so it remains correct when a password contains '?' or '/'. Its
		// normalized TLSConfig also preserves an explicit tls=false. Do not parse
		// the raw DSN a second time: doing so can mistake password text for query
		// parameters and weaken or reject the requested transport policy.
		if strings.TrimSpace(dsn.TLSConfig) == "" {
			applyArchiveMySQLTLSMode(dsn, tlsMode)
		} else {
			dsnTLSMode, err := normalizeArchiveMySQLTLSMode(dsn.TLSConfig)
			if err != nil {
				return nil, fmt.Errorf("invalid tls parameter in archive.db_dsn: %w", err)
			}
			if dsnTLSMode != tlsMode {
				return nil, fmt.Errorf("archive.db_tls %q conflicts with the tls parameter in archive.db_dsn", tlsMode)
			}
			applyArchiveMySQLTLSMode(dsn, dsnTLSMode)
		}
	} else {
		if strings.TrimSpace(cfg.DBHost) == "" {
			return nil, errors.New("archive.db_host is required for mysql")
		}
		if strings.TrimSpace(cfg.DBName) == "" {
			return nil, errors.New("archive.db_name is required for mysql")
		}
		if strings.TrimSpace(cfg.DBUser) == "" {
			return nil, errors.New("archive.db_user is required for mysql")
		}
		if cfg.DBPassword == "" {
			return nil, errors.New("archive mysql password is required")
		}
		port := cfg.DBPort
		if port <= 0 {
			port = 3306
		}
		dsn = mysql.NewConfig()
		dsn.User = strings.TrimSpace(cfg.DBUser)
		dsn.Passwd = cfg.DBPassword
		dsn.Net = "tcp"
		dsn.Addr = net.JoinHostPort(strings.TrimSpace(cfg.DBHost), strconv.Itoa(port))
		dsn.DBName = strings.TrimSpace(cfg.DBName)
		applyArchiveMySQLTLSMode(dsn, tlsMode)
	}
	return dsn, nil
}

func normalizeArchiveMySQLTLSMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "true":
		return "true", nil
	case "false":
		return "false", nil
	case "skip-verify":
		return "skip-verify", nil
	case "preferred":
		return "", errors.New("archive.db_tls=preferred is unsafe because it permits plaintext fallback; use true, skip-verify, or false")
	default:
		return "", errors.New("archive.db_tls must be one of: false, true, skip-verify")
	}
}

func applyArchiveMySQLTLSMode(dsn *mysql.Config, mode string) {
	if mode == "false" {
		dsn.TLSConfig = ""
		return
	}
	dsn.TLSConfig = mode
}

// initMySQLSchema applies forward-only, idempotent schema migrations. MySQL
// auto-commits DDL, so a named lock serializes startup across bridge replicas
// and every migration is safe to resume after an interrupted deployment.
func (s *sqlStore) initMySQLSchema(ctx context.Context) error {
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
