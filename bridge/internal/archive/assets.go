package archive

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
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

type eventClipCandidate struct {
	DeviceID string
	Item     dahua.NVRRecording
}

type activeEventClipAsset struct {
	RecordKind     string
	RecordID       string
	DeviceID       string
	SourceFilePath string
	ClipID         string
}

func (s *SQLiteStore) UpsertClipAsset(ctx context.Context, recordKind string, recordID string, deviceID string, sourceFilePath string, clip mediaapi.ClipInfo) error {
	recordKind = normalizeArchiveRecordKind(recordKind)
	recordID = strings.TrimSpace(recordID)
	deviceID = strings.TrimSpace(deviceID)
	if recordKind == "" || recordID == "" || deviceID == "" || strings.TrimSpace(clip.ID) == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var queued int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM archive_clip_retention WHERE clip_id = ? AND queued = 1`, strings.TrimSpace(clip.ID)).Scan(&queued); err != nil {
		return err
	}
	if queued != 0 {
		// Queueing quarantines the ID until deletion is acknowledged. A reader
		// must not reattach it while filesystem cleanup runs outside SQLite.
		return fmt.Errorf("archive clip is pending retention cleanup")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	clipPath := filepath.ToSlash(strings.TrimSpace(clip.FileName))
	if _, err := tx.ExecContext(ctx, `INSERT INTO bridge_mp4_clips (
		clip_id, device_id, channel, stream_id, start_time, end_time, file_path, status, error_text, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(clip_id) DO UPDATE SET
		device_id=excluded.device_id,
		channel=excluded.channel,
		stream_id=excluded.stream_id,
		start_time=excluded.start_time,
		end_time=excluded.end_time,
		file_path=excluded.file_path,
		status=excluded.status,
		error_text=excluded.error_text,
		updated_at=excluded.updated_at`,
		strings.TrimSpace(clip.ID),
		deviceID,
		clip.Channel,
		strings.TrimSpace(clip.StreamID),
		formatOptionalClipTime(clip.SourceStartAt),
		formatOptionalClipTime(clip.SourceEndAt),
		clipPath,
		string(clip.Status),
		strings.TrimSpace(clip.Error),
		now,
		now,
	); err != nil {
		return err
	}
	if err := rememberArchiveClipOwnership(ctx, tx, recordKind, recordID, deviceID, clip); err != nil {
		return err
	}
	if recordKind == "smd_ivs" {
		_, err := tx.ExecContext(ctx, `UPDATE smd_ivs_events
			SET mp4_clip_id = ?,
				mp4_file_path = ?,
				mp4_status = ?,
				mp4_error = ?,
				source_file_path = CASE
					WHEN ? <> '' THEN ?
					ELSE source_file_path
				END
			WHERE device_id = ? AND event_id = ?`,
			strings.TrimSpace(clip.ID),
			clipPath,
			string(clip.Status),
			strings.TrimSpace(clip.Error),
			strings.TrimSpace(sourceFilePath),
			strings.TrimSpace(sourceFilePath),
			deviceID,
			recordID,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
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
		recordKind := normalizeArchiveRecordKind(item.RecordKind)
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
		'smd_ivs',
		event_id,
		mp4_clip_id,
		mp4_status,
		mp4_error
	FROM smd_ivs_events
	WHERE device_id = ? AND ('smd_ivs|' || event_id) IN (` + strings.Join(placeholders, ",") + `)
		AND mp4_clip_id <> ''`

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
	recordKind = normalizeArchiveRecordKind(recordKind)
	recordID = strings.TrimSpace(recordID)
	deviceID = strings.TrimSpace(deviceID)
	if recordKind == "" || recordID == "" || deviceID == "" {
		return nil
	}
	if recordKind == "smd_ivs" {
		if _, err := s.db.ExecContext(ctx, `UPDATE smd_ivs_events
			SET mp4_clip_id = '', mp4_file_path = '', mp4_status = '', mp4_error = ''
			WHERE device_id = ? AND event_id = ?`,
			deviceID,
			recordID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) CountActiveClipJobs(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM smd_ivs_events
		WHERE LOWER(COALESCE(mp4_status, '')) IN ('recording', 'transcoding', 'queued', 'downloading')`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SQLiteStore) LoadPendingEventClipCandidates(ctx context.Context, cutoff time.Time, readyBefore time.Time, limit int, allowlists map[string][]int) ([]eventClipCandidate, error) {
	if limit <= 0 {
		limit = archiveQueryLimit
	}
	where := `start_time >= ?
			AND end_time <= ?
			AND (
				mp4_clip_id = ''
				OR LOWER(COALESCE(mp4_status, '')) NOT IN ('completed', 'ready', 'recording', 'transcoding', 'queued', 'downloading')
			)`
	args := []any{
		cutoff.In(time.Local).Format(archiveTimeLayout),
		readyBefore.In(time.Local).Format(archiveTimeLayout),
	}
	if filterSQL, filterArgs := archiveEventVideoChannelSQLFilter(allowlists); filterSQL != "" {
		where += ` AND (` + filterSQL + `)`
		args = append(args, filterArgs...)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, `SELECT
		device_id, event_id, channel, start_time, end_time, source_file_path, event_type, video_stream,
		rtsp_main_url, rtsp_sub_url, flags_json
		FROM smd_ivs_events
		WHERE `+where+`
		ORDER BY start_time DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]eventClipCandidate, 0)
	for rows.Next() {
		var (
			deviceID, eventID, startTime, endTime, filePath, recordingType, videoStream, rtspMainURL, rtspSubURL, flagsJSON string
			channel                                                                                                         int
		)
		if err := rows.Scan(&deviceID, &eventID, &channel, &startTime, &endTime, &filePath, &recordingType, &videoStream, &rtspMainURL, &rtspSubURL, &flagsJSON); err != nil {
			return nil, err
		}
		candidates = append(candidates, eventClipCandidate{
			DeviceID: strings.TrimSpace(deviceID),
			Item: dahua.NVRRecording{
				ID:          strings.TrimSpace(eventID),
				RecordKind:  "smd_ivs",
				Source:      "smd_ivs",
				Channel:     channel,
				StartTime:   startTime,
				EndTime:     endTime,
				FilePath:    filePath,
				Type:        recordingType,
				VideoStream: videoStream,
				RTSPMainURL: rtspMainURL,
				RTSPSubURL:  rtspSubURL,
				Flags:       parseJSONStringArray(flagsJSON),
			},
		})
	}
	return candidates, rows.Err()
}

func archiveEventVideoChannelSQLFilter(allowlists map[string][]int) (string, []any) {
	if len(allowlists) == 0 {
		return "", nil
	}

	codes := make([]string, 0, len(allowlists))
	for code := range allowlists {
		if code != "all" {
			codes = append(codes, code)
		}
	}
	slices.Sort(codes)

	clauses := make([]string, 0, len(codes)+1)
	args := make([]any, 0)
	for _, code := range codes {
		eventSQL, eventArgs := archiveEventSQLFilter(code)
		if eventSQL == "" {
			continue
		}
		channelSQL, channelArgs := archiveEventVideoChannelSQLList(allowlists[code])
		clauses = append(clauses, "WHEN ("+eventSQL+") THEN "+channelSQL)
		args = append(args, eventArgs...)
		args = append(args, channelArgs...)
	}

	elseSQL := "1"
	if channels, ok := allowlists["all"]; ok {
		var channelArgs []any
		elseSQL, channelArgs = archiveEventVideoChannelSQLList(channels)
		args = append(args, channelArgs...)
	}
	if len(clauses) == 0 {
		if elseSQL == "1" {
			return "", nil
		}
		return elseSQL, args
	}
	return "CASE " + strings.Join(clauses, " ") + " ELSE " + elseSQL + " END", args
}

func archiveEventVideoChannelSQLList(channels []int) (string, []any) {
	channels = normalizeArchiveEventVideoChannels(channels)
	if len(channels) == 0 {
		return "1", nil
	}
	placeholders := make([]string, 0, len(channels))
	args := make([]any, 0, len(channels))
	for _, channel := range channels {
		placeholders = append(placeholders, "?")
		args = append(args, channel)
	}
	return "channel IN (" + strings.Join(placeholders, ",") + ")", args
}

func (s *SQLiteStore) LoadEventClipAssetsForReconciliation(ctx context.Context, limit int) ([]activeEventClipAsset, error) {
	if limit <= 0 {
		limit = archiveQueryLimit
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		e.device_id, e.event_id, e.source_file_path, e.mp4_clip_id
		FROM smd_ivs_events e LEFT JOIN bridge_mp4_clips c ON c.clip_id=e.mp4_clip_id
		WHERE e.mp4_clip_id <> ''
			AND LOWER(COALESCE(e.mp4_status, '')) IN ('recording', 'transcoding', 'queued', 'downloading', 'completed', 'ready')
		ORDER BY CASE WHEN e.mp4_status IN ('ready','completed') THEN 1 ELSE 0 END,
			COALESCE(c.updated_at, '') ASC, e.event_id
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assets := make([]activeEventClipAsset, 0)
	for rows.Next() {
		var asset activeEventClipAsset
		if err := rows.Scan(&asset.DeviceID, &asset.RecordID, &asset.SourceFilePath, &asset.ClipID); err != nil {
			return nil, err
		}
		asset.RecordKind = "smd_ivs"
		asset.DeviceID = strings.TrimSpace(asset.DeviceID)
		asset.RecordID = strings.TrimSpace(asset.RecordID)
		asset.SourceFilePath = strings.TrimSpace(asset.SourceFilePath)
		asset.ClipID = strings.TrimSpace(asset.ClipID)
		assets = append(assets, asset)
	}
	return assets, rows.Err()
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
	return normalizeArchiveRecordKind(recordKind) + "|" + strings.TrimSpace(recordID)
}

func normalizeArchiveRecordKind(recordKind string) string {
	switch strings.ToLower(strings.TrimSpace(recordKind)) {
	case "event", "smd-ivs", "smd_ivs":
		return "smd_ivs"
	case "file", "chunk", "recording_chunk", "recording-chunk":
		return "recording_chunk"
	default:
		return strings.TrimSpace(recordKind)
	}
}

func formatOptionalClipTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
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

func isActiveArchiveAssetState(value string) bool {
	switch normalizeArchiveAssetState(value) {
	case archiveAssetStateQueued, archiveAssetStateDownloading, archiveAssetStateTranscoding:
		return true
	default:
		return false
	}
}
