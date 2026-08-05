package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	archiveapi "RCooLeR/DahuaBridge/internal/archive"
	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

type stubRuntimeMedia struct {
	status       map[string]media.IntercomStatus
	captureFrame func(context.Context, string, string, int) ([]byte, string, error)
	findClips    func(media.ClipQuery) ([]media.ClipInfo, error)
	getClip      func(string) (media.ClipInfo, error)
	activeClip   func(string) (media.ClipInfo, bool)
}

type stubSnapshotProvider struct {
	mu          sync.Mutex
	body        []byte
	contentType string
	calls       int
	lastChannel int
	err         error
	snapshot    func(context.Context, int) ([]byte, string, error)
}

type stubRecordingSearcher struct {
	find func(context.Context, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)
}

type streamEventsFunc func(context.Context, chan<- dahua.Event) error

func (f streamEventsFunc) StreamEvents(ctx context.Context, sink chan<- dahua.Event) error {
	return f(ctx, sink)
}

type stubRuntimeArchiveReader struct {
	search func(context.Context, string, dahua.NVRRecordingQuery, func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)) (dahua.NVRRecordingSearchResult, error)
	enrich func(context.Context, string, *dahua.NVRRecordingSearchResult, archiveapi.ClipFinder) error
}

func (s stubRuntimeArchiveReader) SearchRecordings(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery, fallback func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)) (dahua.NVRRecordingSearchResult, error) {
	if s.search != nil {
		return s.search(ctx, deviceID, query, fallback)
	}
	return dahua.NVRRecordingSearchResult{}, nil
}

func (s stubRuntimeArchiveReader) EnrichRecordings(ctx context.Context, deviceID string, result *dahua.NVRRecordingSearchResult, clips archiveapi.ClipFinder) error {
	if s.enrich != nil {
		return s.enrich(ctx, deviceID, result, clips)
	}
	return nil
}

func (s stubRuntimeArchiveReader) TrackClipExport(context.Context, string, dahua.NVRPlaybackSessionRequest, media.ClipInfo) error {
	return nil
}

func (s stubRuntimeArchiveReader) EventSummary(_ context.Context, deviceID string, startTime time.Time, endTime time.Time, _ string) (dahua.NVREventSummary, error) {
	return dahua.NVREventSummary{
		DeviceID:  deviceID,
		StartTime: startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
		Items:     []dahua.NVREventSummaryItem{},
		Channels:  []dahua.NVREventChannelSummary{},
	}, nil
}

func (s stubRuntimeArchiveReader) ArchiveCoverage(_ context.Context, deviceID string, channel int) (dahua.NVRArchiveCoverage, error) {
	return dahua.NVRArchiveCoverage{
		DeviceID: deviceID,
		Channel:  channel,
		Chunks:   []dahua.NVRArchiveCoverageChunk{},
	}, nil
}

func (s *stubSnapshotProvider) Snapshot(ctx context.Context, channel int) ([]byte, string, error) {
	s.mu.Lock()
	s.calls++
	s.lastChannel = channel
	snapshot := s.snapshot
	body := append([]byte(nil), s.body...)
	contentType := s.contentType
	err := s.err
	s.mu.Unlock()

	if snapshot != nil {
		return snapshot(ctx, channel)
	}
	if err != nil {
		return nil, "", err
	}
	return body, contentType, nil
}

func (s *stubSnapshotProvider) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *stubSnapshotProvider) channel() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastChannel
}

func TestRuntimeServicesNVRSnapshotDedupesConcurrentMisses(t *testing.T) {
	probes := store.NewProbeStore()
	services := newRuntimeServices(config.Config{}, probes)

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	provider := &stubSnapshotProvider{
		snapshot: func(_ context.Context, channel int) ([]byte, string, error) {
			if channel != 2 {
				t.Fatalf("unexpected channel %d", channel)
			}
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			return []byte("jpeg-bytes"), "image/jpeg", nil
		},
	}
	services.RegisterNVR("west20_nvr", provider, nil, config.DeviceConfig{ID: "west20_nvr"})

	startBarrier := make(chan struct{})
	results := make(chan []byte, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-startBarrier
			body, _, err := services.NVRSnapshot(context.Background(), "west20_nvr", 2)
			if err != nil {
				errs <- err
				return
			}
			results <- body
		}()
	}
	close(startBarrier)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first snapshot fetch")
	}

	time.Sleep(50 * time.Millisecond)
	close(release)

	for range 2 {
		select {
		case err := <-errs:
			t.Fatalf("NVRSnapshot returned error: %v", err)
		case body := <-results:
			if string(body) != "jpeg-bytes" {
				t.Fatalf("unexpected snapshot body %q", string(body))
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for deduped snapshot result")
		}
	}

	if provider.callCount() != 1 {
		t.Fatalf("expected one backend snapshot call, got %d", provider.callCount())
	}
}

