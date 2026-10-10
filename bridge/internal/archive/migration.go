package archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
)

type MigrationStats struct {
	Rows           map[string]int64
	AlreadyApplied bool
}

type archiveMigrationValueKind byte

const (
	archiveMigrationText archiveMigrationValueKind = iota + 1
	archiveMigrationInteger
	archiveMigrationJSON
	archiveMigrationNormalizedText
)

type archiveMigrationColumn struct {
	source string
	target string
	kind   archiveMigrationValueKind
}

type archiveMigrationTable struct {
	name       string
	columns    []archiveMigrationColumn
	keyColumns []int
}

var archiveMigrationTables = []archiveMigrationTable{
	{
		name:       "archive_event_scan_progress",
		keyColumns: []int{0, 1, 2},
		columns: []archiveMigrationColumn{
			migrationTextColumn("device_id"), migrationIntegerColumn("channel"), migrationTextColumn("event_code"),
			migrationTextColumn("recent_end"), migrationTextColumn("backfill_before"),
		},
	},
	{
		name:       "smd_ivs_events",
		keyColumns: []int{0},
		columns: []archiveMigrationColumn{
			migrationTextColumn("event_id"), migrationTextColumn("device_id"), migrationIntegerColumn("channel"),
			migrationTextColumn("event_type"), migrationTextColumn("start_time"), migrationTextColumn("end_time"),
			migrationTextColumn("rtsp_main_url"), migrationTextColumn("rtsp_sub_url"), migrationTextColumn("mp4_clip_id"),
			migrationTextColumn("mp4_file_path"), migrationNormalizedTextColumn("mp4_status"), migrationTextColumn("mp4_error"),
			migrationTextColumn("source_file_path"), migrationTextColumn("video_stream"), migrationJSONColumn("flags_json"),
			migrationTextColumn("first_seen_at"), migrationTextColumn("last_seen_at"),
		},
	},
	{
		name:       "nvr_recording_chunks",
		keyColumns: []int{0},
		columns: []archiveMigrationColumn{
			migrationTextColumn("chunk_id"), migrationTextColumn("device_id"), migrationIntegerColumn("channel"),
			migrationTextColumn("start_time"), migrationTextColumn("end_time"), migrationTextColumn("file_path"),
			migrationTextColumn("video_stream"), migrationIntegerColumn("disk"),
			migrationRenamedIntegerColumn("partition", "partition_number"), migrationIntegerColumn("cluster"),
			migrationIntegerColumn("length_bytes"), migrationIntegerColumn("cut_length_bytes"), migrationJSONColumn("flags_json"),
			migrationTextColumn("first_seen_at"), migrationTextColumn("last_seen_at"),
		},
	},
	{
		name:       "bridge_mp4_clips",
		keyColumns: []int{0},
		columns: []archiveMigrationColumn{
			migrationTextColumn("clip_id"), migrationTextColumn("device_id"), migrationIntegerColumn("channel"),
			migrationTextColumn("stream_id"), migrationTextColumn("start_time"), migrationTextColumn("end_time"),
			migrationTextColumn("file_path"), migrationTextColumn("status"), migrationTextColumn("error_text"),
			migrationTextColumn("created_at"), migrationTextColumn("updated_at"),
		},
	},
	{
		name:       "archive_clip_retention",
		keyColumns: []int{0},
		columns: []archiveMigrationColumn{
			migrationTextColumn("clip_id"), migrationTextColumn("retain_from"), migrationIntegerColumn("queued"),
			migrationTextColumn("last_attempt_at"),
		},
	},
}

func migrationTextColumn(name string) archiveMigrationColumn {
	return archiveMigrationColumn{source: name, target: name, kind: archiveMigrationText}
}

func migrationIntegerColumn(name string) archiveMigrationColumn {
	return archiveMigrationColumn{source: name, target: name, kind: archiveMigrationInteger}
}

func migrationRenamedIntegerColumn(source, target string) archiveMigrationColumn {
	return archiveMigrationColumn{source: source, target: target, kind: archiveMigrationInteger}
}

