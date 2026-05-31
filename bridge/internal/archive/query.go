package archive

import (
	"context"
	"encoding/json"
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
		chunk_id, channel, start_time, end_time, file_path, video_stream, disk, partition, cluster,
		length_bytes, cut_length_bytes, flags_json
		FROM nvr_recording_chunks
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
			RecordKind:     "recording_chunk",
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
	where := `device_id = ? AND channel = ? AND end_time >= ? AND start_time <= ?`
	args := []any{
		strings.TrimSpace(deviceID),
		query.Channel,
		query.StartTime.In(time.Local).Format(archiveTimeLayout),
		query.EndTime.In(time.Local).Format(archiveTimeLayout),
	}
	if filterSQL, filterArgs := archiveEventSQLFilter(query.EventCode); filterSQL != "" {
		where += ` AND (` + filterSQL + `)`
		args = append(args, filterArgs...)
	}
	args = append(args, query.Limit)

	rows, err := s.db.QueryContext(ctx, `SELECT
		event_id, channel, start_time, end_time, source_file_path, event_type, video_stream,
		rtsp_main_url, rtsp_sub_url, flags_json, mp4_clip_id, mp4_status, mp4_error
		FROM smd_ivs_events
		WHERE `+where+`
		ORDER BY start_time DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]dahua.NVRRecording, 0)
	for rows.Next() {
		var (
			eventID, startTime, endTime, filePath, recordingType, videoStream, rtspMainURL, rtspSubURL, flagsJSON string
			mp4ClipID, mp4Status, mp4Error                                                                        string
			channel                                                                                               int
		)
		if err := rows.Scan(&eventID, &channel, &startTime, &endTime, &filePath, &recordingType, &videoStream, &rtspMainURL, &rtspSubURL, &flagsJSON, &mp4ClipID, &mp4Status, &mp4Error); err != nil {
			return nil, err
		}
		item := dahua.NVRRecording{
			ID:          eventID,
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
		}
		if mp4ClipID != "" {
			item.AssetClipID = mp4ClipID
		}
		if mp4Status != "" {
			item.AssetStatus = normalizeArchiveAssetState(mp4Status)
		}
		if mp4Error != "" {
			item.AssetError = mp4Error
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

func shouldSearchEventScope(query dahua.NVRRecordingQuery) bool {
	code, ok := archiveScopeForQuery(query)
	return ok && strings.HasPrefix(code, "event:")
}

func archiveScopeForQuery(query dahua.NVRRecordingQuery) (string, bool) {
	eventCode := normalizeArchiveEventCode(query.EventCode)
	if query.EventOnly || eventCode != "" {
		switch eventCode {
		case "":
			return "event:all", true
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

func archiveEventSQLFilter(eventCode string) (string, []any) {
	code := normalizeArchiveEventCode(eventCode)
	if code == "" {
		return "", nil
	}
	patterns := archiveEventSQLPatterns(code)
	if len(patterns) == 0 {
		return "1 = 0", nil
	}

	clauses := []string{"LOWER(event_type) = ?", "LOWER(flags_json) LIKE ?"}
	args := []any{code, "%\"" + code + "\"%"}
	for _, pattern := range patterns {
		clauses = append(clauses, "LOWER(event_type) LIKE ?", "LOWER(flags_json) LIKE ?")
		args = append(args, pattern, pattern)
	}
	return strings.Join(clauses, " OR "), args
}

func archiveEventSQLPatterns(code string) []string {
	switch normalizeArchiveEventCode(code) {
	case "human":
		return []string{"%smdtypehuman%", "%humandetection%", "%smartmotionhuman%", "%intelliframehuman%"}
	case "vehicle":
		return []string{"%smdtypevehicle%", "%vehicledetection%", "%smartmotionvehicle%", "%motorvehicle%"}
	case "animal":
		return []string{"%smdtypeanimal%", "%animaldetection%"}
	case "tripwire":
		return []string{"%crosslinedetection%", "%tripwire%"}
	case "intrusion":
		return []string{"%crossregiondetection%", "%intrusion%"}
	default:
		return nil
	}
}

func normalizeArchiveEventCode(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	trimmed = strings.TrimPrefix(trimmed, "event.")
	switch trimmed {
	case "", "*", "all", "any", "__all__":
		return ""
	case "human", "humandetection", "smartmotionhuman", "intelliframehuman", "smdtypehuman":
		return "human"
	case "vehicle", "transport", "vehicledetection", "smartmotionvehicle", "motorvehicle", "smdtypevehicle":
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
	recordKind := strings.ToLower(strings.TrimSpace(item.RecordKind))
	if recordKind == "smd_ivs" || recordKind == "event" || shouldTreatAsEvent(item) {
		return archiveEventID(deviceID, item), "smd_ivs"
	}
	return archiveFileID(deviceID, item), "recording_chunk"
}

func shouldTreatAsEvent(item dahua.NVRRecording) bool {
	source := strings.ToLower(strings.TrimSpace(item.Source))
	recordKind := strings.ToLower(strings.TrimSpace(item.RecordKind))
	recordingType := strings.ToLower(strings.TrimSpace(item.Type))
	return recordKind == "smd_ivs" ||
		recordKind == "event" ||
		source == "smd_ivs" ||
		source == "nvr_event" ||
		recordingType == "event" ||
		strings.HasPrefix(recordingType, "event.")
}

func isSMDIVSRecording(item dahua.NVRRecording) bool {
	if !shouldTreatAsEvent(item) {
		return false
	}
	code := normalizeArchiveEventCode(item.Type)
	if code == "" {
		for _, flag := range item.Flags {
			code = normalizeArchiveEventCode(flag)
			if code != "" {
				break
			}
		}
	}
	switch code {
	case "human", "vehicle", "animal", "tripwire", "intrusion":
		return true
	default:
		return false
	}
}

func ensureArchiveRecordIdentity(deviceID string, item *dahua.NVRRecording) {
	if item == nil {
		return
	}
	id, kind := archiveRecordID(deviceID, *item)
	item.ID = id
	item.RecordKind = kind
	if strings.TrimSpace(item.Source) == "" {
		switch kind {
		case "recording_chunk":
			item.Source = "nvr"
		case "smd_ivs":
			item.Source = "smd_ivs"
		}
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