func TestArchiveExportUsesPlaybackSessionForSMDIVS(t *testing.T) {
	request := dahua.NVRPlaybackSessionRequest{
		Channel:   1,
		StartTime: time.Date(2026, 5, 1, 8, 10, 42, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 1, 8, 11, 2, 0, time.UTC),
		Source:    "smd_ivs",
		Type:      "Event.smdTypeHuman",
	}
	if !shouldUsePlaybackSessionArchiveExport(request) {
		t.Fatal("expected smd_ivs archive export to use playback session RTSP source")
	}
}

func TestArchiveLiveSearcherBypassesAttachedArchive(t *testing.T) {
	services := newRuntimeServices(config.Config{}, store.NewProbeStore())
	liveCalls := 0
	archiveCalls := 0
	services.RegisterNVR("west20_nvr", nil, stubRecordingSearcher{
		find: func(_ context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			liveCalls++
			return dahua.NVRRecordingSearchResult{
				DeviceID: "west20_nvr",
				Channel:  query.Channel,
				Items: []dahua.NVRRecording{{
					Source:    "nvr_event",
					Channel:   query.Channel,
					StartTime: "2026-05-01 11:00:00",
					EndTime:   "2026-05-01 11:00:20",
					Type:      "Event.smdTypeHuman",
				}},
			}, nil
		},
	}, config.DeviceConfig{ID: "west20_nvr", ChannelAllowlist: []int{1}})
	services.AttachArchive(stubRuntimeArchiveReader{
		search: func(context.Context, string, dahua.NVRRecordingQuery, func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)) (dahua.NVRRecordingSearchResult, error) {
			archiveCalls++
			return dahua.NVRRecordingSearchResult{Items: []dahua.NVRRecording{}}, nil
		},
	})

	result, err := (archiveLiveSearcher{runtime: services}).NVRRecordings(context.Background(), "west20_nvr", dahua.NVRRecordingQuery{
		Channel:   1,
		StartTime: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Limit:     10,
		EventOnly: true,
		EventCode: "human",
	})
	if err != nil {
		t.Fatalf("live archive search returned error: %v", err)
	}
	if liveCalls != 1 {
		t.Fatalf("live search calls = %d, want 1", liveCalls)
	}
	if archiveCalls != 0 {
		t.Fatalf("attached archive search calls = %d, want 0", archiveCalls)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected live NVR item, got %+v", result.Items)
	}
}

func TestRunEventLoopRetriesAfterConnectFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probes := store.NewProbeStore()
	probes.Set("west20_nvr", &dahua.ProbeResult{
		Root: dahua.Device{ID: "west20_nvr", Kind: dahua.DeviceKindNVR},
		Children: []dahua.Device{{
			ID:       "west20_nvr_channel_01",
			ParentID: "west20_nvr",
			Kind:     dahua.DeviceKindNVRChannel,
			Attributes: map[string]string{
				"channel_index": "1",
			},
		}},
		States: map[string]dahua.DeviceState{
			"west20_nvr_channel_01": {Available: true},
		},
	})

	attempts := make(chan int, 2)
	var calls atomic.Int32
	source := streamEventsFunc(func(ctx context.Context, sink chan<- dahua.Event) error {
		call := int(calls.Add(1))
		attempts <- call
		if call == 1 {
			return errors.New("connect failed")
		}

		sink <- dahua.Event{
			DeviceID:   "west20_nvr",
			DeviceKind: dahua.DeviceKindNVR,
			ChildID:    "west20_nvr_channel_01",
			Code:       "VideoMotion",
			Action:     dahua.EventActionStart,
			Channel:    1,
			OccurredAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		}
		waitForStoredState(ctx, probes, "west20_nvr", "west20_nvr_channel_01", "motion", true)
		cancel()
		<-ctx.Done()
		return ctx.Err()
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		runEventLoopWithRetry(
			ctx,
			zerolog.Nop(),
			metrics.New(buildinfo.Info()),
			probes,
			stubDriver{id: "west20_nvr", kind: dahua.DeviceKindNVR},
			source,
			eventStreamRetryConfig{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond},
		)
	}()

	for want := 1; want <= 2; want++ {
		select {
		case got := <-attempts:
			if got != want {
				t.Fatalf("stream attempt = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for stream attempt %d", want)
		}
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event loop did not stop after context cancellation")
	}

	stored, ok := probes.Get("west20_nvr")
	if !ok {
		t.Fatal("expected stored probe result")
	}
	info := stored.States["west20_nvr_channel_01"].Info
	if info["motion"] != true {
		t.Fatalf("expected retried event stream to update motion state, got %+v", info)
	}
}

func waitForStoredState(ctx context.Context, probes *store.ProbeStore, rootID string, targetID string, key string, want any) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()

	for {
		if stored, ok := probes.Get(rootID); ok {
			if state, ok := stored.States[targetID]; ok && state.Info[key] == want {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-timeout.C:
			return
		case <-ticker.C:
		}
	}
}

func TestRuntimeServicesNVRRecordingsDedupesConcurrentMisses(t *testing.T) {
	probes := store.NewProbeStore()
	services := newRuntimeServices(config.Config{}, probes)

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	services.RegisterNVR("west20_nvr", nil, stubRecordingSearcher{
		find: func(_ context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			calls.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			return dahua.NVRRecordingSearchResult{
				DeviceID:      "west20_nvr",
				Channel:       query.Channel,
				StartTime:     query.StartTime.In(time.Local).Format(bridgeRecordingTimeLayout),
				EndTime:       query.EndTime.In(time.Local).Format(bridgeRecordingTimeLayout),
				Limit:         query.Limit,
				ReturnedCount: 1,
				Items: []dahua.NVRRecording{{
					Source:    "nvr",
					Channel:   query.Channel,
					StartTime: "2026-05-01 02:30:00",
					EndTime:   "2026-05-01 03:00:00",
					Type:      "dav",
				}},
			}, nil
		},
	}, config.DeviceConfig{ID: "west20_nvr"})

	query := dahua.NVRRecordingQuery{
		Channel:   1,
		StartTime: time.Date(2026, 5, 1, 2, 30, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 1, 2, 31, 0, 0, time.UTC),
		Limit:     10,
	}

	startBarrier := make(chan struct{})
	type result struct {
		value dahua.NVRRecordingSearchResult
		err   error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-startBarrier
			value, err := services.NVRRecordings(context.Background(), "west20_nvr", query)
			results <- result{value: value, err: err}
		}()
	}
	close(startBarrier)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first recording search")
	}

	time.Sleep(50 * time.Millisecond)
	close(release)

	for range 2 {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("NVRRecordings returned error: %v", result.err)
			}
			if len(result.value.Items) != 1 || result.value.Items[0].Type != "dav" {
				t.Fatalf("unexpected recording search result %+v", result.value)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for deduped recording search result")
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one backend recording search, got %d", got)
	}
}

func (s stubRecordingSearcher) FindRecordings(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	if s.find != nil {
		return s.find(ctx, query)
	}
	return dahua.NVRRecordingSearchResult{}, nil
}

func (s stubRuntimeMedia) IntercomStatus(streamID string) media.IntercomStatus {
	if s.status != nil {
		if status, ok := s.status[streamID]; ok {
			return status
		}
	}
	return media.IntercomStatus{StreamID: streamID}
}

func (s stubRuntimeMedia) CaptureFrame(ctx context.Context, streamID string, profile string, scaleWidth int) ([]byte, string, error) {
	if s.captureFrame != nil {
		return s.captureFrame(ctx, streamID, profile, scaleWidth)
	}
	return []byte("jpeg"), "image/jpeg", nil
}

func (s stubRuntimeMedia) FindClips(query media.ClipQuery) ([]media.ClipInfo, error) {
	if s.findClips != nil {
		return s.findClips(query)
	}
	return nil, nil
}

func (s stubRuntimeMedia) GetClip(clipID string) (media.ClipInfo, error) {
	if s.getClip != nil {
		return s.getClip(clipID)
	}
	return media.ClipInfo{}, nil
}

func (s stubRuntimeMedia) ActiveClip(streamID string) (media.ClipInfo, bool) {
	if s.activeClip != nil {
		return s.activeClip(streamID)
	}
	return media.ClipInfo{}, false
}

