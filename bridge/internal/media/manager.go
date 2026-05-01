package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/streams"

	"github.com/rs/zerolog"
)

type StreamResolver interface {
	GetStream(string, string, bool) (streams.Entry, streams.Profile, bool)
}

var ErrWorkerLimitReached = errors.New("media worker limit reached")

type WebRTCSessionDescription struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type WebRTCICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type IntercomStatus struct {
	StreamID               string    `json:"stream_id"`
	Active                 bool      `json:"active"`
	SessionCount           int       `json:"session_count"`
	Profiles               []string  `json:"profiles,omitempty"`
	ExternalUplinkEnabled  bool      `json:"external_uplink_enabled"`
	UplinkActive           bool      `json:"uplink_active"`
	UplinkCodec            string    `json:"uplink_codec,omitempty"`
	UplinkPackets          uint64    `json:"uplink_packets,omitempty"`
	UplinkTargetCount      int       `json:"uplink_target_count,omitempty"`
	UplinkForwardedPackets uint64    `json:"uplink_forwarded_packets,omitempty"`
	UplinkForwardErrors    uint64    `json:"uplink_forward_errors,omitempty"`
	LastAccessAt           time.Time `json:"last_access_at,omitempty"`
	StartedAt              time.Time `json:"started_at,omitempty"`
}

type Manager struct {
	cfg      config.MediaConfig
	resolver StreamResolver
	metrics  *metrics.Registry
	logger   zerolog.Logger

	mu                    sync.Mutex
	mjpegWorkers          map[string]*worker
	hlsWorkers            map[string]*hlsWorker
	clipJobs              map[string]*clipJob
	webrtcPeers           map[string]*webrtcSession
	intercomUplinkEnabled map[string]bool
}

type WorkerStatus struct {
	Key                    string    `json:"key"`
	Format                 string    `json:"format,omitempty"`
	StreamID               string    `json:"stream_id"`
	Profile                string    `json:"profile"`
	Viewers                int       `json:"viewers"`
	LastFrameAt            time.Time `json:"last_frame_at,omitempty"`
	LastAccessAt           time.Time `json:"last_access_at,omitempty"`
	LastError              string    `json:"last_error,omitempty"`
	StartedAt              time.Time `json:"started_at,omitempty"`
	SourceURL              string    `json:"source_url,omitempty"`
	Recommended            bool      `json:"recommended"`
	FrameRate              int       `json:"frame_rate"`
	Threads                int       `json:"threads"`
	ScaleWidth             int       `json:"scale_width,omitempty"`
	MaxWorkers             int       `json:"max_workers,omitempty"`
	IdleTimeout            string    `json:"idle_timeout"`
	FFmpegPath             string    `json:"ffmpeg_path"`
	UplinkActive           bool      `json:"uplink_active,omitempty"`
	UplinkPackets          uint64    `json:"uplink_packets,omitempty"`
	UplinkCodec            string    `json:"uplink_codec,omitempty"`
	ExternalUplinkEnabled  bool      `json:"external_uplink_enabled,omitempty"`
	UplinkTargetCount      int       `json:"uplink_target_count,omitempty"`
	UplinkForwardedPackets uint64    `json:"uplink_forwarded_packets,omitempty"`
	UplinkForwardErrors    uint64    `json:"uplink_forward_errors,omitempty"`
	MediaDisabled          bool      `json:"media_disabled,omitempty"`
}

type worker struct {
	key         string
	streamID    string
	profileName string
	profile     streams.Profile
	scaleWidth  int
	parent      *Manager
	logger      zerolog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
	lastFrame   []byte
	lastFrameAt time.Time
	lastError   error
	startedAt   time.Time
	cmd         *exec.Cmd
	ready       chan struct{}
	startErr    chan error
	readyOnce   sync.Once
}

type hlsWorker struct {
	key         string
	streamID    string
	profileName string
	profile     streams.Profile
	parent      *Manager
	logger      zerolog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mu           sync.Mutex
	lastAccessAt time.Time
	lastError    error
	startedAt    time.Time
	outputDir    string
	cmd          *exec.Cmd
	startErr     chan error
}

