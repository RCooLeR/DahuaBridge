package media

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
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

// A live worker's final input failure is a hint: the runtime verifies the
// alternate route before publishing it and reconnecting affected workers.
func (m *Manager) reportLiveSourceFailure(ctx context.Context, streamID string, profile streams.Profile) {
	if ctx.Err() != nil || strings.HasPrefix(streamID, "nvrpb_") || strings.Contains(profile.StreamURL, "/cam/playback") {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if profile.MediaGeneration != m.streamGenerations[streamID] {
		return
	}
	if reporter, ok := m.resolver.(interface{ ReportLiveSourceFailure(string, string) }); ok {
		reporter.ReportLiveSourceFailure(streamID, profile.StreamURL)
	}
}

var ErrWorkerLimitReached = errors.New("media worker limit reached")

var ErrStreamSourceChanged = errors.New("live source changed; reconnect the stream")

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
	cfg            config.MediaConfig
	resolver       StreamResolver
	metrics        *metrics.Registry
	logger         zerolog.Logger
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	closed         bool
	processMu      sync.Mutex
	processSamples map[int]processSample

	// mu owns worker membership, generation checks, and task registration.
	// When both are needed, take mu before an individual worker's mutex.
	mu                    sync.Mutex
	audioProbeMu          sync.Mutex
	mjpegWorkers          map[string]*worker
	hlsWorkers            map[string]*hlsWorker
	dashWorkers           map[string]*dashWorker
	clipJobs              map[string]*clipJob
	webrtcPeers           map[string]*webrtcSession
	intercomUplinkEnabled map[string]bool
	audioProbeCache       map[string]audioProbeCacheEntry
	streamGenerations     map[string]uint64
	retainedOutputs       map[string]*retainedOutput
	retentionSerial       uint64
	relayStatuses         func() []WorkerStatus
}

type WorkerStatus struct {
	State                  string    `json:"state,omitempty"`
	LiveSource             string    `json:"live_source,omitempty"`
	SharedInput            bool      `json:"shared_input,omitempty"`
	Warm                   bool      `json:"warm,omitempty"`
	PreconnectMode         string    `json:"preconnect_mode,omitempty"`
	BytesReceived          uint64    `json:"bytes_received,omitempty"`
	Bitrate                uint64    `json:"bitrate_bps,omitempty"`
	Reconnects             uint64    `json:"reconnects,omitempty"`
	ProcessID              int       `json:"process_id,omitempty"`
	CPUPercent             *float64  `json:"cpu_percent,omitempty"`
	ResidentMemoryBytes    uint64    `json:"resident_memory_bytes,omitempty"`
	Key                    string    `json:"key"`
	Format                 string    `json:"format,omitempty"`
	StreamID               string    `json:"stream_id"`
	Channel                int       `json:"channel,omitempty"`
	Profile                string    `json:"profile"`
	SourceSubtype          int       `json:"source_subtype,omitempty"`
	SourceVideoCodec       string    `json:"source_video_codec,omitempty"`
	SourceAudioCodec       string    `json:"source_audio_codec,omitempty"`
	SourceWidth            int       `json:"source_width,omitempty"`
	SourceHeight           int       `json:"source_height,omitempty"`
	OutputVideoCodec       string    `json:"output_video_codec,omitempty"`
	OutputVideoEncoder     string    `json:"output_video_encoder,omitempty"`
	OutputAudioCodec       string    `json:"output_audio_codec,omitempty"`
	OutputWidth            int       `json:"output_width,omitempty"`
	OutputHeight           int       `json:"output_height,omitempty"`
	RTSPTransport          string    `json:"rtsp_transport,omitempty"`
	InputPreset            string    `json:"input_preset,omitempty"`
	HWAccelActive          bool      `json:"hwaccel_active,omitempty"`
	AudioEnabled           bool      `json:"audio_enabled,omitempty"`
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

	mu                sync.Mutex
	subscribers       map[chan []byte]struct{}
	subscribersClosed bool
	idleGeneration    uint64
	lastFrame         []byte
	lastFrameAt       time.Time
	lastError         error
	startedAt         time.Time
	activeAttempt     ffmpegStartAttempt
	includeAudio      bool
	cmd               *exec.Cmd
	processID         int
	ready             chan struct{}
	startErr          chan error
	readyOnce         sync.Once
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

	mu                  sync.Mutex
	lastAccessAt        time.Time
	lastError           error
	startedAt           time.Time
	outputDir           string
	activeAttempt       ffmpegStartAttempt
	includeAudio        bool
	cmd                 *exec.Cmd
	processID           int
	startErr            chan error
	playlistReadyLogged bool
	completed           bool // Protected by parent.mu; retained output has no running process.
}