func TestRuntimeServicesListStreamsIncludesIntercomRuntimeStatus(t *testing.T) {
	probes := store.NewProbeStore()
	probes.Set("front_vto", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "front_vto",
			Name: "Front Door",
			Kind: dahua.DeviceKindVTO,
			Attributes: map[string]string{
				"lock_count": "1",
			},
		},
		States: map[string]dahua.DeviceState{
			"front_vto": {
				Available: true,
				Info: map[string]any{
					"audio_codec": "PCM",
					"call_state":  "ringing",
				},
			},
		},
	})

	services := newRuntimeServices(config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
		Media: config.MediaConfig{
			WebRTCUplinkTargets: []string{"udp://127.0.0.1:5004"},
		},
	}, probes)
	services.RegisterVTO("front_vto", nil, config.DeviceConfig{
		ID:       "front_vto",
		BaseURL:  "http://vto.example.local",
		Username: "admin",
		Password: "secret",
	})
	services.AttachMedia(stubRuntimeMedia{
		status: map[string]media.IntercomStatus{
			"front_vto": {
				StreamID:               "front_vto",
				Active:                 true,
				SessionCount:           1,
				ExternalUplinkEnabled:  true,
				UplinkActive:           true,
				UplinkCodec:            "audio/opus",
				UplinkPackets:          8,
				UplinkForwardedPackets: 6,
				UplinkForwardErrors:    1,
			},
		},
	})

	entries := services.ListStreams(false)
	if len(entries) != 1 {
		t.Fatalf("expected 1 stream entry, got %d", len(entries))
	}
	if entries[0].Intercom == nil {
		t.Fatal("expected intercom summary")
	}
	if !entries[0].Intercom.BridgeSessionActive || entries[0].Intercom.BridgeSessionCount != 1 {
		t.Fatalf("expected bridge runtime session state, got %+v", entries[0].Intercom)
	}
	if !entries[0].Intercom.ExternalUplinkEnabled || entries[0].Intercom.BridgeUplinkCodec != "audio/opus" {
		t.Fatalf("expected bridge runtime uplink state, got %+v", entries[0].Intercom)
	}
}

func TestRuntimeServicesNVRSnapshotCachesRecentResponses(t *testing.T) {
	probes := store.NewProbeStore()
	services := newRuntimeServices(config.Config{}, probes)
	provider := &stubSnapshotProvider{
		body:        []byte("jpeg-bytes"),
		contentType: "image/jpeg",
	}
	services.RegisterNVR("west20_nvr", provider, nil, config.DeviceConfig{ID: "west20_nvr"})

	body1, contentType1, err := services.NVRSnapshot(context.Background(), "west20_nvr", 2)
	if err != nil {
		t.Fatalf("first NVRSnapshot returned error: %v", err)
	}
	body2, contentType2, err := services.NVRSnapshot(context.Background(), "west20_nvr", 2)
	if err != nil {
		t.Fatalf("second NVRSnapshot returned error: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("expected provider to be called once, got %d", provider.calls)
	}
	if provider.lastChannel != 2 {
		t.Fatalf("expected provider channel 2, got %d", provider.lastChannel)
	}
	if string(body1) != "jpeg-bytes" || string(body2) != "jpeg-bytes" {
		t.Fatalf("unexpected cached snapshot bodies: %q / %q", string(body1), string(body2))
	}
	if contentType1 != "image/jpeg" || contentType2 != "image/jpeg" {
		t.Fatalf("unexpected content types: %q / %q", contentType1, contentType2)
	}
}

func TestRuntimeServicesNVRSnapshotUsesMediaCaptureFirst(t *testing.T) {
	probes := store.NewProbeStore()
	probes.Set("west20_nvr", &dahua.ProbeResult{
		Root: dahua.Device{ID: "west20_nvr", Kind: dahua.DeviceKindNVR},
		Children: []dahua.Device{{
			ID:   "west20_nvr_channel_02",
			Kind: dahua.DeviceKindNVRChannel,
			Name: "Gate",
			Attributes: map[string]string{
				"channel_index":   "2",
				"main_codec":      "H.264",
				"main_resolution": "1920x1080",
				"sub_codec":       "H.264",
				"sub_resolution":  "704x576",
			},
		}},
	})
	services := newRuntimeServices(config.Config{}, probes)
	provider := &stubSnapshotProvider{
		body:        []byte("provider"),
		contentType: "image/jpeg",
	}
	services.RegisterNVR("west20_nvr", provider, nil, config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          "http://192.168.1.10",
		ChannelAllowlist: []int{2},
	})
	services.AttachMedia(stubRuntimeMedia{
		captureFrame: func(_ context.Context, streamID string, profile string, scaleWidth int) ([]byte, string, error) {
			if streamID != "west20_nvr_channel_02" {
				t.Fatalf("unexpected stream id %q", streamID)
			}
			if profile != "quality" {
				t.Fatalf("unexpected profile %q", profile)
			}
			if scaleWidth != 0 {
				t.Fatalf("unexpected scale width %d", scaleWidth)
			}
			return []byte("media"), "image/jpeg", nil
		},
	})

	body, contentType, err := services.NVRSnapshot(context.Background(), "west20_nvr", 2)
	if err != nil {
		t.Fatalf("NVRSnapshot returned error: %v", err)
	}
	if string(body) != "media" || contentType != "image/jpeg" {
		t.Fatalf("unexpected snapshot result body=%q content_type=%q", string(body), contentType)
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not to be called, got %d", provider.calls)
	}
}

