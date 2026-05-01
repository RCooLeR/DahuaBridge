package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
)

type testResolver struct{}

func (testResolver) GetStream(streamID string, profileName string, includeCredentials bool) (streams.Entry, streams.Profile, bool) {
	return streams.Entry{
			ID: streamID,
		}, streams.Profile{
			Name:        profileName,
			StreamURL:   "rtsp://example.local/stream",
			FrameRate:   5,
			Recommended: true,
		}, true
}

func TestExtractFramesPublishesJPEG(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		FrameRate:      5,
		JPEGQuality:    7,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &worker{
		key:         "test:stable",
		streamID:    "test",
		profileName: "stable",
		parent:      manager,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: map[chan []byte]struct{}{},
		ready:       make(chan struct{}),
		startErr:    make(chan error, 1),
	}

	received := make(chan []byte, 2)
	w.subscribers[received] = struct{}{}

	frame1 := []byte{0xFF, 0xD8, 0x01, 0x02, 0xFF, 0xD9}
	frame2 := []byte{0xFF, 0xD8, 0x03, 0x04, 0xFF, 0xD9}
	buffer := append(append([]byte("junk"), frame1...), frame2...)

	rest := w.extractFrames(buffer)
	if len(rest) != 0 {
		t.Fatalf("expected no remainder, got %d bytes", len(rest))
	}

	got1 := <-received
	got2 := <-received
	if !bytes.Equal(got1, frame1) || !bytes.Equal(got2, frame2) {
		t.Fatalf("unexpected frames: %v %v", got1, got2)
	}
}

func TestReadMJPEGTreatsEOFAsUnexpected(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		FrameRate:      5,
		JPEGQuality:    7,
		Threads:        1,
		ScaleWidth:     960,
		ReadBufferSize: 1024,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &worker{
		key:         "test:stable",
		streamID:    "test",
		profileName: "stable",
		parent:      manager,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: map[chan []byte]struct{}{},
		ready:       make(chan struct{}),
		startErr:    make(chan error, 1),
	}

	err := w.readMJPEG(bytes.NewReader([]byte{0xFF, 0xD8, 0x01, 0x02, 0xFF, 0xD9}))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestBuildFilterChain(t *testing.T) {
	filters := buildFilterChain(5, 960, 640, 480)
	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(filters))
	}
	if filters[0] != "fps=5" {
		t.Fatalf("unexpected fps filter %q", filters[0])
	}
	if filters[1] != "scale=960:720" {
		t.Fatalf("unexpected scale filter %q", filters[1])
	}
}

func TestBuildQSVFilterChain(t *testing.T) {
	filters := buildQSVFilterChain(5, 960, 640, 480)
	if len(filters) != 1 {
		t.Fatalf("expected 1 qsv filter, got %d", len(filters))
	}
	if !strings.Contains(filters[0], "vpp_qsv=") {
		t.Fatalf("unexpected qsv filter %q", filters[0])
	}
	if !strings.Contains(filters[0], "framerate=5") {
		t.Fatalf("unexpected qsv framerate filter %q", filters[0])
	}
	if !strings.Contains(filters[0], "w=960") || !strings.Contains(filters[0], "h=720") {
		t.Fatalf("unexpected qsv scale filter %q", filters[0])
	}
	if !strings.Contains(filters[0], "format=nv12") {
		t.Fatalf("unexpected qsv format filter %q", filters[0])
	}
}

func TestComputeScaledDimensions(t *testing.T) {
	width, height, ok := computeScaledDimensions(640, 480, 961)
	if !ok {
		t.Fatal("expected scaled dimensions")
	}
	if width != 960 || height != 720 {
		t.Fatalf("unexpected scaled dimensions %dx%d", width, height)
	}
}

func TestResolvedScaleWidth(t *testing.T) {
	if got := resolvedScaleWidth(498, 0); got != 0 {
		t.Fatalf("expected scaling disabled when config width is 0, got %d", got)
	}
	if got := resolvedScaleWidth(0, 960); got != 960 {
		t.Fatalf("expected configured width when request width missing, got %d", got)
	}
	if got := resolvedScaleWidth(498, 960); got != 498 {
		t.Fatalf("expected request width override when scaling enabled, got %d", got)
	}
}

