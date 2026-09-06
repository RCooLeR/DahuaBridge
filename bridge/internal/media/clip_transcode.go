package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"uuid"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
)

const minimumClipOutputBytesOnProbeFailure int64 = 4 * 1024

func (job *clipJob) run(parent *Manager, profile streams.Profile, duration time.Duration, started chan<- error) {
	defer job.cancel()
	defer close(job.done)
	defer parent.removeClipJob(job.info.ID, job)

	if duration <= 0 {
		if playbackDuration, ok := playbackDurationFromStreamURL(profile.StreamURL); ok {
			duration = playbackDuration
		}
	}
	waitErr := job.runFFmpegAttempt(parent, profile, duration, started, true)
	if waitErr != nil && job.canRetry() && canRemuxClipVideo(profile) {
		job.logger.Info().Msg("clip remux failed validation; retrying with video encoding")
		_ = os.Remove(job.outputPath)
		profile.ForceVideoTranscode = true
		waitErr = job.runFFmpegAttempt(parent, profile, duration, started, false)
	}
	if waitErr != nil && job.canRetry() && strings.TrimSpace(profile.InputPrefixURL) != "" {
		job.logger.Warn().
			Err(waitErr).
			Str("clip_id", job.info.ID).
			Str("source_url", redactURLUserinfo(profile.StreamURL)).
			Str("prefix_url", profile.InputPrefixURL).
			Msg("prefixed iframe clip transcode failed; retrying without iframe prefix")
		_ = os.Remove(job.outputPath)
		retryProfile := profile
		retryProfile.InputPrefixURL = ""
		retryProfile.InputPrefixDuration = 0
		waitErr = job.runFFmpegAttempt(parent, retryProfile, duration, started, false)
	}
	job.complete(parent, waitErr)
}

func (job *clipJob) runFFmpegAttempt(parent *Manager, profile streams.Profile, duration time.Duration, started chan<- error, notifyStarted bool) error {
	// Even finite clips need stdin so shutdown can finalize their MP4 cleanly.
	const disableStdin = false
	includeAudio := parent.shouldIncludeSourceAudio(profile, job.logger)
	job.mu.Lock()
	job.includeAudio = includeAudio
	job.profile = profile
	job.mu.Unlock()
	args := buildClipFFmpegArgs(parent.cfg, profile, duration, job.outputPath, includeAudio, disableStdin)
	outputWidth, outputHeight := profile.SourceWidth, profile.SourceHeight
	job.logger.Debug().
		Str("source_url", redactURLUserinfo(profile.StreamURL)).
		Str("source_video_codec", profile.VideoCodec).
		Str("source_audio_codec", profile.AudioCodec).
		Int("source_width", profile.SourceWidth).
		Int("source_height", profile.SourceHeight).
		Str("output_video_codec", "H.264").
		Str("output_video_encoder", clipVideoEncoder(profile)).
		Str("output_audio_codec", conditionalCodec(includeAudio, "AAC")).
		Int("output_width", outputWidth).
		Int("output_height", outputHeight).
		Str("rtsp_transport", firstNonEmpty(profile.RTSPTransport, "tcp")).
		Str("input_preset", parent.cfg.InputPreset).
		Bool("hwaccel", false).
		Strs("ffmpeg_args", redactFFmpegArgs(args)).
		Msg("starting clip worker")
	job.logger.Debug().
		Str("clip_id", job.info.ID).
		Dur("duration", duration).
		Bool("include_audio", includeAudio).
		Bool("disable_stdin", disableStdin).
		Bool("iframe_prefix", strings.TrimSpace(profile.InputPrefixURL) != "").
		Msg("clip transcode starting")

	cmd := exec.CommandContext(job.ctx, parent.cfg.FFmpegPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		if notifyStarted {
			started <- fmt.Errorf("ffmpeg stdin pipe: %w", err)
		}
		return fmt.Errorf("ffmpeg stdin pipe: %w", err)
	}
	defer stdin.Close()
	// Each attempt owns its command and pipe. A stop cancels the shared job
	// context, finalizing this MP4 and preventing any fallback attempt.
	cmd.Cancel = func() error {
		_, err := io.WriteString(stdin, "q\n")
		_ = stdin.Close()
		return err
	}
	// FFmpeg can ignore stdin if its input is stuck; shutdown must still finish.
	cmd.WaitDelay = 5 * time.Second
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if notifyStarted {
			started <- fmt.Errorf("ffmpeg stderr pipe: %w", err)
		}
		return fmt.Errorf("ffmpeg stderr pipe: %w", err)
	}
	cmd.Stdout = io.Discard

	if err := cmd.Start(); err != nil {
		if notifyStarted {
			started <- fmt.Errorf("start ffmpeg: %w", err)
		}
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	job.mu.Lock()
	job.processID = cmd.Process.Pid
	job.mu.Unlock()
	job.logger.Debug().
		Str("clip_id", job.info.ID).
		Str("source_video_codec", profile.VideoCodec).
		Str("source_audio_codec", profile.AudioCodec).
		Int("source_width", profile.SourceWidth).
		Int("source_height", profile.SourceHeight).
		Str("output_video_codec", "H.264").
		Str("output_video_encoder", clipVideoEncoder(profile)).
		Str("output_audio_codec", conditionalCodec(includeAudio, "AAC")).
		Int("output_width", outputWidth).
		Int("output_height", outputHeight).
		Str("input_preset", parent.cfg.InputPreset).
		Bool("iframe_prefix", strings.TrimSpace(profile.InputPrefixURL) != "").
		Msg("clip transcode started")
	if notifyStarted {
		started <- nil
	}

	stderrDone := drainFFmpegStderr(stderr, 64*1024)

	waitErr := cmd.Wait()
	stderrText := <-stderrDone
	job.mu.Lock()
	stopped := job.stopping
	job.mu.Unlock()
	if stopped && errors.Is(waitErr, context.Canceled) {
		// A graceful user stop finalizes a shorter clip intentionally.
		waitErr = nil
	}
	if waitErr != nil && stderrText != "" {
		waitErr = fmt.Errorf("%w: %s", waitErr, stderrText)
	}
	if waitErr == nil && stopped {
		actual, err := probeMediaDuration(parent.cfg.FFmpegPath, job.outputPath, audioProbeTimeout(parent.cfg.StartTimeout))
		if err != nil || actual <= 0 {
			waitErr = fmt.Errorf("stopped clip has no valid media duration: %v", err)
		} else {
			job.mu.Lock()
			job.info.Duration = actual
			if !job.info.SourceStartAt.IsZero() {
				job.info.SourceEndAt = job.info.SourceStartAt.Add(actual)
			}
			job.mu.Unlock()
		}
	}
	if waitErr == nil && !stopped {
		if validationErr := job.validateClipOutput(parent, profile); validationErr != nil {
			waitErr = validationErr
		}
	}

	return waitErr
}