func TestRuntimeServicesNVRRecordingsDoesNotMergeBridgeClips(t *testing.T) {
	probes := store.NewProbeStore()
	services := newRuntimeServices(config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
	}, probes)
	services.RegisterNVR("west20_nvr", nil, stubRecordingSearcher{
		find: func(_ context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			if query.Channel != 5 {
				t.Fatalf("unexpected channel %d", query.Channel)
			}
			return dahua.NVRRecordingSearchResult{
				DeviceID:      "west20_nvr",
				Channel:       5,
				StartTime:     "2026-04-28 00:00:00",
				EndTime:       "2026-04-28 01:00:00",
				Limit:         query.Limit,
				ReturnedCount: 1,
				Items: []dahua.NVRRecording{{
					Source:    "nvr",
					Channel:   5,
					StartTime: "2026-04-28 00:10:00",
					EndTime:   "2026-04-28 00:20:00",
					Type:      "dav",
				}},
			}, nil
		},
	}, config.DeviceConfig{ID: "west20_nvr"})
	services.AttachMedia(stubRuntimeMedia{
		findClips: func(query media.ClipQuery) ([]media.ClipInfo, error) {
			t.Fatalf("NVR archive search must not query bridge MP4 clips, got %+v", query)
			return nil, nil
		},
	})

	result, err := services.NVRRecordings(context.Background(), "west20_nvr", dahua.NVRRecordingQuery{
		Channel:   5,
		StartTime: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 28, 1, 0, 0, 0, time.UTC),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("NVRRecordings returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 native NVR item, got %d", len(result.Items))
	}
	if result.Items[0].Source != "nvr" {
		t.Fatalf("expected NVR item first, got %+v", result.Items[0])
	}
	if result.Items[0].DownloadURL != "" {
		t.Fatalf("NVR archive items must not expose unverified direct download URLs, got %q", result.Items[0].DownloadURL)
	}
}

func TestRuntimeServicesNVRRecordingsSkipsAssetEnrichmentWhenRequested(t *testing.T) {
	services := newRuntimeServices(config.Config{}, store.NewProbeStore())
	services.RegisterNVR("west20_nvr", nil, stubRecordingSearcher{
		find: func(context.Context, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			t.Fatal("expected attached archive search to serve recording query")
			return dahua.NVRRecordingSearchResult{}, nil
		},
	}, config.DeviceConfig{ID: "west20_nvr"})
	services.AttachArchive(stubRuntimeArchiveReader{
		search: func(_ context.Context, deviceID string, query dahua.NVRRecordingQuery, _ func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)) (dahua.NVRRecordingSearchResult, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if !query.SkipAssetEnrichment {
				t.Fatal("expected skip asset enrichment query flag")
			}
			return dahua.NVRRecordingSearchResult{
				DeviceID:      deviceID,
				Channel:       query.Channel,
				Limit:         query.Limit,
				ReturnedCount: 1,
				Items: []dahua.NVRRecording{{
					ID:         "event_1",
					RecordKind: "event",
					Source:     "nvr_event",
					Channel:    query.Channel,
					StartTime:  "2026-05-01 08:10:00",
					EndTime:    "2026-05-01 08:10:20",
					Type:       "Event.smdTypeHuman",
				}},
			}, nil
		},
		enrich: func(context.Context, string, *dahua.NVRRecordingSearchResult, archiveapi.ClipFinder) error {
			t.Fatal("asset enrichment must not run for DB-only event list searches")
			return nil
		},
	})
	services.AttachMedia(stubRuntimeMedia{
		findClips: func(query media.ClipQuery) ([]media.ClipInfo, error) {
			t.Fatalf("DB-only event list must not scan clips, got %+v", query)
			return nil, nil
		},
	})

	result, err := services.NVRRecordings(context.Background(), "west20_nvr", dahua.NVRRecordingQuery{
		Channel:             1,
		StartTime:           time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndTime:             time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Limit:               20,
		EventOnly:           true,
		EventCode:           "all",
		SkipAssetEnrichment: true,
	})
	if err != nil {
		t.Fatalf("NVRRecordings returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected one event item, got %d", len(result.Items))
	}
	if result.Items[0].AssetClipID != "" || result.Items[0].AssetStatus != "" {
		t.Fatalf("expected DB-only item without MP4 asset fields, got %+v", result.Items[0])
	}
}

