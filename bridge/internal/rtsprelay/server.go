// Package rtsprelay shares live camera/NVR RTP streams behind stable bridge URLs.
package rtsprelay

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/auth"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/liberrors"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"

	"RCooLeR/DahuaBridge/internal/streams"
)

const pathPrefix = "/api/v1/rtsp/live/"

var errUnavailable = errors.New("live RTSP stream unavailable")

type Config struct {
	ListenAddress string
	AuthToken     string
	IdleTimeout   time.Duration
	MaxStreams    int
	StartTimeout  time.Duration
	// Timing overrides are for embedded callers/tests; application defaults are
	// a 15-second reconciliation interval and five-minute recent retention.
	PreconnectInterval time.Duration
	RecentKeepAlive    time.Duration
}

type Resolver interface {
	GetStream(string, string, bool) (streams.Entry, streams.Profile, bool)
	ReportLiveSourceFailure(string, string)
}

type Server struct {
	cfg                Config
	resolver           Resolver
	logger             zerolog.Logger
	ctx                context.Context
	cancel             context.CancelFunc
	mu                 sync.Mutex
	server             *gortsplib.Server
	address            string
	started            bool
	closed             bool
	workers            map[string]*relayStream
	starts             map[string]uint64
	sessions           map[*gortsplib.ServerSession]*relayStream
	conns              map[*gortsplib.ServerConn]*relayStream
	wg                 sync.WaitGroup
	closeOnce          sync.Once
	preconnectWake     chan struct{}
	preconnectRevision uint64
	preconnectMode     string
	preconnectTargets  map[string]streams.LivePreconnectTarget
	preconnectRetry    map[string]time.Time
	preconnectStarting int
}

type relayStream struct {
	key, id, profile string
	ctx              context.Context
	cancel           context.CancelFunc
	ready            chan struct{}
	readyClosed      bool
	err              error
	stream           *gortsplib.ServerStream
	lastUsed         time.Time
	readers          map[*gortsplib.ServerSession]reader
	conns            map[*gortsplib.ServerConn]struct{}
	retired          bool
	startTimedOut    atomic.Bool
	startedAt        time.Time
	source           string
	sourceURL        string
	channel          int
	videoCodec       string
	audioCodec       string
	bytesReceived    atomic.Uint64
	lastPacket       atomic.Int64
	previousBytes    uint64
	previousSample   time.Time
	bitrate          uint64
	pendingDemand    int
	lastViewed       time.Time
	warm             bool
	warmStarting     bool
}

type reader struct {
	conn    *gortsplib.ServerConn
	playing bool
}

func New(cfg Config, resolver Resolver, logger zerolog.Logger) *Server {
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = ":8554"
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 2 * time.Minute
	}
	if cfg.MaxStreams <= 0 {
		cfg.MaxStreams = 36
	}
	if cfg.StartTimeout <= 0 {
		cfg.StartTimeout = 15 * time.Second
	}
	if cfg.PreconnectInterval <= 0 {
		cfg.PreconnectInterval = 15 * time.Second
	}
	if cfg.RecentKeepAlive <= 0 {
		cfg.RecentKeepAlive = 5 * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		cfg: cfg, resolver: resolver, logger: logger, ctx: ctx, cancel: cancel,
		workers: make(map[string]*relayStream), starts: make(map[string]uint64), sessions: make(map[*gortsplib.ServerSession]*relayStream),
		conns:          make(map[*gortsplib.ServerConn]*relayStream),
		preconnectWake: make(chan struct{}, 1), preconnectTargets: make(map[string]streams.LivePreconnectTarget), preconnectRetry: make(map[string]time.Time),
	}
}

// Start binds the TCP listener and returns without waiting for clients.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("RTSP relay is closed")
	}
	if s.started {
		return nil
	}
	if s.resolver == nil {
		return errors.New("RTSP relay resolver is required")
	}
	s.server = &gortsplib.Server{
		RTSPAddress: s.cfg.ListenAddress, Handler: s,
		ReadTimeout: s.cfg.StartTimeout, WriteTimeout: 10 * time.Second,
		AuthMethods: []auth.VerifyMethod{auth.VerifyMethodDigestMD5, auth.VerifyMethodBasic},
		Listen: func(network, address string) (net.Listener, error) {
			listener, err := net.Listen(network, address)
			if err == nil {
				s.address = listener.Addr().String()
			}
			return listener, err
		},
	}
	if err := s.server.Start(); err != nil {
		return err
	}
	s.started = true
	s.wg.Add(1)
	go s.reapIdle()
	if _, ok := s.resolver.(preconnectResolver); ok {
		s.wg.Add(1)
		go s.runPreconnect()
	}
	return nil
}

func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.address
}

