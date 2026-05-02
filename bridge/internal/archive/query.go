package archive

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

const archiveTimeLayout = "2006-01-02 15:04:05"

func (s *SQLiteStore) SearchRecordings(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	result := dahua.NVRRecordingSearchResult{
		DeviceID:  strings.TrimSpace(deviceID),
		Channel:   query.Channel,
		StartTime: query.StartTime.In(time.Local).Format(archiveTimeLayout),
		EndTime:   query.EndTime.In(time.Local).Format(archiveTimeLayout),
		Limit:     query.Limit,
		Items:     []dahua.NVRRecording{},
	}
	if strings.TrimSpace(deviceID) == "" || query.Channel <= 0 {
		return result, nil
	}
	if query.Limit <= 0 {
		query.Limit = 25
		result.Limit = query.Limit
	}

	if shouldSearchEventScope(query) {
		items, err := s.searchEventRows(ctx, deviceID, query)
		if err != nil {
			return dahua.NVRRecordingSearchResult{}, err
		}
		result.Items = items
		result.ReturnedCount = len(items)
		return result, nil
	}

	items, err := s.searchFileRows(ctx, deviceID, query)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, err
	}
	result.Items = items
	result.ReturnedCount = len(items)
	return result, nil
}

func (s *SQLiteStore) searchFileRows(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery) ([]dahua.NVRRecording, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		file_id, channel, start_time, end_time, file_path, video_stream, disk, partition, cluster,
		length_bytes, cut_length_bytes, flags_json
		FROM archive_files
		WHERE device_id = ? AND channel = ? AND end_time >= ? AND start_time <= ?
		ORDER BY start_time DESC
		LIMIT ?`,
		strings.TrimSpace(deviceID),
		query.Channel,
		query.StartTime.In(time.Local).Format(archiveTimeLayout),
		query.EndTime.In(time.Local).Format(archiveTimeLayout),
		query.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]dahua.NVRRecording, 0)
	for rows.Next() {
		var (
			fileID, startTime, endTime, filePath, videoStream, flagsJSON string
			channel, disk, partition, cluster                            int
			lengthBytes, cutLengthBytes                                  int64
		)
		if err := rows.Scan(&fileID, &channel, &startTime, &endTime, &filePath, &videoStream, &disk, &partition, &cluster, &lengthBytes, &cutLengthBytes, &flagsJSON); err != nil {
			return nil, err
		}
		item := dahua.NVRRecording{
			ID:             fileID,
			RecordKind:     "file",
			Source:         "nvr",
			Channel:        channel,
			StartTime:      startTime,
			EndTime:        endTime,
			FilePath:       filePath,
			VideoStream:    videoStream,
			Disk:           disk,
			Partition:      partition,
			Cluster:        cluster,
			LengthBytes:    lengthBytes,
			CutLengthBytes: cutLengthBytes,
			Flags:          parseJSONStringArray(flagsJSON),
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dedupeArchiveFileRows(items), nil
}

func (s *SQLiteStore) searchEventRows(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery) ([]dahua.NVRRecording, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		event_id, channel, start_time, end_time, file_path, source, type, video_stream, flags_json
		FROM archive_events
		WHERE device_id = ? AND channel = ? AND end_time >= ? AND start_time <= ?
		ORDER BY start_time DESC
		LIMIT ?`,
		strings.TrimSpace(deviceID),
		query.Channel,
		query.StartTime.In(time.Local).Format(archiveTimeLayout),
		query.EndTime.In(time.Local).Format(archiveTimeLayout),
		query.Limit*4,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]dahua.NVRRecording, 0)
	for rows.Next() {
		var (
			eventID, startTime, endTime, filePath, source, recordingType, videoStream, flagsJSON string
			channel                                                                              int
		)
		if err := rows.Scan(&eventID, &channel, &startTime, &endTime, &filePath, &source, &recordingType, &videoStream, &flagsJSON); err != nil {
			return nil, err
		}
		item := dahua.NVRRecording{
			ID:          eventID,
			RecordKind:  "event",
			Source:      source,
			Channel:     channel,
			StartTime:   startTime,
			EndTime:     endTime,
			FilePath:    filePath,
			Type:        recordingType,
			VideoStream: videoStream,
			Flags:       parseJSONStringArray(flagsJSON),
		}
		if !matchesArchiveEventQuery(item, query.EventCode) {
			continue
		}
		items = append(items, item)
		if len(items) >= query.Limit {
			break
		}
	}
	return items, rows.Err()
}