type ffmpegStartAttempt struct {
	useHWAccel  bool
	inputPreset string
}

func New(cfg config.MediaConfig, resolver StreamResolver, logger zerolog.Logger, metricsRegistry *metrics.Registry) *Manager {
	manager := &Manager{
		cfg:                   cfg,
		resolver:              resolver,
		metrics:               metricsRegistry,
		logger:                logger.With().Str("component", "media").Logger(),
		mjpegWorkers:          make(map[string]*worker),
		hlsWorkers:            make(map[string]*hlsWorker),
		clipJobs:              make(map[string]*clipJob),
		webrtcPeers:           make(map[string]*webrtcSession),
		intercomUplinkEnabled: make(map[string]bool),
	}
	if metricsRegistry != nil {
		metricsRegistry.SetMediaWorkers(0)
	}
	return manager
}

func (m *Manager) Enabled() bool {
	return m != nil && m.cfg.Enabled
}

func (m *Manager) WebRTCICEServers() []WebRTCICEServer {
	if m == nil {
		return nil
	}

	servers := make([]WebRTCICEServer, 0, len(m.cfg.WebRTCICEServers))
	for _, server := range m.cfg.WebRTCICEServers {
		urls := append([]string(nil), server.URLs...)
		servers = append(servers, WebRTCICEServer{
			URLs:       urls,
			Username:   server.Username,
			Credential: server.Credential,
		})
	}
	return servers
}

func (m *Manager) IntercomStatus(streamID string) IntercomStatus {
	status := IntercomStatus{
		StreamID: strings.TrimSpace(streamID),
	}
	if m == nil || !m.Enabled() || status.StreamID == "" {
		return status
	}
	status.ExternalUplinkEnabled = m.IntercomUplinkEnabled(status.StreamID)

	m.mu.Lock()
	sessions := make([]*webrtcSession, 0, len(m.webrtcPeers))
	for _, session := range m.webrtcPeers {
		if session != nil && session.streamID == status.StreamID {
			sessions = append(sessions, session)
		}
	}
	m.mu.Unlock()

	if len(sessions) == 0 {
		return status
	}

	status.Active = true
	status.SessionCount = len(sessions)
	profiles := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		session.mu.Lock()
		if session.profileName != "" {
			profiles[session.profileName] = struct{}{}
		}
		if session.uplinkActive {
			status.UplinkActive = true
		}
		if status.UplinkCodec == "" && session.uplinkCodec != "" {
			status.UplinkCodec = session.uplinkCodec
		}
		status.UplinkPackets += session.uplinkPackets
		status.UplinkForwardedPackets += session.uplinkForwardedPackets
		status.UplinkForwardErrors += session.uplinkForwardErrors
		if session.uplinkTargetCount > status.UplinkTargetCount {
			status.UplinkTargetCount = session.uplinkTargetCount
		}
		if session.lastAccessAt.After(status.LastAccessAt) {
			status.LastAccessAt = session.lastAccessAt
		}
		if session.startedAt.After(status.StartedAt) {
			status.StartedAt = session.startedAt
		}
		session.mu.Unlock()
	}

	if len(profiles) > 0 {
		status.Profiles = make([]string, 0, len(profiles))
		for profile := range profiles {
			status.Profiles = append(status.Profiles, profile)
		}
		sort.Strings(status.Profiles)
	}

	return status
}

func (m *Manager) IntercomUplinkEnabled(streamID string) bool {
	if m == nil || strings.TrimSpace(streamID) == "" {
		return false
	}
	if len(m.cfg.WebRTCUplinkTargets) == 0 {
		return false
	}

	m.mu.Lock()
	enabled, ok := m.intercomUplinkEnabled[streamID]
	m.mu.Unlock()
	if ok {
		return enabled
	}
	return true
}

