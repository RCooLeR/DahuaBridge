package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/rs/zerolog"
)

type failureReportingResolver struct {
	testResolver
	failures [][2]string
}

func (r *failureReportingResolver) ReportLiveSourceFailure(streamID, failedURL string) {
	r.failures = append(r.failures, [2]string{streamID, failedURL})
}

func TestLiveSourceFailureReportsSkipCancellationAndArchive(t *testing.T) {
	resolver := &failureReportingResolver{}
	m := New(config.MediaConfig{Enabled: true}, resolver, zerolog.Nop(), nil)
	profile := streams.Profile{StreamURL: "rtsp://camera/cam/realmonitor?channel=1"}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	m.reportLiveSourceFailure(canceled, "cam", profile)
	m.reportLiveSourceFailure(context.Background(), "nvrpb_recording", profile)
	m.reportLiveSourceFailure(context.Background(), "cam", streams.Profile{StreamURL: "rtsp://nvr/cam/playback?channel=1"})
	if len(resolver.failures) != 0 {
		t.Fatal("normal cancellation or archive playback triggered live failover")
	}
	m.reportLiveSourceFailure(context.Background(), "cam", profile)
	if len(resolver.failures) != 1 || resolver.failures[0] != [2]string{"cam", profile.StreamURL} {
		t.Fatalf("live failure was not reported with its original input: %v", resolver.failures)
	}
	m.InvalidateStream("cam")
	m.reportLiveSourceFailure(context.Background(), "cam", profile)
	if len(resolver.failures) != 1 {
		t.Fatal("an invalidated worker reported a stale failure")
	}
}

func TestLiveStartupTimeoutsReportInputButMissingOldSegmentsDoNot(t *testing.T) {
	resolver := &failureReportingResolver{}
	m := New(config.MediaConfig{Enabled: true, StartTimeout: time.Millisecond}, resolver, zerolog.Nop(), nil)
	profile := streams.Profile{StreamURL: "rtsp://camera/cam/realmonitor?channel=1"}
	ctx := context.Background()
	hls := &hlsWorker{parent: m, ctx: ctx, streamID: "cam", profile: profile}
	dash := &dashWorker{parent: m, ctx: ctx, streamID: "cam", profile: profile}
	mjpeg := &worker{parent: m, ctx: ctx, streamID: "cam", profile: profile}
	if _, err := hls.readFileWhenReady(ctx, "index.m3u8"); err == nil {
		t.Fatal("expected HLS startup timeout")
	}
	if _, err := dash.readFileWhenReady(ctx, "manifest.mpd"); err == nil {
		t.Fatal("expected DASH startup timeout")
	}
	if err := mjpeg.waitUntilReady(ctx); err == nil {
		t.Fatal("expected MJPEG startup timeout")
	}
	if len(resolver.failures) != 3 {
		t.Fatalf("startup reports=%d, want3", len(resolver.failures))
	}
	_, _ = hls.readFileWhenReady(ctx, "segment_000.ts")
	_, _ = dash.readFileWhenReady(ctx, "chunk_000.m4s")
	if len(resolver.failures) != 3 {
		t.Fatal("missing old segment triggered source failover")
	}
}

