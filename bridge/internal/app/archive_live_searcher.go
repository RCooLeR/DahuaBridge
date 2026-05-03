package app

import (
	"context"
	"fmt"
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
	return s.runtime.GetClip(clipID)
}