type ffmpegStartAttempt struct {
	useHWAccel  bool
	inputPreset string
}

func New(cfg config.MediaConfig, resolver StreamResolver, logger zerolog.Logger, metricsRegistry *metrics.Registry) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		ctx:                   ctx,
		cancel:                cancel,
		cfg:                   cfg,
		resolver:              resolver,
		metrics:               metricsRegistry,
		logger:                logger.With().Str("component", "media").Logger(),
		mjpegWorkers:          make(map[string]*worker),
		hlsWorkers:            make(map[string]*hlsWorker),
		dashWorkers:           make(map[string]*dashWorker),
		clipJobs:              make(map[string]*clipJob),
		webrtcPeers:           make(map[string]*webrtcSession),
		intercomUplinkEnabled: make(map[string]bool),
		audioProbeCache:       make(map[string]audioProbeCacheEntry),
		streamGenerations:     make(map[string]uint64),
		retainedOutputs:       make(map[string]*retainedOutput),
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

	unsubscribed := make(chan struct{})
	var unsubscribeOnce sync.Once
	unsubscribe := func() {
		unsubscribeOnce.Do(func() {
			w.removeSubscriber(ch)
			close(unsubscribed)
		})
	}

	if err := w.waitUntilReady(ctx); err != nil {
		unsubscribe()
		return nil, nil, err
	}

	m.runTask(func() {
		select {
		case <-ctx.Done():
		case <-w.ctx.Done():
		case <-m.ctx.Done():
		case <-unsubscribed:
			return
		}
		unsubscribe()
	})

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
	statuses := m.workerStatusesLocked()
	relayStatuses := m.relayStatuses
	m.mu.Unlock()
	m.enrichProcessStatuses(statuses)
	if relayStatuses != nil {
		statuses = append(statuses, relayStatuses()...)
	}
	return statuses
}

func (m *Manager) AttachRelayStatuses(statuses func() []WorkerStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relayStatuses = statuses
}

func (m *Manager) resolveStream(streamID string, profileName string) (streams.Entry, streams.Profile, string, error) {
	if strings.TrimSpace(profileName) == "" {
		profileName = "stable"
	}

	// Capture before resolving: a concurrent source switch must not let an old
	// resolution start a worker after InvalidateStream has removed the old input.
	m.mu.Lock()
	generation := m.streamGenerations[streamID]
	m.mu.Unlock()
	entry, profile, ok := m.resolver.GetStream(streamID, profileName, true)
	if !ok {
		return streams.Entry{}, streams.Profile{}, "", fmt.Errorf("stream %q profile %q not found", streamID, profileName)
	}
	if strings.TrimSpace(profile.StreamURL) == "" {
		if entry.LiveSource != nil && entry.LiveSource.Source == streams.LiveSourceCamera {
			return streams.Entry{}, streams.Profile{}, "", streams.ErrLiveSourceUnavailable
		}
		return streams.Entry{}, streams.Profile{}, "", fmt.Errorf("stream %q has no video source", streamID)
	}
	profile.MediaGeneration = generation
	if !strings.HasPrefix(streamID, "nvrpb_") && profile.InputDuration == 0 && profile.InputPrefixURL == "" {
		if relay, ok := m.resolver.(interface{ LiveMediaInput(string, string) string }); ok {
			profile.LiveRelayURL = relay.LiveMediaInput(streamID, profileName)
			if profile.LiveRelayURL != "" {
				profile.RTSPTransport = "tcp"
			}
		}
	}
	return entry, profile, profileName, nil
}

