package dahua

import (
	"path"
	"regexp"
	"strings"
	"time"
)

var (
	recordingFileDatePattern = regexp.MustCompile(`/(\d{4}-\d{2}-\d{2})/`)
	recordingFileTimePattern = regexp.MustCompile(`(?i)^(\d{2})\.(\d{2})\.(\d{2})-(\d{2})\.(\d{2})\.(\d{2})`)
)

func ParseRecordingFileTimeRange(filePath string, location *time.Location) (time.Time, time.Time, bool) {
	filePath = strings.TrimSpace(strings.ReplaceAll(filePath, `\`, `/`))
	if filePath == "" {
		return time.Time{}, time.Time{}, false
	}
	if location == nil {
		location = time.Local
	}

	dateMatch := recordingFileDatePattern.FindStringSubmatch(filePath)
	if len(dateMatch) != 2 {
		return time.Time{}, time.Time{}, false
	}
	if _, err := time.ParseInLocation("2006-01-02", dateMatch[1], location); err != nil {
		return time.Time{}, time.Time{}, false
	}

	baseName := path.Base(filePath)
	timeMatch := recordingFileTimePattern.FindStringSubmatch(baseName)
	if len(timeMatch) != 7 {
		return time.Time{}, time.Time{}, false
	}

	startTime, err := time.ParseInLocation(
		"2006-01-02 15:04:05",
		dateMatch[1]+" "+timeMatch[1]+":"+timeMatch[2]+":"+timeMatch[3],
		location,
	)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	endTime, err := time.ParseInLocation(
		"2006-01-02 15:04:05",
		dateMatch[1]+" "+timeMatch[4]+":"+timeMatch[5]+":"+timeMatch[6],
		location,
	)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	if !endTime.After(startTime) {
		endTime = endTime.Add(24 * time.Hour)
	}
	return startTime, endTime, true
}