// LocalStreamURL is used by this process's FFmpeg consumers, not by HA clients.
func (s *Server) LocalStreamURL(id, profile string) string {
	path := pathPrefix + id + "/" + profile
	if _, _, ok := streamPath(path, ""); !ok {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.closed {
		return ""
	}
	host, port, err := net.SplitHostPort(s.address)
	if err != nil {
		return ""
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	u := &url.URL{Scheme: "rtsp", Host: net.JoinHostPort(host, port), Path: path}
	if s.cfg.AuthToken != "" {
		u.User = url.UserPassword("dahuabridge", s.cfg.AuthToken)
	}
	return u.String()
}

// Close prevents new worker registration before canceling and joining all
// existing workers and the warm reconciler. It is safe to call concurrently.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		workers := make([]*relayStream, 0, len(s.workers))
		for _, worker := range s.workers {
			workers = append(workers, worker)
		}
		started, server := s.started, s.server
		s.mu.Unlock()
		s.cancel()
		for _, worker := range workers {
			s.retire(worker)
		}
		if started {
			server.Close()
		}
		s.wg.Wait()
	})
}

// InvalidateStream disconnects readers so they reopen the same URL against the
// newly resolved source. It does not wait for upstream shutdown.
func (s *Server) InvalidateStream(id string) {
	s.mu.Lock()
	s.preconnectRevision++
	for key := range s.preconnectRetry {
		if strings.HasPrefix(key, id+":") {
			delete(s.preconnectRetry, key)
		}
	}
	workers := make([]*relayStream, 0, 2)
	for _, worker := range s.workers {
		if worker.id == id {
			workers = append(workers, worker)
		}
	}
	s.mu.Unlock()
	for _, worker := range workers {
		s.retire(worker)
	}
	s.WakePreconnect()
}

func (s *Server) authorized(conn *gortsplib.ServerConn, request *base.Request) bool {
	return s.cfg.AuthToken == "" || conn.VerifyCredentials(request, "dahuabridge", s.cfg.AuthToken)
}

func (s *Server) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if !s.authorized(ctx.Conn, ctx.Request) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, nil, liberrors.ErrServerAuth{}
	}
	worker, err := s.getStreamForDemand(ctx.Path, ctx.Query, true)
	if err != nil {
		return &base.Response{StatusCode: base.StatusServiceUnavailable}, nil, nil
	}
	defer s.releaseDemand(worker)
	s.mu.Lock()
	defer s.mu.Unlock()
	if worker.retired || s.workers[worker.key] != worker {
		return &base.Response{StatusCode: base.StatusServiceUnavailable}, nil, nil
	}
	s.bindConn(ctx.Conn, worker)
	return &base.Response{StatusCode: base.StatusOK}, worker.stream, nil
}

func (s *Server) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if !s.authorized(ctx.Conn, ctx.Request) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, nil, liberrors.ErrServerAuth{}
	}
	if ctx.Transport == nil || ctx.Transport.Protocol != gortsplib.ProtocolTCP {
		return &base.Response{StatusCode: base.StatusUnsupportedTransport}, nil, nil
	}
	worker, err := s.getStreamForDemand(ctx.Path, ctx.Query, true)
	if err != nil {
		return &base.Response{StatusCode: base.StatusServiceUnavailable}, nil, nil
	}
	defer s.releaseDemand(worker)
	s.mu.Lock()
	defer s.mu.Unlock()
	if worker.retired || s.workers[worker.key] != worker {
		return &base.Response{StatusCode: base.StatusServiceUnavailable}, nil, nil
	}
	if previous := s.sessions[ctx.Session]; previous != nil && previous != worker {
		return &base.Response{StatusCode: base.StatusBadRequest}, nil, nil
	}
	if previous := s.conns[ctx.Conn]; previous != nil && previous != worker {
		return &base.Response{StatusCode: base.StatusServiceUnavailable}, nil, nil
	}
	s.bindConn(ctx.Conn, worker)
	worker.readers[ctx.Session] = reader{conn: ctx.Conn}
	s.sessions[ctx.Session] = worker
	return &base.Response{StatusCode: base.StatusOK}, worker.stream, nil
}

func (s *Server) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	if !s.authorized(ctx.Conn, ctx.Request) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, liberrors.ErrServerAuth{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	worker := s.sessions[ctx.Session]
	if worker == nil || worker.retired {
		return &base.Response{StatusCode: base.StatusServiceUnavailable}, nil
	}
	worker.readers[ctx.Session] = reader{conn: ctx.Conn, playing: true}
	worker.lastUsed = time.Now()
	worker.lastViewed = worker.lastUsed
	return &base.Response{StatusCode: base.StatusOK}, nil
}