func (m *Manager) SetIntercomUplinkEnabled(streamID string, enabled bool) IntercomStatus {
	if m == nil || strings.TrimSpace(streamID) == "" {
		return IntercomStatus{StreamID: strings.TrimSpace(streamID)}
	}

	m.mu.Lock()
	if len(m.cfg.WebRTCUplinkTargets) == 0 {
		delete(m.intercomUplinkEnabled, streamID)
	} else {
		m.intercomUplinkEnabled[streamID] = enabled
	}
	m.mu.Unlock()

	return m.IntercomStatus(streamID)
}

func (m *Manager) StopIntercomSessions(streamID string) IntercomStatus {
	streamID = strings.TrimSpace(streamID)
	if m == nil || !m.Enabled() || streamID == "" {
		return IntercomStatus{StreamID: streamID}
	}

	m.mu.Lock()
	sessions := make([]*webrtcSession, 0, len(m.webrtcPeers))
	for key, session := range m.webrtcPeers {
		if session == nil || session.streamID != streamID {
			continue
		}
		sessions = append(sessions, session)
		delete(m.webrtcPeers, key)
		if m.metrics != nil {
			m.metrics.SetMediaViewers(session.streamID, session.profileName, 0)
		}
	}
	m.setMediaWorkerCountLocked()
	m.mu.Unlock()

	for _, session := range sessions {
		session.cancel()
	}

	return m.IntercomStatus(streamID)
}

func (m *Manager) Subscribe(ctx context.Context, streamID string, profileName string) (<-chan []byte, func(), error) {
	return m.SubscribeScaled(ctx, streamID, profileName, 0)
}

func (m *Manager) SubscribeScaled(ctx context.Context, streamID string, profileName string, scaleWidth int) (<-chan []byte, func(), error) {
	if !m.Enabled() {
		return nil, nil, errors.New("media layer is disabled")
	}

	entry, profile, resolvedProfileName, err := m.resolveStream(streamID, profileName)
	if err != nil {
		return nil, nil, err
	}

	w, err := m.getOrCreateMJPEGWorker(entry, resolvedProfileName, profile, scaleWidth)
	if err != nil {
		return nil, nil, err
	}
	ch := make(chan []byte, 4)
	w.addSubscriber(ch)

	unsubscribe := func() {
		w.removeSubscriber(ch)
	}

	if err := w.waitUntilReady(ctx); err != nil {
		unsubscribe()
		return nil, nil, err
	}

	go func() {
		<-ctx.Done()
		unsubscribe()
	}()

	return ch, unsubscribe, nil
}

func (m *Manager) HLSPlaylist(ctx context.Context, streamID string, profileName string) ([]byte, error) {
	if !m.Enabled() {
		return nil, errors.New("media layer is disabled")
	}

	entry, profile, resolvedProfileName, err := m.resolveStream(streamID, profileName)
	if err != nil {
		return nil, err
	}

	w, err := m.getOrCreateHLSWorker(entry, resolvedProfileName, profile)
	if err != nil {
		return nil, err
	}
	return w.readFileWhenReady(ctx, "index.m3u8")
}

func (m *Manager) HLSSegment(ctx context.Context, streamID string, profileName string, segmentName string) ([]byte, string, error) {
	if !m.Enabled() {
		return nil, "", errors.New("media layer is disabled")
	}

	if err := validateHLSFileName(segmentName); err != nil {
		return nil, "", err
	}

	entry, profile, resolvedProfileName, err := m.resolveStream(streamID, profileName)
	if err != nil {
		return nil, "", err
	}

	w, err := m.getOrCreateHLSWorker(entry, resolvedProfileName, profile)
	if err != nil {
		return nil, "", err
	}

	body, err := w.readFileWhenReady(ctx, segmentName)
	if err != nil {
		return nil, "", err
	}
	return body, contentTypeForHLSFile(segmentName), nil
}

