package rtsprelay

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/liberrors"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"

	"RCooLeR/DahuaBridge/internal/streams"
)

type mockResolver struct {
	mu       sync.Mutex
	url      string
	gate     <-chan struct{}
	calls    atomic.Int32
	entered  chan struct{}
	failures chan string
}

func (r *mockResolver) GetStream(id, profile string, credentials bool) (streams.Entry, streams.Profile, bool) {
	r.calls.Add(1)
	r.mu.Lock()
	upstream, gate := r.url, r.gate
	r.mu.Unlock()
	if r.entered != nil {
		select {
		case r.entered <- struct{}{}:
		default:
		}
	}
	if gate != nil {
		<-gate
	}
	return streams.Entry{ID: id}, streams.Profile{Name: profile, StreamURL: upstream}, credentials
}

func (r *mockResolver) ReportLiveSourceFailure(_ string, upstream string) {
	if r.failures != nil {
		select {
		case r.failures <- upstream:
		default:
		}
	}
}

type mockUpstream struct {
	server    *gortsplib.Server
	stream    *gortsplib.ServerStream
	address   string
	plays     atomic.Int32
	closed    chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	marker    byte
	password  string
}

func newMockUpstream(t *testing.T, marker byte, password string) *mockUpstream {
	t.Helper()
	u := &mockUpstream{marker: marker, password: password, closed: make(chan struct{}), done: make(chan struct{})}
	u.server = &gortsplib.Server{
		RTSPAddress: "127.0.0.1:0", Handler: u,
		Listen: func(network, address string) (net.Listener, error) {
			listener, err := net.Listen(network, address)
			if err == nil {
				u.address = listener.Addr().String()
			}
			return listener, err
		},
	}
	if err := u.server.Start(); err != nil {
		t.Fatal(err)
	}
	u.stream = &gortsplib.ServerStream{
		Server: u.server,
		Desc: &description.Session{Medias: []*description.Media{{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{PayloadTyp: 96, PacketizationMode: 1}},
		}}},
	}
	if err := u.stream.Initialize(); err != nil {
		u.server.Close()
		t.Fatal(err)
	}
	go func() {
		defer close(u.done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		var sequence uint16
		for {
			select {
			case <-u.closed:
				return
			case <-ticker.C:
				sequence++
				_ = u.stream.WritePacketRTP(u.stream.Desc.Medias[0], &rtp.Packet{
					Header:  rtp.Header{Version: 2, PayloadType: 96, SequenceNumber: sequence, Timestamp: uint32(sequence) * 3600, SSRC: 12345, Marker: true},
					Payload: []byte{0x65, u.marker, 0x01},
				})
			}
		}
	}()
	t.Cleanup(u.Close)
	return u
}

func (u *mockUpstream) URL() string {
	value := &url.URL{Scheme: "rtsp", Host: u.address, Path: "/live"}
	if u.password != "" {
		value.User = url.UserPassword("upstream", u.password)
	}
	return value.String()
}

func (u *mockUpstream) Close() {
	u.closeOnce.Do(func() {
		close(u.closed)
		<-u.done
		u.stream.Close()
		u.server.Close()
	})
}

func (u *mockUpstream) authorized(conn *gortsplib.ServerConn, req *base.Request) bool {
	return u.password == "" || conn.VerifyCredentials(req, "upstream", u.password)
}

func (u *mockUpstream) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if !u.authorized(ctx.Conn, ctx.Request) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, nil, liberrors.ErrServerAuth{}
	}
	return &base.Response{StatusCode: base.StatusOK}, u.stream, nil
}

func (u *mockUpstream) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if !u.authorized(ctx.Conn, ctx.Request) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, nil, liberrors.ErrServerAuth{}
	}
	return &base.Response{StatusCode: base.StatusOK}, u.stream, nil
}

func (u *mockUpstream) OnPlay(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	u.plays.Add(1)
	return &base.Response{StatusCode: base.StatusOK}, nil
}

func newTestRelay(t *testing.T, resolver Resolver, cfg Config) *Server {
	t.Helper()
	cfg.ListenAddress = "127.0.0.1:0"
	if cfg.StartTimeout == 0 {
		cfg.StartTimeout = time.Second
	}
	s := New(cfg, resolver, zerolog.Nop())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func relayURL(s *Server, id, profile, token string) string {
	u := &url.URL{Scheme: "rtsp", Host: s.Addr(), Path: pathPrefix + id + "/" + profile}
	if token != "" {
		u.User = url.UserPassword("dahuabridge", token)
	}
	return u.String()
}

func openReader(t *testing.T, streamURL string) (*gortsplib.Client, <-chan byte) {
	t.Helper()
	u, err := base.ParseURL(streamURL)
	if err != nil {
		t.Fatal(err)
	}
	protocol := gortsplib.ProtocolTCP
	c := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host, Protocol: &protocol, ReadTimeout: 2 * time.Second}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	desc, response, err := c.Describe(u)
	if err != nil {
		t.Fatalf("relay describe: %v", err)
	}
	if strings.Contains(string(response.Body), "source-secret") || strings.Contains(string(response.Body), "upstream@") {
		t.Fatal("SDP exposed upstream credentials")
	}
	if err := c.SetupAll(desc.BaseURL, desc.Medias); err != nil {
		t.Fatalf("relay setup: %v", err)
	}
	packets := make(chan byte, 16)
	c.OnPacketRTPAny(func(_ *description.Media, _ format.Format, packet *rtp.Packet) {
		if len(packet.Payload) >= 2 {
			select {
			case packets <- packet.Payload[1]:
			default:
			}
		}
	})
	if _, err := c.Play(nil); err != nil {
		t.Fatalf("relay play: %v", err)
	}
	return c, packets
}

