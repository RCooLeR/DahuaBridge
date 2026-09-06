package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
)

type archiveLiveSearcher struct {
	runtime *runtimeServices
}

func (s archiveLiveSearcher) NVRRecordings(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	if s.runtime == nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("runtime services are not configured")
	}
	s.runtime.mu.RLock()
	searcher, ok := s.runtime.nvrRecordings[deviceID]
	s.runtime.mu.RUnlock()
	if !ok || searcher == nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("%w: %s", dahua.ErrDeviceNotFound, deviceID)
	}

	result, err := searcher.FindRecordings(ctx, query)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, err
	}
	if strings.TrimSpace(result.DeviceID) == "" {
		result.DeviceID = strings.TrimSpace(deviceID)
	}
	if result.Channel <= 0 {
		result.Channel = query.Channel
	}
	if strings.TrimSpace(result.StartTime) == "" && !query.StartTime.IsZero() {
		result.StartTime = query.StartTime.In(time.Local).Format(bridgeRecordingTimeLayout)
	}
	if strings.TrimSpace(result.EndTime) == "" && !query.EndTime.IsZero() {
		result.EndTime = query.EndTime.In(time.Local).Format(bridgeRecordingTimeLayout)
	}
	if result.Limit <= 0 {
		result.Limit = query.Limit
	}
	if result.Items == nil {
		result.Items = []dahua.NVRRecording{}
	}
	return result, nil
}

func (s archiveLiveSearcher) EnsureNVRArchiveClip(ctx context.Context, deviceID string, item dahua.NVRRecording) (media.ClipInfo, error) {
	if s.runtime == nil {
		return media.ClipInfo{}, fmt.Errorf("runtime services are not configured")
	}
	return s.runtime.EnsureNVRArchiveClip(ctx, deviceID, item)
}

func (s archiveLiveSearcher) GetClip(clipID string) (media.ClipInfo, error) {
	if s.runtime == nil {
		return media.ClipInfo{}, fmt.Errorf("runtime services are not configured")
	}
	clip, err := s.runtime.GetClip(clipID)
	if err != nil {
		return clip, err
	}
	if clip.Status == media.ClipStatusRecording {
		s.runtime.mu.RLock()
		reader := s.runtime.media
		s.runtime.mu.RUnlock()
		if reader == nil {
			return clip, media.ErrClipNotFound
		}
		active, ok := reader.ActiveClip(clip.StreamID)
		if !ok || active.ID != clip.ID {
			return clip, media.ErrClipNotFound
		}
	}
	if clip.Status != media.ClipStatusCompleted {
		return clip, err
	}
	s.runtime.mu.RLock()
	files, ok := s.runtime.media.(interface{ ClipFilePath(string) (string, error) })
	s.runtime.mu.RUnlock()
	if !ok {
		return clip, fmt.Errorf("clip file reader unavailable")
	}
	path, err := files.ClipFilePath(clipID)
	if err != nil {
		return clip, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return clip, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return clip, media.ErrClipNotFound
	}
	return clip, nil
}

func (s archiveLiveSearcher) DeleteClip(ctx context.Context, clipID string) error {
	if s.runtime == nil {
		return fmt.Errorf("runtime services are not configured")
	}
	s.runtime.mu.RLock()
	mediaReader := s.runtime.media
	s.runtime.mu.RUnlock()
	deleter, ok := mediaReader.(interface {
		DeleteClip(context.Context, string) error
	})
	if !ok || deleter == nil {
		return fmt.Errorf("media layer does not support clip deletion")
	}
	// Archive cleanup must never use DeleteClip's user-facing stop-and-delete
	// behavior. A clip ID cannot restart after completing, so this check cannot
	// race an inactive ID back into an active job.
	if clip, err := mediaReader.GetClip(clipID); err == nil {
		if active, ok := mediaReader.ActiveClip(clip.StreamID); ok && active.ID == clipID {
			return media.ErrClipAlreadyActive
		}
	}
	return deleter.DeleteClip(ctx, clipID)
}

func (s archiveLiveSearcher) DeleteArchiveClip(ctx context.Context, clip media.ClipInfo) error {
	if s.runtime == nil {
		return fmt.Errorf("runtime services are not configured")
	}
	s.runtime.mu.RLock()
	reader := s.runtime.media
	s.runtime.mu.RUnlock()
	if deleter, ok := reader.(interface {
		DeleteArchiveClip(context.Context, media.ClipInfo) error
	}); ok {
		return deleter.DeleteArchiveClip(ctx, clip)
	}
	return fmt.Errorf("media layer does not support archive clip cleanup")
}
