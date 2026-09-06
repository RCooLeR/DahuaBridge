package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
)

func TestLiveSourceHealthFallsBackInBothDirections(t *testing.T) {
	for _, preferred := range []string{"nvr", "camera"} {
		t.Run(preferred, func(t *testing.T) {
			runtime, _ := liveSourceRuntime(t)
			if _, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_05", preferred); err != nil {
				t.Fatal(err)
			}
			mediaReader := &invalidatingRuntimeMedia{}
			runtime.AttachMedia(mediaReader)
			checked := make(map[string]int)
			runtime.liveHealth.probe = func(_ context.Context, target string) (bool, error) {
				parsed, _ := url.Parse(target)
				source := "nvr"
				if parsed.Hostname() == "camera.local" {
					source = "camera"
				}
				checked[source]++
				if source == preferred {
					return false, errors.New("RTSP unavailable")
				}
				return true, nil
			}
			runtime.checkLiveSourceHealth(context.Background(), "nvr_channel_05", "")
			entry := runtime.ListStreams(true)[0]
			if entry.LiveSource.PreferredSource != preferred || entry.LiveSource.Source != otherLiveSource(preferred) || entry.LiveSource.FallbackReason == "" {
				t.Fatalf("missing reported fallback: %+v", entry.LiveSource)
			}
			if checked[otherLiveSource(preferred)] != len(entry.Profiles) {
				t.Fatalf("alternate was not validated for every profile: %v", checked)
			}
			if len(mediaReader.invalidated) != 1 {
				t.Fatal("effective source change did not reconnect live workers")
			}
			for _, profile := range entry.Profiles {
				if !strings.Contains(profile.RecorderStreamURL, "nvr-user:nvr-pass@nvr.local") {
					t.Fatal("health failover changed archive credentials")
				}
				if profile.AlternativeStreamURL == "" || profile.AlternativeStreamURL == profile.StreamURL {
					t.Fatal("alternate route missing after failover")
				}
			}
			public := runtime.ListStreams(false)[0]
			for _, profile := range public.Profiles {
				if strings.Contains(profile.StreamURL+profile.AlternativeStreamURL+profile.RecorderStreamURL, "@") {
					t.Fatal("health catalog leaked credentials")
				}
			}
		})
	}
}

func TestLiveSourceHealthDoesNotSwitchToUnhealthyAlternate(t *testing.T) {
	for _, name := range []string{"both down", "alternate substream down"} {
		t.Run(name, func(t *testing.T) {
			runtime, _ := liveSourceRuntime(t)
			mediaReader := &invalidatingRuntimeMedia{}
			runtime.AttachMedia(mediaReader)
			runtime.liveHealth.probe = func(_ context.Context, target string) (bool, error) {
				parsed, _ := url.Parse(target)
				return name == "alternate substream down" && parsed.Hostname() == "camera.local" && parsed.Query().Get("subtype") == "0", nil
			}
			runtime.checkLiveSourceHealth(context.Background(), "nvr_channel_05", "")
			if entry := runtime.ListStreams(false)[0]; entry.LiveSource.Source != "nvr" || len(mediaReader.invalidated) != 0 {
				t.Fatal("switched before alternate route was healthy")
			}
		})
	}
}

func TestLiveSourceHealthRetriesPreferredOnlyAfterCooldown(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	if _, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_05", "camera"); err != nil {
		t.Fatal(err)
	}
	cameraHealthy, cameraChecks := false, 0
	runtime.liveHealth.probe = func(_ context.Context, target string) (bool, error) {
		if strings.Contains(target, "@camera.local:") {
			cameraChecks++
			return cameraHealthy, nil
		}
		return true, nil
	}
	runtime.checkLiveSourceHealth(context.Background(), "nvr_channel_05", "")
	if runtime.ListStreams(false)[0].LiveSource.Source != "nvr" {
		t.Fatal("camera failure did not select recorder")
	}
	cameraHealthy = true
	before := cameraChecks
	runtime.checkLiveSourceHealth(context.Background(), "nvr_channel_05", "")
	if cameraChecks != before || runtime.ListStreams(false)[0].LiveSource.Source != "nvr" {
		t.Fatal("failed route retried before cooldown")
	}
	h := &runtime.liveHealth
	h.mu.Lock()
	state := h.states["nvr_channel_05"]
	state.failedUntil["camera"] = time.Now().Add(-time.Second)
	h.states["nvr_channel_05"] = state
	h.mu.Unlock()
	runtime.checkLiveSourceHealth(context.Background(), "nvr_channel_05", "")
	entry := runtime.ListStreams(false)[0]
	if entry.LiveSource.Source != "camera" || entry.LiveSource.FallbackReason != "" {
		t.Fatalf("preferred route did not recover: %+v", entry.LiveSource)
	}
}

