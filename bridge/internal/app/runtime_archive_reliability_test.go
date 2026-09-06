package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
)

type reliabilityClipStarter struct {
	stubRuntimeMedia
	clip   media.ClipInfo
	path   string
	starts int
	active bool
}

func (s *reliabilityClipStarter) Enabled() bool                            { return true }
func (s *reliabilityClipStarter) ActiveClip(string) (media.ClipInfo, bool) { return s.clip, s.active }
func (s *reliabilityClipStarter) FindClips(q media.ClipQuery) ([]media.ClipInfo, error) {
	clip := s.clip
	clip.StreamID = q.StreamID
	return []media.ClipInfo{clip}, nil
}
func (s *reliabilityClipStarter) ClipFilePath(string) (string, error) { return s.path, nil }
func (s *reliabilityClipStarter) StartDirectClip(context.Context, media.DirectClipStartRequest) (media.ClipInfo, error) {
	s.starts++
	return media.ClipInfo{ID: "retry", Status: media.ClipStatusRecording}, nil
}

func TestArchiveRetriesFailedAndMissingOutputs(t *testing.T) {
	for _, item := range []struct {
		name   string
		status media.ClipStatus
		file   bool
		starts int
		active bool
	}{
		{"failed", media.ClipStatusFailed, true, 1, false}, {"missing", media.ClipStatusCompleted, false, 1, false}, {"completed", media.ClipStatusCompleted, true, 0, false}, {"active", media.ClipStatusRecording, false, 0, true}, {"stale recording", media.ClipStatusRecording, false, 1, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			runtime, _ := liveSourceRuntime(t)
			start := time.Now().Add(-time.Hour).Truncate(time.Second)
			end := start.Add(time.Minute)
			starter := &reliabilityClipStarter{clip: media.ClipInfo{ID: "old", Status: item.status, SourceStartAt: start, SourceEndAt: end}, path: filepath.Join(t.TempDir(), "clip.mp4")}
			starter.active = item.active
			if item.file {
				if err := os.WriteFile(starter.path, []byte("video"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			runtime.AttachMedia(starter)
			clip, err := runtime.EnsureNVRArchiveClip(t.Context(), "nvr", dahua.NVRRecording{Source: "nvr_event", Channel: 5, StartTime: start.In(time.Local).Format(bridgeRecordingTimeLayout), EndTime: end.In(time.Local).Format(bridgeRecordingTimeLayout)})
			if err != nil {
				t.Fatal(err)
			}
			if starter.starts != item.starts {
				t.Fatalf("started %d retries, want %d; clip=%+v", starter.starts, item.starts, clip)
			}
		})
	}
}

func TestRecordingSearchCacheEvictsExpiredAndExcessQueries(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	for i := range 1000 {
		runtime.storeRecordingSearch(fmt.Sprint(i), dahua.NVRRecordingSearchResult{})
	}
	if len(runtime.recordingCache) > 128 {
		t.Fatal("cache grew without bound")
	}
	for key, entry := range runtime.recordingCache {
		entry.expiresAt = time.Now().Add(-time.Second)
		runtime.recordingCache[key] = entry
	}
	runtime.storeRecordingSearch("fresh", dahua.NVRRecordingSearchResult{ReturnedCount: 7})
	if len(runtime.recordingCache) != 1 {
		t.Fatal("unvisited expired queries survived insertion")
	}
	result, ok := runtime.cachedRecordingSearch("fresh")
	if !ok || result.ReturnedCount != 7 {
		t.Fatal("fresh query was lost")
	}
}
