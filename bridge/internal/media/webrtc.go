package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
)

type webrtcSession struct {
	key         string
	streamID    string
	profileName string
	profile     streams.Profile
	parent      *Manager
	logger      zerolog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mu                     sync.Mutex
	startedAt              time.Time
	lastAccessAt           time.Time
	lastError              error
	cmd                    *exec.Cmd
	peer                   *webrtc.PeerConnection
	hasAudio               bool
	uplinkActive           bool
	uplinkCodec            string
	uplinkPackets          uint64
	uplinkTargetCount      int
	uplinkForwardedPackets uint64
	uplinkForwardErrors    uint64
}

type uplinkForwarder struct {
	target string
	conn   *net.UDPConn
}

func (m *Manager) WebRTCAnswer(ctx context.Context, streamID string, profileName string, offer WebRTCSessionDescription) (WebRTCSessionDescription, error) {
	if !m.Enabled() {
		return WebRTCSessionDescription{}, errors.New("media layer is disabled")
	}

	entry, profile, resolvedProfileName, err := m.resolveStream(streamID, profileName)
	if err != nil {
		return WebRTCSessionDescription{}, err
	}

	key := fmt.Sprintf("%s:%s:webrtc:%d", entry.ID, resolvedProfileName, time.Now().UnixNano())
	sessionCtx, cancel := context.WithCancel(context.Background())
	session := &webrtcSession{
		key:         key,
		streamID:    entry.ID,
		profileName: resolvedProfileName,
		profile:     profile,
		parent:      m,
		logger: m.logger.With().
			Str("stream_id", entry.ID).
			Str("profile", resolvedProfileName).
			Str("format", "webrtc").
			Logger(),
		ctx:          sessionCtx,
		cancel:       cancel,
		lastAccessAt: time.Now(),
	}

	m.mu.Lock()
	if m.cfg.MaxWorkers > 0 && m.activeWorkerCountLocked() >= m.cfg.MaxWorkers {
		err := fmt.Errorf("%w: %d active, max %d", ErrWorkerLimitReached, m.activeWorkerCountLocked(), m.cfg.MaxWorkers)
		m.mu.Unlock()
		if m.metrics != nil {
			m.metrics.ObserveMediaStart(entry.ID, resolvedProfileName, err)
		}
		cancel()
		return WebRTCSessionDescription{}, err
	}
	m.webrtcPeers[key] = session
	m.setMediaWorkerCountLocked()
	if m.metrics != nil {
		m.metrics.SetMediaViewers(entry.ID, resolvedProfileName, 1)
	}
	m.mu.Unlock()

	answer, err := session.start(ctx, offer)
	if err != nil {
		if m.metrics != nil {
			m.metrics.ObserveMediaStart(entry.ID, resolvedProfileName, err)
		}
		session.cancel()
		m.removeWebRTCSession(key, session)
		return WebRTCSessionDescription{}, err
	}

	if m.metrics != nil {
		m.metrics.ObserveMediaStart(entry.ID, resolvedProfileName, nil)
	}
	return answer, nil
}

func (s *webrtcSession) start(ctx context.Context, offer WebRTCSessionDescription) (WebRTCSessionDescription, error) {
	remoteDescription, err := toPionSessionDescription(offer)
	if err != nil {
		return WebRTCSessionDescription{}, err
	}

	videoConn, err := listenLocalRTP()
	if err != nil {
		return WebRTCSessionDescription{}, fmt.Errorf("open local rtp socket: %w", err)
	}
	videoPort := udpPort(videoConn.LocalAddr())
	audioConn, err := listenLocalRTP()
	if err != nil {
		_ = videoConn.Close()
		return WebRTCSessionDescription{}, fmt.Errorf("open local audio rtp socket: %w", err)
	}
	audioPort := udpPort(audioConn.LocalAddr())

	peerConnection, tracks, err := newPeerConnection(s.parent.WebRTCICEServers())
	if err != nil {
		_ = videoConn.Close()
		_ = audioConn.Close()
		return WebRTCSessionDescription{}, err
	}

	s.mu.Lock()
	s.startedAt = time.Now()
	s.peer = peerConnection
	s.mu.Unlock()

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.touch()
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateClosed:
			s.cancel()
		}
	})
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		s.touch()
		if track == nil || track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}

		s.mu.Lock()
		s.uplinkActive = true
		s.uplinkCodec = strings.TrimSpace(track.Codec().MimeType)
		s.mu.Unlock()

		go s.receiveIncomingAudio(track)
	})

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	if err := peerConnection.SetRemoteDescription(remoteDescription); err != nil {
		_ = peerConnection.Close()
		_ = videoConn.Close()
		_ = audioConn.Close()
		return WebRTCSessionDescription{}, fmt.Errorf("set remote description: %w", err)
	}

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		_ = peerConnection.Close()
		_ = videoConn.Close()
		_ = audioConn.Close()
		return WebRTCSessionDescription{}, fmt.Errorf("create answer: %w", err)
	}
	if err := peerConnection.SetLocalDescription(answer); err != nil {
		_ = peerConnection.Close()
		_ = videoConn.Close()
		_ = audioConn.Close()
		return WebRTCSessionDescription{}, fmt.Errorf("set local description: %w", err)
	}

	select {
	case <-ctx.Done():
		_ = peerConnection.Close()
		_ = videoConn.Close()
		_ = audioConn.Close()
		return WebRTCSessionDescription{}, ctx.Err()
	case <-gatherComplete:
	}

	for _, sender := range tracks.senders {
		go drainRTCP(s.ctx, sender)
	}
	go s.forwardRTP(videoConn, tracks.video)
	go s.forwardRTP(audioConn, tracks.audio)
	cmd, err := s.startFFmpeg(videoPort, audioPort)
	if err != nil {
		_ = peerConnection.Close()
		_ = videoConn.Close()
		_ = audioConn.Close()
		return WebRTCSessionDescription{}, err
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	go s.closePeerOnCancel()
	go s.waitForFFmpeg(cmd, videoConn, audioConn)

	localDescription := peerConnection.LocalDescription()
	if localDescription == nil {
		return WebRTCSessionDescription{}, errors.New("local description is not available")
	}

	return WebRTCSessionDescription{
		Type: localDescription.Type.String(),
		SDP:  localDescription.SDP,
	}, nil
}