func (m *Manager) ListWorkers() []WorkerStatus {
	if !m.Enabled() {
		return []WorkerStatus{{
			MediaDisabled: true,
			MaxWorkers:    m.cfg.MaxWorkers,
			IdleTimeout:   m.cfg.IdleTimeout.String(),
			FFmpegPath:    m.cfg.FFmpegPath,
			FrameRate:     m.cfg.FrameRate,
			ScaleWidth:    m.cfg.ScaleWidth,
			Threads:       m.cfg.Threads,
		}}
	}

	m.mu.Lock()
	mjpegWorkers := make([]*worker, 0, len(m.mjpegWorkers))
	for _, w := range m.mjpegWorkers {
		mjpegWorkers = append(mjpegWorkers, w)
	}
	hlsWorkers := make([]*hlsWorker, 0, len(m.hlsWorkers))
	for _, w := range m.hlsWorkers {
		hlsWorkers = append(hlsWorkers, w)
	}
	webrtcPeers := make([]*webrtcSession, 0, len(m.webrtcPeers))
	for _, session := range m.webrtcPeers {
		webrtcPeers = append(webrtcPeers, session)
	}
	m.mu.Unlock()

	statuses := make([]WorkerStatus, 0, len(mjpegWorkers)+len(hlsWorkers)+len(webrtcPeers))
	for _, w := range mjpegWorkers {
		statuses = append(statuses, w.status())
	}
	for _, w := range hlsWorkers {
		statuses = append(statuses, w.status())
	}
	for _, session := range webrtcPeers {
		statuses = append(statuses, session.status())
	}
	return statuses
}

func (m *Manager) resolveStream(streamID string, profileName string) (streams.Entry, streams.Profile, string, error) {
	if strings.TrimSpace(profileName) == "" {
		profileName = "stable"
	}

	entry, profile, ok := m.resolver.GetStream(streamID, profileName, true)
	if !ok {
		return streams.Entry{}, streams.Profile{}, "", fmt.Errorf("stream %q profile %q not found", streamID, profileName)
	}
	return entry, profile, profileName, nil
}

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
	if m.metrics != nil {
		m.metrics.ObserveMediaStart(entry.ID, profileName, nil)
	}
	go w.run()
	return w, nil
}

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
		if m.metrics != nil {
			m.metrics.SetMediaViewers(w.streamID, w.profileName, 0)
		}
	}
}

func (m *Manager) removeHLSWorker(key string, w *hlsWorker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.hlsWorkers[key]; ok && existing == w {
		delete(m.hlsWorkers, key)
		m.setMediaWorkerCountLocked()
		if m.metrics != nil {
			m.metrics.SetMediaViewers(w.streamID, w.profileName, 0)
		}
	}
}

func (m *Manager) removeWebRTCSession(key string, session *webrtcSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.webrtcPeers[key]; ok && existing == session {
		delete(m.webrtcPeers, key)
		m.setMediaWorkerCountLocked()
		if m.metrics != nil {
			m.metrics.SetMediaViewers(session.streamID, session.profileName, 0)
		}
	}
}

func (m *Manager) activeWorkerCountLocked() int {
	return len(m.mjpegWorkers) + len(m.hlsWorkers) + len(m.clipJobs) + len(m.webrtcPeers)
}

func (m *Manager) setMediaWorkerCountLocked() {
	if m.metrics != nil {
		m.metrics.SetMediaWorkers(m.activeWorkerCountLocked())
	}
}

func (w *worker) addSubscriber(ch chan []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.subscribers[ch] = struct{}{}
	if w.parent.metrics != nil {
		w.parent.metrics.SetMediaViewers(w.streamID, w.profileName, len(w.subscribers))
	}
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
	viewers := len(w.subscribers)
	w.mu.Unlock()

	if ok {
		close(ch)
	}
	if w.parent.metrics != nil {
		w.parent.metrics.SetMediaViewers(w.streamID, w.profileName, viewers)
	}
	if empty {
		go w.stopWhenIdle()
	}
}

func (w *worker) stopWhenIdle() {
	timer := time.NewTimer(w.parent.cfg.IdleTimeout)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-w.ctx.Done():
		return
	}

	w.mu.Lock()
	empty := len(w.subscribers) == 0
	w.mu.Unlock()
	if empty {
		w.logger.Info().Msg("stopping idle media worker")
		w.cancel()
	}
}