func expectPacket(t *testing.T, packets <-chan byte, marker byte) {
	t.Helper()
	select {
	case got := <-packets:
		if got != marker {
			t.Fatalf("wrong upstream payload: got %x want %x", got, marker)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not forward RTP")
	}
}

func TestRelaySharesAuthenticatedUpstreamAndForwardsRTP(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "source-secret")
	resolver := &mockResolver{url: upstream.URL()}
	relay := newTestRelay(t, resolver, Config{AuthToken: "bridge-token"})
	stableURL := relayURL(relay, "cam1", "quality", "bridge-token")
	_, first := openReader(t, stableURL)
	_, second := openReader(t, relay.LocalStreamURL("cam1", "quality"))
	expectPacket(t, first, 0xA1)
	expectPacket(t, second, 0xA1)
	if upstream.plays.Load() != 1 || resolver.calls.Load() != 1 {
		t.Fatalf("viewers did not share upstream: plays=%d lookups=%d", upstream.plays.Load(), resolver.calls.Load())
	}
	statuses := relay.Statuses()
	if len(statuses) != 1 || statuses[0].Viewers != 2 || statuses[0].BytesReceived == 0 || statuses[0].LastPacketAt.IsZero() || !statuses[0].Ready {
		t.Fatalf("relay activity missing from diagnostics: %+v", statuses)
	}
}

func TestRelayReconnectsSameURLAfterSourceInvalidation(t *testing.T) {
	firstSource := newMockUpstream(t, 0xA1, "")
	secondSource := newMockUpstream(t, 0xB2, "")
	resolver := &mockResolver{url: firstSource.URL()}
	relay := newTestRelay(t, resolver, Config{})
	stableURL := relayURL(relay, "cam1", "quality", "")
	client, packets := openReader(t, stableURL)
	expectPacket(t, packets, 0xA1)
	closed := make(chan error, 1)
	go func() { closed <- client.Wait() }()
	resolver.mu.Lock()
	resolver.url = secondSource.URL()
	resolver.mu.Unlock()
	relay.InvalidateStream("cam1")
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("invalidation did not disconnect old reader")
	}
	_, replacement := openReader(t, stableURL)
	expectPacket(t, replacement, 0xB2)
}

func TestRelayRejectsMissingWrongAndBypassedPlayAuthentication(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	resolver := &mockResolver{url: upstream.URL()}
	relay := newTestRelay(t, resolver, Config{AuthToken: "bridge-token"})
	for _, token := range []string{"", "wrong-token"} {
		u, _ := base.ParseURL(relayURL(relay, "cam1", "quality", token))
		c := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host}
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		_, _, err := c.Describe(u)
		c.Close()
		if err == nil {
			t.Fatal("unauthorized DESCRIBE succeeded")
		}
	}
	if resolver.calls.Load() != 0 {
		t.Fatal("unauthorized reader opened an upstream")
	}
	conn, err := net.Dial("tcp", relay.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	reader := bufio.NewReader(conn)
	uri := relayURL(relay, "cam1", "quality", "")
	response := rawRequest(t, conn, reader, fmt.Sprintf("SETUP %s/trackID=0 RTSP/1.0\r\nCSeq: 1\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n\r\n", uri))
	if response.StatusCode != base.StatusUnauthorized {
		t.Fatalf("unauthorized SETUP returned %d", response.StatusCode)
	}
	header := base64.StdEncoding.EncodeToString([]byte("dahuabridge:bridge-token"))
	response = rawRequest(t, conn, reader, fmt.Sprintf("SETUP %s/trackID=0 RTSP/1.0\r\nCSeq: 2\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\nAuthorization: Basic %s\r\n\r\n", uri, header))
	if response.StatusCode != base.StatusOK {
		t.Fatalf("authenticated SETUP returned %d", response.StatusCode)
	}
	session := strings.Split(response.Header["Session"][0], ";")[0]
	response = rawRequest(t, conn, reader, fmt.Sprintf("PLAY %s RTSP/1.0\r\nCSeq: 3\r\nSession: %s\r\n\r\n", uri, session))
	if response.StatusCode != base.StatusUnauthorized {
		t.Fatalf("PLAY without credentials returned %d", response.StatusCode)
	}
}