func TestWaitUntilReadyReturnsWorkerError(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		FrameRate:      5,
		JPEGQuality:    7,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &worker{
		key:         "test:stable",
		streamID:    "test",
		profileName: "stable",
		parent:      manager,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: map[chan []byte]struct{}{},
		ready:       make(chan struct{}),
		startErr:    make(chan error, 1),
	}

	wantErr := errors.New("ffmpeg failed")
	w.setError(wantErr)

	if err := w.waitUntilReady(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("expected worker error, got %v", err)
	}
}

func TestStopWhenIdleCancelsWorker(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    10 * time.Millisecond,
		MaxWorkers:     2,
		FrameRate:      5,
		JPEGQuality:    7,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &worker{
		key:         "test:stable",
		streamID:    "test",
		profileName: "stable",
		parent:      manager,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: map[chan []byte]struct{}{},
		ready:       make(chan struct{}),
		startErr:    make(chan error, 1),
	}

	done := make(chan struct{})
	go func() {
		w.stopWhenIdle()
		close(done)
	}()

	select {
	case <-w.ctx.Done():
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected idle worker to be cancelled")
	}

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected stopWhenIdle to return")
	}
}

func TestListWorkersDisabled(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        false,
		IdleTimeout:    time.Second,
		MaxWorkers:     14,
		FrameRate:      5,
		JPEGQuality:    7,
		Threads:        1,
		ScaleWidth:     960,
		FFmpegPath:     "ffmpeg",
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	statuses := manager.ListWorkers()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 disabled status entry, got %d", len(statuses))
	}
	if !statuses[0].MediaDisabled {
		t.Fatalf("expected media-disabled status, got %+v", statuses[0])
	}
	if statuses[0].MaxWorkers != 14 {
		t.Fatalf("unexpected max workers: %+v", statuses[0])
	}
}

func TestGetOrCreateWorkerRejectsWhenLimitReached(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     1,
		FrameRate:      5,
		JPEGQuality:    7,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	manager.mjpegWorkers["existing:stable"] = &worker{key: "existing:stable"}

	_, err := manager.getOrCreateMJPEGWorker(
		streams.Entry{ID: "another"},
		"stable",
		streams.Profile{Name: "stable", StreamURL: "rtsp://example.local/stream"},
		0,
	)
	if !errors.Is(err, ErrWorkerLimitReached) {
		t.Fatalf("expected worker limit error, got %v", err)
	}
}

func TestBuildHLSArgs(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		FrameRate:      5,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &hlsWorker{
		key:         "test:stable",
		streamID:    "test",
		profileName: "stable",
		profile: streams.Profile{
			Name:         "stable",
			StreamURL:    "rtsp://example.local/stream",
			FrameRate:    5,
			SourceWidth:  640,
			SourceHeight: 480,
		},
		parent:       manager,
		ctx:          ctx,
		cancel:       cancel,
		lastAccessAt: time.Now(),
		startErr:     make(chan error, 1),
	}

	args := w.buildFFmpegArgs(ffmpegStartAttempt{useHWAccel: true, inputPreset: manager.cfg.InputPreset})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-f hls") {
		t.Fatalf("expected hls output args, got %q", joined)
	}
	if !strings.Contains(joined, "-c:v libx264") {
		t.Fatalf("expected h264 transcode args, got %q", joined)
	}
	if !strings.Contains(joined, "-map 0:v:0") {
		t.Fatalf("expected explicit video mapping args, got %q", joined)
	}
	if !strings.Contains(joined, "-vf fps=5,scale=960:720") {
		t.Fatalf("expected software filter chain, got %q", joined)
	}
	if !strings.Contains(joined, "-map 0:a:0?") {
		t.Fatalf("expected optional audio mapping args, got %q", joined)
	}
	if !strings.Contains(joined, "-c:a aac") || !strings.Contains(joined, "-ar 48000") {
		t.Fatalf("expected aac audio transcode args in hls path, got %q", joined)
	}
	if strings.Contains(joined, "-an") {
		t.Fatalf("did not expect audio to be disabled in hls args, got %q", joined)
	}
	if !strings.Contains(joined, "-hls_list_size 6") || !strings.Contains(joined, "delete_segments") {
		t.Fatalf("expected live hls to keep a bounded deleting playlist, got %q", joined)
	}
	if !strings.Contains(joined, "index.m3u8") {
		t.Fatalf("expected playlist output arg, got %q", joined)
	}
}