func (job *clipJob) validateClipOutput(parent *Manager, profile streams.Profile) error {
	expectedDuration := job.expectedOutputDuration()
	if expectedDuration <= 2*time.Second {
		return nil
	}

	probedDuration, err := probeMediaDuration(parent.cfg.FFmpegPath, job.outputPath, audioProbeTimeout(parent.cfg.StartTimeout))
	if err != nil {
		sizeBytes, statErr := clipOutputSizeBytes(job.outputPath)
		job.logger.Warn().
			Err(err).
			Str("clip_id", job.info.ID).
			Str("output_path", job.outputPath).
			Int64("output_bytes", sizeBytes).
			Msg("clip output duration probe failed")
		if statErr != nil || sizeBytes < minimumClipOutputBytesOnProbeFailure {
			return fmt.Errorf("clip output validation failed after duration probe error: %w", err)
		}
		return nil
	}

	minDuration := minimumValidClipDuration(expectedDuration)
	if probedDuration >= minDuration {
		return nil
	}

	job.logger.Warn().
		Str("clip_id", job.info.ID).
		Dur("expected_duration", expectedDuration).
		Dur("minimum_valid_duration", minDuration).
		Dur("probed_duration", probedDuration).
		Bool("iframe_prefix", strings.TrimSpace(profile.InputPrefixURL) != "").
		Msg("clip output duration shorter than archive event window")

	return fmt.Errorf(
		"clip output duration %s is shorter than expected archive event duration %s",
		probedDuration.Round(100*time.Millisecond),
		expectedDuration.Round(100*time.Millisecond),
	)
}

func (job *clipJob) expectedOutputDuration() time.Duration {
	if !job.info.SourceStartAt.IsZero() && !job.info.SourceEndAt.IsZero() && job.info.SourceEndAt.After(job.info.SourceStartAt) {
		return job.info.SourceEndAt.Sub(job.info.SourceStartAt)
	}
	return job.info.Duration
}

func minimumValidClipDuration(expected time.Duration) time.Duration {
	if expected <= 0 {
		return 0
	}
	tolerance := expected / 5
	if tolerance < time.Second {
		tolerance = time.Second
	}
	if tolerance > 3*time.Second {
		tolerance = 3 * time.Second
	}
	minDuration := expected - tolerance
	if minDuration < 1500*time.Millisecond {
		return 1500 * time.Millisecond
	}
	return minDuration
}