func migrationJSONColumn(name string) archiveMigrationColumn {
	return archiveMigrationColumn{source: name, target: name, kind: archiveMigrationJSON}
}

func migrationNormalizedTextColumn(name string) archiveMigrationColumn {
	return archiveMigrationColumn{source: name, target: name, kind: archiveMigrationNormalizedText}
}

type archiveMigrationSnapshot struct {
	schemaVersion int
	fingerprint   string
	rows          map[string]int64
	digests       map[string]string
}

// MigrateSQLiteToMySQL copies a consistent SQLite read transaction into an
// initialized MySQL archive schema. The bridge must be stopped while this runs;
// a matching database receipt makes reruns read-only, while a changed source or
// a nonempty target without a receipt is rejected.
func MigrateSQLiteToMySQL(ctx context.Context, sourcePath string, cfg config.ArchiveConfig) (MigrationStats, error) {
	stats := MigrationStats{Rows: make(map[string]int64, len(archiveMigrationTables))}
	source, err := openArchiveSQLiteReadOnly(sourcePath)
	if err != nil {
		return stats, err
	}
	defer source.Close()
	if err := configureArchiveSQLiteDB(ctx, source); err != nil {
		return stats, fmt.Errorf("validate source sqlite database (stop all writers; preserve any WAL/SHM sidecars and provide a complete readable pair or allow SQLite write access to their directory for lock bookkeeping): %w", err)
	}
	var sourceVersion int
	if err := source.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&sourceVersion); err != nil {
		return stats, fmt.Errorf("read source sqlite schema version: %w", err)
	}
	if sourceVersion != sqliteArchiveSchemaVersion {
		return stats, fmt.Errorf("source sqlite schema version is %d; expected %d", sourceVersion, sqliteArchiveSchemaVersion)
	}

	target, err := openArchiveMySQLDB(ctx, cfg)
	if err != nil {
		return stats, fmt.Errorf("open migration target mysql db: %w", err)
	}
	defer target.Close()
	if err := newMySQLStore(target).InitSchema(ctx); err != nil {
		return stats, fmt.Errorf("initialize migration target mysql schema: %w", err)
	}

	sourceTx, err := source.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return stats, fmt.Errorf("begin sqlite migration snapshot: %w", err)
	}
	defer sourceTx.Rollback()
	snapshot, err := inspectArchiveMigrationSnapshot(ctx, sourceTx, sourceVersion, false)
	if err != nil {
		return stats, fmt.Errorf("fingerprint sqlite migration source: %w", err)
	}
	stats.Rows = snapshot.rows

	targetConn, err := target.Conn(ctx)
	if err != nil {
		return stats, fmt.Errorf("reserve mysql migration connection: %w", err)
	}
	defer targetConn.Close()
	var migrationLocked int
	if err := targetConn.QueryRowContext(ctx, `SELECT GET_LOCK('dahuabridge_archive_sqlite_import', 30)`).Scan(&migrationLocked); err != nil {
		return stats, fmt.Errorf("acquire mysql archive import lock: %w", err)
	}
	if migrationLocked != 1 {
		return stats, errors.New("acquire mysql archive import lock: timed out")
	}
	defer releaseArchiveMigrationLock(targetConn)

	// Serializable empty-table reads hold next-key locks until the receipt and
	// imported rows commit, preventing an already-running bridge from racing the
	// one-time bootstrap between validation and verification.
	targetTx, err := targetConn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return stats, fmt.Errorf("begin mysql archive import: %w", err)
	}
	defer targetTx.Rollback()

	receipt, receiptFound, err := readArchiveMigrationReceipt(ctx, targetTx)
	if err != nil {
		return stats, err
	}
	if receiptFound {
		if receipt.sourceSchemaVersion != snapshot.schemaVersion || receipt.sourceFingerprint != snapshot.fingerprint {
			return stats, errors.New("mysql archive has a receipt for a different SQLite snapshot; refusing to overwrite newer target data")
		}
		if err := receipt.validate(snapshot); err != nil {
			return stats, fmt.Errorf("validate mysql archive migration receipt: %w", err)
		}
		stats.AlreadyApplied = true
		if err := targetTx.Commit(); err != nil {
			return stats, fmt.Errorf("finish matching mysql archive migration receipt: %w", err)
		}
		if err := sourceTx.Commit(); err != nil {
			return stats, fmt.Errorf("finish sqlite migration snapshot: %w", err)
		}
		return stats, nil
	}

	if err := requireEmptyArchiveMigrationTarget(ctx, targetTx); err != nil {
		return stats, err
	}
	for _, table := range archiveMigrationTables {
		copied, err := copyArchiveMigrationTable(ctx, sourceTx, targetTx, table)
		if err != nil {
			return stats, err
		}
		if copied != snapshot.rows[table.name] {
			return stats, fmt.Errorf("copy sqlite table %s: copied %d rows, expected %d", table.name, copied, snapshot.rows[table.name])
		}
	}
	for _, table := range archiveMigrationTables {
		targetCount, targetDigest, err := archiveTableFullDigest(ctx, targetTx, table, true)
		if err != nil {
			return stats, fmt.Errorf("verify mysql table %s: %w", table.name, err)
		}
		if targetCount != snapshot.rows[table.name] {
			return stats, fmt.Errorf("verify mysql table %s: got %d rows, expected %d", table.name, targetCount, snapshot.rows[table.name])
		}
		if hex.EncodeToString(targetDigest[:]) != snapshot.digests[table.name] {
			return stats, fmt.Errorf("verify mysql table %s: full-row digest mismatch", table.name)
		}
	}
	if err := writeArchiveMigrationReceipt(ctx, targetTx, snapshot); err != nil {
		return stats, err
	}
	if err := targetTx.Commit(); err != nil {
		return stats, fmt.Errorf("commit mysql archive import: %w", err)
	}
	if err := sourceTx.Commit(); err != nil {
		return stats, fmt.Errorf("finish sqlite migration snapshot: %w", err)
	}
	return stats, nil
}