func TestBuildPlaybackHLSArgsKeepsSegmentsForPlayerFetches(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		FrameRate:      5,
		Threads:        1,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &hlsWorker{
		key:         "nvrpb_test:stable",
		streamID:    "nvrpb_test",
		profileName: "stable",
		profile: streams.Profile{
			Name:      "stable",
			StreamURL: "rtsp://example.local/playback",
			FrameRate: 5,
		},
		parent:       manager,
		ctx:          ctx,
		cancel:       cancel,
		lastAccessAt: time.Now(),
		startErr:     make(chan error, 1),
	}

	args := w.buildFFmpegArgs(ffmpegStartAttempt{useHWAccel: false, inputPreset: manager.cfg.InputPreset})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-hls_list_size 0") {
		t.Fatalf("expected playback hls to keep a complete playlist, got %q", joined)
	}
	if strings.Contains(joined, "delete_segments") || strings.Contains(joined, "omit_endlist") {
		t.Fatalf("expected playback hls to keep segments until worker cleanup, got %q", joined)
	}
}

func TestValidateHLSFileNameRejectsTraversal(t *testing.T) {
	if err := validateHLSFileName("../segment.ts"); err == nil {
		t.Fatal("expected traversal name to be rejected")
	}
	if err := validateHLSFileName("segment_000.ts"); err != nil {
		t.Fatalf("expected valid segment name, got %v", err)
	}
}

func TestBuildHLSArgsWithoutHWAccel(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		VideoEncoder:   "qsv",
		FrameRate:      5,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
		HWAccelArgs:    []string{"-hwaccel", "qsv"},
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &hlsWorker{
		key:         "test:stable",
		streamID:    "test",
		profileName: "stable",
		profile: streams.Profile{
			Name:         "stable",
			StreamURL:    "rtsp://example.local/stream",
			FrameRate:    5,
			SourceWidth:  640,
			SourceHeight: 480,
		},
		parent:       manager,
		ctx:          ctx,
		cancel:       cancel,
		lastAccessAt: time.Now(),
		startErr:     make(chan error, 1),
	}

	args := w.buildFFmpegArgs(ffmpegStartAttempt{useHWAccel: false, inputPreset: manager.cfg.InputPreset})
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-hwaccel") {
		t.Fatalf("expected hwaccel args to be omitted, got %q", joined)
	}
	if !strings.Contains(joined, "-c:v libx264") {
		t.Fatalf("expected software fallback encoder args, got %q", joined)
	}
}

func TestBuildHLSArgsWithQSVEncoder(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		VideoEncoder:   "qsv",
		FrameRate:      5,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
		HWAccelArgs:    []string{"-hwaccel", "qsv"},
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &hlsWorker{
		key:         "test:stable",
		streamID:    "test",
		profileName: "stable",
		profile: streams.Profile{
			Name:         "stable",
			StreamURL:    "rtsp://example.local/stream",
			FrameRate:    5,
			SourceWidth:  640,
			SourceHeight: 480,
		},
		parent:       manager,
		ctx:          ctx,
		cancel:       cancel,
		lastAccessAt: time.Now(),
		startErr:     make(chan error, 1),
	}

	args := w.buildFFmpegArgs(ffmpegStartAttempt{useHWAccel: true, inputPreset: manager.cfg.InputPreset})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c:v h264_qsv") {
		t.Fatalf("expected qsv encoder args, got %q", joined)
	}
	if !strings.Contains(joined, "-hwaccel_output_format qsv") {
		t.Fatalf("expected explicit qsv hwaccel output format, got %q", joined)
	}
	if !strings.Contains(joined, "-pix_fmt nv12") {
		t.Fatalf("expected qsv pixel format args, got %q", joined)
	}
	if !strings.Contains(joined, "-vf vpp_qsv=framerate=5:w=960:h=720:format=nv12") {
		t.Fatalf("expected qsv filter chain, got %q", joined)
	}
	if strings.Contains(joined, "-preset veryfast") {
		t.Fatalf("did not expect libx264 preset args in qsv mode, got %q", joined)
	}
}