func (w *worker) stopNowIfIdle() {
	w.mu.Lock()
	empty := len(w.subscribers) == 0
	w.mu.Unlock()
	if empty {
		w.cancel()
	}
}

func (w *worker) run() {
	defer w.parent.removeMJPEGWorker(w.key, w)
	defer w.closeSubscribers()

	attempts := buildFFmpegStartAttempts(w.parent.cfg)
	for index, attempt := range attempts {
		if index > 0 {
			w.logger.Info().
				Bool("hwaccel", attempt.useHWAccel).
				Str("input_preset", attempt.inputPreset).
				Msg("starting mjpeg fallback attempt")
		}

		args := w.buildFFmpegArgs(attempt)
		w.logger.Debug().
			Bool("hwaccel", attempt.useHWAccel).
			Str("input_preset", attempt.inputPreset).
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
		w.mu.Unlock()

		if err := cmd.Start(); err != nil {
			w.setError(fmt.Errorf("start ffmpeg: %w", err))
			return
		}

		stderrDone := make(chan string, 1)
		go func() {
			body, _ := io.ReadAll(io.LimitReader(stderr, 64*1024))
			stderrDone <- strings.TrimSpace(string(body))
		}()

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
	args = append(args, buildRTSPInputArgs(w.profile, attempt.inputPreset)...)
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
			buffer = w.extractFrames(buffer)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("read mjpeg stdout: %w", io.ErrUnexpectedEOF)
			}
			return fmt.Errorf("read mjpeg stdout: %w", err)
		}
	}
}

func (w *worker) extractFrames(buffer []byte) []byte {
	for {
		start := bytes.Index(buffer, []byte{0xFF, 0xD8})
		if start < 0 {
			if len(buffer) > 1024*1024 {
				return nil
			}
			return buffer
		}
		end := bytes.Index(buffer[start+2:], []byte{0xFF, 0xD9})
		if end < 0 {
			if start > 0 {
				return append([]byte(nil), buffer[start:]...)
			}
			return buffer
		}

		end += start + 4
		frame := append([]byte(nil), buffer[start:end]...)
		w.publishFrame(frame)
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
		Key:         w.key,
		Format:      "mjpeg",
		StreamID:    w.streamID,
		Profile:     w.profileName,
		Viewers:     len(w.subscribers),
		LastFrameAt: w.lastFrameAt,
		StartedAt:   w.startedAt,
		SourceURL:   redactURLUserinfo(w.profile.StreamURL),
		Recommended: w.profile.Recommended,
		FrameRate:   maxInt(w.profile.FrameRate, w.parent.cfg.FrameRate),
		Threads:     w.parent.cfg.Threads,
		ScaleWidth:  w.scaleWidth,
		MaxWorkers:  w.parent.cfg.MaxWorkers,
		IdleTimeout: w.parent.cfg.IdleTimeout.String(),
		FFmpegPath:  w.parent.cfg.FFmpegPath,
	}
	if w.lastError != nil {
		status.LastError = w.lastError.Error()
	}
	return status
}

func (w *hlsWorker) run() {
	outputDir := ""
	retainOutput := false

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
			w.logger.Info().Bool("hwaccel", attempt.useHWAccel).Str("input_preset", attempt.inputPreset).Msg("starting hls fallback attempt")
		}

		args := w.buildFFmpegArgs(attempt)
		w.logger.Debug().Bool("hwaccel", attempt.useHWAccel).Str("input_preset", attempt.inputPreset).Str("hls_output_dir", outputDir).Strs("ffmpeg_args", redactFFmpegArgs(args)).Msg("starting hls worker")

		cmd := exec.CommandContext(w.ctx, w.parent.cfg.FFmpegPath, args...)
		cmd.Dir = outputDir

		stderr, err := cmd.StderrPipe()
		if err != nil {
			w.setError(fmt.Errorf("ffmpeg stderr pipe: %w", err))
			return
		}

		w.mu.Lock()
		w.cmd = cmd
		w.mu.Unlock()

		if err := cmd.Start(); err != nil {
			w.setError(fmt.Errorf("start ffmpeg: %w", err))
			return
		}

		stderrDone := make(chan string, 1)
		go func() {
			body, _ := io.ReadAll(io.LimitReader(stderr, 128*1024))
			stderrDone <- strings.TrimSpace(string(body))
		}()

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

		// A clean FFmpeg exit is normal for finite NVR playback and SMB/IVS/event clips.
		// Keep the generated playlist and segments addressable through the existing worker
		// instead of removing the worker/output directory immediately.
		if w.isPlaybackStream() || w.parent.cfg.HLSKeepAfterExit > 0 {
			retainOutput = true
		}
		return
	}
}

