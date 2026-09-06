package app

import (
	"context"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/liberrors"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/rtsprelay"
)

// Exercise real authenticated RTSP control and RTP forwarding: downstream
// clients know only the bridge URL, while an upstream disconnect is reported
// by the relay and resolved by the production background health workers.
func TestLiveRelayReconnectsSameURLAfterBidirectionalUpstreamFailure(t *testing.T) {
	for _, preferred := range []string{"nvr", "camera"} {
		t.Run(preferred+" preferred", func(t *testing.T) {
			nvr := startLiveRelayTestUpstream(t, "nvr-user", "nvr-pass", "5", 0x31)
			camera := startLiveRelayTestUpstream(t, "camera-user", "camera-pass", "1", 0x42)
			runtime, deviceCfg := liveSourceRuntime(t)
			deviceCfg.BaseURL = "rtsp://" + nvr.address
			deviceCfg.DirectIPCCredentials[0].DirectIPCIP = "127.0.0.1"
			_, cameraPort, err := net.SplitHostPort(camera.address)
			if err != nil {
				t.Fatal(err)
			}
			runtime.probes.Update("nvr", func(probe *dahua.ProbeResult) {
				probe.Children[0].Attributes["direct_ipc_rtsp_port"] = cameraPort
			})
			runtime.RegisterNVR("nvr", nil, nil, deviceCfg)
			runtime.cfg.Media.Enabled = true
			runtime.cfg.HomeAssistant.PublicBaseURL = "http://127.0.0.1"
			runtime.cfg.HTTP.AuthToken = "relay-token:@"
			if _, err := runtime.SetDefaultLiveSource(t.Context(), preferred); err != nil {
				t.Fatal(err)
			}

			relay := rtsprelay.New(rtsprelay.Config{
				ListenAddress: "127.0.0.1:0", AuthToken: runtime.cfg.HTTP.AuthToken,
				StartTimeout: 3 * time.Second, IdleTimeout: time.Minute, MaxStreams: 2,
			}, runtime, zerolog.Nop())
			if err := relay.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(relay.Close)
			runtime.cfg.Media.RTSPListenAddress = relay.Addr()
			runtime.AttachLiveRelay(relay)
			stopHealth := runtime.StartLiveSourceHealth(t.Context())
			t.Cleanup(stopHealth)

			const streamID = "nvr_channel_05"
			publicEntry := runtime.ListHomeAssistantStreams(true)[0]
			stableURL := publicEntry.Profiles["stable"].StreamURL
			selected, alternate := nvr, camera
			if preferred == "camera" {
				selected, alternate = camera, nvr
			}
			first := readLiveRelayTestMarker(t, stableURL, selected.marker)

			// No direct health invocation, preference change, or alternate URL is
			// supplied: closing the upstream must drive the production failover.
			selected.Close()
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			for {
				entry, _, ok := runtime.GetStream(streamID, "stable", true)
				if ok && entry.LiveSource.Source == otherLiveSource(preferred) {
					if entry.LiveSource.PreferredSource != preferred || entry.LiveSource.FallbackReason == "" {
						t.Fatalf("failover lost preference or reason: %+v", entry.LiveSource)
					}
					break
				}
				select {
				case <-deadline.C:
					t.Fatal("relay disconnect did not trigger runtime failover")
				case <-tick.C:
				}
			}
			first.Close()
			after := runtime.ListHomeAssistantStreams(true)[0].Profiles["stable"]
			if after.StreamURL != stableURL || after.AlternativeStreamURL != "" || after.RecorderStreamURL != "" {
				t.Fatal("public catalog changed the downstream URL or exposed an upstream")
			}
			readLiveRelayTestMarker(t, stableURL, alternate.marker)
		})
	}
}

type liveRelayTestUpstream struct {
	server   *gortsplib.Server
	stream   *gortsplib.ServerStream
	address  string
	username string
	password string
	channel  string
	marker   byte
	cancel   context.CancelFunc
	done     chan struct{}
	once     sync.Once
}