type archiveMigrationReceipt struct {
	sourceFingerprint   string
	sourceSchemaVersion int
	rowsJSON            string
	digestsJSON         string
}

func readArchiveMigrationReceipt(ctx context.Context, tx *sql.Tx) (archiveMigrationReceipt, bool, error) {
	var receipt archiveMigrationReceipt
	err := tx.QueryRowContext(ctx, `SELECT source_fingerprint, source_schema_version, source_rows_json, source_digests_json
		FROM archive_sqlite_import_receipt WHERE receipt_id = 1 FOR UPDATE`).Scan(
		&receipt.sourceFingerprint,
		&receipt.sourceSchemaVersion,
		&receipt.rowsJSON,
		&receipt.digestsJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return archiveMigrationReceipt{}, false, nil
	}
	if err != nil {
		return archiveMigrationReceipt{}, false, fmt.Errorf("read mysql archive migration receipt: %w", err)
	}
	return receipt, true, nil
}

func (r archiveMigrationReceipt) validate(snapshot archiveMigrationSnapshot) error {
	var rows map[string]int64
	if err := json.Unmarshal([]byte(r.rowsJSON), &rows); err != nil {
		return fmt.Errorf("decode receipt row counts: %w", err)
	}
	var digests map[string]string
	if err := json.Unmarshal([]byte(r.digestsJSON), &digests); err != nil {
		return fmt.Errorf("decode receipt table digests: %w", err)
	}
	if !equalInt64Maps(rows, snapshot.rows) || !equalStringMaps(digests, snapshot.digests) {
		return errors.New("receipt metadata does not match its source fingerprint")
	}
	return nil
}

