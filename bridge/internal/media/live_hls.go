package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/streams"
)

func (m *Manager) getOrCreateHLSWorker(entry streams.Entry, profileName string, profile streams.Profile) (*hlsWorker, error) {
	key := entry.ID + ":" + profileName

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.hlsWorkers[key]; ok {
		existing.touch()
		return existing, nil
	}
	if m.cfg.MaxWorkers > 0 && m.activeWorkerCountLocked() >= m.cfg.MaxWorkers {
		err := fmt.Errorf("%w: %d active, max %d", ErrWorkerLimitReached, m.activeWorkerCountLocked(), m.cfg.MaxWorkers)
		if m.metrics != nil {
			m.metrics.ObserveMediaStart(entry.ID, profileName, err)
		}
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &hlsWorker{
		key:         key,
		streamID:    entry.ID,
		profileName: profileName,
		profile:     profile,
		parent:      m,
		logger: m.logger.With().
			Str("stream_id", entry.ID).
			Str("profile", profileName).
			Str("format", "hls").
			Logger(),
		ctx:          ctx,
		cancel:       cancel,
		lastAccessAt: time.Now(),
		startErr:     make(chan error, 1),
	}
	m.hlsWorkers[key] = w
	m.setMediaWorkerCountLocked()
	m.logWorkerInventoryLocked("added", w.status())
	if m.metrics != nil {
		m.metrics.ObserveMediaStart(entry.ID, profileName, nil)
	}
	go w.run()
	return w, nil
}

func (m *Manager) removeHLSWorker(key string, w *hlsWorker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.hlsWorkers[key]; ok && existing == w {
		delete(m.hlsWorkers, key)
		m.setMediaWorkerCountLocked()
		m.logWorkerInventoryLocked("removed", w.status())
		if m.metrics != nil {
			m.metrics.SetMediaViewers(w.streamID, w.profileName, 0)
		}
	}
}

func (w *hlsWorker) run() {
	outputDir := ""
	retainOutput := false
	includeAudio := w.parent.shouldIncludeSourceAudio(w.profile, w.logger)

	defer func() {
		if retainOutput && outputDir != "" && w.parent.cfg.HLSKeepAfterExit > 0 {
			keepFor := w.parent.cfg.HLSKeepAfterExit
			w.logger.Info().Str("hls_output_dir", outputDir).Dur("keep_after_exit", keepFor).Msg("retaining completed hls output")
			time.AfterFunc(keepFor, func() {
				w.parent.removeHLSWorker(w.key, w)
				if err := os.RemoveAll(outputDir); err != nil {
					w.logger.Warn().Err(err).Str("hls_output_dir", outputDir).Msg("failed to cleanup retained hls output")
				}
			})
			return
		}

		w.parent.removeHLSWorker(w.key, w)
		if outputDir != "" {
			if err := os.RemoveAll(outputDir); err != nil {
				w.logger.Warn().Err(err).Str("hls_output_dir", outputDir).Msg("failed to cleanup hls output")
			}
		}
	}()

	outputRoot := strings.TrimSpace(w.parent.cfg.HLSTmpDir)
	if outputRoot == "" {
		outputRoot = "/data/tmp/dahuabridge/hls"
	}
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		w.setError(fmt.Errorf("create hls temp root: %w", err))
		return
	}

	var err error
	outputDir, err = os.MkdirTemp(outputRoot, safeHLSDirectoryPrefix(w.key)+"-*")
	if err != nil {
		w.setError(fmt.Errorf("create hls temp dir: %w", err))
		return
	}
	w.logger.Debug().
		Str("worker_key", w.key).
		Str("hls_output_dir", outputDir).
		Bool("include_audio", includeAudio).
		Msg("prepared hls worker output")

	w.mu.Lock()
	w.outputDir = outputDir
	if w.startedAt.IsZero() {
		w.startedAt = time.Now()
	}
	w.mu.Unlock()

	go w.stopWhenIdle()

	attempts := buildFFmpegStartAttempts(w.parent.cfg)
	for index, attempt := range attempts {
		if index > 0 {
			w.logger.Debug().Bool("hwaccel", attempt.useHWAccel).Str("input_preset", attempt.inputPreset).Msg("starting hls fallback attempt")
		}

		args := w.buildFFmpegArgs(attempt, includeAudio)
		outputWidth, outputHeight := resolvedOutputDimensions(w.profile.SourceWidth, w.profile.SourceHeight, w.parent.cfg.ScaleWidth)
		w.logger.Debug().
			Bool("hwaccel", attempt.useHWAccel).
			Str("input_preset", attempt.inputPreset).
			Str("source_video_codec", w.profile.VideoCodec).
			Str("source_audio_codec", w.profile.AudioCodec).
			Int("source_width", w.profile.SourceWidth).
			Int("source_height", w.profile.SourceHeight).
			Str("output_video_codec", "H.264").
			Str("output_video_encoder", workerH264EncoderLabel(w.parent.cfg, attempt.useHWAccel)).
			Str("output_audio_codec", conditionalCodec(includeAudio, "AAC")).
			Int("output_width", outputWidth).
			Int("output_height", outputHeight).
			Str("hls_output_dir", outputDir).
			Strs("ffmpeg_args", redactFFmpegArgs(args)).
			Msg("starting hls worker")

		cmd := exec.CommandContext(w.ctx, w.parent.cfg.FFmpegPath, args...)
		cmd.Dir = outputDir

		stderr, err := cmd.StderrPipe()
		if err != nil {
			w.setError(fmt.Errorf("ffmpeg stderr pipe: %w", err))
			return
		}

		w.mu.Lock()
		w.cmd = cmd
		w.activeAttempt = attempt
		w.includeAudio = includeAudio
		w.mu.Unlock()

		if err := cmd.Start(); err != nil {
			w.setError(fmt.Errorf("start ffmpeg: %w", err))
			return
		}
		w.logger.Debug().
			Bool("hwaccel", attempt.useHWAccel).
			Str("input_preset", attempt.inputPreset).
			Bool("include_audio", includeAudio).
			Str("hls_output_dir", outputDir).
			Msg("hls ffmpeg started")

		stderrDone := drainFFmpegStderr(stderr, 128*1024)

		waitErr := cmd.Wait()
		stderrText := <-stderrDone

		if errors.Is(w.ctx.Err(), context.Canceled) {
			return
		}

		if stderrText != "" {
			w.logger.Debug().Bool("hwaccel", attempt.useHWAccel).Str("input_preset", attempt.inputPreset).Str("ffmpeg_stderr", stderrText).Msg("hls worker stderr")
		}

		if waitErr != nil {
			if stderrText != "" {
				waitErr = fmt.Errorf("%w: %s", waitErr, stderrText)
			}
			if index < len(attempts)-1 {
				w.logger.Warn().
					Bool("hwaccel", attempt.useHWAccel).
					Str("input_preset", attempt.inputPreset).
					Err(waitErr).
					Msg("hls worker attempt failed")
				continue
			}
			w.setError(waitErr)
			return
		}

		if w.isPlaybackStream() || w.parent.cfg.HLSKeepAfterExit > 0 {
			retainOutput = true
		}
		w.logger.Debug().Bool("retain_output", retainOutput).Msg("hls worker exited cleanly")
		return
	}
}

