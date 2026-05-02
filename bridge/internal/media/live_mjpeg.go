package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"time"

	"RCooLeR/DahuaBridge/internal/streams"
)

func (m *Manager) getOrCreateMJPEGWorker(entry streams.Entry, profileName string, profile streams.Profile, scaleWidth int) (*worker, error) {
	effectiveScaleWidth := resolvedScaleWidth(scaleWidth, m.cfg.ScaleWidth)
	key := fmt.Sprintf("%s:%s:w%d", entry.ID, profileName, effectiveScaleWidth)

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.mjpegWorkers[key]; ok {
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
	w := &worker{
		key:         key,
		streamID:    entry.ID,
		profileName: profileName,
		profile:     profile,
		scaleWidth:  effectiveScaleWidth,
		parent:      m,
		logger: m.logger.With().
			Str("stream_id", entry.ID).
			Str("profile", profileName).
			Str("format", "mjpeg").
			Int("scale_width", effectiveScaleWidth).
			Logger(),
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[chan []byte]struct{}),
		ready:       make(chan struct{}),
		startErr:    make(chan error, 1),
	}
	m.mjpegWorkers[key] = w
	m.setMediaWorkerCountLocked()
	m.logWorkerInventoryLocked("added", w.status())
	if m.metrics != nil {
		m.metrics.ObserveMediaStart(entry.ID, profileName, nil)
	}
	go w.run()
	return w, nil
}

func (m *Manager) removeMJPEGWorker(key string, w *worker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.mjpegWorkers[key]; ok && existing == w {
		delete(m.mjpegWorkers, key)
		m.setMediaWorkerCountLocked()
		m.logWorkerInventoryLocked("removed", w.status())
		if m.metrics != nil {
			m.metrics.SetMediaViewers(w.streamID, w.profileName, 0)
		}
	}
}

func (w *worker) addSubscriber(ch chan []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.subscribers[ch] = struct{}{}
	w.idleGeneration++
	if w.parent.metrics != nil {
		w.parent.metrics.SetMediaViewers(w.streamID, w.profileName, len(w.subscribers))
	}
	w.logger.Debug().Int("viewers", len(w.subscribers)).Msg("mjpeg subscriber added")
	if len(w.lastFrame) > 0 {
		frame := append([]byte(nil), w.lastFrame...)
		select {
		case ch <- frame:
		default:
		}
	}
}

func (w *worker) removeSubscriber(ch chan []byte) {
	w.mu.Lock()
	_, ok := w.subscribers[ch]
	if ok {
		delete(w.subscribers, ch)
	}
	empty := len(w.subscribers) == 0
	idleGeneration := w.idleGeneration
	if empty {
		w.idleGeneration++
		idleGeneration = w.idleGeneration
	}
	viewers := len(w.subscribers)
	w.mu.Unlock()

	if ok {
		close(ch)
	}
	if w.parent.metrics != nil {
		w.parent.metrics.SetMediaViewers(w.streamID, w.profileName, viewers)
	}
	if ok {
		w.logger.Debug().Int("viewers", viewers).Msg("mjpeg subscriber removed")
	}
	if empty {
		go w.stopWhenIdle(idleGeneration)
	}
}

func (w *worker) stopWhenIdle(idleGeneration uint64) {
	timer := time.NewTimer(w.parent.cfg.IdleTimeout)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-w.ctx.Done():
		return
	}

	w.mu.Lock()
	empty := len(w.subscribers) == 0
	sameIdleWindow := w.idleGeneration == idleGeneration
	w.mu.Unlock()
	if empty && sameIdleWindow {
		w.logger.Debug().Msg("stopping idle media worker")
		w.cancel()
	}
}

