package httpserver

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/streams"

	"github.com/go-chi/chi/v5"
)

type archiveClipTracker interface {
	TrackNVRArchiveClip(context.Context, string, dahua.NVRPlaybackSessionRequest, mediaapi.ClipInfo) error
}

type nvrArchiveIFrameDownloader interface {
	NVRDownloadRecordingIFrame(context.Context, string, dahua.NVRRecordingClipRequest) (dahua.NVRRecordingDownload, error)
}

type nvrArchiveInspector interface {
	NVREventSummary(context.Context, string, time.Time, time.Time, string) (dahua.NVREventSummary, error)
	NVRArchiveCoverage(context.Context, string, int) (dahua.NVRArchiveCoverage, error)
}

func (c *controller) registerDeviceRoutes(router chi.Router) {
	router.Get("/api/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, c.probes.List())
	})
	router.Get("/api/v1/devices/{deviceID}", func(w http.ResponseWriter, r *http.Request) {
		deviceID := chi.URLParam(r, "deviceID")
		result, ok := c.probes.Get(deviceID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/devices/probe-all", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "action layer is not configured"})
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		results := c.actions.ProbeAllDevices(actionCtx)
		successCount := 0
		errorCount := 0
		for _, result := range results {
			if strings.TrimSpace(result.Error) == "" {
				successCount++
			} else {
				errorCount++
			}
		}

		statusCode := http.StatusOK
		statusText := "ok"
		if errorCount > 0 {
			statusCode = http.StatusMultiStatus
			statusText = "partial_error"
		}

		writeJSON(w, statusCode, map[string]any{
			"status":        statusText,
			"device_count":  len(results),
			"success_count": successCount,
			"error_count":   errorCount,
			"results":       results,
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/devices/{deviceID}/probe", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := c.actions.ProbeDevice(actionCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/devices/{deviceID}/credentials", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		var update dahua.DeviceConfigUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid json body")
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := c.actions.RotateDeviceCredentials(actionCtx, chi.URLParam(r, "deviceID"), update)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
}

func (c *controller) registerNVRRoutes(router chi.Router) {
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/inventory/refresh", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		actionCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := c.actions.RefreshNVRInventory(actionCtx, chi.URLParam(r, "deviceID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"result": result,
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Get("/api/v1/nvr/{deviceID}/recordings", func(w http.ResponseWriter, r *http.Request) {
		query, err := parseNVRRecordingQuery(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		searchCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		result, err := c.snapshots.NVRRecordings(searchCtx, chi.URLParam(r, "deviceID"), query)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		normalizeNVRRecordingSearchResult(&result)
		attachNVRRecordingExportURLs(r, chi.URLParam(r, "deviceID"), &result)
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Get("/api/v1/nvr/{deviceID}/events/summary", func(w http.ResponseWriter, r *http.Request) {
		query, err := parseNVREventSummaryQuery(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		summaryCtx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()

		inspector, ok := c.snapshots.(nvrArchiveInspector)
		if !ok {
			writeServiceUnavailableError(w, "archive summary is not configured")
			return
		}
		summary, err := inspector.NVREventSummary(
			summaryCtx,
			chi.URLParam(r, "deviceID"),
			query.StartTime,
			query.EndTime,
			query.EventCode,
		)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, summary)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Get("/api/v1/nvr/{deviceID}/recordings/coverage", func(w http.ResponseWriter, r *http.Request) {
		channel, err := parseOptionalPositiveInt(r.URL.Query().Get("channel"))
		if err != nil || channel <= 0 {
			writeInvalidRequestError(w, fmt.Errorf("invalid channel"))
			return
		}

		coverageCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		inspector, ok := c.snapshots.(nvrArchiveInspector)
		if !ok {
			writeServiceUnavailableError(w, "archive coverage is not configured")
			return
		}
		coverage, err := inspector.NVRArchiveCoverage(coverageCtx, chi.URLParam(r, "deviceID"), channel)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, coverage)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/nvr/{deviceID}/recordings/export", func(w http.ResponseWriter, r *http.Request) {
		if c.media == nil || !c.media.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "media layer is disabled"})
			return
		}

		playbackRequest, profile, duration, err := parseNVRRecordingExportRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		deviceID := chi.URLParam(r, "deviceID")
		if strings.TrimSpace(playbackRequest.FilePath) != "" {
			clip, err := c.startDirectNVRRecordingExport(
				r.Context(),
				deviceID,
				playbackRequest,
				profile,
				duration,
			)
			if err != nil {
				writeClassifiedActionError(w, err, http.StatusBadGateway)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "ok",
				"clip":   clipAPIResponse(r, clip),
			})
			return
		}

		session, err := c.snapshots.CreateNVRPlaybackSession(r.Context(), deviceID, playbackRequest)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		clip, err := c.media.StartClip(r.Context(), mediaapi.ClipStartRequest{
			StreamID:    session.StreamID,
			ProfileName: profile,
			Duration:    duration,
		})
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		if tracker, ok := c.snapshots.(archiveClipTracker); ok {
			if err := tracker.TrackNVRArchiveClip(r.Context(), deviceID, playbackRequest, clip); err != nil {
				writeClassifiedActionError(w, err, http.StatusBadGateway)
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"session": session,
			"clip":    clipAPIResponse(r, clip),
		})
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/nvr/{deviceID}/recordings/download", func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimSpace(r.URL.Query().Get("file_path"))
		if filePath == "" {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "file_path is required")
			return
		}

		download, err := c.snapshots.NVRDownloadRecording(r.Context(), chi.URLParam(r, "deviceID"), filePath)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		defer download.Body.Close()

		contentType := strings.TrimSpace(download.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		fileName := strings.TrimSpace(download.FileName)
		if fileName == "" {
			fileName = "recording.dav"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(fileName)+`"`)
		if download.ContentLength > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(download.ContentLength, 10))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, download.Body)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Get("/api/v1/nvr/{deviceID}/channels/{channel}/controls", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}

		controlCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		capabilities, err := c.actions.NVRChannelControlCapabilities(controlCtx, chi.URLParam(r, "deviceID"), channel)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, capabilities)
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/ptz", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		request, err := parseNVRPTZRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		request.Channel = channel

		controlCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		if err := c.actions.ControlNVRPTZ(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"device_id":   chi.URLParam(r, "deviceID"),
			"channel":     channel,
			"action":      request.Action,
			"command":     request.Command,
			"speed":       request.Speed,
			"duration_ms": request.Duration.Milliseconds(),
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/aux", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		request, err := parseNVRAuxRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		request.Channel = channel

		controlCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		if err := c.actions.ControlNVRAux(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"device_id":   chi.URLParam(r, "deviceID"),
			"channel":     channel,
			"action":      request.Action,
			"output":      request.Output,
			"duration_ms": request.Duration.Milliseconds(),
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/audio/mute", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}
		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		muted, err := parseVTOMuteRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}
		controlCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := c.actions.ControlNVRAudio(controlCtx, chi.URLParam(r, "deviceID"), dahua.NVRAudioRequest{
			Channel: channel,
			Muted:   muted,
		}); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":               "ok",
			"device_id":            chi.URLParam(r, "deviceID"),
			"channel":              channel,
			"muted":                muted,
			"stream_audio_enabled": !muted,
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/recording", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		request, err := parseNVRRecordingRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		request.Channel = channel

		controlCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		if err := c.actions.ControlNVRRecording(controlCtx, chi.URLParam(r, "deviceID"), request); err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"device_id": chi.URLParam(r, "deviceID"),
			"channel":   channel,
			"action":    request.Action,
		})
	})
	router.With(rateLimitMiddleware(c.adminLimiter)).Post("/api/v1/nvr/{deviceID}/channels/{channel}/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		if c.actions == nil {
			writeServiceUnavailableError(w, "action layer is not configured")
			return
		}

		request, err := parseNVRDiagnosticActionRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		channel, err := strconv.Atoi(chi.URLParam(r, "channel"))
		if err != nil || channel <= 0 {
			writeErrorPayload(w, http.StatusBadRequest, "invalid_request", "invalid channel")
			return
		}
		request.Channel = channel

		controlCtx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()

		result, err := c.actions.NVRDiagnosticAction(controlCtx, chi.URLParam(r, "deviceID"), request)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/nvr/{deviceID}/playback/sessions", func(w http.ResponseWriter, r *http.Request) {
		request, err := parseNVRPlaybackSessionRequest(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		session, err := c.snapshots.CreateNVRPlaybackSession(r.Context(), chi.URLParam(r, "deviceID"), request)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Get("/api/v1/nvr/playback/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		session, err := c.snapshots.GetNVRPlaybackSession(chi.URLParam(r, "sessionID"))
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadGateway)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
	router.With(rateLimitMiddleware(c.mediaLimiter)).Post("/api/v1/nvr/playback/sessions/{sessionID}/seek", func(w http.ResponseWriter, r *http.Request) {
		seekTime, err := parseNVRPlaybackSeekTime(r)
		if err != nil {
			writeInvalidRequestError(w, err)
			return
		}

		session, err := c.snapshots.SeekNVRPlaybackSession(r.Context(), chi.URLParam(r, "sessionID"), seekTime)
		if err != nil {
			writeClassifiedActionError(w, err, http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, session)
	})
}

func (c *controller) startDirectNVRRecordingExport(
	ctx context.Context,
	deviceID string,
	request dahua.NVRPlaybackSessionRequest,
	profileName string,
	duration time.Duration,
) (mediaapi.ClipInfo, error) {
	if c.media == nil {
		return mediaapi.ClipInfo{}, fmt.Errorf("media layer is not configured")
	}

	download, err := c.snapshots.NVRDownloadRecordingClip(ctx, deviceID, dahua.NVRRecordingClipRequest{
		Channel:     request.Channel,
		StartTime:   request.StartTime,
		EndTime:     request.EndTime,
		FilePath:    request.FilePath,
		Source:      request.Source,
		Type:        request.Type,
		VideoStream: request.VideoStream,
	})
	if err != nil {
		return mediaapi.ClipInfo{}, err
	}
	defer download.Body.Close()

	tempDir := strings.TrimSpace(c.archiveTempDir)
	if tempDir != "" {
		if err := os.MkdirAll(tempDir, 0o755); err != nil {
			return mediaapi.ClipInfo{}, fmt.Errorf("create archive temporary directory: %w", err)
		}
	}
	tempFile, err := os.CreateTemp(tempDir, "dahuabridge-recording-*.dav")
	if err != nil {
		return mediaapi.ClipInfo{}, fmt.Errorf("create temporary recording file: %w", err)
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
		return mediaapi.ClipInfo{}, fmt.Errorf("copy recording to temporary file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return mediaapi.ClipInfo{}, fmt.Errorf("close temporary recording file: %w", err)
	}

	entry, streamProfile := c.lookupNVRStreamProfile(deviceID, request.Channel, profileName)
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
	if sourceEndAt.After(sourceStartAt) {
		if clippedDuration := sourceEndAt.Sub(sourceStartAt); clippedDuration > 0 && (duration <= 0 || clippedDuration < duration) {
			duration = clippedDuration
		}
	}

	prefixSourceURL, prefixDuration, err := c.downloadOptionalNVRRecordingIFrame(ctx, deviceID, request, tempDir)
	if err != nil {
		return mediaapi.ClipInfo{}, err
	}
	if strings.TrimSpace(prefixSourceURL) != "" {
		cleanupPaths = append(cleanupPaths, prefixSourceURL)
	}

	clip, err := c.media.StartDirectClip(ctx, mediaapi.DirectClipStartRequest{
		StreamID:                      archiveExportStreamID(deviceID, request),
		RootDeviceID:                  firstNonEmpty(strings.TrimSpace(entry.RootDeviceID), deviceID),
		SourceDeviceID:                firstNonEmpty(strings.TrimSpace(entry.ID), fmt.Sprintf("%s_channel_%02d", deviceID, request.Channel)),
		DeviceKind:                    dahua.DeviceKindNVRChannel,
		Name:                          firstNonEmpty(strings.TrimSpace(entry.Name), fmt.Sprintf("Channel %d", request.Channel)),
		Channel:                       request.Channel,
		ProfileName:                   profileName,
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
		return mediaapi.ClipInfo{}, err
	}
	cleanupTemp = false
	if tracker, ok := c.snapshots.(archiveClipTracker); ok {
		if err := tracker.TrackNVRArchiveClip(ctx, deviceID, request, clip); err != nil {
			return mediaapi.ClipInfo{}, err
		}
	}
	return clip, nil
}

func (c *controller) downloadOptionalNVRRecordingIFrame(
	ctx context.Context,
	deviceID string,
	request dahua.NVRPlaybackSessionRequest,
	tempDir string,
) (string, time.Duration, error) {
	if !shouldUseOptionalNVRRecordingIFrame(request) {
		return "", 0, nil
	}

	iframeDownloader, ok := c.snapshots.(nvrArchiveIFrameDownloader)
	if !ok {
		return "", 0, nil
	}

	download, err := iframeDownloader.NVRDownloadRecordingIFrame(ctx, deviceID, dahua.NVRRecordingClipRequest{
		Channel:     request.Channel,
		StartTime:   request.StartTime,
		EndTime:     request.EndTime,
		Source:      request.Source,
		Type:        request.Type,
		VideoStream: request.VideoStream,
	})
	if err != nil {
		return "", 0, nil
	}
	defer download.Body.Close()

	tempFile, err := os.CreateTemp(tempDir, "dahuabridge-iframe-*.dav")
	if err != nil {
		return "", 0, fmt.Errorf("create temporary iframe file: %w", err)
	}
	defer tempFile.Close()

	written, err := io.Copy(tempFile, download.Body)
	if err != nil {
		_ = os.Remove(tempFile.Name())
		return "", 0, fmt.Errorf("copy iframe recording to temporary file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", 0, fmt.Errorf("close temporary iframe file: %w", err)
	}
	if written <= 0 {
		_ = os.Remove(tempFile.Name())
		return "", 0, nil
	}
	return tempFile.Name(), mediaapi.ArchiveIFramePrefixDuration, nil
}

func archiveExportStreamID(deviceID string, request dahua.NVRPlaybackSessionRequest) string {
	base := firstNonEmpty(strings.TrimSpace(deviceID), "nvr")
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

func shouldUseOptionalNVRRecordingIFrame(request dahua.NVRPlaybackSessionRequest) bool {
	if request.Channel <= 0 || request.StartTime.IsZero() || request.EndTime.IsZero() || !request.EndTime.After(request.StartTime) {
		return false
	}
	source := strings.ToLower(strings.TrimSpace(request.Source))
	recordingType := strings.ToLower(strings.TrimSpace(request.Type))
	return source == "nvr_event" || recordingType == "event" || strings.HasPrefix(recordingType, "event.")
}

func (c *controller) lookupNVRStreamProfile(deviceID string, channel int, profileName string) (streams.Entry, streams.Profile) {
	profileName = strings.TrimSpace(profileName)
	for _, entry := range c.snapshots.ListStreams(false) {
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