func TestBuildWebRTCArgs(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		FrameRate:      5,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &webrtcSession{
		key:         "test:stable:webrtc",
		streamID:    "test",
		profileName: "stable",
		profile: streams.Profile{
			Name:         "stable",
			StreamURL:    "rtsp://example.local/stream",
			FrameRate:    5,
			SourceWidth:  640,
			SourceHeight: 480,
		},
		parent: manager,
		ctx:    ctx,
		cancel: cancel,
	}

	args := session.buildFFmpegArgs(51000, 51002, ffmpegStartAttempt{useHWAccel: false, inputPreset: manager.cfg.InputPreset})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-f rtp") {
		t.Fatalf("expected rtp output args, got %q", joined)
	}
	if !strings.Contains(joined, "rtp://127.0.0.1:51000?pkt_size=1200") {
		t.Fatalf("expected local rtp target, got %q", joined)
	}
	if !strings.Contains(joined, "rtp://127.0.0.1:51002?pkt_size=1200") {
		t.Fatalf("expected local audio rtp target, got %q", joined)
	}
	if !strings.Contains(joined, "-c:v libx264") {
		t.Fatalf("expected h264 transcode args, got %q", joined)
	}
	if !strings.Contains(joined, "-vf fps=5,scale=960:720") {
		t.Fatalf("expected software filter chain, got %q", joined)
	}
	if !strings.Contains(joined, "-map 0:a:0?") || !strings.Contains(joined, "-c:a libopus") {
		t.Fatalf("expected optional opus audio args in webrtc path, got %q", joined)
	}
	if strings.Contains(joined, "-an") {
		t.Fatalf("did not expect audio to be disabled in webrtc args, got %q", joined)
	}
}

func TestBuildWebRTCArgsWithQSVEncoder(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		VideoEncoder:   "qsv",
		FrameRate:      5,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
		HWAccelArgs:    []string{"-hwaccel", "qsv"},
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &webrtcSession{
		key:         "test:stable:webrtc",
		streamID:    "test",
		profileName: "stable",
		profile: streams.Profile{
			Name:         "stable",
			StreamURL:    "rtsp://example.local/stream",
			FrameRate:    5,
			SourceWidth:  640,
			SourceHeight: 480,
		},
		parent: manager,
		ctx:    ctx,
		cancel: cancel,
	}

	args := session.buildFFmpegArgs(51000, 51002, ffmpegStartAttempt{useHWAccel: true, inputPreset: manager.cfg.InputPreset})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c:v h264_qsv") {
		t.Fatalf("expected qsv encoder args, got %q", joined)
	}
	if !strings.Contains(joined, "-hwaccel_output_format qsv") {
		t.Fatalf("expected explicit qsv hwaccel output format, got %q", joined)
	}
	if !strings.Contains(joined, "-vf vpp_qsv=framerate=5:w=960:h=720:format=nv12") {
		t.Fatalf("expected qsv filter chain, got %q", joined)
	}
	if strings.Contains(joined, "-preset ultrafast") {
		t.Fatalf("did not expect libx264 preset args in qsv mode, got %q", joined)
	}
}

func TestBuildMJPEGArgsWithQSVEncoder(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:        true,
		StartTimeout:   time.Second,
		IdleTimeout:    time.Second,
		MaxWorkers:     2,
		VideoEncoder:   "qsv",
		FrameRate:      5,
		JPEGQuality:    7,
		Threads:        1,
		ScaleWidth:     960,
		HLSSegmentTime: 2 * time.Second,
		HLSListSize:    6,
		HWAccelArgs:    []string{"-hwaccel", "qsv"},
	}, testResolver{}, zerolog.Nop(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &worker{
		key:         "test:stable",
		streamID:    "test",
		profileName: "stable",
		profile: streams.Profile{
			Name:         "stable",
			StreamURL:    "rtsp://example.local/stream",
			FrameRate:    5,
			SourceWidth:  640,
			SourceHeight: 480,
		},
		scaleWidth:  manager.cfg.ScaleWidth,
		parent:      manager,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: map[chan []byte]struct{}{},
		ready:       make(chan struct{}),
		startErr:    make(chan error, 1),
	}

	args := w.buildFFmpegArgs(ffmpegStartAttempt{useHWAccel: true, inputPreset: manager.cfg.InputPreset})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c:v mjpeg_qsv") {
		t.Fatalf("expected qsv mjpeg encoder args, got %q", joined)
	}
	if !strings.Contains(joined, "-hwaccel_output_format qsv") {
		t.Fatalf("expected explicit qsv hwaccel output format, got %q", joined)
	}
	if !strings.Contains(joined, "-global_quality 70") {
		t.Fatalf("expected mapped qsv quality args, got %q", joined)
	}
	if !strings.Contains(joined, "-vf vpp_qsv=framerate=5:w=960:h=720:format=nv12") {
		t.Fatalf("expected qsv mjpeg filter chain, got %q", joined)
	}
}