func (w *hlsWorker) buildFFmpegArgs(attempt ffmpegStartAttempt) []string {
	frameRate := w.parent.cfg.FrameRate
	if w.profile.FrameRate > 0 {
		frameRate = w.profile.FrameRate
	}
	segmentSeconds := int(maxInt(int(w.parent.cfg.HLSSegmentTime/time.Second), 1))
	gopSize := maxInt(frameRate*segmentSeconds, frameRate)
	hlsListSize := strconv.Itoa(w.parent.cfg.HLSListSize)
	hlsFlags := "delete_segments+independent_segments+omit_endlist+temp_file"
	if w.isPlaybackStream() {
		hlsListSize = "0"
		hlsFlags = "independent_segments+temp_file"
	}

	args := []string{
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(w.parent.cfg),
	}
	args = appendInputHWAccelArgs(args, w.parent.cfg, attempt.useHWAccel)
	args = append(args, buildRTSPInputArgs(w.profile, attempt.inputPreset)...)
	if w.parent.cfg.Threads > 0 {
		args = append(args, "-threads", strconv.Itoa(w.parent.cfg.Threads))
	}
	args = appendVideoFilterArgs(args, w.parent.cfg, w.parent.cfg.ScaleWidth, w.profile, attempt.useHWAccel, frameRate)
	args = append(args,
		"-map", "0:v:0",
		"-map", "0:a:0?",
	)
	args = appendVideoEncoderArgs(args, w.parent.cfg, attempt.useHWAccel, gopSize, "veryfast")
	args = append(args,
		"-c:a", "aac",
		"-b:a", "96k",
		"-ac", "2",
		"-ar", "48000",
		"-f", "hls",
		"-hls_time", formatFFmpegSeconds(w.parent.cfg.HLSSegmentTime),
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
				w.logger.Info().Msg("stopping idle media worker")
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
		Key:          w.key,
		Format:       "hls",
		StreamID:     w.streamID,
		Profile:      w.profileName,
		Viewers:      1,
		LastAccessAt: w.lastAccessAt,
		StartedAt:    w.startedAt,
		SourceURL:    redactURLUserinfo(w.profile.StreamURL),
		Recommended:  w.profile.Recommended,
		FrameRate:    maxInt(w.profile.FrameRate, w.parent.cfg.FrameRate),
		Threads:      w.parent.cfg.Threads,
		ScaleWidth:   w.parent.cfg.ScaleWidth,
		MaxWorkers:   w.parent.cfg.MaxWorkers,
		IdleTimeout:  w.parent.cfg.IdleTimeout.String(),
		FFmpegPath:   w.parent.cfg.FFmpegPath,
	}
	if w.lastError != nil {
		status.LastError = w.lastError.Error()
	}
	return status
}

func safeHLSDirectoryPrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "stream"
	}
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", " ", "_")
	safe := replacer.Replace(trimmed)
	if len(safe) > 96 {
		safe = safe[:96]
	}
	return safe
}
func removeDirectoryAfter(logger zerolog.Logger, dir string, delay time.Duration) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	<-timer.C
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn().Err(err).Str("dir", dir).Msg("failed to remove expired hls cache directory")
	}
}

