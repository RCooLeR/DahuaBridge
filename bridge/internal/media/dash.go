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
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/streams"

	"github.com/rs/zerolog"
)

type dashWorker struct {
	key         string
	streamID    string
	profileName string
	profile     streams.Profile
	parent      *Manager
	logger      zerolog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mu                sync.Mutex
	lastAccessAt      time.Time
	lastError         error
	startedAt         time.Time
	outputDir         string
	activeAttempt     ffmpegStartAttempt
	includeAudio      bool
	cmd               *exec.Cmd
	startErr          chan error
	manifestReadyOnce sync.Once
}

func (m *Manager) DASHManifest(ctx context.Context, streamID string, profileName string) ([]byte, error) {
	if !m.Enabled() {
		return nil, errors.New("media layer is disabled")
	}

	entry, profile, resolvedProfileName, err := m.resolveStream(streamID, profileName)
	if err != nil {
		return nil, err
	}

	w, err := m.getOrCreateDASHWorker(entry, resolvedProfileName, profile)
	if err != nil {
		return nil, err
	}
	return w.readFileWhenReady(ctx, "manifest.mpd")
}

func (m *Manager) DASHAsset(ctx context.Context, streamID string, profileName string, assetName string) ([]byte, string, error) {
	if !m.Enabled() {
		return nil, "", errors.New("media layer is disabled")
	}
	if err := validateHLSFileName(assetName); err != nil {
		return nil, "", err
	}

	entry, profile, resolvedProfileName, err := m.resolveStream(streamID, profileName)
	if err != nil {
		return nil, "", err
	}

	w, err := m.getOrCreateDASHWorker(entry, resolvedProfileName, profile)
	if err != nil {
		return nil, "", err
	}
	body, err := w.readFileWhenReady(ctx, assetName)
	if err != nil {
		return nil, "", err
	}
	return body, contentTypeForDASHFile(assetName), nil
}

func (m *Manager) getOrCreateDASHWorker(entry streams.Entry, profileName string, profile streams.Profile) (*dashWorker, error) {
	key := entry.ID + ":" + profileName

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.dashWorkers[key]; ok {
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
	w := &dashWorker{
		key:         key,
		streamID:    entry.ID,
		profileName: profileName,
		profile:     profile,
		parent:      m,
		logger: m.logger.With().
			Str("stream_id", entry.ID).
			Str("profile", profileName).
			Str("format", "dash").
			Logger(),
		ctx:          ctx,
		cancel:       cancel,
		lastAccessAt: time.Now(),
		startErr:     make(chan error, 1),
	}
	m.dashWorkers[key] = w
	m.setMediaWorkerCountLocked()
	m.logWorkerInventoryLocked("added", w.status())
	if m.metrics != nil {
		m.metrics.ObserveMediaStart(entry.ID, profileName, nil)
	}
	go w.run()
	return w, nil
}

func (m *Manager) removeDASHWorker(key string, w *dashWorker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.dashWorkers[key]; ok && existing == w {
		delete(m.dashWorkers, key)
		m.setMediaWorkerCountLocked()
		m.logWorkerInventoryLocked("removed", w.status())
		if m.metrics != nil {
			m.metrics.SetMediaViewers(w.streamID, w.profileName, 0)
		}
	}
}

func (w *dashWorker) run() {
	outputDir := ""
	retainOutput := false
	includeAudio := w.parent.shouldIncludeSourceAudio(w.profile, w.logger)

	defer func() {
		if retainOutput && outputDir != "" && w.parent.cfg.HLSKeepAfterExit > 0 {
			keepFor := w.parent.cfg.HLSKeepAfterExit
			w.logger.Info().Str("dash_output_dir", outputDir).Dur("keep_after_exit", keepFor).Msg("retaining completed dash output")
			time.AfterFunc(keepFor, func() {
				w.parent.removeDASHWorker(w.key, w)
				if err := os.RemoveAll(outputDir); err != nil {
					w.logger.Warn().Err(err).Str("dash_output_dir", outputDir).Msg("failed to cleanup retained dash output")
				}
			})
			return
		}

		w.parent.removeDASHWorker(w.key, w)
		if outputDir != "" {
			if err := os.RemoveAll(outputDir); err != nil {
				w.logger.Warn().Err(err).Str("dash_output_dir", outputDir).Msg("failed to cleanup dash output")
			}
		}
	}()

	outputRoot := strings.TrimSpace(w.parent.cfg.HLSTmpDir)
	if outputRoot == "" {
		outputRoot = "/data/tmp/dahuabridge/hls"
	}
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		w.setError(fmt.Errorf("create dash temp root: %w", err))
		return
	}

	var err error
	outputDir, err = os.MkdirTemp(outputRoot, safeHLSDirectoryPrefix(w.key)+"-dash-*")
	if err != nil {
		w.setError(fmt.Errorf("create dash temp dir: %w", err))
		return
	}
	w.logger.Debug().
		Str("worker_key", w.key).
		Str("dash_output_dir", outputDir).
		Bool("include_audio", includeAudio).
		Msg("prepared dash worker output")

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
			w.logger.Debug().Bool("hwaccel", attempt.useHWAccel).Str("input_preset", attempt.inputPreset).Msg("starting dash fallback attempt")
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
			Str("dash_output_dir", outputDir).
			Strs("ffmpeg_args", redactFFmpegArgs(args)).
			Msg("starting dash worker")

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
			Str("dash_output_dir", outputDir).
			Msg("dash ffmpeg started")

		stderrDone := drainFFmpegStderr(stderr, 128*1024)

		waitErr := cmd.Wait()
		stderrText := <-stderrDone

		if errors.Is(w.ctx.Err(), context.Canceled) {
			return
		}

		if stderrText != "" {
			w.logger.Debug().Bool("hwaccel", attempt.useHWAccel).Str("input_preset", attempt.inputPreset).Str("ffmpeg_stderr", stderrText).Msg("dash worker stderr")
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
					Msg("dash worker attempt failed")
				continue
			}
			w.setError(waitErr)
			return
		}

		if w.isPlaybackStream() || w.parent.cfg.HLSKeepAfterExit > 0 {
			retainOutput = true
		}
		w.logger.Debug().Bool("retain_output", retainOutput).Msg("dash worker exited cleanly")
		return
	}
}