func rawRequest(t *testing.T, conn net.Conn, reader *bufio.Reader, request string) base.Response {
	t.Helper()
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	var response base.Response
	if err := response.Unmarshal(reader); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestRelayRejectsArchiveAndArbitraryPaths(t *testing.T) {
	resolver := &mockResolver{}
	relay := newTestRelay(t, resolver, Config{})
	for _, path := range []string{"/api/v1/rtsp/live/nvrpb_123/quality", "/api/v1/rtsp/live/cam/quality?url=rtsp://camera/live", "/api/v1/rtsp/live/http://camera/quality", "/api/v1/rtsp/live/cam/playback", "/other"} {
		u, _ := base.ParseURL("rtsp://" + relay.Addr() + path)
		c := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host}
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		_, _, err := c.Describe(u)
		c.Close()
		if err == nil {
			t.Fatalf("unsupported relay path accepted: %s", path)
		}
	}
	if resolver.calls.Load() != 0 {
		t.Fatal("unsupported path reached resolver")
	}
}

func TestRelayIdleCleanupAndStreamLimit(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	resolver := &mockResolver{url: upstream.URL()}
	relay := newTestRelay(t, resolver, Config{MaxStreams: 1, IdleTimeout: 80 * time.Millisecond})
	client, packets := openReader(t, relayURL(relay, "cam1", "quality", ""))
	expectPacket(t, packets, 0xA1)
	if _, err := relay.getStream(pathPrefix+"cam1/stable", ""); err == nil {
		t.Fatal("stream limit was not enforced")
	}
	client.Close()
	deadline := time.Now().Add(time.Second)
	for {
		relay.mu.Lock()
		count := len(relay.workers)
		relay.mu.Unlock()
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("idle upstream was not removed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, replacement := openReader(t, relayURL(relay, "cam1", "stable", ""))
	expectPacket(t, replacement, 0xA1)
}

func TestRelayInvalidationPreventsStaleStartupPublication(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	gate := make(chan struct{})
	resolver := &mockResolver{url: upstream.URL(), gate: gate, entered: make(chan struct{}, 1)}
	relay := newTestRelay(t, resolver, Config{})
	result := make(chan error, 1)
	go func() { _, err := relay.getStream(pathPrefix+"cam1/quality", ""); result <- err }()
	<-resolver.entered
	relay.InvalidateStream("cam1")
	close(gate)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("invalidated startup was published")
		}
	case <-time.After(time.Second):
		t.Fatal("invalidation did not release waiting request")
	}
	resolver.mu.Lock()
	resolver.gate = nil
	resolver.mu.Unlock()
	_, packets := openReader(t, relayURL(relay, "cam1", "quality", ""))
	expectPacket(t, packets, 0xA1)
	if upstream.plays.Load() != 1 {
		t.Fatal("stale worker opened another upstream")
	}
}

func TestRelayIdleSnapshotCannotRetireActiveReader(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	relay := newTestRelay(t, &mockResolver{url: upstream.URL()}, Config{})
	_, packets := openReader(t, relayURL(relay, "cam1", "quality", ""))
	expectPacket(t, packets, 0xA1)
	relay.mu.Lock()
	worker := relay.workers["cam1:quality"]
	// Simulate an old last-use timestamp selected by the reaper, followed by
	// PLAY before the snapshot is retired. Playing state must be checked again.
	worker.lastUsed = time.Now().Add(-time.Hour)
	relay.mu.Unlock()
	relay.retireBefore(worker, time.Now())
	relay.mu.Lock()
	retired := worker.retired
	relay.mu.Unlock()
	if retired {
		t.Fatal("idle snapshot retired a reader that is now playing")
	}
	expectPacket(t, packets, 0xA1)
}

func TestRelayStartupTimeoutInterruptsSilentUpstream(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	resolver := &mockResolver{url: "rtsp://" + listener.Addr().String() + "/live", failures: make(chan string, 1)}
	relay := newTestRelay(t, resolver, Config{StartTimeout: 80 * time.Millisecond})
	started := time.Now()
	if _, err := relay.getStream(pathPrefix+"cam1/quality", ""); err == nil {
		t.Fatal("silent upstream was published")
	}
	if time.Since(started) > time.Second {
		t.Fatal("startup was not bounded")
	}
	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(time.Second):
		t.Fatal("relay never contacted upstream")
	}
	select {
	case <-resolver.failures:
	case <-time.After(time.Second):
		t.Fatal("startup timeout was not reported")
	}
	closed := make(chan struct{})
	go func() { relay.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("upstream read prevented relay shutdown")
	}
}

func TestRelayReportsTerminalUpstreamFailure(t *testing.T) {
	upstream := newMockUpstream(t, 0xA1, "")
	resolver := &mockResolver{url: upstream.URL(), failures: make(chan string, 1)}
	relay := newTestRelay(t, resolver, Config{})
	client, packets := openReader(t, relayURL(relay, "cam1", "quality", ""))
	expectPacket(t, packets, 0xA1)
	upstream.Close()
	select {
	case failed := <-resolver.failures:
		if failed != upstream.URL() {
			t.Fatal("wrong upstream reported failed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("terminal upstream failure was not reported")
	}
	closed := make(chan error, 1)
	go func() { closed <- client.Wait() }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("failed upstream did not disconnect relay reader")
	}
}