type webrtcTracks struct {
	video   *webrtc.TrackLocalStaticRTP
	audio   *webrtc.TrackLocalStaticRTP
	senders []*webrtc.RTPSender
}

func newPeerConnection(iceServers []WebRTCICEServer) (*webrtc.PeerConnection, webrtcTracks, error) {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, webrtcTracks{}, fmt.Errorf("register webrtc codecs: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: toPionICEServers(iceServers),
	})
	if err != nil {
		return nil, webrtcTracks{}, fmt.Errorf("create peer connection: %w", err)
	}

	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
		MimeType:    webrtc.MimeTypeH264,
		ClockRate:   90000,
		SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
	}, "video", "dahuabridge")
	if err != nil {
		_ = peerConnection.Close()
		return nil, webrtcTracks{}, fmt.Errorf("create video track: %w", err)
	}

	videoSender, err := peerConnection.AddTrack(videoTrack)
	if err != nil {
		_ = peerConnection.Close()
		return nil, webrtcTracks{}, fmt.Errorf("add video track: %w", err)
	}

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: 48000,
		Channels:  2,
	}, "audio", "dahuabridge")
	if err != nil {
		_ = peerConnection.Close()
		return nil, webrtcTracks{}, fmt.Errorf("create audio track: %w", err)
	}

	audioSender, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		_ = peerConnection.Close()
		return nil, webrtcTracks{}, fmt.Errorf("add audio track: %w", err)
	}

	return peerConnection, webrtcTracks{
		video:   videoTrack,
		audio:   audioTrack,
		senders: []*webrtc.RTPSender{videoSender, audioSender},
	}, nil
}