func (s *SQLiteStore) MarkCoverage(ctx context.Context, scope string, deviceID string, channel int, startTime time.Time, endTime time.Time, syncedAt time.Time) error {
	scope = strings.TrimSpace(scope)
	deviceID = strings.TrimSpace(deviceID)
	if scope == "" || deviceID == "" || channel <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO archive_sync_coverage (
		scope, device_id, channel, window_start, window_end, synced_at
	) VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(scope, device_id, channel, window_start, window_end) DO UPDATE SET
		synced_at=excluded.synced_at`,
		scope,
		deviceID,
		channel,
		startTime.In(time.Local).Format(archiveTimeLayout),
		endTime.In(time.Local).Format(archiveTimeLayout),
		syncedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) IsCoverageComplete(ctx context.Context, scope string, deviceID string, channel int, windows [][2]time.Time) (bool, error) {
	scope = strings.TrimSpace(scope)
	deviceID = strings.TrimSpace(deviceID)
	if scope == "" || deviceID == "" || channel <= 0 || len(windows) == 0 {
		return false, nil
	}
	var count int
	for _, window := range windows {
		if window[0].IsZero() || window[1].IsZero() {
			return false, nil
		}
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM archive_sync_coverage
			WHERE scope = ? AND device_id = ? AND channel = ? AND window_start = ? AND window_end = ?`,
			scope,
			deviceID,
			channel,
			window[0].In(time.Local).Format(archiveTimeLayout),
			window[1].In(time.Local).Format(archiveTimeLayout),
		).Scan(&count); err != nil {
			if err == sql.ErrNoRows {
				return false, nil
			}
			return false, err
		}
		if count == 0 {
			return false, nil
		}
	}
	return true, nil
}

func shouldSearchEventScope(query dahua.NVRRecordingQuery) bool {
	code, ok := archiveScopeForQuery(query)
	return ok && strings.HasPrefix(code, "event:")
}

func archiveScopeForQuery(query dahua.NVRRecordingQuery) (string, bool) {
	eventCode := normalizeArchiveEventCode(query.EventCode)
	if query.EventOnly || eventCode != "" {
		switch eventCode {
		case "human", "vehicle", "animal", "tripwire", "intrusion":
			return "event:" + eventCode, true
		default:
			return "", false
		}
	}
	return "archive", true
}

func matchesArchiveEventQuery(item dahua.NVRRecording, eventCode string) bool {
	code := normalizeArchiveEventCode(eventCode)
	if code == "" {
		return true
	}
	candidates := append([]string{item.Type}, item.Flags...)
	for _, candidate := range candidates {
		if normalizeArchiveEventCode(candidate) == code {
			return true
		}
	}
	return false
}

func normalizeArchiveEventCode(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	trimmed = strings.TrimPrefix(trimmed, "event.")
	switch trimmed {
	case "", "*", "all", "any", "__all__":
		return ""
	case "human", "humandetection", "smartmotionhuman", "intelliframehuman", "smdtypehuman":
		return "human"
	case "vehicle", "vehicledetection", "smartmotionvehicle", "motorvehicle", "smdtypevehicle":
		return "vehicle"
	case "animal", "animaldetection", "smdtypeanimal":
		return "animal"
	case "tripwire", "crosslinedetection":
		return "tripwire"
	case "intrusion", "crossregiondetection":
		return "intrusion"
	default:
		return trimmed
	}
}

func parseJSONStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func archiveRecordID(deviceID string, item dahua.NVRRecording) (string, string) {
	if strings.EqualFold(strings.TrimSpace(item.RecordKind), "event") || shouldTreatAsEvent(item) {
		return archiveEventID(deviceID, item), "event"
	}
	return archiveFileID(deviceID, item), "file"
}

func shouldTreatAsEvent(item dahua.NVRRecording) bool {
	source := strings.ToLower(strings.TrimSpace(item.Source))
	recordingType := strings.ToLower(strings.TrimSpace(item.Type))
	return source == "nvr_event" || recordingType == "event" || strings.HasPrefix(recordingType, "event.")
}

func buildCoverageWindows(startTime time.Time, endTime time.Time, windowSize time.Duration) [][2]time.Time {
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) || windowSize <= 0 {
		return nil
	}
	windows := make([][2]time.Time, 0)
	for from := startTime.Truncate(windowSize); from.Before(endTime); from = from.Add(windowSize) {
		to := from.Add(windowSize)
		if to.After(endTime) {
			to = endTime
		}
		windows = append(windows, [2]time.Time{from, to})
	}
	return windows
}

func ensureArchiveRecordIdentity(deviceID string, item *dahua.NVRRecording) {
	if item == nil {
		return
	}
	id, kind := archiveRecordID(deviceID, *item)
	item.ID = id
	item.RecordKind = kind
	if strings.TrimSpace(item.Source) == "" && kind == "file" {
		item.Source = "nvr"
	}
}

func dedupeArchiveFileRows(items []dahua.NVRRecording) []dahua.NVRRecording {
	if len(items) < 2 {
		return items
	}
	bestByPath := make(map[string]dahua.NVRRecording, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.FilePath)
		if key == "" {
			key = strings.TrimSpace(item.ID)
		}
		existing, ok := bestByPath[key]
		if !ok {
			bestByPath[key] = item
			order = append(order, key)
			continue
		}
		if archiveFileRowRank(item) > archiveFileRowRank(existing) {
			bestByPath[key] = item
		}
	}
	result := make([]dahua.NVRRecording, 0, len(order))
	for _, key := range order {
		result = append(result, bestByPath[key])
	}
	return result
}

func archiveFileRowRank(item dahua.NVRRecording) int64 {
	rank := item.LengthBytes
	if rank <= 0 {
		rank = item.CutLengthBytes
	}
	if startTime, okStart := parseArchiveLocalTime(item.StartTime); okStart {
		if endTime, okEnd := parseArchiveLocalTime(item.EndTime); okEnd && endTime.After(startTime) {
			rank += int64(endTime.Sub(startTime) / time.Second)
		}
	}
	return rank
}

func summarizeScope(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return "archive"
	}
	return fmt.Sprintf("scope %s", scope)
}