func probeMediaDuration(ffmpegPath string, mediaPath string, timeout time.Duration) (time.Duration, error) {
	if strings.TrimSpace(mediaPath) == "" {
		return 0, fmt.Errorf("media path is empty")
	}
	probeCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		mediaPath,
	}
	cmd := exec.CommandContext(probeCtx, ffprobePath(ffmpegPath), args...)
	body, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("probe media duration: %w: %s", err, strings.TrimSpace(string(body)))
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse media duration: %w", err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("media duration is not positive")
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func clipOutputSizeBytes(mediaPath string) (int64, error) {
	if strings.TrimSpace(mediaPath) == "" {
		return 0, fmt.Errorf("media path is empty")
	}
	info, err := os.Stat(mediaPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (job *clipJob) complete(parent *Manager, waitErr error) {
	job.mu.Lock()
	defer job.mu.Unlock()
	defer func() {
		for _, path := range job.temporarySourcePaths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				job.logger.Warn().Err(err).Str("path", path).Msg("failed to cleanup temporary clip source")
			}
		}
		job.temporarySourcePaths = nil
	}()

	job.waitErr = waitErr
	job.info.EndedAt = time.Now().UTC()
	if waitErr != nil {
		job.info.Status = ClipStatusFailed
		job.info.Error = waitErr.Error()
	} else {
		job.info.Status = ClipStatusCompleted
		job.info.Error = ""
	}

	if stat, err := os.Stat(job.outputPath); err == nil {
		job.info.Bytes = stat.Size()
	}
	if err := parent.persistClip(job.info); err != nil {
		job.logger.Warn().Err(err).Msg("persist clip metadata failed")
	}
	if waitErr != nil {
		job.logger.Error().Err(waitErr).Msg("clip worker stopped")
		return
	}
	job.logger.Debug().
		Str("clip_id", job.info.ID).
		Int64("bytes", job.info.Bytes).
		Str("status", string(job.info.Status)).
		Msg("clip transcode completed")
}