func (s *Server) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	if !s.authorized(ctx.Conn, ctx.Request) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, liberrors.ErrServerAuth{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if worker := s.sessions[ctx.Session]; worker != nil {
		if worker.readers[ctx.Session].playing {
			worker.lastViewed = time.Now()
		}
		worker.readers[ctx.Session] = reader{conn: ctx.Conn}
		worker.lastUsed = time.Now()
	}
	return &base.Response{StatusCode: base.StatusOK}, nil
}

func (s *Server) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if worker := s.sessions[ctx.Session]; worker != nil {
		if worker.readers[ctx.Session].playing {
			worker.lastViewed = time.Now()
		}
		delete(worker.readers, ctx.Session)
		worker.lastUsed = time.Now()
		delete(s.sessions, ctx.Session)
	}
}

func (s *Server) bindConn(conn *gortsplib.ServerConn, worker *relayStream) {
	if previous := s.conns[conn]; previous != nil {
		delete(previous.conns, conn)
	}
	s.conns[conn] = worker
	worker.conns[conn] = struct{}{}
}

func (s *Server) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if worker := s.conns[ctx.Conn]; worker != nil {
		delete(worker.conns, ctx.Conn)
		delete(s.conns, ctx.Conn)
	}
}

func streamPath(path, query string) (string, string, bool) {
	if query != "" {
		return "", "", false
	}
	path = "/" + strings.TrimPrefix(path, "/")
	if !strings.HasPrefix(path, pathPrefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, pathPrefix), "/")
	if len(parts) != 2 || parts[0] == "" || strings.HasPrefix(parts[0], "nvrpb_") || (parts[1] != "quality" && parts[1] != "stable") {
		return "", "", false
	}
	for _, ch := range parts[0] {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return "", "", false
		}
	}
	return parts[0], parts[1], true
}

func (s *Server) getStream(path, query string) (*relayStream, error) {
	return s.getStreamForDemand(path, query, false)
}

// A successful demand lookup owns a reservation until the handler binds its
// connection/session and calls releaseDemand. This closes the eviction gap
// between upstream readiness and downstream DESCRIBE/SETUP completion.
func (s *Server) getStreamForDemand(path, query string, demand bool) (*relayStream, error) {
	id, profile, ok := streamPath(path, query)
	if !ok {
		return nil, errUnavailable
	}
	key := id + ":" + profile
	s.mu.Lock()
	if s.closed || !s.started {
		s.mu.Unlock()
		return nil, errUnavailable
	}
	worker := s.workers[key]
	var cleanup func()
	if worker == nil {
		if len(s.workers) >= s.cfg.MaxStreams {
			if demand {
				if victim := s.idleWarmWorkerLocked(); victim != nil {
					cleanup = s.retireLocked(victim, time.Time{})
				}
			}
			if len(s.workers) >= s.cfg.MaxStreams {
				s.mu.Unlock()
				return nil, errUnavailable
			}
		}
		worker = s.startWorkerLocked(id, profile, false)
	}
	if demand {
		worker.pendingDemand++
	}
	worker.lastUsed = time.Now()
	s.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}
	select {
	case <-worker.ready:
	case <-worker.ctx.Done():
		if demand {
			s.releaseDemand(worker)
		}
		return nil, errUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if worker.err != nil || worker.retired || s.workers[key] != worker {
		if demand {
			worker.pendingDemand--
		}
		return nil, errUnavailable
	}
	return worker, nil
}

