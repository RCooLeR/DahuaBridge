package app

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/streams"
)

type runtimeDirectClipStarter interface {
	Enabled() bool
	StartDirectClip(context.Context, media.DirectClipStartRequest) (media.ClipInfo, error)
	FindClips(media.ClipQuery) ([]media.ClipInfo, error)
}

func (r *runtimeServices) EnsureNVRArchiveClip(ctx context.Context, deviceID string, item dahua.NVRRecording) (media.ClipInfo, error) {
	r.mu.RLock()
	mediaReader := r.media
	r.mu.RUnlock()

	starter, ok := mediaReader.(runtimeDirectClipStarter)
	if !ok || starter == nil || !starter.Enabled() {
		return media.ClipInfo{}, fmt.Errorf("media layer is not configured")
	}

	startTime, startErr := time.ParseInLocation(bridgeRecordingTimeLayout, strings.TrimSpace(item.StartTime), time.Local)
	endTime, endErr := time.ParseInLocation(bridgeRecordingTimeLayout, strings.TrimSpace(item.EndTime), time.Local)
	if startErr != nil || endErr != nil || !endTime.After(startTime) {
		return media.ClipInfo{}, fmt.Errorf("invalid archive clip window")
	}

	request := dahua.NVRPlaybackSessionRequest{
		Channel:     item.Channel,
		StartTime:   startTime,
		EndTime:     endTime,
		FilePath:    strings.TrimSpace(item.FilePath),
		Source:      strings.TrimSpace(item.Source),
		Type:        strings.TrimSpace(item.Type),
		VideoStream: strings.TrimSpace(item.VideoStream),
	}

	streamID := runtimeArchiveExportStreamID(deviceID, request)
	if clips, err := starter.FindClips(media.ClipQuery{
		StreamID:  streamID,
		StartTime: request.StartTime.UTC().Add(-2 * time.Second),
		EndTime:   request.EndTime.UTC().Add(2 * time.Second),
		Limit:     8,
	}); err == nil {
		for _, clip := range clips {
			if clip.StreamID != streamID || !clipMatchesWindow(clip, request.StartTime, request.EndTime) {
				continue
			}
			_ = r.TrackNVRArchiveClip(ctx, deviceID, request, clip)
			return clip, nil
		}
	}

	download, err := r.NVRDownloadRecordingClip(ctx, deviceID, dahua.NVRRecordingClipRequest{
		Channel:     request.Channel,
		StartTime:   request.StartTime,
		EndTime:     request.EndTime,
		FilePath:    request.FilePath,
		Source:      request.Source,
		Type:        request.Type,
		VideoStream: request.VideoStream,
	})
	if err != nil {
		return media.ClipInfo{}, err
	}
	defer download.Body.Close()

	tempDir := strings.TrimSpace(r.cfg.Archive.TempDir)
	if tempDir != "" {
		if err := os.MkdirAll(tempDir, 0o755); err != nil {
			return media.ClipInfo{}, fmt.Errorf("create archive temporary directory: %w", err)
		}
	}
	tempFile, err := os.CreateTemp(tempDir, "dahuabridge-recording-*.dav")
	if err != nil {
		return media.ClipInfo{}, fmt.Errorf("create temporary recording file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanupTemp := true
	cleanupPaths := []string{tempPath}
	defer func() {
		_ = tempFile.Close()
		if cleanupTemp {
			for _, cleanupPath := range cleanupPaths {
				if strings.TrimSpace(cleanupPath) != "" {
					_ = os.Remove(cleanupPath)
				}
			}
		}
	}()

	if _, err := io.Copy(tempFile, download.Body); err != nil {
		return media.ClipInfo{}, fmt.Errorf("copy recording to temporary file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return media.ClipInfo{}, fmt.Errorf("close temporary recording file: %w", err)
	}

	entry, streamProfile := r.lookupNVRArchiveStreamProfile(deviceID, request.Channel, "quality")
	sourceStartAt := request.StartTime
	sourceEndAt := request.EndTime
	inputSeekOffset := time.Duration(0)
	if fileStart, fileEnd, ok := dahua.ParseRecordingFileTimeRange(request.FilePath, time.Local); ok {
		if request.StartTime.After(fileStart) {
			inputSeekOffset = request.StartTime.Sub(fileStart)
		}
		if !fileEnd.IsZero() && sourceEndAt.After(fileEnd) {
			sourceEndAt = fileEnd
		}
	}
	duration := request.EndTime.Sub(request.StartTime)
	if sourceEndAt.After(sourceStartAt) {
		if clippedDuration := sourceEndAt.Sub(sourceStartAt); clippedDuration > 0 && (duration <= 0 || clippedDuration < duration) {
			duration = clippedDuration
		}
	}

	prefixSourceURL, prefixDuration := r.downloadOptionalArchiveIFrame(ctx, deviceID, request, tempDir)
	if strings.TrimSpace(prefixSourceURL) != "" {
		cleanupPaths = append(cleanupPaths, prefixSourceURL)
	}

	clip, err := starter.StartDirectClip(ctx, media.DirectClipStartRequest{
		StreamID:                      streamID,
		RootDeviceID:                  firstNonEmptyPlayback(strings.TrimSpace(entry.RootDeviceID), deviceID),
		SourceDeviceID:                firstNonEmptyPlayback(strings.TrimSpace(entry.ID), fmt.Sprintf("%s_channel_%02d", deviceID, request.Channel)),
		DeviceKind:                    dahua.DeviceKindNVRChannel,
		Name:                          firstNonEmptyPlayback(strings.TrimSpace(entry.Name), fmt.Sprintf("Channel %d", request.Channel)),
		Channel:                       request.Channel,
		ProfileName:                   "quality",
		Duration:                      duration,
		SourceURL:                     tempPath,
		VideoCodec:                    streamProfile.VideoCodec,
		AudioCodec:                    streamProfile.AudioCodec,
		SourceWidth:                   streamProfile.SourceWidth,
		SourceHeight:                  streamProfile.SourceHeight,
		Recommended:                   streamProfile.Recommended,
		SourceStartAt:                 sourceStartAt,
		SourceEndAt:                   sourceEndAt,
		InputSeekOffset:               inputSeekOffset,
		PrefixSourceURL:               prefixSourceURL,
		PrefixDuration:                prefixDuration,
		TemporarySourcePathsToCleanup: cleanupPaths,
	})
	if err != nil {
		if errors.Is(err, media.ErrClipAlreadyActive) {
			if clips, findErr := starter.FindClips(media.ClipQuery{
				StreamID:  streamID,
				StartTime: request.StartTime.UTC().Add(-2 * time.Second),
				EndTime:   request.EndTime.UTC().Add(2 * time.Second),
				Limit:     8,
			}); findErr == nil {
				for _, existing := range clips {
					if existing.StreamID == streamID && clipMatchesWindow(existing, request.StartTime, request.EndTime) {
						_ = r.TrackNVRArchiveClip(ctx, deviceID, request, existing)
						return existing, nil
					}
				}
			}
		}
		return media.ClipInfo{}, err
	}
	if err := r.TrackNVRArchiveClip(ctx, deviceID, request, clip); err != nil {
		return media.ClipInfo{}, err
	}
	cleanupTemp = false
	return clip, nil
}

func (r *runtimeServices) downloadOptionalArchiveIFrame(ctx context.Context, deviceID string, request dahua.NVRPlaybackSessionRequest, tempDir string) (string, time.Duration) {
	if !shouldUseOptionalArchiveIFrame(request) {
		return "", 0
	}

	download, err := r.NVRDownloadRecordingIFrame(ctx, deviceID, dahua.NVRRecordingClipRequest{
		Channel:     request.Channel,
		StartTime:   request.StartTime,
		EndTime:     request.EndTime,
		Source:      request.Source,
		Type:        request.Type,
		VideoStream: request.VideoStream,
	})
	if err != nil {
		return "", 0
	}
	defer download.Body.Close()

	tempFile, err := os.CreateTemp(tempDir, "dahuabridge-iframe-*.dav")
	if err != nil {
		return "", 0
	}
	defer tempFile.Close()

	written, err := io.Copy(tempFile, download.Body)
	if err != nil {
		_ = os.Remove(tempFile.Name())
		return "", 0
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", 0
	}
	if written <= 0 {
		_ = os.Remove(tempFile.Name())
		return "", 0
	}
	return tempFile.Name(), media.ArchiveIFramePrefixDuration
}

func (r *runtimeServices) lookupNVRArchiveStreamProfile(deviceID string, channel int, profileName string) (streams.Entry, streams.Profile) {
	profileName = strings.TrimSpace(profileName)
	for _, entry := range r.ListStreams(false) {
		if entry.DeviceKind != dahua.DeviceKindNVRChannel {
			continue
		}
		if entry.RootDeviceID != deviceID || entry.Channel != channel {
			continue
		}
		if profileName != "" {
			if profile, ok := entry.Profiles[profileName]; ok {
				return entry, profile
			}
		}
		if recommended := strings.TrimSpace(entry.RecommendedProfile); recommended != "" {
			if profile, ok := entry.Profiles[recommended]; ok {
				return entry, profile
			}
		}
		for _, profile := range entry.Profiles {
			return entry, profile
		}
		return entry, streams.Profile{}
	}
	return streams.Entry{}, streams.Profile{}
}

func runtimeArchiveExportStreamID(deviceID string, request dahua.NVRPlaybackSessionRequest) string {
	base := firstNonEmptyPlayback(strings.TrimSpace(deviceID), "nvr")
	identity := strings.Join([]string{
		base,
		fmt.Sprintf("%d", request.Channel),
		request.StartTime.UTC().Format(time.RFC3339Nano),
		request.EndTime.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(request.FilePath),
		strings.TrimSpace(request.Source),
		strings.TrimSpace(request.Type),
		strings.TrimSpace(request.VideoStream),
	}, "|")
	sum := sha1.Sum([]byte(identity))
	return fmt.Sprintf("nvr_export_%s_%x", base, sum[:8])
}

func shouldUseOptionalArchiveIFrame(request dahua.NVRPlaybackSessionRequest) bool {
	if request.Channel <= 0 || request.StartTime.IsZero() || request.EndTime.IsZero() || !request.EndTime.After(request.StartTime) {
		return false
	}
	source := strings.ToLower(strings.TrimSpace(request.Source))
	recordingType := strings.ToLower(strings.TrimSpace(request.Type))
	return source == "nvr_event" || recordingType == "event" || strings.HasPrefix(recordingType, "event.")
}

func clipMatchesWindow(clip media.ClipInfo, startTime time.Time, endTime time.Time) bool {
	clipStart := clip.SourceStartAt
	if clipStart.IsZero() {
		clipStart = clip.StartedAt
	}
	clipEnd := clip.SourceEndAt
	if clipEnd.IsZero() {
		clipEnd = clip.EndedAt
	}
	if clipStart.IsZero() || clipEnd.IsZero() || !clipEnd.After(clipStart) {
		return false
	}
	const tolerance = 2 * time.Second
	return timeDeltaWithin(clipStart, startTime.UTC(), tolerance) &&
		timeDeltaWithin(clipEnd, endTime.UTC(), tolerance)
}

func timeDeltaWithin(left time.Time, right time.Time, tolerance time.Duration) bool {
	delta := left.Sub(right)
	if delta < 0 {
		delta = -delta
	}
	return delta <= tolerance
}