func TestRuntimeServicesNVRRecordingsCachesRecentSearches(t *testing.T) {
	probes := store.NewProbeStore()
	services := newRuntimeServices(config.Config{}, probes)

	calls := 0
	services.RegisterNVR("west20_nvr", nil, stubRecordingSearcher{
		find: func(_ context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			calls++
			return dahua.NVRRecordingSearchResult{
				DeviceID:      "west20_nvr",
				Channel:       query.Channel,
				StartTime:     query.StartTime.In(time.Local).Format(bridgeRecordingTimeLayout),
				EndTime:       query.EndTime.In(time.Local).Format(bridgeRecordingTimeLayout),
				Limit:         query.Limit,
				ReturnedCount: 1,
				Items: []dahua.NVRRecording{{
					Source:    "nvr",
					Channel:   query.Channel,
					StartTime: "2026-05-01 02:30:00",
					EndTime:   "2026-05-01 03:00:00",
					Type:      "dav",
				}},
			}, nil
		},
	}, config.DeviceConfig{ID: "west20_nvr"})

	query := dahua.NVRRecordingQuery{
		Channel:   1,
		StartTime: time.Date(2026, 5, 1, 2, 30, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 1, 2, 31, 0, 0, time.UTC),
		Limit:     10,
	}

	first, err := services.NVRRecordings(context.Background(), "west20_nvr", query)
	if err != nil {
		t.Fatalf("first NVRRecordings returned error: %v", err)
	}
	second, err := services.NVRRecordings(context.Background(), "west20_nvr", query)
	if err != nil {
		t.Fatalf("second NVRRecordings returned error: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected cached search to call backend once, got %d", calls)
	}
	if len(first.Items) != 1 || len(second.Items) != 1 {
		t.Fatalf("unexpected cached result sizes: %d / %d", len(first.Items), len(second.Items))
	}
	first.Items[0].Type = "mutated"
	if second.Items[0].Type != "dav" {
		t.Fatalf("expected cached result clone, got %+v", second.Items[0])
	}
}

func TestRuntimeServicesListStreamsIncludesCaptureSummary(t *testing.T) {
	probes := store.NewProbeStore()
	probes.Set("west20_nvr", &dahua.ProbeResult{
		Root: dahua.Device{ID: "west20_nvr", Kind: dahua.DeviceKindNVR},
		Children: []dahua.Device{{
			ID:   "west20_nvr_channel_05",
			Kind: dahua.DeviceKindNVRChannel,
			Name: "Gate",
			Attributes: map[string]string{
				"channel_index":   "5",
				"main_codec":      "H.264",
				"main_resolution": "1920x1080",
				"sub_codec":       "H.264",
				"sub_resolution":  "704x576",
			},
		}},
	})
	services := newRuntimeServices(config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
	}, probes)
	services.RegisterNVR("west20_nvr", nil, nil, config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          "http://192.168.1.10",
		ChannelAllowlist: []int{5},
	})
	services.AttachMedia(stubRuntimeMedia{
		activeClip: func(streamID string) (media.ClipInfo, bool) {
			if streamID != "west20_nvr_channel_05" {
				return media.ClipInfo{}, false
			}
			return media.ClipInfo{
				ID:        "clip_active",
				StreamID:  streamID,
				Profile:   "stable",
				Status:    media.ClipStatusRecording,
				StartedAt: time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC),
			}, true
		},
	})

	entries := services.ListStreams(false)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Capture == nil {
		t.Fatal("expected capture summary")
	}
	if !strings.Contains(entries[0].Capture.SnapshotURL, "/api/v1/media/snapshot/west20_nvr_channel_05?profile=quality") {
		t.Fatalf("unexpected snapshot url %q", entries[0].Capture.SnapshotURL)
	}
	if !strings.Contains(entries[0].Capture.StartRecordingURL, "/api/v1/media/streams/west20_nvr_channel_05/recordings?profile=quality") {
		t.Fatalf("unexpected start recording url %q", entries[0].Capture.StartRecordingURL)
	}
	if !entries[0].Capture.Active || entries[0].Capture.ActiveClipID != "clip_active" {
		t.Fatalf("unexpected active clip summary %+v", entries[0].Capture)
	}
	if !strings.Contains(entries[0].Capture.StopRecordingURL, "/api/v1/media/recordings/clip_active/stop") {
		t.Fatalf("unexpected stop recording url %q", entries[0].Capture.StopRecordingURL)
	}
}

