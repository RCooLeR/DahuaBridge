package media

import (
	"context"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/rs/zerolog"
)

type sharedInputResolver struct{ failures []string }

func (r *sharedInputResolver) GetStream(id, profile string, _ bool) (streams.Entry, streams.Profile, bool) {
	return streams.Entry{ID: id}, streams.Profile{StreamURL: "rtsp://camera/live", RTSPTransport: "udp"}, true
}
func (r *sharedInputResolver) LiveMediaInput(id, profile string) string {
	return "rtsp://bridge:8554/api/v1/rtsp/live/" + id + "/" + profile
}
func (r *sharedInputResolver) ReportLiveSourceFailure(_ string, source string) {
	r.failures = append(r.failures, source)
}

func TestSharedInputPreservesOriginalFailureSource(t *testing.T) {
	r := &sharedInputResolver{}
	m := New(config.MediaConfig{}, r, zerolog.Nop(), nil)
	defer m.Close()
	_, profile, _, err := m.resolveStream("cam", "stable")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(buildRTSPInputArgs(profile, "stable"), " ")
	if !strings.Contains(args, "-i rtsp://bridge:8554/") || !strings.Contains(args, "-rtsp_transport tcp") || strings.Contains(args, "camera") {
		t.Fatalf("not using shared TCP input: %s", args)
	}
	m.reportLiveSourceFailure(context.Background(), "cam", profile)
	if len(r.failures) != 1 || r.failures[0] != "rtsp://camera/live" {
		t.Fatalf("wrong route failed: %v", r.failures)
	}
	_, archive, _, err := m.resolveStream("nvrpb_test", "stable")
	if err != nil || archive.LiveRelayURL != "" {
		t.Fatal("archive was sent through live relay")
	}
}

func TestClipRemuxRequiresCompatibleUnfilteredSource(t *testing.T) {
	profile := streams.Profile{VideoCodec: "H.264"}
	if !canRemuxClipVideo(profile) {
		t.Fatal("compatible H264 did not use remux")
	}
	for _, incompatible := range []streams.Profile{
		{VideoCodec: "H.265"}, {VideoCodec: ""}, {VideoCodec: "H264", InputSeekOffset: int64(time.Second)},
		{VideoCodec: "H264", InputPrefixURL: "frame.jpg"}, {VideoCodec: "H264", ForceVideoTranscode: true},
		{VideoCodec: "H264", UseWallclockAsTimestamps: true},
	} {
		if canRemuxClipVideo(incompatible) {
			t.Fatalf("incompatible source remuxed: %+v", incompatible)
		}
	}
}

func TestDASHCanceledWorkerReturnsWithoutStartupTimeout(t *testing.T) {
	m := New(config.MediaConfig{StartTimeout: time.Minute}, testResolver{}, zerolog.Nop(), nil)
	defer m.Close()
	ctx, cancel := context.WithCancel(m.ctx)
	cancel()
	w := &dashWorker{parent: m, ctx: ctx}
	done := make(chan error, 1)
	go func() { _, err := w.readFileWhenReady(context.Background(), "manifest.mpd"); done <- err }()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("DASH kept polling a canceled worker")
	}
}