func (w *worker) run() {
	defer w.parent.removeMJPEGWorker(w.key, w)
	defer w.closeSubscribers()

	attempts := buildFFmpegStartAttempts(w.parent.cfg)
	for index, attempt := range attempts {
		if index > 0 {
			w.logger.Debug().
				Bool("hwaccel", attempt.useHWAccel).
				Str("input_preset", attempt.inputPreset).
				Msg("starting mjpeg fallback attempt")
		}

		args := w.buildFFmpegArgs(attempt)
		outputWidth, outputHeight := resolvedOutputDimensions(w.profile.SourceWidth, w.profile.SourceHeight, w.scaleWidth)
		w.logger.Debug().
			Bool("hwaccel", attempt.useHWAccel).
			Str("input_preset", attempt.inputPreset).
			Str("source_video_codec", w.profile.VideoCodec).
			Str("source_audio_codec", w.profile.AudioCodec).
			Int("source_width", w.profile.SourceWidth).
			Int("source_height", w.profile.SourceHeight).
			Str("output_video_codec", "MJPEG").
			Str("output_video_encoder", workerMJPEGEncoderLabel(w.parent.cfg, attempt.useHWAccel)).
			Int("output_width", outputWidth).
			Int("output_height", outputHeight).
			Strs("ffmpeg_args", redactFFmpegArgs(args)).
			Msg("starting mjpeg worker")
		cmd := exec.CommandContext(w.ctx, w.parent.cfg.FFmpegPath, args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			w.setError(fmt.Errorf("ffmpeg stdout pipe: %w", err))
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			w.setError(fmt.Errorf("ffmpeg stderr pipe: %w", err))
			return
		}

		w.mu.Lock()
		w.cmd = cmd
		if w.startedAt.IsZero() {
			w.startedAt = time.Now()
		}
		w.activeAttempt = attempt
		w.includeAudio = false
		w.mu.Unlock()

		if err := cmd.Start(); err != nil {
			w.setError(fmt.Errorf("start ffmpeg: %w", err))
			return
		}
		w.logger.Debug().
			Bool("hwaccel", attempt.useHWAccel).
			Str("input_preset", attempt.inputPreset).
			Msg("mjpeg ffmpeg started")

		stderrDone := drainFFmpegStderr(stderr, 64*1024)

		readErr := w.readMJPEG(stdout)
		waitErr := cmd.Wait()
		stderrText := <-stderrDone

		switch {
		case errors.Is(w.ctx.Err(), context.Canceled):
			return
		case readErr != nil:
			if stderrText != "" {
				w.logger.Debug().
					Bool("hwaccel", attempt.useHWAccel).
					Str("input_preset", attempt.inputPreset).
					Str("ffmpeg_stderr", stderrText).
					Msg("mjpeg worker stderr")
				readErr = fmt.Errorf("%w: %s", readErr, stderrText)
			}
			if index < len(attempts)-1 {
				w.logger.Warn().
					Bool("hwaccel", attempt.useHWAccel).
					Str("input_preset", attempt.inputPreset).
					Err(readErr).
					Msg("mjpeg worker attempt failed")
				continue
			}
			w.setError(readErr)
			return
		case waitErr != nil:
			if stderrText != "" {
				w.logger.Debug().
					Bool("hwaccel", attempt.useHWAccel).
					Str("input_preset", attempt.inputPreset).
					Str("ffmpeg_stderr", stderrText).
					Msg("mjpeg worker stderr")
				waitErr = fmt.Errorf("%w: %s", waitErr, stderrText)
			}
			if index < len(attempts)-1 {
				w.logger.Warn().
					Bool("hwaccel", attempt.useHWAccel).
					Str("input_preset", attempt.inputPreset).
					Err(waitErr).
					Msg("mjpeg worker attempt failed")
				continue
			}
			w.setError(waitErr)
			return
		default:
			w.logger.Debug().Msg("mjpeg worker exited cleanly")
			return
		}
	}
}

func (w *worker) buildFFmpegArgs(attempt ffmpegStartAttempt) []string {
	frameRate := w.parent.cfg.FrameRate
	if w.profile.FrameRate > 0 {
		frameRate = w.profile.FrameRate
	}

	args := []string{
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(w.parent.cfg),
	}
	args = appendInputHWAccelArgs(args, w.parent.cfg, attempt.useHWAccel)
	args = append(args, buildInputArgsWithWallclock(w.profile, attempt.inputPreset, w.profile.UseWallclockAsTimestamps)...)
	if playbackDuration, ok := playbackDurationFromProfile(w.profile); ok {
		args = append(args, "-t", formatFFmpegSeconds(playbackDuration))
	}
	args = append(args, "-an")
	if w.parent.cfg.Threads > 0 {
		args = append(args, "-threads", strconv.Itoa(w.parent.cfg.Threads))
	}
	args = appendVideoFilterArgs(args, w.parent.cfg, w.scaleWidth, w.profile, attempt.useHWAccel, frameRate)
	args = appendMJPEGEncoderArgs(args, w.parent.cfg, attempt.useHWAccel)
	args = append(args, "-f", "mjpeg", "pipe:1")
	return args
}

func (w *worker) readMJPEG(r io.Reader) error {
	reader := bufio.NewReaderSize(r, w.parent.cfg.ReadBufferSize)
	buffer := make([]byte, 0, 256*1024)
	frameCount := 0

	for {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		default:
		}

		chunk := make([]byte, 64*1024)
		n, err := reader.Read(chunk)
		if n > 0 {
			buffer = append(buffer, chunk[:n]...)
			var published int
			buffer, published = w.extractFramesWithCount(buffer)
			frameCount += published
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if frameCount > 0 && !hasIncompleteMJPEGFrame(buffer) {
					return nil
				}
				return fmt.Errorf("read mjpeg stdout: %w", io.ErrUnexpectedEOF)
			}
			return fmt.Errorf("read mjpeg stdout: %w", err)
		}
	}
}