// InvalidateStream reconnects live viewers after an upstream change. Finite
// recordings keep their original input so changing the viewer does not cut a clip.
func (m *Manager) InvalidateStream(streamID string) {
	if m == nil || streamID == "" {
		return
	}
	m.mu.Lock()
	if m.streamGenerations == nil {
		m.streamGenerations = make(map[string]uint64)
	}
	m.streamGenerations[streamID]++
	var cancels []context.CancelFunc
	for key, w := range m.mjpegWorkers {
		if w.streamID == streamID {
			cancels = append(cancels, w.cancel)
			delete(m.mjpegWorkers, key)
		}
	}
	for key, w := range m.hlsWorkers {
		if w.streamID == streamID {
			cancels = append(cancels, w.cancel)
			delete(m.hlsWorkers, key)
		}
	}
	for key, w := range m.dashWorkers {
		if w.streamID == streamID {
			cancels = append(cancels, w.cancel)
			delete(m.dashWorkers, key)
		}
	}
	for key, session := range m.webrtcPeers {
		if session.streamID == streamID {
			cancels = append(cancels, session.cancel)
			delete(m.webrtcPeers, key)
		}
	}
	if m.metrics != nil {
		m.metrics.SetMediaViewers(streamID, "quality", 0)
		m.metrics.SetMediaViewers(streamID, "stable", 0)
	}
	m.setMediaWorkerCountLocked()
	m.mu.Unlock()
	for _, cancel := range cancels {
		if cancel != nil {
			cancel()
		}
	}
}

func (m *Manager) removeWebRTCSession(key string, session *webrtcSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.webrtcPeers[key]; ok && existing == session {
		delete(m.webrtcPeers, key)
		m.setMediaWorkerCountLocked()
		m.logWorkerInventoryLocked("removed", session.status())
		if m.metrics != nil {
			m.metrics.SetMediaViewers(session.streamID, session.profileName, 0)
		}
	}
}

func (m *Manager) activeWorkerCountLocked() int {
	count := len(m.mjpegWorkers) + len(m.clipJobs) + len(m.webrtcPeers)
	for _, w := range m.hlsWorkers {
		if !w.completed {
			count++
		}
	}
	for _, w := range m.dashWorkers {
		if !w.completed {
			count++
		}
	}
	return count
}

func (m *Manager) setMediaWorkerCountLocked() {
	if m.metrics != nil {
		m.metrics.SetMediaWorkers(m.activeWorkerCountLocked())
	}
}

func redactFFmpegArgs(args []string) []string {
	redacted := make([]string, 0, len(args))
	for _, arg := range args {
		redacted = append(redacted, redactURLUserinfo(arg))
	}
	return redacted
}

func (m *Manager) workerStatusesLocked() []WorkerStatus {
	statuses := make([]WorkerStatus, 0, len(m.mjpegWorkers)+len(m.hlsWorkers)+len(m.dashWorkers)+len(m.clipJobs)+len(m.webrtcPeers))
	for _, w := range m.mjpegWorkers {
		statuses = append(statuses, w.status())
	}
	for _, w := range m.hlsWorkers {
		status := w.status()
		if w.completed {
			status.State = "retained"
		}
		statuses = append(statuses, status)
	}
	for _, w := range m.dashWorkers {
		status := w.status()
		if w.completed {
			status.State = "retained"
		}
		statuses = append(statuses, status)
	}
	for _, job := range m.clipJobs {
		statuses = append(statuses, job.status())
	}
	for _, session := range m.webrtcPeers {
		statuses = append(statuses, session.status())
	}
	return statuses
}

func (m *Manager) logWorkerInventoryLocked(action string, focus WorkerStatus) {
	m.logger.Info().
		Str("worker_action", action).
		Str("worker", workerStatusSummaryLine(focus)).
		Int("active_workers", m.activeWorkerCountLocked()).
		Msg("media worker inventory updated")
}

func workerStatusSortKey(status WorkerStatus) string {
	return fmt.Sprintf("%04d:%s:%s:%s:%s", status.Channel, workerStatusScope(status), strings.ToLower(status.Format), workerProfileStreamLabel(status.Profile), status.Key)
}

func workerStatusSummaryLine(status WorkerStatus) string {
	label := workerStatusTypeLabel(status)
	stream := workerProfileStreamLabel(status.Profile)
	details := workerStatusDebugDetails(status)
	if status.Channel > 0 {
		return fmt.Sprintf("%s channel %d stream %s%s", label, status.Channel, stream, details)
	}
	return fmt.Sprintf("%s %s stream %s%s", label, status.StreamID, stream, details)
}