func TestInvalidateStreamOnlyReconnectsItsLiveWorkers(t *testing.T) {
	m := New(config.MediaConfig{Enabled: true}, testResolver{}, zerolog.Nop(), nil)
	var contexts []context.Context
	nextContext := func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		contexts = append(contexts, ctx)
		return ctx, cancel
	}
	ctx, cancel := nextContext()
	m.mjpegWorkers["cam:stable:w960"] = &worker{streamID: "cam", ctx: ctx, cancel: cancel}
	ctx, cancel = nextContext()
	m.hlsWorkers["cam:stable"] = &hlsWorker{streamID: "cam", ctx: ctx, cancel: cancel}
	ctx, cancel = nextContext()
	m.dashWorkers["cam:quality"] = &dashWorker{streamID: "cam", ctx: ctx, cancel: cancel}
	ctx, cancel = nextContext()
	m.webrtcPeers["cam:quality:peer"] = &webrtcSession{streamID: "cam", ctx: ctx, cancel: cancel}
	otherCtx, otherCancel := context.WithCancel(context.Background())
	t.Cleanup(otherCancel)
	m.hlsWorkers["other:stable"] = &hlsWorker{streamID: "other", ctx: otherCtx, cancel: otherCancel}
	clip := &clipJob{}
	m.clipJobs["recording"] = clip

	m.InvalidateStream("cam")

	for _, ctx := range contexts {
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("old upstream is still running")
		}
	}
	if otherCtx.Err() != nil || m.hlsWorkers["other:stable"] == nil {
		t.Fatal("another camera was interrupted")
	}
	if len(m.mjpegWorkers)+len(m.dashWorkers)+len(m.webrtcPeers) != 0 || len(m.hlsWorkers) != 1 {
		t.Fatal("old live workers remain reusable")
	}
	if m.clipJobs["recording"] != clip {
		t.Fatal("changing live source interrupted a recording")
	}
}

func TestSourceSwitchRejectsResolutionsFromBeforeTheSwitch(t *testing.T) {
	m := New(config.MediaConfig{Enabled: true}, testResolver{}, zerolog.Nop(), nil)
	entry, stale, profileName, err := m.resolveStream("cam", "stable")
	if err != nil {
		t.Fatal(err)
	}
	m.InvalidateStream("cam")
	if _, err = m.getOrCreateHLSWorker(entry, profileName, stale); !errors.Is(err, ErrStreamSourceChanged) {
		t.Fatalf("HLS accepted old upstream: %v", err)
	}
	if _, err = m.getOrCreateDASHWorker(entry, profileName, stale); !errors.Is(err, ErrStreamSourceChanged) {
		t.Fatalf("DASH accepted old upstream: %v", err)
	}
	if _, err = m.getOrCreateMJPEGWorker(entry, profileName, stale, 960); !errors.Is(err, ErrStreamSourceChanged) {
		t.Fatalf("MJPEG accepted old upstream: %v", err)
	}
	_, fresh, _, err := m.resolveStream("cam", "stable")
	if err != nil || fresh.MediaGeneration == stale.MediaGeneration {
		t.Fatal("new requests do not see the changed source")
	}
	// Existing viewers of the new input still share a worker.
	replacement := &hlsWorker{streamID: "cam", profile: fresh, parent: m, ctx: context.Background()}
	m.hlsWorkers["cam:stable"] = replacement
	got, err := m.getOrCreateHLSWorker(entry, profileName, fresh)
	if err != nil || got != replacement {
		t.Fatalf("new input is not shared: %v", err)
	}
	// Deferred cleanup of the old worker must not remove the replacement.
	m.removeHLSWorker("cam:stable", &hlsWorker{streamID: "cam", profile: stale})
	if m.hlsWorkers["cam:stable"] != replacement {
		t.Fatal("old worker cleanup removed the replacement")
	}
}

type switchingResolver struct {
	onResolve func()
}

func (r switchingResolver) GetStream(id, profile string, _ bool) (streams.Entry, streams.Profile, bool) {
	r.onResolve()
	return streams.Entry{ID: id}, streams.Profile{Name: profile, StreamURL: "rtsp://old/stream"}, true
}

func TestWebRTCSourceSwitchDuringResolutionDoesNotStartOldInput(t *testing.T) {
	m := New(config.MediaConfig{Enabled: true}, nil, zerolog.Nop(), nil)
	m.resolver = switchingResolver{onResolve: func() { m.InvalidateStream("cam") }}
	_, err := m.WebRTCAnswer(context.Background(), "cam", "stable", WebRTCSessionDescription{})
	if !errors.Is(err, ErrStreamSourceChanged) || len(m.webrtcPeers) != 0 {
		t.Fatalf("WebRTC started a stale input: %v", err)
	}
}
