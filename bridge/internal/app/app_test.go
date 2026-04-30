package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/store"
)

type stubRuntimeMedia struct {
	status       map[string]media.IntercomStatus
	captureFrame func(context.Context, string, string, int) ([]byte, string, error)
	findClips    func(media.ClipQuery) ([]media.ClipInfo, error)
	activeClip   func(string) (media.ClipInfo, bool)
}

type stubSnapshotProvider struct {
	body        []byte
	contentType string
	calls       int
	lastChannel int
	err         error
}

type stubRecordingSearcher struct {
	find func(context.Context, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)
}

func (s *stubSnapshotProvider) Snapshot(_ context.Context, channel int) ([]byte, string, error) {
	s.calls++
	s.lastChannel = channel
	if s.err != nil {
		return nil, "", s.err
	}
	return append([]byte(nil), s.body...), s.contentType, nil
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

func TestRuntimeServicesNVRRecordingsMergesBridgeClips(t *testing.T) {
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
			if query.RootDeviceID != "west20_nvr" || query.Channel != 5 {
				t.Fatalf("unexpected clip query %+v", query)
			}
			return []media.ClipInfo{{
				ID:           "clip_1",
				StreamID:     "west20_nvr_channel_05",
				RootDeviceID: "west20_nvr",
				DeviceKind:   dahua.DeviceKindNVRChannel,
				Channel:      5,
				Profile:      "stable",
				Status:       media.ClipStatusCompleted,
				StartedAt:    time.Date(2026, 4, 28, 0, 30, 0, 0, time.UTC),
				EndedAt:      time.Date(2026, 4, 28, 0, 30, 15, 0, time.UTC),
				Duration:     15 * time.Second,
				Bytes:        4096,
				FileName:     "clip_1.mp4",
			}}, nil
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
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Source != "nvr" {
		t.Fatalf("expected NVR item first, got %+v", result.Items[0])
	}
	if result.Items[0].DownloadURL != "" {
		t.Fatalf("NVR archive items must not expose unverified direct download URLs, got %q", result.Items[0].DownloadURL)
	}
	if result.Items[1].Source != "bridge" || result.Items[1].ClipID != "clip_1" {
		t.Fatalf("expected bridge clip second, got %+v", result.Items[1])
	}
	if !strings.Contains(result.Items[1].DownloadURL, "/api/v1/media/recordings/clip_1/download") {
		t.Fatalf("unexpected bridge download url %q", result.Items[1].DownloadURL)
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
	if !strings.Contains(entries[0].Capture.StartRecordingURL, "/api/v1/media/streams/west20_nvr_channel_05/recordings") {
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
			StableFrameRate:    5,
			SubstreamFrameRate: 7,
		},
	}, probes)
	services.RegisterNVR("west20_nvr", nil, nil, config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          "http://192.168.150.10",
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
	if !strings.Contains(profile.StreamURL, "/cam/realmonitor?") {
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
	_, stableProfile, ok := services.GetStream(session.StreamID, "stable", true)
	if !ok {
		t.Fatal("expected stable playback stream to resolve")
	}
	if !strings.Contains(stableProfile.StreamURL, "subtype=0") {
		t.Fatalf("expected stable playback profile to use main subtype, got %q", stableProfile.StreamURL)
	}
	if stableProfile.SourceWidth != 1920 || stableProfile.SourceHeight != 1080 {
		t.Fatalf("expected stable playback profile to use main source size, got %dx%d", stableProfile.SourceWidth, stableProfile.SourceHeight)
	}
}

func TestRuntimeServicesSeekNVRPlaybackSessionReturnsNewSession(t *testing.T) {
	services := newRuntimeServices(config.Config{}, store.NewProbeStore())
	services.RegisterNVR("west20_nvr", nil, nil, config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          "http://192.168.150.10",
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