func workerStatusDebugDetails(status WorkerStatus) string {
	parts := make([]string, 0, 4)

	inputVideo := workerCodecLabel(status.SourceVideoCodec)
	if inputVideo != "" {
		if resolution := workerResolutionLabel(status.SourceWidth, status.SourceHeight); resolution != "" {
			inputVideo += " " + resolution
		}
	}
	inputAudio := workerCodecLabel(status.SourceAudioCodec)
	if inputVideo != "" || inputAudio != "" {
		input := inputVideo
		if inputAudio != "" {
			if input != "" {
				input += "/"
			}
			input += inputAudio
		}
		if input != "" {
			parts = append(parts, "in "+input)
		}
	}

	outputVideo := workerCodecLabel(status.OutputVideoCodec)
	if outputVideo != "" {
		if resolution := workerResolutionLabel(status.OutputWidth, status.OutputHeight); resolution != "" {
			outputVideo += " " + resolution
		}
	}
	outputAudio := workerCodecLabel(status.OutputAudioCodec)
	if outputVideo != "" || outputAudio != "" {
		output := outputVideo
		if outputAudio != "" {
			if output != "" {
				output += "/"
			}
			output += outputAudio
		}
		if output != "" {
			parts = append(parts, "out "+output)
		}
	}

	if status.InputPreset != "" {
		parts = append(parts, status.InputPreset)
	}
	if status.HWAccelActive {
		parts = append(parts, "hw")
	} else if status.OutputVideoEncoder != "" {
		parts = append(parts, "sw")
	}

	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, " | ") + "]"
}

func workerStatusTypeLabel(status WorkerStatus) string {
	if strings.EqualFold(strings.TrimSpace(status.Format), "clip") {
		if workerStatusScope(status) == "archive" {
			return "MP4 export"
		}
		return "MP4 clip"
	}
	scopePrefix := "Live"
	if workerStatusScope(status) == "archive" {
		scopePrefix = "Archive"
	}
	return scopePrefix + " " + workerFormatLabel(status.Format)
}

func workerStatusScope(status WorkerStatus) string {
	if strings.HasPrefix(strings.TrimSpace(status.StreamID), "nvrpb_") {
		return "archive"
	}
	return "live"
}

func workerFormatLabel(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "hls":
		return "HLS"
	case "dash":
		return "DASH"
	case "mjpeg":
		return "MJPEG"
	case "webrtc":
		return "WebRTC"
	case "clip":
		return "MP4"
	default:
		normalized := strings.TrimSpace(format)
		if normalized == "" {
			return "Worker"
		}
		return strings.ToUpper(normalized)
	}
}

func workerProfileStreamLabel(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "quality", "default", "main":
		return "main"
	case "stable", "substream", "sub":
		return "substream"
	case "":
		return "default"
	default:
		return strings.TrimSpace(profile)
	}
}

func workerCodecLabel(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "", "unknown":
		return ""
	case "h264", "h.264", "smart h.264+", "h.264b", "h.264h", "h.264m":
		return "H.264"
	case "h265", "h.265", "hevc", "smart h.265+", "h.265h":
		return "H.265"
	case "aac":
		return "AAC"
	case "mjpeg", "mjpg":
		return "MJPEG"
	case "opus", "audio/opus":
		return "Opus"
	case "g.711a":
		return "G.711A"
	case "g.711u":
		return "G.711U"
	default:
		return strings.TrimSpace(codec)
	}
}

func workerResolutionLabel(width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func resolvedOutputDimensions(sourceWidth int, sourceHeight int, requestedScaleWidth int) (int, int) {
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return 0, 0
	}
	if targetWidth, targetHeight, ok := computeScaledDimensions(sourceWidth, sourceHeight, requestedScaleWidth); ok {
		return targetWidth, targetHeight
	}
	return sourceWidth, sourceHeight
}

func workerH264EncoderLabel(cfg config.MediaConfig, useHWAccel bool) string {
	if useQSVVideoEncoder(cfg, useHWAccel) {
		return "h264_qsv"
	}
	return "libx264"
}

func workerMJPEGEncoderLabel(cfg config.MediaConfig, useHWAccel bool) string {
	if useQSVVideoEncoder(cfg, useHWAccel) {
		return "mjpeg_qsv"
	}
	return "mjpeg"
}

func conditionalCodec(enabled bool, codec string) string {
	if !enabled {
		return ""
	}
	return codec
}

func detectWorkerChannel(streamID string, sourceURL string) int {
	streamID = strings.TrimSpace(streamID)
	if _, channelText, ok := strings.CutLast(streamID, "_channel_"); ok {
		channel, err := strconv.Atoi(strings.TrimLeft(channelText, "0"))
		if err == nil && channel > 0 {
			return channel
		}
		if strings.HasSuffix(streamID, "_channel_00") {
			return 0
		}
	}

	parsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil {
		return 0
	}
	channel, err := strconv.Atoi(strings.TrimSpace(parsed.Query().Get("channel")))
	if err != nil || channel <= 0 {
		return 0
	}
	return channel
}
