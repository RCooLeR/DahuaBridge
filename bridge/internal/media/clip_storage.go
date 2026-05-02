package media

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) removeClipJob(id string, job *clipJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.clipJobs[id]; ok && existing == job {
		delete(m.clipJobs, id)
		m.setMediaWorkerCountLocked()
		m.logWorkerInventoryLocked("removed", job.status())
	}
}

func matchesClipQuery(info ClipInfo, query ClipQuery) bool {
	if query.StreamID != "" && info.StreamID != query.StreamID {
		return false
	}
	if query.RootDeviceID != "" && info.RootDeviceID != query.RootDeviceID {
		return false
	}
	if query.Channel > 0 && info.Channel != query.Channel {
		return false
	}
	if !query.StartTime.IsZero() {
		_, end := clipQueryWindow(info)
		if end.Before(query.StartTime) {
			return false
		}
	}
	start, _ := clipQueryWindow(info)
	if !query.EndTime.IsZero() && start.After(query.EndTime) {
		return false
	}
	return true
}

func clipQueryWindow(info ClipInfo) (time.Time, time.Time) {
	start := info.SourceStartAt
	if start.IsZero() {
		start = info.StartedAt
	}
	end := info.SourceEndAt
	if end.IsZero() {
		end = info.EndedAt
	}
	if end.IsZero() {
		end = start
	}
	return start, end
}

func (m *Manager) persistClip(info ClipInfo) error {
	if strings.TrimSpace(m.cfg.ClipPath) == "" {
		return errClipStorageMissing
	}
	if err := os.MkdirAll(m.cfg.ClipPath, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.cfg.ClipPath, info.ID+".json"), body, 0o644)
}

func (m *Manager) loadClip(clipID string) (ClipInfo, error) {
	clipID = strings.TrimSpace(clipID)
	if clipID == "" {
		return ClipInfo{}, ErrClipNotFound
	}
	body, err := os.ReadFile(filepath.Join(strings.TrimSpace(m.cfg.ClipPath), clipID+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ClipInfo{}, ErrClipNotFound
		}
		return ClipInfo{}, err
	}
	var info ClipInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return ClipInfo{}, err
	}
	return info, nil
}

func clipSourceWindow(streamURL string, duration time.Duration) (time.Time, time.Time) {
	sourceURL, err := url.Parse(strings.TrimSpace(streamURL))
	if err != nil {
		return time.Time{}, time.Time{}
	}
	query := sourceURL.Query()
	startTime, err := time.Parse("2006_01_02_15_04_05", strings.TrimSpace(query.Get("starttime")))
	if err != nil {
		return time.Time{}, time.Time{}
	}
	endTime, err := time.Parse("2006_01_02_15_04_05", strings.TrimSpace(query.Get("endtime")))
	if err != nil {
		endTime = time.Time{}
	}
	if duration > 0 {
		durationEnd := startTime.Add(duration)
		if endTime.IsZero() || durationEnd.Before(endTime) {
			endTime = durationEnd
		}
	}
	return startTime.UTC(), endTime.UTC()
}

func clipFilePath(clipPath string, fileName string) (string, error) {
	if strings.TrimSpace(fileName) == "" {
		return "", fmt.Errorf("clip file name is empty")
	}
	return filepath.Join(strings.TrimSpace(clipPath), fileName), nil
}