func TestRuntimeServicesCreateNVRPlaybackSessionResolvesPlaybackStream(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() {
		time.Local = previousLocal
	})

	probes := store.NewProbeStore()
	probes.Set("west20_nvr", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "west20_nvr",
			Kind: dahua.DeviceKindNVR,
		},
		Children: []dahua.Device{
			{
				ID:   "west20_nvr_channel_01",
				Kind: dahua.DeviceKindNVRChannel,
				Name: "Entrance",
				Attributes: map[string]string{
					"channel_index":   "1",
					"main_codec":      "H.264",
					"main_resolution": "1920x1080",
					"sub_codec":       "H.264",
					"sub_resolution":  "704x576",
				},
			},
		},
	})

	services := newRuntimeServices(config.Config{
		HomeAssistant: config.HomeAssistantConfig{
			PublicBaseURL: "http://bridge.local:8080",
		},
		Media: config.MediaConfig{
			StableFrameRate: 5,
		},
	}, probes)
	services.RegisterNVR("west20_nvr", nil, nil, config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          "http://192.0.2.10",
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{1},
	})

	session, err := services.CreateNVRPlaybackSession(context.Background(), "west20_nvr", dahua.NVRPlaybackSessionRequest{
		Channel:   1,
		StartTime: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 28, 1, 0, 0, 0, time.UTC),
		SeekTime:  time.Date(2026, 4, 28, 0, 15, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateNVRPlaybackSession returned error: %v", err)
	}
	if session.StreamID == "" {
		t.Fatal("expected playback session stream id")
	}
	if !strings.Contains(session.Profiles["quality"].HLSURL, "/api/v1/media/hls/"+session.StreamID+"/quality/index.m3u8") {
		t.Fatalf("unexpected quality hls url: %+v", session.Profiles["quality"])
	}

	entry, profile, ok := services.GetStream(session.StreamID, "quality", true)
	if !ok {
		t.Fatal("expected playback stream to resolve")
	}
	if entry.Channel != 1 || entry.RootDeviceID != "west20_nvr" {
		t.Fatalf("unexpected playback entry: %+v", entry)
	}
	if !strings.Contains(profile.StreamURL, "/cam/playback?") {
		t.Fatalf("expected playback stream url, got %q", profile.StreamURL)
	}
	if !strings.Contains(profile.StreamURL, "starttime=2026_04_28_00_15_00") {
		t.Fatalf("expected seek time in playback stream url, got %q", profile.StreamURL)
	}
	if !strings.Contains(profile.StreamURL, "subtype=0") {
		t.Fatalf("expected main subtype in playback stream url, got %q", profile.StreamURL)
	}
	if !strings.Contains(profile.StreamURL, "assistant:secret@") {
		t.Fatalf("expected credentialed playback stream url, got %q", profile.StreamURL)
	}
	if _, ok := session.Profiles["default"]; ok {
		t.Fatalf("unexpected default playback alias %+v", session.Profiles)
	}
	if _, ok := session.Profiles["substream"]; ok {
		t.Fatalf("unexpected substream playback alias %+v", session.Profiles)
	}
	_, stableProfile, ok := services.GetStream(session.StreamID, "stable", true)
	if !ok {
		t.Fatal("expected stable playback stream to resolve")
	}
	if !strings.Contains(stableProfile.StreamURL, "subtype=1") {
		t.Fatalf("expected stable playback profile to use substream subtype, got %q", stableProfile.StreamURL)
	}
	if stableProfile.SourceWidth != 704 || stableProfile.SourceHeight != 576 {
		t.Fatalf("expected stable playback profile to use substream source size, got %dx%d", stableProfile.SourceWidth, stableProfile.SourceHeight)
	}
	_, stableAlias, ok := services.GetStream(session.StreamID, "substream", true)
	if !ok || stableAlias.StreamURL != stableProfile.StreamURL {
		t.Fatalf("expected substream alias to resolve to stable profile, got ok=%v profile=%+v", ok, stableAlias)
	}
	_, qualityAlias, ok := services.GetStream(session.StreamID, "default", true)
	if !ok || qualityAlias.StreamURL != profile.StreamURL {
		t.Fatalf("expected default alias to resolve to quality profile, got ok=%v profile=%+v", ok, qualityAlias)
	}
}