func (s *webrtcSession) startFFmpeg(videoPort int, audioPort int) (*exec.Cmd, error) {
	args := s.buildFFmpegArgs(videoPort, audioPort)
	s.logger.Debug().Strs("ffmpeg_args", redactFFmpegArgs(args)).Msg("starting webrtc ffmpeg")
	cmd := exec.CommandContext(s.ctx, s.parent.cfg.FFmpegPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stderr.Close()
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	go func() {
		<-s.ctx.Done()
		_ = stderr.Close()
	}()
	go s.captureFFmpegStderr(stderr)
	return cmd, nil
}

func (s *webrtcSession) captureFFmpegStderr(stderr io.ReadCloser) {
	body, _ := io.ReadAll(io.LimitReader(stderr, 64*1024))
	message := strings.TrimSpace(string(body))
	if message == "" || s.ctx.Err() != nil {
		return
	}
	s.logger.Debug().Str("ffmpeg_stderr", message).Msg("webrtc ffmpeg stderr")
	s.setError(errors.New(message))
}

func (s *webrtcSession) buildFFmpegArgs(videoPort int, audioPort int) []string {
	frameRate := s.parent.cfg.FrameRate
	if s.profile.FrameRate > 0 {
		frameRate = s.profile.FrameRate
	}
	gopSize := maxInt(frameRate, frameRate*2)

	args := []string{
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(s.parent.cfg),
	}
	args = append(args, s.parent.cfg.HWAccelArgs...)
	args = append(args, buildRTSPInputArgs(s.profile, s.parent.cfg.InputPreset)...)
	if s.parent.cfg.Threads > 0 {
		args = append(args, "-threads", strconv.Itoa(s.parent.cfg.Threads))
	}
	args = append(args,
		"-map", "0:v:0",
	)
	args = appendVideoEncoderArgs(args, s.parent.cfg, hardwareAccelEnabled(s.parent.cfg.HWAccelArgs), gopSize, "ultrafast")
	filterChain := buildFilterChain(frameRate, s.parent.cfg.ScaleWidth, s.parent.cfg.ScaleHeight)
	if len(filterChain) > 0 {
		args = append(args, "-vf", strings.Join(filterChain, ","))
	}
	args = append(args,
		"-f", "rtp",
		fmt.Sprintf("rtp://127.0.0.1:%d?pkt_size=1200", videoPort),
	)
	args = append(args,
		"-map", "0:a:0?",
		"-vn",
		"-c:a", "libopus",
		"-ac", "2",
		"-ar", "48000",
		"-application", "lowdelay",
		"-frame_duration", "20",
		"-f", "rtp",
		fmt.Sprintf("rtp://127.0.0.1:%d?pkt_size=1200", audioPort),
	)
	return args
}

func (s *webrtcSession) forwardRTP(conn *net.UDPConn, track *webrtc.TrackLocalStaticRTP) {
	defer conn.Close()
	if track == nil {
		return
	}

	buffer := make([]byte, 1600)
	packetReceived := false
	for {
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			if s.ctx.Err() != nil {
				return
			}
			s.setError(fmt.Errorf("set rtp read deadline: %w", err))
			s.cancel()
			return
		}

		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			s.setError(fmt.Errorf("read webrtc rtp: %w", err))
			s.cancel()
			return
		}

		var packet rtp.Packet
		if err := packet.Unmarshal(buffer[:n]); err != nil {
			continue
		}
		packetReceived = true
		if err := track.WriteRTP(&packet); err != nil {
			if s.ctx.Err() != nil {
				return
			}
			s.setError(fmt.Errorf("write webrtc rtp: %w", err))
			s.cancel()
			return
		}
		if !s.hasAudio && track.Kind() == webrtc.RTPCodecTypeAudio && packetReceived {
			s.mu.Lock()
			s.hasAudio = true
			s.mu.Unlock()
		}
		s.touch()
	}
}

func (s *webrtcSession) receiveIncomingAudio(track *webrtc.TrackRemote) {
	forwarders := newUplinkForwarders(s.parent.cfg.WebRTCUplinkTargets, s.logger)
	defer closeUplinkForwarders(forwarders)

	s.mu.Lock()
	s.uplinkTargetCount = len(forwarders)
	s.mu.Unlock()

	for {
		if s.ctx.Err() != nil {
			return
		}

		packet, _, err := track.ReadRTP()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			if !errors.Is(err, io.EOF) {
				s.setError(fmt.Errorf("read incoming webrtc audio: %w", err))
			}
			return
		}
		if packet == nil {
			continue
		}

		s.mu.Lock()
		s.uplinkActive = true
		s.uplinkPackets++
		s.mu.Unlock()
		s.forwardIncomingAudioPacket(forwarders, packet)
		s.touch()
	}
}

func (s *webrtcSession) forwardIncomingAudioPacket(forwarders []*uplinkForwarder, packet *rtp.Packet) {
	if len(forwarders) == 0 || packet == nil {
		return
	}
	if !s.parent.IntercomUplinkEnabled(s.streamID) {
		return
	}

	payload, err := packet.Marshal()
	if err != nil {
		s.mu.Lock()
		s.uplinkForwardErrors++
		s.mu.Unlock()
		s.logger.Warn().Err(err).Msg("marshal incoming audio rtp for uplink forwarding failed")
		return
	}

	for _, forwarder := range forwarders {
		if forwarder == nil || forwarder.conn == nil {
			continue
		}
		if _, err := forwarder.conn.Write(payload); err != nil {
			s.mu.Lock()
			s.uplinkForwardErrors++
			s.mu.Unlock()
			s.logger.Warn().Err(err).Str("target", forwarder.target).Msg("forward incoming audio rtp to uplink target failed")
			continue
		}
		s.mu.Lock()
		s.uplinkForwardedPackets++
		s.mu.Unlock()
	}
}

func (s *webrtcSession) closePeerOnCancel() {
	<-s.ctx.Done()
	s.mu.Lock()
	peer := s.peer
	s.mu.Unlock()
	if peer != nil {
		_ = peer.Close()
	}
}

func (s *webrtcSession) waitForFFmpeg(cmd *exec.Cmd, conns ...*net.UDPConn) {
	defer s.parent.removeWebRTCSession(s.key, s)
	defer s.cancel()
	for _, conn := range conns {
		if conn != nil {
			defer conn.Close()
		}
	}

	err := cmd.Wait()
	if errors.Is(s.ctx.Err(), context.Canceled) {
		return
	}
	if err != nil {
		s.setError(err)
		s.cancel()
	}
}

