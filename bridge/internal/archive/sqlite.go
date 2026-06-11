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

const sqliteArchiveSchemaVersion = 2

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) InitSchema(ctx context.Context) error {
	var currentVersion int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&currentVersion); err != nil {
		return err
	}
	if currentVersion != sqliteArchiveSchemaVersion {
		if err := s.dropSchema(ctx); err != nil {
			return err
		}
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS smd_ivs_events (
			event_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			channel INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			rtsp_main_url TEXT NOT NULL DEFAULT '',
			rtsp_sub_url TEXT NOT NULL DEFAULT '',
			mp4_clip_id TEXT NOT NULL DEFAULT '',
			mp4_file_path TEXT NOT NULL DEFAULT '',
			mp4_status TEXT NOT NULL DEFAULT '',
			mp4_error TEXT NOT NULL DEFAULT '',
			source_file_path TEXT NOT NULL DEFAULT '',
			video_stream TEXT NOT NULL DEFAULT '',
			flags_json TEXT NOT NULL DEFAULT '[]',
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_smd_ivs_events_device_channel_time
			ON smd_ivs_events(device_id, channel, start_time, end_time)`,
		`CREATE INDEX IF NOT EXISTS idx_smd_ivs_events_type
			ON smd_ivs_events(device_id, event_type, start_time)`,
		`CREATE TABLE IF NOT EXISTS nvr_recording_chunks (
			chunk_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			channel INTEGER NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			file_path TEXT NOT NULL,
			video_stream TEXT NOT NULL DEFAULT '',
			disk INTEGER NOT NULL DEFAULT 0,
			partition INTEGER NOT NULL DEFAULT 0,
			cluster INTEGER NOT NULL DEFAULT 0,
			length_bytes INTEGER NOT NULL DEFAULT 0,
			cut_length_bytes INTEGER NOT NULL DEFAULT 0,
			flags_json TEXT NOT NULL DEFAULT '[]',
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nvr_recording_chunks_device_channel_time
			ON nvr_recording_chunks(device_id, channel, start_time, end_time)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_nvr_recording_chunks_path
			ON nvr_recording_chunks(device_id, file_path)`,
		`CREATE TABLE IF NOT EXISTS bridge_mp4_clips (
			clip_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			channel INTEGER NOT NULL DEFAULT 0,
			stream_id TEXT NOT NULL DEFAULT '',
			start_time TEXT NOT NULL DEFAULT '',
			end_time TEXT NOT NULL DEFAULT '',
			file_path TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			error_text TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bridge_mp4_clips_device_channel_time
			ON bridge_mp4_clips(device_id, channel, start_time, end_time)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, sqliteArchiveSchemaVersion)); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) dropSchema(ctx context.Context) error {
	for _, table := range []string{
		"archive_event_files",
		"archive_events",
		"archive_files",
		"archive_sync_coverage",
		"transcoded_assets",
		"transcode_jobs",
		"smd_ivs_events",
		"nvr_recording_chunks",
		"bridge_mp4_clips",
	} {
		if _, err := s.db.ExecContext(ctx, `DROP TABLE IF EXISTS `+table); err != nil {
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

	statement, err := tx.PrepareContext(ctx, `INSERT INTO nvr_recording_chunks (
		chunk_id, device_id, channel, start_time, end_time, file_path, video_stream, disk, partition, cluster,
		length_bytes, cut_length_bytes, flags_json, first_seen_at, last_seen_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(chunk_id) DO UPDATE SET
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

	eventStatement, err := tx.PrepareContext(ctx, `INSERT INTO smd_ivs_events (
		event_id, device_id, channel, event_type, start_time, end_time, rtsp_main_url, rtsp_sub_url,
		source_file_path, video_stream, flags_json, first_seen_at, last_seen_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(event_id) DO UPDATE SET
		event_type=excluded.event_type,
		video_stream=excluded.video_stream,
		rtsp_main_url=excluded.rtsp_main_url,
		rtsp_sub_url=excluded.rtsp_sub_url,
		source_file_path=excluded.source_file_path,
		flags_json=excluded.flags_json,
		last_seen_at=excluded.last_seen_at`)
	if err != nil {
		return err
	}
	defer eventStatement.Close()

	for _, item := range items {
		if !isSMDIVSRecording(item) {
			continue
		}
		eventRow, ok := archiveEventRowFromRecording(deviceID, item, seenAt)
		if !ok {
			continue
		}
		if _, err = eventStatement.ExecContext(ctx,
			eventRow.EventID,
			eventRow.DeviceID,
			eventRow.Channel,
			eventRow.Type,
			eventRow.StartTime,
			eventRow.EndTime,
			eventRow.RTSPMainURL,
			eventRow.RTSPSubURL,
			eventRow.FilePath,
			eventRow.VideoStream,
			eventRow.FlagsJSON,
			eventRow.FirstSeenAt,
			eventRow.LastSeenAt,
		); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (s *SQLiteStore) PruneOlderThan(ctx context.Context, cutoff time.Time) (clipIDs []string, err error) {
	formatted := cutoff.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Collect candidate clip IDs before pruning event rows. A clip is returned
	// for filesystem deletion only after the row delete proves no retained event
	// still references it.
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT mp4_clip_id
		FROM smd_ivs_events
		WHERE last_seen_at < ?
			AND mp4_clip_id <> ''
			AND LOWER(COALESCE(mp4_status, '')) NOT IN ('recording', 'transcoding', 'queued', 'downloading')
		ORDER BY mp4_clip_id`,
		formatted,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var clipID string
		if err = rows.Scan(&clipID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		clipID = strings.TrimSpace(clipID)
		if clipID != "" {
			clipIDs = append(clipIDs, clipID)
		}
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	statements := []string{
		`DELETE FROM smd_ivs_events
			WHERE last_seen_at < ?
				AND LOWER(COALESCE(mp4_status, '')) NOT IN ('recording', 'transcoding', 'queued', 'downloading')`,
		`DELETE FROM nvr_recording_chunks WHERE last_seen_at < ?`,
	}
	args := [][]any{
		{formatted},
		{formatted},
	}
	for index, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement, args[index]...); err != nil {
			return nil, err
		}
	}
	deletedClipIDs := make([]string, 0, len(clipIDs))
	for _, clipID := range clipIDs {
		result, execErr := tx.ExecContext(ctx, `DELETE FROM bridge_mp4_clips
			WHERE clip_id = ?
				AND NOT EXISTS (
					SELECT 1 FROM smd_ivs_events WHERE mp4_clip_id = ?
				)`,
			clipID,
			clipID,
		)
		if execErr != nil {
			err = execErr
			return nil, err
		}
		if rowsAffected, rowsErr := result.RowsAffected(); rowsErr == nil && rowsAffected > 0 {
			deletedClipIDs = append(deletedClipIDs, clipID)
		}
	}
	err = tx.Commit()
	if err != nil {
		return nil, err
	}
	return deletedClipIDs, nil
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
	RTSPMainURL string
	RTSPSubURL  string
	FlagsJSON   string
	FirstSeenAt string
	LastSeenAt  string
}

func archiveFileRowFromRecording(deviceID string, item dahua.NVRRecording, seenAt time.Time) (archiveFileRow, bool) {
	if shouldTreatAsEvent(item) {
		return archiveFileRow{}, false
	}
	filePath := strings.TrimSpace(item.FilePath)
	startTime := strings.TrimSpace(item.StartTime)
	endTime := strings.TrimSpace(item.EndTime)
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
		RTSPMainURL: strings.TrimSpace(item.RTSPMainURL),
		RTSPSubURL:  strings.TrimSpace(item.RTSPSubURL),
		FlagsJSON:   string(flagsJSON),
		FirstSeenAt: seenAt.UTC().Format(time.RFC3339Nano),
		LastSeenAt:  seenAt.UTC().Format(time.RFC3339Nano),
	}, true
}

func archiveFileID(deviceID string, item dahua.NVRRecording) string {
	filePath := strings.TrimSpace(item.FilePath)
	if filePath != "" {
		return stableArchiveID("chunk", []string{
			strings.TrimSpace(deviceID),
			filePath,
		})
	}
	return stableArchiveID("file", []string{
		strings.TrimSpace(deviceID),
		fmt.Sprintf("%d", item.Channel),
		strings.TrimSpace(item.StartTime),
		strings.TrimSpace(item.EndTime),
		strings.TrimSpace(item.FilePath),
	})
}

func archiveEventID(deviceID string, item dahua.NVRRecording) string {
	eventCode := normalizeArchiveEventCode(item.Type)
	if eventCode == "" {
		eventCode = strings.TrimSpace(item.Type)
	}
	return stableArchiveID("event", []string{
		strings.TrimSpace(deviceID),
		fmt.Sprintf("%d", item.Channel),
		strings.TrimSpace(item.StartTime),
		strings.TrimSpace(item.EndTime),
		strings.TrimSpace(item.FilePath),
		eventCode,
		strings.TrimSpace(item.VideoStream),
	})
}

func stableArchiveID(prefix string, values []string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "|")))
	return prefix + "_" + hex.EncodeToString(sum[:16])
}
