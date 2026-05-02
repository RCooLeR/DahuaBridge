package archive

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) InitSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS archive_files (
			file_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			channel INTEGER NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			file_path TEXT NOT NULL,
			video_stream TEXT NOT NULL,
			disk INTEGER NOT NULL DEFAULT 0,
			partition INTEGER NOT NULL DEFAULT 0,
			cluster INTEGER NOT NULL DEFAULT 0,
			length_bytes INTEGER NOT NULL DEFAULT 0,
			cut_length_bytes INTEGER NOT NULL DEFAULT 0,
			flags_json TEXT NOT NULL DEFAULT '[]',
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_archive_files_device_channel_time
			ON archive_files(device_id, channel, start_time, end_time)`,
		`CREATE INDEX IF NOT EXISTS idx_archive_files_path
			ON archive_files(device_id, file_path)`,
		`CREATE TABLE IF NOT EXISTS archive_events (
			event_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			channel INTEGER NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			file_path TEXT NOT NULL,
			source TEXT NOT NULL,
			type TEXT NOT NULL,
			video_stream TEXT NOT NULL,
			flags_json TEXT NOT NULL DEFAULT '[]',
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_archive_events_device_channel_time
			ON archive_events(device_id, channel, start_time, end_time)`,
		`CREATE INDEX IF NOT EXISTS idx_archive_events_type
			ON archive_events(device_id, type, start_time)`,
		`CREATE TABLE IF NOT EXISTS archive_event_files (
			event_id TEXT NOT NULL,
			file_id TEXT NOT NULL,
			linked_at TEXT NOT NULL,
			PRIMARY KEY(event_id, file_id)
		)`,
		`CREATE TABLE IF NOT EXISTS archive_sync_coverage (
			scope TEXT NOT NULL,
			device_id TEXT NOT NULL,
			channel INTEGER NOT NULL,
			window_start TEXT NOT NULL,
			window_end TEXT NOT NULL,
			synced_at TEXT NOT NULL,
			PRIMARY KEY(scope, device_id, channel, window_start, window_end)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_archive_sync_coverage_lookup
			ON archive_sync_coverage(scope, device_id, channel, window_start, window_end)`,
		`CREATE TABLE IF NOT EXISTS transcode_jobs (
			job_id TEXT PRIMARY KEY,
			record_kind TEXT NOT NULL,
			record_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			status TEXT NOT NULL,
			source_file_path TEXT NOT NULL DEFAULT '',
			output_path TEXT NOT NULL DEFAULT '',
			error_text TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_transcode_jobs_record
			ON transcode_jobs(record_kind, record_id, updated_at)`,
		`CREATE TABLE IF NOT EXISTS transcoded_assets (
			asset_id TEXT PRIMARY KEY,
			record_kind TEXT NOT NULL,
			record_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			format TEXT NOT NULL,
			status TEXT NOT NULL,
			path TEXT NOT NULL DEFAULT '',
			size_bytes INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			ready_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_transcoded_assets_record
			ON transcoded_assets(record_kind, record_id, updated_at)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) UpsertArchiveFiles(ctx context.Context, deviceID string, items []dahua.NVRRecording, seenAt time.Time) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	statement, err := tx.PrepareContext(ctx, `INSERT INTO archive_files (
		file_id, device_id, channel, start_time, end_time, file_path, video_stream, disk, partition, cluster,
		length_bytes, cut_length_bytes, flags_json, first_seen_at, last_seen_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(file_id) DO UPDATE SET
		video_stream=excluded.video_stream,
		disk=excluded.disk,
		partition=excluded.partition,
		cluster=excluded.cluster,
		length_bytes=excluded.length_bytes,
		cut_length_bytes=excluded.cut_length_bytes,
		flags_json=excluded.flags_json,
		last_seen_at=excluded.last_seen_at`)
	if err != nil {
		return err
	}
	defer statement.Close()

	for _, item := range items {
		row, ok := archiveFileRowFromRecording(deviceID, item, seenAt)
		if !ok {
			continue
		}
		if _, err = statement.ExecContext(ctx,
			row.FileID,
			row.DeviceID,
			row.Channel,
			row.StartTime,
			row.EndTime,
			row.FilePath,
			row.VideoStream,
			row.Disk,
			row.Partition,
			row.Cluster,
			row.LengthBytes,
			row.CutLengthBytes,
			row.FlagsJSON,
			row.FirstSeenAt,
			row.LastSeenAt,
		); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (s *SQLiteStore) UpsertArchiveEvents(ctx context.Context, deviceID string, items []dahua.NVRRecording, seenAt time.Time) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	eventStatement, err := tx.PrepareContext(ctx, `INSERT INTO archive_events (
		event_id, device_id, channel, start_time, end_time, file_path, source, type, video_stream, flags_json, first_seen_at, last_seen_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(event_id) DO UPDATE SET
		file_path=excluded.file_path,
		source=excluded.source,
		type=excluded.type,
		video_stream=excluded.video_stream,
		flags_json=excluded.flags_json,
		last_seen_at=excluded.last_seen_at`)
	if err != nil {
		return err
	}
	defer eventStatement.Close()

	fileStatement, err := tx.PrepareContext(ctx, `INSERT INTO archive_files (
		file_id, device_id, channel, start_time, end_time, file_path, video_stream, disk, partition, cluster,
		length_bytes, cut_length_bytes, flags_json, first_seen_at, last_seen_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(file_id) DO UPDATE SET
		video_stream=excluded.video_stream,
		disk=excluded.disk,
		partition=excluded.partition,
		cluster=excluded.cluster,
		length_bytes=excluded.length_bytes,
		cut_length_bytes=excluded.cut_length_bytes,
		flags_json=excluded.flags_json,
		last_seen_at=excluded.last_seen_at`)
	if err != nil {
		return err
	}
	defer fileStatement.Close()

	linkStatement, err := tx.PrepareContext(ctx, `INSERT INTO archive_event_files (event_id, file_id, linked_at)
		VALUES (?, ?, ?)
		ON CONFLICT(event_id, file_id) DO UPDATE SET linked_at=excluded.linked_at`)
	if err != nil {
		return err
	}
	defer linkStatement.Close()

	for _, item := range items {
		eventRow, ok := archiveEventRowFromRecording(deviceID, item, seenAt)
		if !ok {
			continue
		}
		if _, err = eventStatement.ExecContext(ctx,
			eventRow.EventID,
			eventRow.DeviceID,
			eventRow.Channel,
			eventRow.StartTime,
			eventRow.EndTime,
			eventRow.FilePath,
			eventRow.Source,
			eventRow.Type,
			eventRow.VideoStream,
			eventRow.FlagsJSON,
			eventRow.FirstSeenAt,
			eventRow.LastSeenAt,
		); err != nil {
			return err
		}

		fileRow, ok := archiveFileRowFromRecording(deviceID, item, seenAt)
		if !ok {
			continue
		}
		if _, err = fileStatement.ExecContext(ctx,
			fileRow.FileID,
			fileRow.DeviceID,
			fileRow.Channel,
			fileRow.StartTime,
			fileRow.EndTime,
			fileRow.FilePath,
			fileRow.VideoStream,
			fileRow.Disk,
			fileRow.Partition,
			fileRow.Cluster,
			fileRow.LengthBytes,
			fileRow.CutLengthBytes,
			fileRow.FlagsJSON,
			fileRow.FirstSeenAt,
			fileRow.LastSeenAt,
		); err != nil {
			return err
		}
		if _, err = linkStatement.ExecContext(ctx, eventRow.EventID, fileRow.FileID, seenAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (s *SQLiteStore) PruneOlderThan(ctx context.Context, cutoff time.Time) (err error) {
	formatted := cutoff.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	statements := []string{
		`DELETE FROM archive_event_files WHERE event_id IN (SELECT event_id FROM archive_events WHERE last_seen_at < ?)
			OR file_id IN (SELECT file_id FROM archive_files WHERE last_seen_at < ?)`,
		`DELETE FROM archive_events WHERE last_seen_at < ?`,
		`DELETE FROM archive_files WHERE last_seen_at < ?`,
	}
	args := [][]any{
		{formatted, formatted},
		{formatted},
		{formatted},
	}
	for index, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement, args[index]...); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

type archiveFileRow struct {
	FileID         string
	DeviceID       string
	Channel        int
	StartTime      string
	EndTime        string
	FilePath       string
	VideoStream    string
	Disk           int
	Partition      int
	Cluster        int
	LengthBytes    int64
	CutLengthBytes int64
	FlagsJSON      string
	FirstSeenAt    string
	LastSeenAt     string
}

type archiveEventRow struct {
	EventID     string
	DeviceID    string
	Channel     int
	StartTime   string
	EndTime     string
	FilePath    string
	Source      string
	Type        string
	VideoStream string
	FlagsJSON   string
	FirstSeenAt string
	LastSeenAt  string
}

func archiveFileRowFromRecording(deviceID string, item dahua.NVRRecording, seenAt time.Time) (archiveFileRow, bool) {
	filePath := strings.TrimSpace(item.FilePath)
	startTime := strings.TrimSpace(item.StartTime)
	endTime := strings.TrimSpace(item.EndTime)
	if shouldTreatAsEvent(item) && filePath != "" {
		if fileStart, fileEnd, ok := dahua.ParseRecordingFileTimeRange(filePath, time.Local); ok {
			startTime = fileStart.In(time.Local).Format(archiveTimeLayout)
			endTime = fileEnd.In(time.Local).Format(archiveTimeLayout)
		}
	}
	if strings.TrimSpace(deviceID) == "" || item.Channel <= 0 || filePath == "" || startTime == "" || endTime == "" {
		return archiveFileRow{}, false
	}
	flagsJSON, err := json.Marshal(item.Flags)
	if err != nil {
		flagsJSON = []byte("[]")
	}
	return archiveFileRow{
		FileID:         archiveFileID(deviceID, item),
		DeviceID:       strings.TrimSpace(deviceID),
		Channel:        item.Channel,
		StartTime:      startTime,
		EndTime:        endTime,
		FilePath:       filePath,
		VideoStream:    strings.TrimSpace(item.VideoStream),
		Disk:           item.Disk,
		Partition:      item.Partition,
		Cluster:        item.Cluster,
		LengthBytes:    item.LengthBytes,
		CutLengthBytes: item.CutLengthBytes,
		FlagsJSON:      string(flagsJSON),
		FirstSeenAt:    seenAt.UTC().Format(time.RFC3339Nano),
		LastSeenAt:     seenAt.UTC().Format(time.RFC3339Nano),
	}, true
}

func archiveEventRowFromRecording(deviceID string, item dahua.NVRRecording, seenAt time.Time) (archiveEventRow, bool) {
	startTime := strings.TrimSpace(item.StartTime)
	endTime := strings.TrimSpace(item.EndTime)
	if strings.TrimSpace(deviceID) == "" || item.Channel <= 0 || startTime == "" || endTime == "" {
		return archiveEventRow{}, false
	}
	flagsJSON, err := json.Marshal(item.Flags)
	if err != nil {
		flagsJSON = []byte("[]")
	}
	return archiveEventRow{
		EventID:     archiveEventID(deviceID, item),
		DeviceID:    strings.TrimSpace(deviceID),
		Channel:     item.Channel,
		StartTime:   startTime,
		EndTime:     endTime,
		FilePath:    strings.TrimSpace(item.FilePath),
		Source:      strings.TrimSpace(item.Source),
		Type:        strings.TrimSpace(item.Type),
		VideoStream: strings.TrimSpace(item.VideoStream),
		FlagsJSON:   string(flagsJSON),
		FirstSeenAt: seenAt.UTC().Format(time.RFC3339Nano),
		LastSeenAt:  seenAt.UTC().Format(time.RFC3339Nano),
	}, true
}

func archiveFileID(deviceID string, item dahua.NVRRecording) string {
	return stableArchiveID("file", []string{
		strings.TrimSpace(deviceID),
		fmt.Sprintf("%d", item.Channel),
		strings.TrimSpace(item.StartTime),
		strings.TrimSpace(item.EndTime),
		strings.TrimSpace(item.FilePath),
	})
}

func archiveEventID(deviceID string, item dahua.NVRRecording) string {
	return stableArchiveID("event", []string{
		strings.TrimSpace(deviceID),
		fmt.Sprintf("%d", item.Channel),
		strings.TrimSpace(item.StartTime),
		strings.TrimSpace(item.EndTime),
		strings.TrimSpace(item.FilePath),
		strings.TrimSpace(item.Source),
		strings.TrimSpace(item.Type),
		strings.TrimSpace(item.VideoStream),
	})
}

func stableArchiveID(prefix string, values []string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "|")))
	return prefix + "_" + hex.EncodeToString(sum[:16])
}