func TestMediaFailureSkipsFailedRouteAndRejectsObsoleteURL(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	entry := runtime.ListStreams(true)[0]
	failedURL := entry.Profiles["quality"].StreamURL
	checks := 0
	runtime.liveHealth.probe = func(_ context.Context, target string) (bool, error) {
		checks++
		if strings.Contains(target, "@nvr.local:") {
			t.Error("media failure should mark failed NVR route without reaccepting DESCRIBE success")
		}
		return true, nil
	}
	runtime.checkLiveSourceHealth(context.Background(), entry.ID, failedURL)
	if runtime.ListStreams(false)[0].LiveSource.Source != "camera" || checks != 2 {
		t.Fatal("reported media failure did not validate alternate")
	}
	before := checks
	runtime.checkLiveSourceHealth(context.Background(), entry.ID, failedURL)
	if checks != before || runtime.ListStreams(false)[0].LiveSource.Source != "camera" {
		t.Fatal("obsolete failed URL changed new selection")
	}
}

func TestLateHealthResultCannotUndoNewPreference(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	runtime.liveHealth.probe = func(ctx context.Context, target string) (bool, error) {
		if strings.Contains(target, "@nvr.local:") {
			once.Do(func() { close(entered) })
			select {
			case <-release:
			case <-ctx.Done():
				return false, ctx.Err()
			}
			return false, nil
		}
		return true, nil
	}
	go func() { defer close(done); runtime.checkLiveSourceHealth(context.Background(), "nvr_channel_05", "") }()
	<-entered
	// Return to the same route signature to exercise the revision guard as well
	// as the simpler signature mismatch check.
	for _, source := range []string{"camera", "nvr"} {
		if _, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_05", source); err != nil {
			t.Fatal(err)
		}
	}
	close(release)
	<-done
	if runtime.ListStreams(false)[0].LiveSource.Source != "nvr" {
		t.Fatal("old health result overwrote newer user preference")
	}
}

func TestHealthLoopStopsOutstandingProbes(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	runtime.probes.Update("nvr", func(probe *dahua.ProbeResult) {
		for channel := 6; channel < 15; channel++ {
			probe.Children = append(probe.Children, dahua.Device{ID: fmt.Sprintf("nvr_channel_%02d", channel), Kind: dahua.DeviceKindNVRChannel, Attributes: map[string]string{"channel_index": fmt.Sprint(channel)}})
		}
	})
	entered := make(chan struct{}, 8)
	var active atomic.Int32
	runtime.liveHealth.probe = func(ctx context.Context, _ string) (bool, error) {
		active.Add(1)
		defer active.Add(-1)
		entered <- struct{}{}
		<-ctx.Done()
		return false, ctx.Err()
	}
	stop := runtime.StartLiveSourceHealth(context.Background())
	for range liveSourceHealthWorkers {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("health worker did not start")
		}
	}
	if active.Load() != liveSourceHealthWorkers {
		t.Fatal("health probe concurrency is not bounded")
	}
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("health shutdown did not cancel in-flight probe")
	}
	if active.Load() != 0 {
		t.Fatal("probe remained after health loop stopped")
	}
}

type healthNotifyingMedia struct {
	stubRuntimeMedia
	changes chan string
}

func (m healthNotifyingMedia) InvalidateStream(id string) { m.changes <- id }

func TestMediaFailureReporterQueuesValidation(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	initialChecks := make(chan struct{})
	var nvrChecks atomic.Int32
	runtime.liveHealth.probe = func(_ context.Context, target string) (bool, error) {
		if strings.Contains(target, "@nvr.local:") && nvrChecks.Add(1) == 2 {
			close(initialChecks)
		}
		return true, nil
	}
	changes := make(chan string, 4)
	runtime.AttachMedia(healthNotifyingMedia{changes: changes})
	stop := runtime.StartLiveSourceHealth(context.Background())
	defer stop()
	select {
	case <-initialChecks:
	case <-time.After(time.Second):
		t.Fatal("initial health probe did not run")
	}
	entry := runtime.ListStreams(true)[0]
	runtime.ReportLiveSourceFailure(entry.ID, entry.Profiles["quality"].StreamURL)
	select {
	case id := <-changes:
		if id != entry.ID {
			t.Fatal("unexpected invalidated stream")
		}
	case <-time.After(time.Second):
		t.Fatal("failure report did not validate alternate")
	}
	if runtime.ListStreams(false)[0].LiveSource.Source != "camera" {
		t.Fatal("failure report did not update effective source")
	}
}

func TestExplicitCameraPreferenceWithoutCredentialsReportsFallback(t *testing.T) {
	runtime, cfg := liveSourceRuntime(t)
	cfg.DirectIPCCredentials = nil
	runtime.UpdateDeviceConfig("nvr", cfg)
	entry, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_05", streams.LiveSourceCamera)
	if err != nil || entry.LiveSource.PreferredSource != "camera" || entry.LiveSource.Source != "nvr" || entry.LiveSource.FallbackReason == "" {
		t.Fatalf("unavailable camera preference was not saved with fallback: %+v, %v", entry, err)
	}
}