func TestBuildPlaybackRTSPURLPreservesDahuaQueryOrderAndCredentials(t *testing.T) {
	previousLocal := time.Local
	local := time.FixedZone("EEST", 3*60*60)
	time.Local = local
	t.Cleanup(func() {
		time.Local = previousLocal
	})

	actual := buildPlaybackRTSPURL(
		config.DeviceConfig{
			BaseURL:  "http://192.0.2.10",
			Username: "assistant",
			Password: "secret",
		},
		5,
		0,
		time.Date(2026, 5, 4, 4, 30, 0, 0, local),
		time.Date(2026, 5, 4, 5, 0, 0, 0, local),
		true,
	)
	expected := "rtsp://example-user:example-password@192.0.2.10:554/cam/playback?channel=5&subtype=0&starttime=2026_05_04_04_30_00&endtime=2026_05_04_05_00_00"
	if actual != expected {
		t.Fatalf("unexpected playback RTSP URL\nwant: %s\n got: %s", expected, actual)
	}
}

func TestRuntimeServicesPlaybackRTSPUsesLocalWallClock(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.FixedZone("EEST", 3*60*60)
	t.Cleanup(func() {
		time.Local = previousLocal
	})

	services := newRuntimeServices(config.Config{}, store.NewProbeStore())
	services.RegisterNVR("west20_nvr", nil, nil, config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          "http://192.0.2.10",
		ChannelAllowlist: []int{1},
	})

	session, err := services.CreateNVRPlaybackSession(context.Background(), "west20_nvr", dahua.NVRPlaybackSessionRequest{
		Channel:   1,
		StartTime: time.Date(2026, 4, 27, 21, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 27, 22, 0, 0, 0, time.UTC),
		SeekTime:  time.Date(2026, 4, 27, 21, 15, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateNVRPlaybackSession returned error: %v", err)
	}

	_, profile, ok := services.GetStream(session.StreamID, "quality", true)
	if !ok {
		t.Fatal("expected playback stream to resolve")
	}
	if !strings.Contains(profile.StreamURL, "starttime=2026_04_28_00_15_00") {
		t.Fatalf("expected local seek time in playback stream url, got %q", profile.StreamURL)
	}
	if !strings.Contains(profile.StreamURL, "endtime=2026_04_28_01_00_00") {
		t.Fatalf("expected local end time in playback stream url, got %q", profile.StreamURL)
	}
}

func TestRuntimeServicesSeekNVRPlaybackSessionReturnsNewSession(t *testing.T) {
	services := newRuntimeServices(config.Config{}, store.NewProbeStore())
	services.RegisterNVR("west20_nvr", nil, nil, config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          "http://192.0.2.10",
		ChannelAllowlist: []int{1},
	})

	session, err := services.CreateNVRPlaybackSession(context.Background(), "west20_nvr", dahua.NVRPlaybackSessionRequest{
		Channel:   1,
		StartTime: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 28, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateNVRPlaybackSession returned error: %v", err)
	}

	seeked, err := services.SeekNVRPlaybackSession(context.Background(), session.ID, time.Date(2026, 4, 28, 0, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("SeekNVRPlaybackSession returned error: %v", err)
	}
	if seeked.ID == session.ID {
		t.Fatalf("expected new playback session id, got %q", seeked.ID)
	}
	if seeked.SeekTime != "2026-04-28T00:30:00Z" {
		t.Fatalf("unexpected seek time %q", seeked.SeekTime)
	}
}
