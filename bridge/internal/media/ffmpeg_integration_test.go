package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/rs/zerolog"
)

func ffmpegFixture(t *testing.T) (string, string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg integration test requires ffmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("FFmpeg integration test requires ffprobe")
	}
	source := filepath.Join(t.TempDir(), "source.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10", "-t", "3", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-g", "10", "-movflags", "+faststart", source}
	if output, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	return ffmpeg, source
}

func TestFFmpegCompletedArchiveReleasesCapacityAndRetainsReadableOutput(t *testing.T) {
	ffmpeg, source := ffmpegFixture(t)
	m := New(config.MediaConfig{Enabled: true, FFmpegPath: ffmpeg, MaxWorkers: 1, FrameRate: 10,
		StartTimeout: 10 * time.Second, IdleTimeout: time.Minute, HLSKeepAfterExit: time.Hour,
		HLSTmpDir: t.TempDir(), HLSSegmentTime: time.Second, HLSListSize: 3, Threads: 1,
	}, testResolver{}, zerolog.Nop(), nil)
	defer m.Close()
	profile := streams.Profile{StreamURL: source, VideoCodec: "H.264", SourceWidth: 320, SourceHeight: 180, InputDuration: int64(3 * time.Second)}
	w, err := m.getOrCreateHLSWorker(streams.Entry{ID: "nvrpb_one"}, "quality", profile)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.ctx.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("HLS did not finish")
	}
	if body, err := w.readFileWhenReady(context.Background(), "index.m3u8"); err != nil || !strings.Contains(string(body), "#EXT-X-ENDLIST") {
		t.Fatalf("retained playlist: %v %s", err, body)
	}
	dash, err := m.getOrCreateDASHWorker(streams.Entry{ID: "nvrpb_two"}, "quality", profile)
	if err != nil {
		t.Fatalf("finished HLS blocked another process: %v", err)
	}
	select {
	case <-dash.ctx.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("DASH did not finish")
	}
	if body, err := dash.readFileWhenReady(context.Background(), "manifest.mpd"); err != nil || !strings.Contains(string(body), "<MPD") {
		t.Fatalf("retained DASH: %v %s", err, body)
	}
	m.Close()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.hlsWorkers)+len(m.dashWorkers)+len(m.retainedOutputs) != 0 {
		t.Fatal("shutdown left retained workers")
	}
}

func TestFFmpegH264ClipRemuxProducesDecodableMP4(t *testing.T) {
	ffmpeg, source := ffmpegFixture(t)
	m := New(config.MediaConfig{Enabled: true, FFmpegPath: ffmpeg, ClipPath: t.TempDir(), StartTimeout: 10 * time.Second}, testResolver{}, zerolog.Nop(), nil)
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := m.StartDirectClip(ctx, DirectClipStartRequest{StreamID: "export", SourceURL: source, VideoCodec: "H.264", Duration: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for info.Status == ClipStatusRecording {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
		info, err = m.GetClip(info.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if info.Status != ClipStatusCompleted {
		t.Fatalf("clip failed: %+v", info)
	}
	path, err := m.ClipFilePath(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput(); err != nil || len(output) != 0 {
		t.Fatalf("invalid MP4: %v %s", err, output)
	}
}

func TestFFmpegStoppingFiniteRemuxDoesNotStartAnotherAttempt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX FFmpeg pacing wrapper")
	}
	ffmpeg, source := ffmpegFixture(t)
	dir := t.TempDir()
	callLog, wrapper := filepath.Join(dir, "calls"), filepath.Join(dir, "ffmpeg")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	script := "#!/bin/sh\nprintf x >> " + quote(callLog) + "\nexec " + quote(ffmpeg) + " -re \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ffprobe, _ := exec.LookPath("ffprobe")
	if err := os.Symlink(ffprobe, filepath.Join(dir, "ffprobe")); err != nil {
		t.Fatal(err)
	}
	m := New(config.MediaConfig{Enabled: true, FFmpegPath: wrapper, ClipPath: t.TempDir(), StartTimeout: 10 * time.Second}, testResolver{}, zerolog.Nop(), nil)
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := m.StartDirectClip(ctx, DirectClipStartRequest{StreamID: "stopped", SourceURL: source, VideoCodec: "H.264", Duration: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := m.StopClip(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if calls, err := os.ReadFile(callLog); err != nil || string(calls) != "x" {
		t.Fatalf("stop started a replacement process: %q %v", calls, err)
	}
	info, err = m.GetClip(info.ID)
	if err != nil || info.Status != ClipStatusCompleted {
		t.Fatalf("graceful stop failed: %+v %v", info, err)
	}
}