func (w *hlsWorker) buildFFmpegArgs(attempt ffmpegStartAttempt, includeAudio bool) []string {
	frameRate := w.parent.cfg.FrameRate
	if w.profile.FrameRate > 0 {
		frameRate = w.profile.FrameRate
	}
	segmentDuration := formatFFmpegSeconds(w.parent.cfg.HLSSegmentTime)
	segmentSeconds := int(maxInt(int(w.parent.cfg.HLSSegmentTime/time.Second), 1))
	gopSize := maxInt(frameRate*segmentSeconds, frameRate)
	hlsListSize := strconv.Itoa(w.parent.cfg.HLSListSize)
	hlsFlags := "append_list+delete_segments+independent_segments+omit_endlist+temp_file"
	if w.isPlaybackStream() {
		hlsListSize = "0"
		hlsFlags = "independent_segments+temp_file"
	}

	args := []string{
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(w.parent.cfg),
	}
	args = appendInputHWAccelArgs(args, w.parent.cfg, attempt.useHWAccel)
	args = append(args, buildInputArgsWithWallclock(w.profile, attempt.inputPreset, false)...)
	if playbackDuration, ok := playbackDurationFromProfile(w.profile); ok {
		args = append(args, "-t", formatFFmpegSeconds(playbackDuration))
	}
	if w.parent.cfg.Threads > 0 {
		args = append(args, "-threads", strconv.Itoa(w.parent.cfg.Threads))
	}
	args = appendVideoFilterArgs(args, w.parent.cfg, w.parent.cfg.ScaleWidth, w.profile, attempt.useHWAccel, frameRate)
	args = append(args, "-map", "0:v:0")
	args = appendVideoEncoderArgs(args, w.parent.cfg, attempt.useHWAccel, gopSize, "veryfast")
	args = append(args,
		"-fps_mode", "cfr",
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%s)", segmentDuration),
	)
	if includeAudio {
		args = append(args,
			"-map", "0:a:0?",
			"-c:a", "aac",
			"-b:a", "96k",
			"-ac", "2",
			"-ar", "48000",
		)
	} else {
		args = append(args, "-an")
	}
	args = append(args,
		"-hls_allow_cache", "0",
		"-muxpreload", "0",
		"-muxdelay", "0",
		"-f", "hls",
		"-hls_time", segmentDuration,
		"-hls_list_size", hlsListSize,
		"-hls_flags", hlsFlags,
		"-hls_segment_filename", "segment_%03d.ts",
		"index.m3u8",
	)
	return args
}