func TestMapSoftwareJPEGQualityToQSV(t *testing.T) {
	if got := mapSoftwareJPEGQualityToQSV(7); got != 70 {
		t.Fatalf("expected mapped quality 70, got %d", got)
	}
	if got := mapSoftwareJPEGQualityToQSV(0); got != 80 {
		t.Fatalf("expected default mapped quality 80, got %d", got)
	}
}

func TestAppendInputHWAccelArgsAddsExplicitQSVOutputFormat(t *testing.T) {
	args := appendInputHWAccelArgs(nil, config.MediaConfig{
		HWAccelArgs: []string{"-init_hw_device", "qsv=hw@va", "-hwaccel", "qsv"},
	}, true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-hwaccel_output_format qsv") {
		t.Fatalf("expected qsv output format in args, got %q", joined)
	}
}

func TestAppendInputHWAccelArgsDoesNotDuplicateOutputFormat(t *testing.T) {
	args := appendInputHWAccelArgs(nil, config.MediaConfig{
		HWAccelArgs: []string{"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"},
	}, true)
	count := 0
	for _, arg := range args {
		if arg == "-hwaccel_output_format" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one hwaccel_output_format arg, got %d in %+v", count, args)
	}
}

func TestBuildFFmpegStartAttempts(t *testing.T) {
	attempts := buildFFmpegStartAttempts(config.MediaConfig{
		InputPreset: "low_latency",
		HWAccelArgs: []string{"-hwaccel", "qsv"},
	})
	if len(attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %+v", attempts)
	}
	if !attempts[0].useHWAccel || attempts[0].inputPreset != "low_latency" {
		t.Fatalf("unexpected first attempt %+v", attempts[0])
	}
	if attempts[1].useHWAccel || attempts[1].inputPreset != "low_latency" {
		t.Fatalf("unexpected second attempt %+v", attempts[1])
	}
	if attempts[2].useHWAccel || attempts[2].inputPreset != "stable" {
		t.Fatalf("unexpected third attempt %+v", attempts[2])
	}
}

func TestBuildFFmpegStartAttemptsStableOnly(t *testing.T) {
	attempts := buildFFmpegStartAttempts(config.MediaConfig{
		InputPreset: "stable",
	})
	if len(attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %+v", attempts)
	}
	if attempts[0].useHWAccel || attempts[0].inputPreset != "stable" {
		t.Fatalf("unexpected stable-only attempt %+v", attempts[0])
	}
}

func TestBuildRTSPInputArgsLowLatency(t *testing.T) {
	args := buildRTSPInputArgs(streams.Profile{StreamURL: "rtsp://example.local/stream"}, "low_latency")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-fflags +discardcorrupt+nobuffer") {
		t.Fatalf("expected low-latency fflags, got %q", joined)
	}
	if !strings.Contains(joined, "-flags low_delay") {
		t.Fatalf("expected low-delay flags, got %q", joined)
	}
}

func TestBuildRTSPInputArgsStable(t *testing.T) {
	args := buildRTSPInputArgs(streams.Profile{StreamURL: "rtsp://example.local/stream"}, "stable")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-fflags +discardcorrupt") {
		t.Fatalf("expected stable discardcorrupt flag, got %q", joined)
	}
	if strings.Contains(joined, "nobuffer") || strings.Contains(joined, "low_delay") {
		t.Fatalf("did not expect low-latency flags in stable mode, got %q", joined)
	}
}

func TestIsHardwareAccelFailure(t *testing.T) {
	if !isHardwareAccelFailure("No device available for decoder: device type qsv needed for codec hevc_qsv.") {
		t.Fatal("expected qsv decoder error to trigger fallback")
	}
	if isHardwareAccelFailure("unexpected status 401 Unauthorized") {
		t.Fatal("did not expect unrelated error to trigger fallback")
	}
}