func (s *Server) run(worker *relayStream) {
	defer s.wg.Done()
	defer s.finishWorker(worker)
	timer := time.AfterFunc(s.cfg.StartTimeout, func() {
		worker.startTimedOut.Store(true)
		worker.cancel()
	})
	defer timer.Stop()
	entry, profile, ok := s.resolver.GetStream(worker.id, worker.profile, true)
	if !ok || entry.ID != worker.id || profile.InputDuration != 0 || profile.InputPrefixURL != "" || worker.ctx.Err() != nil {
		return
	}
	u, err := base.ParseURL(profile.StreamURL)
	if err != nil || (u.Scheme != "rtsp" && u.Scheme != "rtsps") || u.Host == "" || strings.Contains(strings.ToLower(u.Path), "playback") || strings.HasPrefix(u.Path, pathPrefix) {
		return
	}
	s.mu.Lock()
	if _, exists := s.starts[worker.key]; !exists && len(s.starts) >= 2*s.cfg.MaxStreams {
		for key := range s.starts {
			if s.workers[key] == nil {
				delete(s.starts, key)
				break
			}
		}
	}
	s.starts[worker.key]++
	worker.startedAt = time.Now()
	worker.sourceURL = profile.StreamURL
	worker.channel = entry.Channel
	worker.videoCodec, worker.audioCodec = profile.VideoCodec, profile.AudioCodec
	if entry.LiveSource != nil {
		worker.source = entry.LiveSource.Source
	}
	s.mu.Unlock()
	reportFailure := false
	defer func() {
		if reportFailure && s.ctx.Err() == nil && (worker.ctx.Err() == nil || worker.startTimedOut.Load()) {
			s.resolver.ReportLiveSourceFailure(worker.id, profile.StreamURL)
			s.logger.Warn().Str("stream_id", worker.id).Str("profile", worker.profile).Msg("RTSP upstream stopped; reconnect will resolve the live source")
		}
	}()
	protocol := gortsplib.ProtocolTCP
	client := &gortsplib.Client{
		Scheme: u.Scheme, Host: u.Host, Protocol: &protocol,
		ReadTimeout: s.cfg.StartTimeout, WriteTimeout: s.cfg.StartTimeout,
		UserAgent: "DahuaBridge/RTSP",
	}
	if err := client.Start(); err != nil {
		reportFailure = true
		return
	}
	defer client.Close()
	stopCancellation := context.AfterFunc(worker.ctx, func() {
		if conn := client.NetConn(); conn != nil {
			_ = conn.Close()
		}
		client.Close()
	})
	defer stopCancellation()
	desc, _, err := client.Describe(u)
	if err != nil || len(desc.Medias) == 0 {
		reportFailure = true
		return
	}
	if err := client.SetupAll(desc.BaseURL, desc.Medias); err != nil {
		reportFailure = true
		return
	}
	stream := &gortsplib.ServerStream{Server: s.server, Desc: desc}
	if err := stream.Initialize(); err != nil {
		return
	}
	defer stream.Close()
	client.OnPacketRTPAny(func(media *description.Media, _ format.Format, packet *rtp.Packet) {
		worker.bytesReceived.Add(uint64(packet.MarshalSize()))
		worker.lastPacket.Store(time.Now().UnixNano())
		_ = stream.WritePacketRTP(media, packet)
	})
	if _, err := client.Play(nil); err != nil {
		reportFailure = true
		return
	}
	timer.Stop()
	s.mu.Lock()
	if worker.retired || worker.ctx.Err() != nil || s.workers[worker.key] != worker {
		s.mu.Unlock()
		return
	}
	worker.stream = stream
	s.finishWarmStartLocked(worker)
	worker.readyClosed = true
	close(worker.ready)
	s.mu.Unlock()
	s.signalPreconnect()
	_ = client.Wait()
	reportFailure = true
}

func (s *Server) retire(worker *relayStream) {
	s.retireBefore(worker, time.Time{})
}

// A nonzero cutoff makes the idle check atomic with removing the worker: a
// reader that starts playing after the reaper's snapshot keeps its stream.
func (s *Server) retireBefore(worker *relayStream, cutoff time.Time) {
	s.mu.Lock()
	cleanup := s.retireLocked(worker, cutoff)
	s.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}
}

// Remove ownership under s.mu, but return the socket cleanup for the caller to
// run after unlocking; gortsplib close callbacks also acquire this mutex.
func (s *Server) retireLocked(worker *relayStream, cutoff time.Time) func() {
	if worker.retired {
		return nil
	}
	if !cutoff.IsZero() {
		if worker.lastUsed.After(cutoff) || worker.pendingDemand > 0 || s.keepWarmLocked(worker, time.Now()) {
			return nil
		}
		for _, reader := range worker.readers {
			if reader.playing {
				return nil
			}
		}
	}
	worker.retired = true
	if s.workers[worker.key] == worker {
		delete(s.workers, worker.key)
	}
	if !worker.readyClosed {
		worker.err = errUnavailable
		worker.readyClosed = true
		close(worker.ready)
	}
	stream := worker.stream
	readers := worker.readers
	worker.readers = nil
	conns := worker.conns
	worker.conns = nil
	for conn := range conns {
		delete(s.conns, conn)
	}
	for session := range readers {
		delete(s.sessions, session)
	}
	return func() {
		worker.cancel()
		if stream != nil {
			stream.Close()
		}
		for session, reader := range readers {
			session.Close()
			reader.conn.Close()
		}
		for conn := range conns {
			conn.Close()
		}
	}
}

func (s *Server) reapIdle() {
	defer s.wg.Done()
	interval := min(s.cfg.IdleTimeout/4, time.Second)
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			var idle []*relayStream
			for _, worker := range s.workers {
				playing := false
				for _, reader := range worker.readers {
					playing = playing || reader.playing
				}
				if !playing && worker.pendingDemand == 0 && !s.keepWarmLocked(worker, now) && now.Sub(worker.lastUsed) >= s.cfg.IdleTimeout {
					idle = append(idle, worker)
				}
			}
			s.mu.Unlock()
			for _, worker := range idle {
				s.retireBefore(worker, now.Add(-s.cfg.IdleTimeout))
			}
		}
	}
}