func (w *hlsWorker) isPlaybackStream() bool {
	return strings.HasPrefix(strings.TrimSpace(w.streamID), "nvrpb_")
}

func (w *hlsWorker) stopWhenIdle() {
	interval := w.parent.cfg.IdleTimeout / 4
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			lastAccessAt := w.lastAccessAt
			w.mu.Unlock()
			if time.Since(lastAccessAt) >= w.parent.cfg.IdleTimeout {
				w.logger.Debug().Msg("stopping idle media worker")
				w.cancel()
				return
			}
		}
	}
}

func (w *hlsWorker) touch() {
	w.mu.Lock()
	w.lastAccessAt = time.Now()
	w.mu.Unlock()
	if w.parent.metrics != nil {
		w.parent.metrics.SetMediaViewers(w.streamID, w.profileName, 1)
	}
}

func (w *hlsWorker) readFileWhenReady(ctx context.Context, fileName string) ([]byte, error) {
	w.touch()
	timeout := time.NewTimer(w.parent.cfg.StartTimeout)
	defer timeout.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		w.mu.Lock()
		outputDir := w.outputDir
		lastError := w.lastError
		w.mu.Unlock()

		if lastError != nil {
			return nil, lastError
		}
		if outputDir != "" {
			body, err := os.ReadFile(filepath.Join(outputDir, fileName))
			if err == nil && len(body) > 0 {
				w.touch()
				if fileName == "index.m3u8" {
					w.mu.Lock()
					if !w.playlistReadyLogged {
						w.playlistReadyLogged = true
						w.logger.Debug().Str("file_name", fileName).Msg("hls playlist ready")
					}
					w.mu.Unlock()
				}
				return body, nil
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-w.ctx.Done():
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.lastError != nil {
				return nil, w.lastError
			}
			return nil, context.Canceled
		case err := <-w.startErr:
			return nil, err
		case <-timeout.C:
			return nil, fmt.Errorf("timed out waiting for hls asset %q", fileName)
		case <-ticker.C:
		}
	}
}

func (w *hlsWorker) setError(err error) {
	w.mu.Lock()
	w.lastError = err
	w.mu.Unlock()
	select {
	case w.startErr <- err:
	default:
	}
	w.logger.Error().Err(err).Msg("media worker stopped")
}

func (w *hlsWorker) status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	status := WorkerStatus{
		Key:              w.key,
		Format:           "hls",
		StreamID:         w.streamID,
		Channel:          detectWorkerChannel(w.streamID, w.profile.StreamURL),
		Profile:          w.profileName,
		SourceSubtype:    w.profile.Subtype,
		SourceVideoCodec: w.profile.VideoCodec,
		SourceAudioCodec: w.profile.AudioCodec,
		SourceWidth:      w.profile.SourceWidth,
		SourceHeight:     w.profile.SourceHeight,
		Viewers:          1,
		LastAccessAt:     w.lastAccessAt,
		StartedAt:        w.startedAt,
		SourceURL:        redactURLUserinfo(w.profile.StreamURL),
		Recommended:      w.profile.Recommended,
		FrameRate:        maxInt(w.profile.FrameRate, w.parent.cfg.FrameRate),
		Threads:          w.parent.cfg.Threads,
		ScaleWidth:       w.parent.cfg.ScaleWidth,
		MaxWorkers:       w.parent.cfg.MaxWorkers,
		IdleTimeout:      w.parent.cfg.IdleTimeout.String(),
		FFmpegPath:       w.parent.cfg.FFmpegPath,
		RTSPTransport:    firstNonEmpty(w.profile.RTSPTransport, "tcp"),
		InputPreset:      w.activeAttempt.inputPreset,
		HWAccelActive:    w.activeAttempt.useHWAccel,
		AudioEnabled:     w.includeAudio,
	}
	status.OutputVideoCodec = "H.264"
	status.OutputVideoEncoder = workerH264EncoderLabel(w.parent.cfg, w.activeAttempt.useHWAccel)
	if w.includeAudio {
		status.OutputAudioCodec = "AAC"
	}
	status.OutputWidth, status.OutputHeight = resolvedOutputDimensions(w.profile.SourceWidth, w.profile.SourceHeight, w.parent.cfg.ScaleWidth)
	if w.lastError != nil {
		status.LastError = w.lastError.Error()
	}
	return status
}