func (job *clipJob) stop(ctx context.Context) error {
	job.mu.Lock()
	job.stopping = true
	cancel := job.cancel
	done := job.done
	job.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (job *clipJob) canRetry() bool {
	job.mu.Lock()
	defer job.mu.Unlock()
	return job.ctx.Err() == nil && !job.stopping && job.processID != 0
}

func (job *clipJob) snapshot() ClipInfo {
	job.mu.Lock()
	defer job.mu.Unlock()
	return job.info
}

func (job *clipJob) status() WorkerStatus {
	job.mu.Lock()
	defer job.mu.Unlock()

	frameRate := job.profile.FrameRate
	rtspTransport := firstNonEmpty(job.profile.RTSPTransport, "tcp")
	threads := 0
	maxWorkers := 0
	ffmpegPath := ""
	inputPreset := ""
	if job.parent != nil {
		frameRate = maxInt(job.profile.FrameRate, job.parent.cfg.FrameRate)
		threads = job.parent.cfg.Threads
		maxWorkers = job.parent.cfg.MaxWorkers
		ffmpegPath = job.parent.cfg.FFmpegPath
		inputPreset = job.parent.cfg.InputPreset
	}

	return WorkerStatus{
		ProcessID:          job.processID,
		Key:                job.info.ID,
		Format:             "clip",
		StreamID:           job.info.StreamID,
		Channel:            job.info.Channel,
		Profile:            job.info.Profile,
		SourceSubtype:      job.profile.Subtype,
		SourceVideoCodec:   job.profile.VideoCodec,
		SourceAudioCodec:   job.profile.AudioCodec,
		SourceWidth:        job.profile.SourceWidth,
		SourceHeight:       job.profile.SourceHeight,
		OutputVideoCodec:   "H.264",
		OutputVideoEncoder: clipVideoEncoder(job.profile),
		OutputAudioCodec:   conditionalCodec(job.includeAudio, "AAC"),
		OutputWidth:        job.profile.SourceWidth,
		OutputHeight:       job.profile.SourceHeight,
		RTSPTransport:      rtspTransport,
		InputPreset:        inputPreset,
		HWAccelActive:      false,
		AudioEnabled:       job.includeAudio,
		Viewers:            0,
		StartedAt:          job.info.StartedAt,
		LastAccessAt:       job.info.EndedAt,
		LastError:          job.info.Error,
		SourceURL:          redactURLUserinfo(job.profile.StreamURL),
		Recommended:        job.profile.Recommended,
		FrameRate:          frameRate,
		Threads:            threads,
		MaxWorkers:         maxWorkers,
		FFmpegPath:         ffmpegPath,
	}
}

func buildClipFFmpegArgs(cfg config.MediaConfig, profile streams.Profile, duration time.Duration, outputPath string, includeAudio bool, disableStdin bool) []string {
	if strings.TrimSpace(profile.InputPrefixURL) != "" {
		return buildPrefixedClipFFmpegArgs(cfg, profile, duration, outputPath, includeAudio, disableStdin)
	}

	args := []string{
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(cfg),
	}
	if disableStdin {
		args = append(args, "-nostdin")
	}
	args = append(args, buildInputArgsWithWallclock(profile, cfg.InputPreset, profile.UseWallclockAsTimestamps)...)
	if duration > 0 {
		args = append(args, "-t", formatFFmpegSeconds(duration))
	}

	// MP4 exports intentionally ignore live max-width. Resolution follows the selected source profile.
	args = append(args, "-map", "0:v:0")
	if canRemuxClipVideo(profile) {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-profile:v", "high")
	}
	args = append(args,
		"-tag:v", "avc1",
		"-movflags", "+faststart",
		"-y",
	)
	if includeAudio {
		args = append(args,
			"-map", "0:a:0?",
			"-c:a", "aac",
			"-b:a", "128k",
			"-ac", "2",
			"-ar", "48000",
		)
	} else {
		args = append(args, "-an")
	}
	args = append(args, outputPath)
	return args
}

// canRemuxClipVideo limits packet copy to an unchanged H.264 source timeline.
// Accurate trimming, prefix concatenation, or timestamp repair needs re-encoding;
// failed remux validation also forces that encoder path on the next attempt.
func canRemuxClipVideo(profile streams.Profile) bool {
	codec := strings.ToLower(strings.NewReplacer(".", "", "-", "").Replace(strings.TrimSpace(profile.VideoCodec)))
	return !profile.ForceVideoTranscode && (codec == "h264" || codec == "avc" || codec == "avc1") &&
		profile.InputSeekOffset == 0 && profile.InputPrefixURL == "" && !profile.UseWallclockAsTimestamps
}

func clipVideoEncoder(profile streams.Profile) string {
	if canRemuxClipVideo(profile) {
		return "copy"
	}
	return "libx264"
}

func buildPrefixedClipFFmpegArgs(cfg config.MediaConfig, profile streams.Profile, duration time.Duration, outputPath string, includeAudio bool, disableStdin bool) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(cfg),
	}
	if disableStdin {
		args = append(args, "-nostdin")
	}

	prefixDuration := time.Duration(profile.InputPrefixDuration)
	if prefixDuration <= 0 {
		prefixDuration = ArchiveIFramePrefixDuration
	}
	if duration > 0 && prefixDuration > duration {
		prefixDuration = duration
	}
	args = append(args,
		"-i", strings.TrimSpace(profile.InputPrefixURL),
		"-i", profile.StreamURL,
	)

	fullSourceStartFilter := "setpts=PTS-STARTPTS"
	if seekOffset := time.Duration(profile.InputSeekOffset) + prefixDuration; seekOffset > 0 {
		fullSourceStartFilter = "trim=start=" + formatFFmpegSeconds(seekOffset) + ",setpts=PTS-STARTPTS"
	}
	filterComplex := "[0:v:0]setpts=PTS-STARTPTS[v0];[1:v:0]" + fullSourceStartFilter + "[v1];[v0][v1]concat=n=2:v=1:a=0[v]"
	if includeAudio {
		audioStartFilter := "asetpts=PTS-STARTPTS"
		if seekOffset := time.Duration(profile.InputSeekOffset); seekOffset > 0 {
			audioStartFilter = "atrim=start=" + formatFFmpegSeconds(seekOffset) + ",asetpts=PTS-STARTPTS"
		}
		filterComplex += ";[1:a:0]" + audioStartFilter + "[a]"
	}
	args = append(args, "-filter_complex", filterComplex, "-map", "[v]")
	if includeAudio {
		args = append(args, "-map", "[a]")
	}
	if duration > 0 {
		args = append(args, "-t", formatFFmpegSeconds(duration))
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
		"-profile:v", "high",
		"-tag:v", "avc1",
		"-movflags", "+faststart",
		"-y",
	)
	if includeAudio {
		args = append(args,
			"-c:a", "aac",
			"-b:a", "128k",
			"-ac", "2",
			"-ar", "48000",
		)
	} else {
		args = append(args, "-an")
	}
	args = append(args, outputPath)
	return args
}

func newClipID() string {
	return "clip_" + uuid.NewV7().String()
}