func redactURLUserinfo(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func resolvedScaleWidth(requested int, configured int) int {
	if configured <= 0 {
		return 0
	}
	if requested > 0 {
		return requested
	}
	return configured
}

func buildFFmpegStartAttempts(cfg config.MediaConfig) []ffmpegStartAttempt {
	attempts := make([]ffmpegStartAttempt, 0, 3)
	if hardwareAccelEnabled(cfg.HWAccelArgs) {
		attempts = append(attempts, ffmpegStartAttempt{
			useHWAccel:  true,
			inputPreset: cfg.InputPreset,
		})
	}
	attempts = append(attempts, ffmpegStartAttempt{
		useHWAccel:  false,
		inputPreset: cfg.InputPreset,
	})
	if !strings.EqualFold(strings.TrimSpace(cfg.InputPreset), "stable") {
		attempts = append(attempts, ffmpegStartAttempt{
			useHWAccel:  false,
			inputPreset: "stable",
		})
	}
	return attempts
}

func buildRTSPInputArgs(profile streams.Profile, inputPreset string) []string {
	args := []string{
		"-rtsp_transport", firstNonEmpty(profile.RTSPTransport, "tcp"),
	}
	switch strings.ToLower(strings.TrimSpace(inputPreset)) {
	case "stable":
		args = append(args,
			"-fflags", "+discardcorrupt",
		)
	default:
		args = append(args,
			"-fflags", "+discardcorrupt+nobuffer",
			"-flags", "low_delay",
		)
	}
	args = append(args, "-i", profile.StreamURL)
	return args
}

func buildFilterChain(frameRate int, scaleWidth int, sourceWidth int, sourceHeight int) []string {
	filters := []string{"fps=" + strconv.Itoa(frameRate)}
	if scaleWidth > 0 {
		targetWidth, targetHeight, ok := computeScaledDimensions(sourceWidth, sourceHeight, scaleWidth)
		if ok {
			filters = append(filters, fmt.Sprintf("scale=%d:%d", targetWidth, targetHeight))
		} else {
			filters = append(filters, fmt.Sprintf("scale=%d:-2", scaleWidth))
		}
	}
	return filters
}

func buildQSVFilterChain(frameRate int, scaleWidth int, sourceWidth int, sourceHeight int) []string {
	options := []string{fmt.Sprintf("framerate=%d", frameRate)}
	if scaleWidth > 0 {
		targetWidth, targetHeight, ok := computeScaledDimensions(sourceWidth, sourceHeight, scaleWidth)
		if ok {
			options = append(options,
				fmt.Sprintf("w=%d", targetWidth),
				fmt.Sprintf("h=%d", targetHeight),
			)
		}
	}
	options = append(options, "format=nv12")
	return []string{"vpp_qsv=" + strings.Join(options, ":")}
}

func computeScaledDimensions(sourceWidth int, sourceHeight int, targetWidth int) (int, int, bool) {
	if sourceWidth <= 0 || sourceHeight <= 0 || targetWidth <= 0 {
		return 0, 0, false
	}

	targetHeight := (sourceHeight * targetWidth) / sourceWidth
	if targetHeight <= 0 {
		return 0, 0, false
	}
	if targetHeight%2 != 0 {
		targetHeight--
	}
	if targetHeight <= 0 {
		return 0, 0, false
	}
	if targetWidth%2 != 0 {
		targetWidth--
	}
	if targetWidth <= 0 {
		return 0, 0, false
	}
	return targetWidth, targetHeight, true
}

func validateHLSFileName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("invalid hls asset name")
	}
	if name != filepath.Base(name) || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return errors.New("invalid hls asset name")
	}
	return nil
}

func contentTypeForHLSFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	default:
		return "application/octet-stream"
	}
}

func formatFFmpegSeconds(duration time.Duration) string {
	seconds := duration.Seconds()
	formatted := strconv.FormatFloat(seconds, 'f', 3, 64)
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	if formatted == "" {
		return "1"
	}
	return formatted
}

func ffmpegLogLevel(cfg config.MediaConfig) string {
	level := strings.ToLower(strings.TrimSpace(cfg.FFmpegLogLevel))
	switch level {
	case "quiet", "panic", "fatal", "error", "warning", "info", "verbose", "debug", "trace":
		return level
	default:
		return "error"
	}
}