func TestWebRTCICEServersReturnsCopy(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled: true,
		WebRTCICEServers: []config.WebRTCICEServerConfig{
			{
				URLs:       []string{"stun:stun.example.net:3478"},
				Username:   "user",
				Credential: "secret",
			},
		},
	}, testResolver{}, zerolog.Nop(), nil)

	servers := manager.WebRTCICEServers()
	if len(servers) != 1 {
		t.Fatalf("expected one ice server, got %d", len(servers))
	}
	if servers[0].URLs[0] != "stun:stun.example.net:3478" {
		t.Fatalf("unexpected ice server urls %+v", servers[0].URLs)
	}

	servers[0].URLs[0] = "mutated"
	fresh := manager.WebRTCICEServers()
	if fresh[0].URLs[0] != "stun:stun.example.net:3478" {
		t.Fatalf("expected manager ice servers to be immutable copy, got %+v", fresh[0].URLs)
	}
}

func TestToPionICEServersSkipsEmptyEntries(t *testing.T) {
	servers := toPionICEServers([]WebRTCICEServer{
		{
			URLs: []string{" ", ""},
		},
		{
			URLs:       []string{" stun:stun.example.net:3478 "},
			Username:   " user ",
			Credential: " secret ",
		},
	})

	if len(servers) != 1 {
		t.Fatalf("expected one pion ice server, got %d", len(servers))
	}
	if servers[0].URLs[0] != "stun:stun.example.net:3478" {
		t.Fatalf("unexpected pion ice urls %+v", servers[0].URLs)
	}
	if servers[0].Username != "user" || servers[0].Credential != "secret" {
		t.Fatalf("unexpected pion ice credentials %+v", servers[0])
	}
}

func TestForwardIncomingAudioPacketWritesToConfiguredTarget(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer listener.Close()

	forwarders := newUplinkForwarders([]string{"udp://" + listener.LocalAddr().String()}, zerolog.Nop())
	defer closeUplinkForwarders(forwarders)
	if len(forwarders) != 1 {
		t.Fatalf("expected one forwarder, got %d", len(forwarders))
	}

	session := &webrtcSession{
		streamID: "front_vto",
		parent: New(config.MediaConfig{
			Enabled:             true,
			WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
		}, testResolver{}, zerolog.Nop(), nil),
		logger: zerolog.Nop(),
	}
	packet := &rtp.Packet{
		Header:  rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: 1, Timestamp: 480, SSRC: 42},
		Payload: []byte{0x11, 0x22, 0x33},
	}

	done := make(chan []byte, 1)
	go func() {
		buffer := make([]byte, 1500)
		_ = listener.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, readErr := listener.ReadFromUDP(buffer)
		if readErr == nil {
			done <- append([]byte(nil), buffer[:n]...)
		}
	}()

	session.forwardIncomingAudioPacket(forwarders, packet)

	select {
	case body := <-done:
		var got rtp.Packet
		if err := got.Unmarshal(body); err != nil {
			t.Fatalf("unmarshal forwarded rtp: %v", err)
		}
		if got.SequenceNumber != packet.SequenceNumber || !bytes.Equal(got.Payload, packet.Payload) {
			t.Fatalf("unexpected forwarded packet: %+v", got)
		}
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("timed out waiting for forwarded audio packet")
	}

	if session.uplinkForwardedPackets != 1 {
		t.Fatalf("expected 1 forwarded packet, got %d", session.uplinkForwardedPackets)
	}
	if session.uplinkForwardErrors != 0 {
		t.Fatalf("expected 0 forward errors, got %d", session.uplinkForwardErrors)
	}
}