func writeArchiveMigrationReceipt(ctx context.Context, tx *sql.Tx, snapshot archiveMigrationSnapshot) error {
	rowsJSON, err := json.Marshal(snapshot.rows)
	if err != nil {
		return fmt.Errorf("encode migration receipt row counts: %w", err)
	}
	digestsJSON, err := json.Marshal(snapshot.digests)
	if err != nil {
		return fmt.Errorf("encode migration receipt table digests: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO archive_sqlite_import_receipt (
		receipt_id, source_fingerprint, source_schema_version, source_rows_json, source_digests_json, imported_at
	) VALUES (1, ?, ?, ?, ?, ?)`,
		snapshot.fingerprint,
		snapshot.schemaVersion,
		string(rowsJSON),
		string(digestsJSON),
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record mysql archive migration receipt: %w", err)
	}
	return nil
}

func requireEmptyArchiveMigrationTarget(ctx context.Context, tx *sql.Tx) error {
	for _, table := range archiveMigrationTables {
		var count int64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+quoteMySQLIdentifier(table.name)).Scan(&count); err != nil {
			return fmt.Errorf("check mysql migration target table %s: %w", table.name, err)
		}
		if count != 0 {
			return fmt.Errorf("mysql migration target must be empty before its first import: table %s contains %d rows", table.name, count)
		}
	}
	return nil
}

func releaseArchiveMigrationLock(conn *sql.Conn) {
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var released sql.NullInt64
	_ = conn.QueryRowContext(releaseCtx, `SELECT RELEASE_LOCK('dahuabridge_archive_sqlite_import')`).Scan(&released)
}

func openArchiveSQLiteReadOnly(sourcePath string) (*sql.DB, error) {
	absPath, err := filepath.Abs(strings.TrimSpace(sourcePath))
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite source path: %w", err)
	}
	if info, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("stat sqlite source: %w", err)
	} else if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sqlite source %q is not a regular file", absPath)
	}
	dsn, err := archiveSQLiteReadOnlyDSN(absPath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite source: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func archiveSQLiteReadOnlyDSN(absPath string) (string, error) {
	uriPath := filepath.ToSlash(absPath)
	if volume := filepath.VolumeName(absPath); volume != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := &url.URL{Scheme: "file", Path: uriPath}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Set("_busy_timeout", "5000")
	query.Set("_query_only", "1")
	uri.RawQuery = query.Encode()

	// SQLite normally needs to create or update shared-memory state before it
	// can read a WAL database, even when every SQL statement is read-only. A
	// cleanly stopped database without journal sidecars is safe to open as an
	// immutable snapshot, which also permits a read-only filesystem mount. Do
	// not use immutable mode when any sidecar exists: doing so could silently
	// omit committed transactions that are present only in the WAL.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		sidecarPath := absPath + suffix
		if _, err := os.Lstat(sidecarPath); err == nil {
			return uri.String(), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat sqlite source sidecar %q: %w", sidecarPath, err)
		}
	}
	query.Set("immutable", "1")
	uri.RawQuery = query.Encode()
	return uri.String(), nil
}

func copyArchiveMigrationTable(ctx context.Context, source *sql.Tx, target *sql.Tx, table archiveMigrationTable) (int64, error) {
	selectColumns := make([]string, len(table.columns))
	for index, column := range table.columns {
		selectColumns[index] = quoteSQLiteIdentifier(column.source)
	}
	rows, err := source.QueryContext(ctx, `SELECT `+strings.Join(selectColumns, ",")+` FROM `+quoteSQLiteIdentifier(table.name))
	if err != nil {
		return 0, fmt.Errorf("read sqlite table %s: %w", table.name, err)
	}
	defer rows.Close()

	statement, err := target.PrepareContext(ctx, mysqlMigrationInsert(table))
	if err != nil {
		return 0, fmt.Errorf("prepare mysql import for %s: %w", table.name, err)
	}
	defer statement.Close()

	var count int64
	values := make([]any, len(table.columns))
	pointers := make([]any, len(values))
	for index := range values {
		pointers[index] = &values[index]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return 0, fmt.Errorf("scan sqlite table %s: %w", table.name, err)
		}
		for index, column := range table.columns {
			if column.kind != archiveMigrationNormalizedText || values[index] == nil {
				continue
			}
			raw, err := archiveMigrationTextBytes(values[index])
			if err != nil {
				return 0, fmt.Errorf("normalize sqlite table %s column %s: %w", table.name, column.source, err)
			}
			values[index] = strings.ToLower(strings.TrimSpace(string(raw)))
		}
		if _, err := statement.ExecContext(ctx, values...); err != nil {
			return 0, fmt.Errorf("write mysql table %s row %d: %w", table.name, count+1, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read sqlite table %s: %w", table.name, err)
	}
	return count, nil
}

type archiveRowQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func inspectArchiveMigrationSnapshot(ctx context.Context, queryer archiveRowQueryer, schemaVersion int, mysqlTarget bool) (archiveMigrationSnapshot, error) {
	snapshot := archiveMigrationSnapshot{
		schemaVersion: schemaVersion,
		rows:          make(map[string]int64, len(archiveMigrationTables)),
		digests:       make(map[string]string, len(archiveMigrationTables)),
	}
	for _, table := range archiveMigrationTables {
		count, digest, err := archiveTableFullDigest(ctx, queryer, table, mysqlTarget)
		if err != nil {
			return archiveMigrationSnapshot{}, fmt.Errorf("fingerprint table %s: %w", table.name, err)
		}
		snapshot.rows[table.name] = count
		snapshot.digests[table.name] = hex.EncodeToString(digest[:])
	}
	fingerprint, err := archiveMigrationSnapshotFingerprint(snapshot)
	if err != nil {
		return archiveMigrationSnapshot{}, err
	}
	snapshot.fingerprint = fingerprint
	return snapshot, nil
}

func archiveTableFullDigest(ctx context.Context, queryer archiveRowQueryer, table archiveMigrationTable, mysqlTarget bool) (int64, [sha256.Size]byte, error) {
	quotedColumns := make([]string, len(table.columns))
	for index, column := range table.columns {
		if mysqlTarget {
			quotedColumns[index] = quoteMySQLIdentifier(column.target)
		} else {
			quotedColumns[index] = quoteSQLiteIdentifier(column.source)
		}
	}
	quote := quoteSQLiteIdentifier
	if mysqlTarget {
		quote = quoteMySQLIdentifier
	}
	if len(table.keyColumns) == 0 {
		return 0, [sha256.Size]byte{}, errors.New("migration table has no primary-key ordering metadata")
	}
	orderColumns := make([]string, len(table.keyColumns))
	for index, columnIndex := range table.keyColumns {
		if columnIndex < 0 || columnIndex >= len(quotedColumns) {
			return 0, [sha256.Size]byte{}, fmt.Errorf("primary-key column index %d is out of range", columnIndex)
		}
		orderColumns[index] = quotedColumns[columnIndex]
	}
	query := `SELECT ` + strings.Join(quotedColumns, ",") + ` FROM ` + quote(table.name) +
		` ORDER BY ` + strings.Join(orderColumns, ",")
	rows, err := queryer.QueryContext(ctx, query)
	if err != nil {
		return 0, [sha256.Size]byte{}, err
	}
	defer rows.Close()
	hash := sha256.New()
	var length [8]byte
	var count int64
	values := make([]any, len(table.columns))
	pointers := make([]any, len(values))
	for index := range values {
		pointers[index] = &values[index]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return 0, [sha256.Size]byte{}, err
		}
		record := make([]byte, 0, len(values)*24)
		for index, value := range values {
			var err error
			record, err = appendArchiveMigrationValue(record, table.columns[index].kind, value)
			if err != nil {
				return 0, [sha256.Size]byte{}, fmt.Errorf("column %s: %w", table.columns[index].source, err)
			}
		}
		binary.BigEndian.PutUint64(length[:], uint64(len(record)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(record)
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, [sha256.Size]byte{}, err
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return count, result, nil
}

func appendArchiveMigrationValue(record []byte, kind archiveMigrationValueKind, value any) ([]byte, error) {
	record = append(record, byte(kind))
	if value == nil {
		return append(record, 0), nil
	}
	record = append(record, 1)
	var payload []byte
	switch kind {
	case archiveMigrationText, archiveMigrationNormalizedText:
		var err error
		payload, err = archiveMigrationTextBytes(value)
		if err != nil {
			return nil, err
		}
		if kind == archiveMigrationNormalizedText {
			payload = []byte(strings.ToLower(strings.TrimSpace(string(payload))))
		}
	case archiveMigrationInteger:
		integer, err := archiveMigrationInt64(value)
		if err != nil {
			return nil, err
		}
		payload = make([]byte, 8)
		binary.BigEndian.PutUint64(payload, uint64(integer))
	case archiveMigrationJSON:
		raw, err := archiveMigrationTextBytes(value)
		if err != nil {
			return nil, err
		}
		if !json.Valid(raw) {
			return nil, errors.New("invalid JSON")
		}
		var decoded []string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("decode JSON string array: %w", err)
		}
		payload, err = json.Marshal(decoded)
		if err != nil {
			return nil, fmt.Errorf("canonicalize JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported migration value kind %d", kind)
	}
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(payload)))
	record = append(record, length[:]...)
	record = append(record, payload...)
	return record, nil
}

func archiveMigrationTextBytes(value any) ([]byte, error) {
	switch typed := value.(type) {
	case string:
		return []byte(typed), nil
	case []byte:
		return bytes.Clone(typed), nil
	default:
		return nil, fmt.Errorf("expected text, got %T", value)
	}
}

func archiveMigrationInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		if uint64(typed) > uint64(1<<63-1) {
			return 0, fmt.Errorf("integer %d overflows int64", typed)
		}
		return int64(typed), nil
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		if typed > uint64(1<<63-1) {
			return 0, fmt.Errorf("integer %d overflows int64", typed)
		}
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("expected integer, got %T", value)
	}
}

func archiveMigrationSnapshotFingerprint(snapshot archiveMigrationSnapshot) (string, error) {
	payload := make([]byte, 0, 512)
	payload = appendLengthPrefixedMigrationBytes(payload, []byte("dahuabridge-sqlite-archive-import-v2"))
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], uint64(snapshot.schemaVersion))
	payload = append(payload, number[:]...)
	for _, table := range archiveMigrationTables {
		payload = appendLengthPrefixedMigrationBytes(payload, []byte(table.name))
		for _, column := range table.columns {
			payload = appendLengthPrefixedMigrationBytes(payload, []byte(column.source))
			payload = appendLengthPrefixedMigrationBytes(payload, []byte(column.target))
			payload = append(payload, byte(column.kind))
		}
		binary.BigEndian.PutUint64(number[:], uint64(len(table.keyColumns)))
		payload = append(payload, number[:]...)
		for _, columnIndex := range table.keyColumns {
			if columnIndex < 0 || columnIndex >= len(table.columns) {
				return "", fmt.Errorf("primary-key column index %d is out of range for %s", columnIndex, table.name)
			}
			binary.BigEndian.PutUint64(number[:], uint64(columnIndex))
			payload = append(payload, number[:]...)
		}
		binary.BigEndian.PutUint64(number[:], uint64(snapshot.rows[table.name]))
		payload = append(payload, number[:]...)
		digest, err := hex.DecodeString(snapshot.digests[table.name])
		if err != nil || len(digest) != sha256.Size {
			return "", fmt.Errorf("decode table digest for %s", table.name)
		}
		payload = append(payload, digest...)
	}
	result := sha256.Sum256(payload)
	return hex.EncodeToString(result[:]), nil
}

func appendLengthPrefixedMigrationBytes(target []byte, value []byte) []byte {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	target = append(target, length[:]...)
	return append(target, value...)
}

func mysqlMigrationInsert(table archiveMigrationTable) string {
	columns := make([]string, len(table.columns))
	placeholders := make([]string, len(table.columns))
	for index, column := range table.columns {
		columns[index] = quoteMySQLIdentifier(column.target)
		placeholders[index] = "?"
	}
	return `INSERT INTO ` + quoteMySQLIdentifier(table.name) + ` (` + strings.Join(columns, ",") + `) VALUES (` + strings.Join(placeholders, ",") + `)`
}

func quoteSQLiteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quoteMySQLIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func equalInt64Maps(left, right map[string]int64) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func equalStringMaps(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