func (s *webrtcSession) touch() {
	s.mu.Lock()
	s.lastAccessAt = time.Now()
	s.mu.Unlock()
}

func (s *webrtcSession) setError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	if s.lastError == nil {
		s.lastError = err
	}
	s.mu.Unlock()
	s.logger.Error().Err(err).Msg("webrtc session error")
}

func (s *webrtcSession) status() WorkerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := WorkerStatus{
		Key:          s.key,
		Format:       "webrtc",
		StreamID:     s.streamID,
		Profile:      s.profileName,
		Viewers:      1,
		LastAccessAt: s.lastAccessAt,
		StartedAt:    s.startedAt,
		SourceURL:    redactURLUserinfo(s.profile.StreamURL),
		Recommended:  s.profile.Recommended,
		FrameRate:    maxInt(s.profile.FrameRate, s.parent.cfg.FrameRate),
		Threads:      s.parent.cfg.Threads,
		ScaleWidth:   s.parent.cfg.ScaleWidth,
		ScaleHeight:  s.parent.cfg.ScaleHeight,
		MaxWorkers:   s.parent.cfg.MaxWorkers,
		IdleTimeout:  s.parent.cfg.IdleTimeout.String(),
		FFmpegPath:   s.parent.cfg.FFmpegPath,
	}
	if s.lastError != nil {
		status.LastError = s.lastError.Error()
	}
	status.UplinkActive = s.uplinkActive
	status.UplinkPackets = s.uplinkPackets
	status.UplinkCodec = s.uplinkCodec
	status.ExternalUplinkEnabled = s.parent.IntercomUplinkEnabled(s.streamID)
	status.UplinkTargetCount = s.uplinkTargetCount
	status.UplinkForwardedPackets = s.uplinkForwardedPackets
	status.UplinkForwardErrors = s.uplinkForwardErrors
	return status
}

func drainRTCP(ctx context.Context, sender *webrtc.RTPSender) {
	if sender == nil {
		return
	}

	buffer := make([]byte, 1500)
	for {
		if ctx.Err() != nil {
			return
		}
		if _, _, err := sender.Read(buffer); err != nil {
			return
		}
	}
}

func listenLocalRTP() (*net.UDPConn, error) {
	return net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})
}

func udpPort(addr net.Addr) int {
	if udpAddr, ok := addr.(*net.UDPAddr); ok {
		return udpAddr.Port
	}
	return 0
}

func toPionSessionDescription(input WebRTCSessionDescription) (webrtc.SessionDescription, error) {
	typ := webrtc.NewSDPType(strings.TrimSpace(input.Type))
	if typ == webrtc.SDPTypeUnknown {
		return webrtc.SessionDescription{}, fmt.Errorf("unsupported sdp type %q", input.Type)
	}
	sdp := strings.TrimSpace(input.SDP)
	if sdp == "" {
		return webrtc.SessionDescription{}, errors.New("sdp is required")
	}
	return webrtc.SessionDescription{
		Type: typ,
		SDP:  sdp,
	}, nil
}

func toPionICEServers(input []WebRTCICEServer) []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0, len(input))
	for _, server := range input {
		urls := make([]string, 0, len(server.URLs))
		for _, rawURL := range server.URLs {
			if strings.TrimSpace(rawURL) != "" {
				urls = append(urls, strings.TrimSpace(rawURL))
			}
		}
		if len(urls) == 0 {
			continue
		}
		servers = append(servers, webrtc.ICEServer{
			URLs:       urls,
			Username:   strings.TrimSpace(server.Username),
			Credential: strings.TrimSpace(server.Credential),
		})
	}
	return servers
}

func newUplinkForwarders(targets []string, logger zerolog.Logger) []*uplinkForwarder {
	forwarders := make([]*uplinkForwarder, 0, len(targets))
	for _, rawTarget := range targets {
		forwarder, err := dialUplinkForwarder(rawTarget)
		if err != nil {
			logger.Warn().Err(err).Str("target", rawTarget).Msg("skip invalid webrtc uplink target")
			continue
		}
		forwarders = append(forwarders, forwarder)
	}
	return forwarders
}

func dialUplinkForwarder(rawTarget string) (*uplinkForwarder, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawTarget))
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("missing host")
	}

	addr, err := net.ResolveUDPAddr("udp", parsed.Host)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	return &uplinkForwarder{
		target: rawTarget,
		conn:   conn,
	}, nil
}

func closeUplinkForwarders(forwarders []*uplinkForwarder) {
	for _, forwarder := range forwarders {
		if forwarder != nil && forwarder.conn != nil {
			_ = forwarder.conn.Close()
		}
	}
}
