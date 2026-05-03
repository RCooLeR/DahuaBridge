package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
)

func (job *clipJob) run(parent *Manager, profile streams.Profile, duration time.Duration, started chan<- error) {
	defer close(job.done)
	defer parent.removeClipJob(job.info.ID, job)

	if duration <= 0 {
		if playbackDuration, ok := playbackDurationFromStreamURL(profile.StreamURL); ok {
			duration = playbackDuration
		}
	}
	waitErr := job.runFFmpegAttempt(parent, profile, duration, started, true)
	if waitErr != nil && strings.TrimSpace(profile.InputPrefixURL) != "" {
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
	disableStdin := duration > 0
	includeAudio := parent.shouldIncludeSourceAudio(profile, job.logger)
	if strings.TrimSpace(profile.InputPrefixURL) != "" {
		includeAudio = false
	}
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
		Str("output_video_encoder", "libx264").
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

	cmd := exec.Command(parent.cfg.FFmpegPath, args...)
	var stdin io.WriteCloser
	var err error
	if !disableStdin {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			if notifyStarted {
				started <- fmt.Errorf("ffmpeg stdin pipe: %w", err)
			}
			return fmt.Errorf("ffmpeg stdin pipe: %w", err)
		}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if notifyStarted {
			started <- fmt.Errorf("ffmpeg stderr pipe: %w", err)
		}
		return fmt.Errorf("ffmpeg stderr pipe: %w", err)
	}
	cmd.Stdout = io.Discard

	job.mu.Lock()
	job.stdin = stdin
	job.cmd = cmd
	job.mu.Unlock()

	if err := cmd.Start(); err != nil {
		if notifyStarted {
			started <- fmt.Errorf("start ffmpeg: %w", err)
		}
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	job.logger.Debug().
		Str("clip_id", job.info.ID).
		Str("source_video_codec", profile.VideoCodec).
		Str("source_audio_codec", profile.AudioCodec).
		Int("source_width", profile.SourceWidth).
		Int("source_height", profile.SourceHeight).
		Str("output_video_codec", "H.264").
		Str("output_video_encoder", "libx264").
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
	if waitErr != nil && stderrText != "" {
		waitErr = fmt.Errorf("%w: %s", waitErr, stderrText)
	}
	if waitErr == nil {
		if validationErr := job.validateClipOutput(parent, profile); validationErr != nil {
			waitErr = validationErr
		}
	}

	job.mu.Lock()
	job.stdin = nil
	job.cmd = nil
	job.mu.Unlock()

	return waitErr
}

func (job *clipJob) validateClipOutput(parent *Manager, profile streams.Profile) error {
	expectedDuration := job.expectedOutputDuration()
	if expectedDuration <= 2*time.Second {
		return nil
	}

	probedDuration, err := probeMediaDuration(parent.cfg.FFmpegPath, job.outputPath, audioProbeTimeout(parent.cfg.StartTimeout))
	if err != nil {
		job.logger.Warn().
			Err(err).
			Str("clip_id", job.info.ID).
			Str("output_path", job.outputPath).
			Msg("clip output duration probe failed")
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
	stdin := job.stdin
	cmd := job.cmd
	done := job.done
	job.mu.Unlock()

	if stdin != nil {
		job.logger.Info().Str("clip_id", job.info.ID).Msg("clip stop requested")
		_, _ = io.WriteString(stdin, "q\n")
		_ = stdin.Close()
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	case <-timer.C:
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return nil
	}
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
		OutputVideoEncoder: "libx264",
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
		return buildPrefixedClipFFmpegArgs(cfg, profile, duration, outputPath, disableStdin)
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
	args = append(args,
		"-map", "0:v:0",
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

func buildPrefixedClipFFmpegArgs(cfg config.MediaConfig, profile streams.Profile, duration time.Duration, outputPath string, disableStdin bool) []string {
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
	args = append(args,
		"-filter_complex",
		"[0:v:0]setpts=PTS-STARTPTS[v0];[1:v:0]"+fullSourceStartFilter+"[v1];[v0][v1]concat=n=2:v=1:a=0[v]",
		"-map", "[v]",
	)
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
		"-an",
		"-y",
		outputPath,
	)
	return args
}

func newClipID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "clip_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return "clip_" + hex.EncodeToString(buffer)
}
