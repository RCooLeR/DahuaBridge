package archive

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
)

const (
	archiveAssetStateMissing     = "missing"
	archiveAssetStateIndexed     = "indexed"
	archiveAssetStateQueued      = "queued"
	archiveAssetStateDownloading = "downloading"
	archiveAssetStateTranscoding = "transcoding"
	archiveAssetStateReady       = "ready"
	archiveAssetStateFailed      = "failed"
)

type storedClipAsset struct {
	RecordKind string
	RecordID   string
	ClipID     string
	Status     string
	ErrorText  string
}

func (s *SQLiteStore) UpsertClipAsset(ctx context.Context, recordKind string, recordID string, deviceID string, sourceFilePath string, clip mediaapi.ClipInfo) error {
	recordKind = strings.TrimSpace(recordKind)
	recordID = strings.TrimSpace(recordID)
	deviceID = strings.TrimSpace(deviceID)
	if recordKind == "" || recordID == "" || deviceID == "" || strings.TrimSpace(clip.ID) == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	assetID := "asset_" + strings.TrimSpace(clip.ID)
	readyAt := ""
	if clip.Status == mediaapi.ClipStatusCompleted {
		readyAt = now
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO transcoded_assets (
		asset_id, record_kind, record_id, device_id, format, status, path, size_bytes, created_at, updated_at, ready_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(asset_id) DO UPDATE SET
		status=excluded.status,
		path=excluded.path,
		size_bytes=excluded.size_bytes,
		updated_at=excluded.updated_at,
		ready_at=excluded.ready_at`,
		assetID,
		recordKind,
		recordID,
		deviceID,
		"mp4",
		string(clip.Status),
		strings.TrimSpace(clip.FileName),
		clip.Bytes,
		now,
		now,
		readyAt,
	); err != nil {
		return err
	}
	jobID := "job_" + recordKind + "_" + recordID
	outputPath := strings.TrimSpace(clip.FileName)
	if outputPath != "" {
		outputPath = filepath.ToSlash(outputPath)
	}
	startedAt := ""
	if !clip.StartedAt.IsZero() {
		startedAt = clip.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	finishedAt := ""
	if !clip.EndedAt.IsZero() {
		finishedAt = clip.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO transcode_jobs (
		job_id, record_kind, record_id, device_id, status, source_file_path, output_path, error_text, created_at, updated_at, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(job_id) DO UPDATE SET
		status=excluded.status,
		source_file_path=CASE
			WHEN excluded.source_file_path <> '' THEN excluded.source_file_path
			ELSE transcode_jobs.source_file_path
		END,
		output_path=excluded.output_path,
		error_text=excluded.error_text,
		updated_at=excluded.updated_at,
		started_at=excluded.started_at,
		finished_at=excluded.finished_at`,
		jobID,
		recordKind,
		recordID,
		deviceID,
		string(clip.Status),
		strings.TrimSpace(sourceFilePath),
		outputPath,
		strings.TrimSpace(clip.Error),
		now,
		now,
		startedAt,
		finishedAt,
	); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) LoadClipAssets(ctx context.Context, deviceID string, items []dahua.NVRRecording) (map[string]storedClipAsset, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || len(items) == 0 {
		return map[string]storedClipAsset{}, nil
	}

	args := make([]any, 0, len(items)+1)
	placeholders := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	args = append(args, deviceID)
	for _, item := range items {
		recordKind := strings.TrimSpace(item.RecordKind)
		recordID := strings.TrimSpace(item.ID)
		if recordKind == "" || recordID == "" {
			continue
		}
		key := archiveRecordKey(recordKind, recordID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		args = append(args, key)
		placeholders = append(placeholders, "?")
	}
	if len(placeholders) == 0 {
		return map[string]storedClipAsset{}, nil
	}

	query := `SELECT
		a.record_kind,
		a.record_id,
		a.asset_id,
		a.status,
		COALESCE(j.error_text, '')
	FROM transcoded_assets a
	LEFT JOIN transcode_jobs j
		ON j.record_kind = a.record_kind AND j.record_id = a.record_id
	WHERE a.device_id = ? AND (a.record_kind || '|' || a.record_id) IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]storedClipAsset, len(placeholders))
	for rows.Next() {
		var asset storedClipAsset
		var assetID string
		if err := rows.Scan(&asset.RecordKind, &asset.RecordID, &assetID, &asset.Status, &asset.ErrorText); err != nil {
			return nil, err
		}
		asset.ClipID = strings.TrimPrefix(strings.TrimSpace(assetID), "asset_")
		result[archiveRecordKey(asset.RecordKind, asset.RecordID)] = asset
	}
	return result, rows.Err()
}

func (s *SQLiteStore) DeleteClipAsset(ctx context.Context, recordKind string, recordID string, deviceID string) error {
	recordKind = strings.TrimSpace(recordKind)
	recordID = strings.TrimSpace(recordID)
	deviceID = strings.TrimSpace(deviceID)
	if recordKind == "" || recordID == "" || deviceID == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM transcoded_assets
		WHERE device_id = ? AND record_kind = ? AND record_id = ?`,
		deviceID,
		recordKind,
		recordID,
	); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM transcode_jobs
		WHERE device_id = ? AND record_kind = ? AND record_id = ?`,
		deviceID,
		recordKind,
		recordID,
	); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) CountActiveClipJobs(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transcode_jobs
		WHERE status IN ('recording', 'transcoding', 'queued', 'downloading')`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func clipURLPaths(clipID string) (string, string, string, string) {
	clipID = strings.TrimSpace(clipID)
	if clipID == "" {
		return "", "", "", ""
	}
	return fmt.Sprintf("/api/v1/media/recordings/%s/play", clipID),
		fmt.Sprintf("/api/v1/media/recordings/%s/download", clipID),
		fmt.Sprintf("/api/v1/media/recordings/%s", clipID),
		fmt.Sprintf("/api/v1/media/recordings/%s/stop", clipID)
}

func archiveRecordKey(recordKind string, recordID string) string {
	return strings.TrimSpace(recordKind) + "|" + strings.TrimSpace(recordID)
}

func applyStoredArchiveAsset(item *dahua.NVRRecording, asset storedClipAsset) {
	if item == nil {
		return
	}
	if clipID := strings.TrimSpace(asset.ClipID); clipID != "" {
		item.AssetClipID = clipID
	}
	if status := normalizeArchiveAssetState(asset.Status); status != "" {
		item.AssetStatus = status
	}
	if errorText := strings.TrimSpace(asset.ErrorText); errorText != "" {
		item.AssetError = errorText
	}
}

func applyClipArchiveAsset(item *dahua.NVRRecording, clip mediaapi.ClipInfo) {
	if item == nil {
		return
	}
	item.AssetClipID = strings.TrimSpace(clip.ID)
	item.AssetStatus = normalizeArchiveAssetState(string(clip.Status))
	item.AssetError = strings.TrimSpace(clip.Error)
}

func clearArchiveAsset(item *dahua.NVRRecording) {
	if item == nil {
		return
	}
	item.AssetStatus = archiveAssetStateIndexed
	item.AssetClipID = ""
	item.AssetError = ""
	item.AssetPlaybackURL = ""
	item.AssetDownloadURL = ""
	item.AssetSelfURL = ""
	item.AssetStopURL = ""
}

func normalizeArchiveAssetState(value string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "", archiveAssetStateIndexed:
		return archiveAssetStateIndexed
	case archiveAssetStateMissing:
		return archiveAssetStateMissing
	case archiveAssetStateQueued:
		return archiveAssetStateQueued
	case archiveAssetStateDownloading:
		return archiveAssetStateDownloading
	case archiveAssetStateTranscoding, string(mediaapi.ClipStatusRecording):
		return archiveAssetStateTranscoding
	case archiveAssetStateReady, string(mediaapi.ClipStatusCompleted):
		return archiveAssetStateReady
	case archiveAssetStateFailed:
		return archiveAssetStateFailed
	default:
		if normalized == string(mediaapi.ClipStatusFailed) {
			return archiveAssetStateFailed
		}
		return normalized
	}
}