func hardwareAccelEnabled(args []string) bool {
	return len(args) > 0
}

func appendInputHWAccelArgs(args []string, cfg config.MediaConfig, useHWAccel bool) []string {
	if !useHWAccel {
		return args
	}

	args = append(args, cfg.HWAccelArgs...)
	if qsvHWAccelConfigured(cfg.HWAccelArgs) && !containsArg(cfg.HWAccelArgs, "-hwaccel_output_format") {
		args = append(args, "-hwaccel_output_format", "qsv")
	}
	return args
}

func qsvHWAccelConfigured(args []string) bool {
	for _, arg := range args {
		if strings.Contains(strings.ToLower(strings.TrimSpace(arg)), "qsv") {
			return true
		}
	}
	return false
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), target) {
			return true
		}
	}
	return false
}

func useQSVVideoEncoder(cfg config.MediaConfig, useHWAccel bool) bool {
	return useHWAccel &&
		hardwareAccelEnabled(cfg.HWAccelArgs) &&
		strings.EqualFold(strings.TrimSpace(cfg.VideoEncoder), "qsv")
}

func appendVideoFilterArgs(args []string, cfg config.MediaConfig, scaleWidth int, profile streams.Profile, useHWAccel bool, frameRate int) []string {
	var filterChain []string
	if useQSVVideoEncoder(cfg, useHWAccel) {
		filterChain = buildQSVFilterChain(frameRate, scaleWidth, profile.SourceWidth, profile.SourceHeight)
	} else {
		filterChain = buildFilterChain(frameRate, scaleWidth, profile.SourceWidth, profile.SourceHeight)
	}
	if len(filterChain) == 0 {
		return args
	}
	return append(args, "-vf", strings.Join(filterChain, ","))
}

func appendVideoEncoderArgs(args []string, cfg config.MediaConfig, useHWAccel bool, gopSize int, softwarePreset string) []string {
	if useQSVVideoEncoder(cfg, useHWAccel) {
		return append(args,
			"-c:v", "h264_qsv",
			"-pix_fmt", "nv12",
			"-profile:v", "baseline",
			"-g", strconv.Itoa(gopSize),
			"-keyint_min", strconv.Itoa(gopSize),
			"-sc_threshold", "0",
			"-bf", "0",
		)
	}

	return append(args,
		"-c:v", "libx264",
		"-preset", softwarePreset,
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-profile:v", "baseline",
		"-g", strconv.Itoa(gopSize),
		"-keyint_min", strconv.Itoa(gopSize),
		"-sc_threshold", "0",
	)
}

func appendMJPEGEncoderArgs(args []string, cfg config.MediaConfig, useHWAccel bool) []string {
	if useQSVVideoEncoder(cfg, useHWAccel) {
		return append(args,
			"-c:v", "mjpeg_qsv",
			"-global_quality", strconv.Itoa(mapSoftwareJPEGQualityToQSV(cfg.JPEGQuality)),
		)
	}

	return append(args,
		"-q:v", strconv.Itoa(cfg.JPEGQuality),
	)
}

func mapSoftwareJPEGQualityToQSV(jpegQuality int) int {
	if jpegQuality <= 0 {
		return 80
	}
	quality := 100 - ((jpegQuality - 1) * 5)
	if quality < 1 {
		return 1
	}
	if quality > 100 {
		return 100
	}
	return quality
}

func isHardwareAccelFailure(stderrText string) bool {
	text := strings.ToLower(strings.TrimSpace(stderrText))
	if text == "" {
		return false
	}
	return strings.Contains(text, "device creation failed") ||
		strings.Contains(text, "hardware device setup failed") ||
		strings.Contains(text, "no device available for decoder") ||
		strings.Contains(text, "hevc_qsv") ||
		strings.Contains(text, "h264_qsv")
}

func redactFFmpegArgs(args []string) []string {
	redacted := make([]string, 0, len(args))
	for _, arg := range args {
		redacted = append(redacted, redactURLUserinfo(arg))
	}
	return redacted
}