func (w *dashWorker) buildFFmpegArgs(attempt ffmpegStartAttempt, includeAudio bool) []string {
	frameRate := w.parent.cfg.FrameRate
	if w.profile.FrameRate > 0 {
		frameRate = w.profile.FrameRate
	}
	segmentDuration := formatFFmpegSeconds(w.parent.cfg.HLSSegmentTime)
	segmentSeconds := int(maxInt(int(w.parent.cfg.HLSSegmentTime/time.Second), 1))
	gopSize := maxInt(frameRate*segmentSeconds, frameRate)
	windowSize := strconv.Itoa(maxInt(w.parent.cfg.HLSListSize, 1))
	extraWindowSize := strconv.Itoa(maxInt(w.parent.cfg.HLSListSize/2, 1))
	if w.isPlaybackStream() {
		windowSize = "0"
		extraWindowSize = "0"
	}

	adaptationSets := "id=0,streams=v"
	if includeAudio {
		adaptationSets += " id=1,streams=a"
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
		"-muxpreload", "0",
		"-muxdelay", "0",
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
		"-streaming", "1",
		"-use_template", "1",
		"-use_timeline", "1",
		"-seg_duration", segmentDuration,
		"-window_size", windowSize,
		"-extra_window_size", extraWindowSize,
		"-remove_at_exit", "0",
		"-init_seg_name", "init-$RepresentationID$.m4s",
		"-media_seg_name", "chunk-$RepresentationID$-$Number%05d$.m4s",
		"-adaptation_sets", adaptationSets,
		"-f", "dash",
		"manifest.mpd",
	)
	return args
}

func (w *dashWorker) isPlaybackStream() bool {
	return strings.HasPrefix(strings.TrimSpace(w.streamID), "nvrpb_")
}

func (w *dashWorker) stopWhenIdle() {
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

func (w *dashWorker) touch() {
	w.mu.Lock()
	w.lastAccessAt = time.Now()
	w.mu.Unlock()
	if w.parent.metrics != nil {
		w.parent.metrics.SetMediaViewers(w.streamID, w.profileName, 1)
	}
}

func (w *dashWorker) readFileWhenReady(ctx context.Context, fileName string) ([]byte, error) {
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
				if fileName == "manifest.mpd" {
					w.manifestReadyOnce.Do(func() {
						w.logger.Debug().Str("file_name", fileName).Msg("dash manifest ready")
					})
				}
				return body, nil
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("read dash output %q: %w", fileName, err)
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, fmt.Errorf("timed out waiting for dash asset %q", fileName)
		case <-ticker.C:
		}
	}
}

func (w *dashWorker) setError(err error) {
	w.mu.Lock()
	w.lastError = err
	w.mu.Unlock()
	select {
	case w.startErr <- err:
	default:
	}
	w.logger.Error().Err(err).Msg("media worker stopped")
}

func (w *dashWorker) status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	status := WorkerStatus{
		Key:              w.key,
		Format:           "dash",
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

func contentTypeForDASHFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mpd":
		return "application/dash+xml"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}