func TestIntercomStatusAggregatesWebRTCSessions(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:             true,
		WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
	}, testResolver{}, zerolog.Nop(), nil)

	now := time.Now()
	manager.webrtcPeers["front_vto:stable:1"] = &webrtcSession{
		streamID:               "front_vto",
		profileName:            "stable",
		startedAt:              now.Add(-10 * time.Second),
		lastAccessAt:           now.Add(-2 * time.Second),
		uplinkActive:           true,
		uplinkCodec:            "audio/opus",
		uplinkPackets:          15,
		uplinkTargetCount:      1,
		uplinkForwardedPackets: 12,
		uplinkForwardErrors:    1,
	}
	manager.webrtcPeers["front_vto:quality:2"] = &webrtcSession{
		streamID:               "front_vto",
		profileName:            "quality",
		startedAt:              now.Add(-5 * time.Second),
		lastAccessAt:           now.Add(-1 * time.Second),
		uplinkPackets:          20,
		uplinkTargetCount:      2,
		uplinkForwardedPackets: 19,
	}
	manager.webrtcPeers["other:stable:1"] = &webrtcSession{
		streamID:    "other",
		profileName: "stable",
	}

	status := manager.IntercomStatus("front_vto")
	if !status.Active || status.SessionCount != 2 {
		t.Fatalf("unexpected intercom status %+v", status)
	}
	if !status.ExternalUplinkEnabled {
		t.Fatalf("expected external uplink enabled %+v", status)
	}
	if !status.UplinkActive || status.UplinkCodec != "audio/opus" {
		t.Fatalf("unexpected uplink status %+v", status)
	}
	if status.UplinkPackets != 35 || status.UplinkForwardedPackets != 31 || status.UplinkForwardErrors != 1 {
		t.Fatalf("unexpected uplink counters %+v", status)
	}
	if status.UplinkTargetCount != 2 {
		t.Fatalf("unexpected target count %+v", status)
	}
	if len(status.Profiles) != 2 || status.Profiles[0] != "quality" || status.Profiles[1] != "stable" {
		t.Fatalf("unexpected profiles %+v", status.Profiles)
	}
}

func TestSetIntercomUplinkEnabledOverridesDefault(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled:             true,
		WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
	}, testResolver{}, zerolog.Nop(), nil)

	if !manager.IntercomUplinkEnabled("front_vto") {
		t.Fatal("expected default enabled uplink export")
	}

	status := manager.SetIntercomUplinkEnabled("front_vto", false)
	if status.ExternalUplinkEnabled {
		t.Fatalf("expected disabled status %+v", status)
	}
	if manager.IntercomUplinkEnabled("front_vto") {
		t.Fatal("expected explicit disabled uplink export")
	}
}

func TestStopIntercomSessionsRemovesMatchingWebRTCPeers(t *testing.T) {
	manager := New(config.MediaConfig{
		Enabled: true,
	}, testResolver{}, zerolog.Nop(), nil)

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel1()
	defer cancel2()
	defer cancel3()

	manager.webrtcPeers["front_vto:stable:1"] = &webrtcSession{
		key:      "front_vto:stable:1",
		streamID: "front_vto",
		ctx:      ctx1,
		cancel:   cancel1,
	}
	manager.webrtcPeers["front_vto:quality:2"] = &webrtcSession{
		key:      "front_vto:quality:2",
		streamID: "front_vto",
		ctx:      ctx2,
		cancel:   cancel2,
	}
	manager.webrtcPeers["yard_ipc:stable:1"] = &webrtcSession{
		key:      "yard_ipc:stable:1",
		streamID: "yard_ipc",
		ctx:      ctx3,
		cancel:   cancel3,
	}

	status := manager.StopIntercomSessions("front_vto")
	if status.Active || status.SessionCount != 0 {
		t.Fatalf("expected no remaining front_vto sessions, got %+v", status)
	}
	if len(manager.webrtcPeers) != 1 {
		t.Fatalf("expected only unrelated session to remain, got %d", len(manager.webrtcPeers))
	}
	if _, ok := manager.webrtcPeers["yard_ipc:stable:1"]; !ok {
		t.Fatalf("expected unrelated stream session to remain, got %+v", manager.webrtcPeers)
	}
	select {
	case <-ctx1.Done():
	default:
		t.Fatal("expected first matching session context to be canceled")
	}
	select {
	case <-ctx2.Done():
	default:
		t.Fatal("expected second matching session context to be canceled")
	}
	select {
	case <-ctx3.Done():
		t.Fatal("expected unrelated session context to remain active")
	default:
	}
}

func TestToPionSessionDescriptionRejectsUnknownType(t *testing.T) {
	_, err := toPionSessionDescription(WebRTCSessionDescription{
		Type: "bogus",
		SDP:  "v=0\r\n",
	})
	if err == nil {
		t.Fatal("expected invalid sdp type error")
	}
}