func (w *worker) extractFrames(buffer []byte) []byte {
	buffer, _ = w.extractFramesWithCount(buffer)
	return buffer
}

func (w *worker) extractFramesWithCount(buffer []byte) ([]byte, int) {
	published := 0
	for {
		start := bytes.Index(buffer, []byte{0xFF, 0xD8})
		if start < 0 {
			if len(buffer) > 1024*1024 {
				return nil, published
			}
			return buffer, published
		}
		end := bytes.Index(buffer[start+2:], []byte{0xFF, 0xD9})
		if end < 0 {
			if start > 0 {
				return append([]byte(nil), buffer[start:]...), published
			}
			return buffer, published
		}

		end += start + 4
		frame := append([]byte(nil), buffer[start:end]...)
		w.publishFrame(frame)
		published++
		buffer = buffer[end:]
	}
}

func (w *worker) publishFrame(frame []byte) {
	w.mu.Lock()
	w.lastFrame = append(w.lastFrame[:0], frame...)
	w.lastFrameAt = time.Now()
	subs := make([]chan []byte, 0, len(w.subscribers))
	for ch := range w.subscribers {
		subs = append(subs, ch)
	}
	w.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- frame:
		default:
			if w.parent.metrics != nil {
				w.parent.metrics.ObserveMediaFrameDrop(w.streamID, w.profileName)
			}
		}
	}
	if w.parent.metrics != nil {
		w.parent.metrics.ObserveMediaFrame(w.streamID, w.profileName)
	}
	w.readyOnce.Do(func() {
		w.logger.Debug().Msg("mjpeg worker ready")
		close(w.ready)
	})
}

func (w *worker) setError(err error) {
	w.mu.Lock()
	w.lastError = err
	w.mu.Unlock()
	select {
	case w.startErr <- err:
	default:
	}
	w.readyOnce.Do(func() {
		close(w.ready)
	})
	w.logger.Error().Err(err).Msg("media worker stopped")
}

func (w *worker) closeSubscribers() {
	w.mu.Lock()
	subs := make([]chan []byte, 0, len(w.subscribers))
	for ch := range w.subscribers {
		subs = append(subs, ch)
		delete(w.subscribers, ch)
	}
	w.mu.Unlock()

	for _, ch := range subs {
		close(ch)
	}
}

func (w *worker) waitUntilReady(ctx context.Context) error {
	w.mu.Lock()
	hasFrame := len(w.lastFrame) > 0
	lastErr := w.lastError
	ready := w.ready
	startErr := w.startErr
	w.mu.Unlock()

	if hasFrame {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}

	timeout := time.NewTimer(w.parent.cfg.StartTimeout)
	defer timeout.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timeout.C:
		return fmt.Errorf("timed out waiting for first media frame")
	case err := <-startErr:
		return err
	case <-ready:
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.lastError
	}
}

func (w *worker) status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	status := WorkerStatus{
		Key:              w.key,
		Format:           "mjpeg",
		StreamID:         w.streamID,
		Channel:          detectWorkerChannel(w.streamID, w.profile.StreamURL),
		Profile:          w.profileName,
		SourceSubtype:    w.profile.Subtype,
		SourceVideoCodec: w.profile.VideoCodec,
		SourceAudioCodec: w.profile.AudioCodec,
		SourceWidth:      w.profile.SourceWidth,
		SourceHeight:     w.profile.SourceHeight,
		Viewers:          len(w.subscribers),
		LastFrameAt:      w.lastFrameAt,
		StartedAt:        w.startedAt,
		SourceURL:        redactURLUserinfo(w.profile.StreamURL),
		Recommended:      w.profile.Recommended,
		FrameRate:        maxInt(w.profile.FrameRate, w.parent.cfg.FrameRate),
		Threads:          w.parent.cfg.Threads,
		ScaleWidth:       w.scaleWidth,
		MaxWorkers:       w.parent.cfg.MaxWorkers,
		IdleTimeout:      w.parent.cfg.IdleTimeout.String(),
		FFmpegPath:       w.parent.cfg.FFmpegPath,
		RTSPTransport:    firstNonEmpty(w.profile.RTSPTransport, "tcp"),
		InputPreset:      w.activeAttempt.inputPreset,
		HWAccelActive:    w.activeAttempt.useHWAccel,
		AudioEnabled:     false,
	}
	status.OutputVideoCodec = "MJPEG"
	status.OutputVideoEncoder = workerMJPEGEncoderLabel(w.parent.cfg, w.activeAttempt.useHWAccel)
	status.OutputWidth, status.OutputHeight = resolvedOutputDimensions(w.profile.SourceWidth, w.profile.SourceHeight, w.scaleWidth)
	if w.lastError != nil {
		status.LastError = w.lastError.Error()
	}
	return status
}