func startLiveRelayTestUpstream(t *testing.T, username, password, channel string, marker byte) *liveRelayTestUpstream {
	t.Helper()
	upstream := &liveRelayTestUpstream{username: username, password: password, channel: channel, marker: marker, done: make(chan struct{})}
	upstream.server = &gortsplib.Server{
		RTSPAddress: "127.0.0.1:0", Handler: upstream,
		ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second,
		Listen: func(network, address string) (net.Listener, error) {
			listener, err := net.Listen(network, address)
			if err == nil {
				upstream.address = listener.Addr().String()
			}
			return listener, err
		},
	}
	if err := upstream.server.Start(); err != nil {
		t.Fatal(err)
	}
	media := &description.Media{Type: description.MediaTypeVideo, Formats: []format.Format{&format.H264{PayloadTyp: 96, PacketizationMode: 1}}}
	upstream.stream = &gortsplib.ServerStream{Server: upstream.server, Desc: &description.Session{Medias: []*description.Media{media}}}
	if err := upstream.stream.Initialize(); err != nil {
		upstream.server.Close()
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	upstream.cancel = cancel
	go func() {
		defer close(upstream.done)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		var sequence uint16
		var timestamp uint32
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sequence++
				timestamp += 1800
				_ = upstream.stream.WritePacketRTP(media, &rtp.Packet{
					Header:  rtp.Header{Version: 2, Marker: true, PayloadType: 96, SequenceNumber: sequence, Timestamp: timestamp, SSRC: uint32(marker)},
					Payload: []byte{0x65, marker, 0x80},
				})
			}
		}
	}()
	t.Cleanup(upstream.Close)
	return upstream
}

func (u *liveRelayTestUpstream) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if !ctx.Conn.VerifyCredentials(ctx.Request, u.username, u.password) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, nil, liberrors.ErrServerAuth{}
	}
	query, err := url.ParseQuery(ctx.Request.URL.RawQuery)
	if err != nil || query.Get("channel") != u.channel {
		return &base.Response{StatusCode: base.StatusBadRequest}, nil, nil
	}
	return &base.Response{StatusCode: base.StatusOK}, u.stream, nil
}

func (u *liveRelayTestUpstream) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if !ctx.Conn.VerifyCredentials(ctx.Request, u.username, u.password) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, nil, liberrors.ErrServerAuth{}
	}
	return &base.Response{StatusCode: base.StatusOK}, u.stream, nil
}

func (u *liveRelayTestUpstream) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	if !ctx.Conn.VerifyCredentials(ctx.Request, u.username, u.password) {
		return &base.Response{StatusCode: base.StatusUnauthorized}, liberrors.ErrServerAuth{}
	}
	return &base.Response{StatusCode: base.StatusOK}, nil
}

func (u *liveRelayTestUpstream) Close() {
	u.once.Do(func() {
		u.cancel()
		<-u.done
		u.server.Close()
		u.stream.Close()
	})
}

func readLiveRelayTestMarker(t *testing.T, streamURL string, want byte) *gortsplib.Client {
	t.Helper()
	u, err := base.ParseURL(streamURL)
	if err != nil {
		t.Fatal(err)
	}
	protocol := gortsplib.ProtocolTCP
	client := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host, Protocol: &protocol, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	desc, _, err := client.Describe(u)
	if err != nil {
		t.Fatalf("describe public relay: %v", err)
	}
	if err := client.SetupAll(desc.BaseURL, desc.Medias); err != nil {
		t.Fatalf("set up public relay: %v", err)
	}
	markers := make(chan byte, 1)
	client.OnPacketRTPAny(func(_ *description.Media, _ format.Format, packet *rtp.Packet) {
		if len(packet.Payload) > 1 {
			select {
			case markers <- packet.Payload[1]:
			default:
			}
		}
	})
	if _, err := client.Play(nil); err != nil {
		t.Fatalf("play public relay: %v", err)
	}
	select {
	case got := <-markers:
		if got != want {
			t.Fatalf("relay forwarded upstream marker %#x, want %#x", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("public relay did not forward an RTP packet")
	}
	return client
}
