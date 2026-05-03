package archive

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type coverageChunk struct {
	StartTime time.Time
	EndTime   time.Time
}

func (s *SQLiteStore) LoadArchiveCoverage(ctx context.Context, deviceID string, channel int) ([]coverageChunk, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT start_time, end_time
		FROM archive_files
		WHERE device_id = ? AND channel = ?
		ORDER BY start_time ASC, end_time ASC`,
		strings.TrimSpace(deviceID),
		channel,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chunks := make([]coverageChunk, 0)
	for rows.Next() {
		var startText, endText string
		if err := rows.Scan(&startText, &endText); err != nil {
			return nil, err
		}
		startTime, okStart := parseArchiveLocalTime(startText)
		endTime, okEnd := parseArchiveLocalTime(endText)
		if !okStart || !okEnd || !endTime.After(startTime) {
			continue
		}
		chunks = append(chunks, coverageChunk{
			StartTime: startTime,
			EndTime:   endTime,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func (s *SQLiteStore) LoadEventSummaryCounts(
	ctx context.Context,
	deviceID string,
	startTime time.Time,
	endTime time.Time,
	eventCode string,
) (map[int]map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT channel, type, flags_json
		FROM archive_events
		WHERE device_id = ? AND end_time >= ? AND start_time <= ?
		ORDER BY channel ASC, start_time ASC`,
		strings.TrimSpace(deviceID),
		startTime.In(time.Local).Format(archiveTimeLayout),
		endTime.In(time.Local).Format(archiveTimeLayout),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	filterCode := normalizeArchiveEventCode(eventCode)
	counts := make(map[int]map[string]int)
	for rows.Next() {
		var (
			channel   int
			typeText  string
			flagsJSON string
		)
		if err := rows.Scan(&channel, &typeText, &flagsJSON); err != nil {
			return nil, err
		}
		code := archiveSummaryCode(typeText, flagsJSON)
		if code == "" {
			continue
		}
		if filterCode != "" && code != filterCode {
			continue
		}
		channelCounts := counts[channel]
		if channelCounts == nil {
			channelCounts = make(map[string]int)
			counts[channel] = channelCounts
		}
		channelCounts[code]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func archiveSummaryCode(typeText string, flagsJSON string) string {
	candidates := make([]string, 0, 6)
	candidates = append(candidates, typeText)

	if strings.TrimSpace(flagsJSON) != "" {
		var flags []string
		if err := json.Unmarshal([]byte(flagsJSON), &flags); err == nil {
			candidates = append(candidates, flags...)
		}
	}

	for _, candidate := range candidates {
		switch normalizeArchiveEventCode(candidate) {
		case "human", "vehicle", "animal", "tripwire", "intrusion":
			return normalizeArchiveEventCode(candidate)
		}
	}
	return ""
}

func firstAndLastCoverageChunk(chunks []coverageChunk) (coverageChunk, coverageChunk, bool) {
	if len(chunks) == 0 {
		return coverageChunk{}, coverageChunk{}, false
	}
	first := chunks[0]
	last := chunks[len(chunks)-1]
	for _, chunk := range chunks[1:] {
		if chunk.StartTime.Before(first.StartTime) {
			first = chunk
		}
		if chunk.EndTime.After(last.EndTime) {
			last = chunk
		}
	}
	return first, last, true
}

func archiveCountMapTotal(values map[string]int) int {
	total := 0
	for _, count := range values {
		total += count
	}
	return total
}

func archiveCountMapKeys(values map[int]map[string]int) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func archiveCoverageWindowsFromRows(rows *sql.Rows) ([]coverageChunk, error) {
	chunks := make([]coverageChunk, 0)
	for rows.Next() {
		var startText, endText string
		if err := rows.Scan(&startText, &endText); err != nil {
			return nil, err
		}
		startTime, okStart := parseArchiveLocalTime(startText)
		endTime, okEnd := parseArchiveLocalTime(endText)
		if !okStart || !okEnd || !endTime.After(startTime) {
			continue
		}
		chunks = append(chunks, coverageChunk{
			StartTime: startTime,
			EndTime:   endTime,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}
