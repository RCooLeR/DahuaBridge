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
	disableStdin := duration > 0
	includeAudio := parent.shouldIncludeSourceAudio(profile, job.logger)
	job.mu.Lock()
	job.includeAudio = includeAudio
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
		Msg("clip transcode starting")

	cmd := exec.Command(parent.cfg.FFmpegPath, args...)
	var stdin io.WriteCloser
	var err error
	if !disableStdin {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			started <- fmt.Errorf("ffmpeg stdin pipe: %w", err)
			job.complete(parent, err)
			return
		}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		started <- fmt.Errorf("ffmpeg stderr pipe: %w", err)
		job.complete(parent, err)
		return
	}
	cmd.Stdout = io.Discard

	job.mu.Lock()
	job.stdin = stdin
	job.cmd = cmd
	job.mu.Unlock()

	if err := cmd.Start(); err != nil {
		started <- fmt.Errorf("start ffmpeg: %w", err)
		job.complete(parent, err)
		return
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
		Msg("clip transcode started")
	started <- nil

	stderrDone := drainFFmpegStderr(stderr, 64*1024)

	waitErr := cmd.Wait()
	stderrText := <-stderrDone
	if waitErr != nil && stderrText != "" {
		waitErr = fmt.Errorf("%w: %s", waitErr, stderrText)
	}
	job.complete(parent, waitErr)
}

func (job *clipJob) complete(parent *Manager, waitErr error) {
	job.mu.Lock()
	defer job.mu.Unlock()
	defer func() {
		if path := strings.TrimSpace(job.temporarySourcePath); path != "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				job.logger.Warn().Err(err).Str("path", path).Msg("failed to cleanup temporary clip source")
			}
			job.temporarySourcePath = ""
		}
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
	args := []string{
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(cfg),
	}
	if disableStdin {
		args = append(args, "-nostdin")
	}
	args = append(args, buildRTSPInputArgs(profile, cfg.InputPreset)...)
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

func newClipID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "clip_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return "clip_" + hex.EncodeToString(buffer)
}
