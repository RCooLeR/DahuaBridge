package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	mediaapi "RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/store"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/rs/zerolog"
)

type stubProbeReader struct {
	list  func() []*dahua.ProbeResult
	get   func(string) (*dahua.ProbeResult, bool)
	stats func() store.Stats
}

func (s stubProbeReader) List() []*dahua.ProbeResult {
	if s.list != nil {
		return s.list()
	}
	return nil
}

func (s stubProbeReader) Get(deviceID string) (*dahua.ProbeResult, bool) {
	if s.get != nil {
		return s.get(deviceID)
	}
	return nil, false
}

func (s stubProbeReader) Stats() store.Stats {
	if s.stats != nil {
		return s.stats()
	}
	return store.Stats{}
}

type stubSnapshotReader struct {
	listStreams              func(bool) []streams.Entry
	adminSettings            func() map[string]any
	nvrRecordings            func(context.Context, string, dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error)
	nvrDownloadRecording     func(context.Context, string, string) (dahua.NVRRecordingDownload, error)
	createNVRPlaybackSession func(context.Context, string, dahua.NVRPlaybackSessionRequest) (dahua.NVRPlaybackSession, error)
	getNVRPlaybackSession    func(string) (dahua.NVRPlaybackSession, error)
	seekNVRPlaybackSession   func(context.Context, string, time.Time) (dahua.NVRPlaybackSession, error)
}

func (stubSnapshotReader) NVRSnapshot(context.Context, string, int) ([]byte, string, error) {
	return nil, "", nil
}
func (s stubSnapshotReader) NVRRecordings(ctx context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	if s.nvrRecordings != nil {
		return s.nvrRecordings(ctx, deviceID, query)
	}
	return dahua.NVRRecordingSearchResult{}, nil
}
func (s stubSnapshotReader) NVRDownloadRecording(ctx context.Context, deviceID string, filePath string) (dahua.NVRRecordingDownload, error) {
	if s.nvrDownloadRecording != nil {
		return s.nvrDownloadRecording(ctx, deviceID, filePath)
	}
	return dahua.NVRRecordingDownload{}, nil
}
func (s stubSnapshotReader) CreateNVRPlaybackSession(ctx context.Context, deviceID string, request dahua.NVRPlaybackSessionRequest) (dahua.NVRPlaybackSession, error) {
	if s.createNVRPlaybackSession != nil {
		return s.createNVRPlaybackSession(ctx, deviceID, request)
	}
	return dahua.NVRPlaybackSession{}, nil
}
func (s stubSnapshotReader) GetNVRPlaybackSession(sessionID string) (dahua.NVRPlaybackSession, error) {
	if s.getNVRPlaybackSession != nil {
		return s.getNVRPlaybackSession(sessionID)
	}
	return dahua.NVRPlaybackSession{}, nil
}
func (s stubSnapshotReader) SeekNVRPlaybackSession(ctx context.Context, sessionID string, seekTime time.Time) (dahua.NVRPlaybackSession, error) {
	if s.seekNVRPlaybackSession != nil {
		return s.seekNVRPlaybackSession(ctx, sessionID, seekTime)
	}
	return dahua.NVRPlaybackSession{}, nil
}
func (stubSnapshotReader) VTOSnapshot(context.Context, string) ([]byte, string, error) {
	return nil, "", nil
}
func (stubSnapshotReader) IPCSnapshot(context.Context, string) ([]byte, string, error) {
	return nil, "", nil
}
func (s stubSnapshotReader) ListStreams(includeCredentials bool) []streams.Entry {
	if s.listStreams != nil {
		return s.listStreams(includeCredentials)
	}
	return nil
}

func (s stubSnapshotReader) AdminSettings() map[string]any {
	if s.adminSettings != nil {
		return s.adminSettings()
	}
	return nil
}

type stubMediaReader struct {
	enabled                  bool
	subscribe                func(context.Context, string, string) (<-chan []byte, func(), error)
	subscribeScaled          func(context.Context, string, string, int) (<-chan []byte, func(), error)
	captureFrame             func(context.Context, string, string, int) ([]byte, string, error)
	hlsPlaylist              func(context.Context, string, string) ([]byte, error)
	hlsSegment               func(context.Context, string, string, string) ([]byte, string, error)
	startClip                func(context.Context, mediaapi.ClipStartRequest) (mediaapi.ClipInfo, error)
	stopClip                 func(context.Context, string) (mediaapi.ClipInfo, error)
	getClip                  func(string) (mediaapi.ClipInfo, error)
	findClips                func(mediaapi.ClipQuery) ([]mediaapi.ClipInfo, error)
	clipFilePath             func(string) (string, error)
	webrtcOffer              func(context.Context, string, string, mediaapi.WebRTCSessionDescription) (mediaapi.WebRTCSessionDescription, error)
	iceServers               func() []mediaapi.WebRTCICEServer
	intercomStatus           func(string) mediaapi.IntercomStatus
	stopIntercomSessions     func(string) mediaapi.IntercomStatus
	setIntercomUplinkEnabled func(string, bool) mediaapi.IntercomStatus
	listWorker               func() []mediaapi.WorkerStatus
}

type stubActionReader struct {
	unlock        func(context.Context, string, int) error
	answer        func(context.Context, string) error
	hangup        func(context.Context, string) error
	vtoControls   func(context.Context, string) (dahua.VTOControlCapabilities, error)
	vtoOutputVol  func(context.Context, string, int, int) error
	vtoInputVol   func(context.Context, string, int, int) error
	vtoMute       func(context.Context, string, bool) error
	vtoRecord     func(context.Context, string, bool) error
	nvrControls   func(context.Context, string, int) (dahua.NVRChannelControlCapabilities, error)
	nvrPTZ        func(context.Context, string, dahua.NVRPTZRequest) error
	nvrAux        func(context.Context, string, dahua.NVRAuxRequest) error
	nvrAudio      func(context.Context, string, dahua.NVRAudioRequest) error
	nvrRecording  func(context.Context, string, dahua.NVRRecordingRequest) error
	nvrDiagnostic func(context.Context, string, dahua.NVRDiagnosticActionRequest) (dahua.NVRDiagnosticActionResult, error)
	probe         func(context.Context, string) (*dahua.ProbeResult, error)
	probeAll      func(context.Context) []dahua.ProbeActionResult
	rotate        func(context.Context, string, dahua.DeviceConfigUpdate) (*dahua.ProbeResult, error)
	refreshNVR    func(context.Context, string) (*dahua.ProbeResult, error)
}

type stubEventReader struct {
	list  func(string, string, dahua.DeviceKind, string, string, int) []dahua.Event
	stats func() map[string]any
	clear func() int
}

func (s stubMediaReader) Enabled() bool {
	return s.enabled
}

func (s stubMediaReader) Subscribe(ctx context.Context, streamID string, profile string) (<-chan []byte, func(), error) {
	if s.subscribe != nil {
		return s.subscribe(ctx, streamID, profile)
	}
	if s.subscribeScaled != nil {
		return s.subscribeScaled(ctx, streamID, profile, 0)
	}
	ch := make(chan []byte)
	close(ch)
	return ch, func() {}, nil
}

func (s stubMediaReader) SubscribeScaled(ctx context.Context, streamID string, profile string, scaleWidth int) (<-chan []byte, func(), error) {
	if s.subscribeScaled != nil {
		return s.subscribeScaled(ctx, streamID, profile, scaleWidth)
	}
	if s.subscribe != nil {
		return s.subscribe(ctx, streamID, profile)
	}
	ch := make(chan []byte)
	close(ch)
	return ch, func() {}, nil
}

func (s stubMediaReader) CaptureFrame(ctx context.Context, streamID string, profile string, scaleWidth int) ([]byte, string, error) {
	if s.captureFrame != nil {
		return s.captureFrame(ctx, streamID, profile, scaleWidth)
	}
	return []byte("jpeg"), "image/jpeg", nil
}

func (s stubMediaReader) ListWorkers() []mediaapi.WorkerStatus {
	if s.listWorker != nil {
		return s.listWorker()
	}
	return nil
}

func (s stubMediaReader) WebRTCICEServers() []mediaapi.WebRTCICEServer {
	if s.iceServers != nil {
		return s.iceServers()
	}
	return nil
}

func (s stubMediaReader) IntercomStatus(streamID string) mediaapi.IntercomStatus {
	if s.intercomStatus != nil {
		return s.intercomStatus(streamID)
	}
	return mediaapi.IntercomStatus{StreamID: streamID}
}

func (s stubMediaReader) StopIntercomSessions(streamID string) mediaapi.IntercomStatus {
	if s.stopIntercomSessions != nil {
		return s.stopIntercomSessions(streamID)
	}
	return mediaapi.IntercomStatus{StreamID: streamID}
}

func (s stubMediaReader) SetIntercomUplinkEnabled(streamID string, enabled bool) mediaapi.IntercomStatus {
	if s.setIntercomUplinkEnabled != nil {
		return s.setIntercomUplinkEnabled(streamID, enabled)
	}
	return mediaapi.IntercomStatus{
		StreamID:              streamID,
		ExternalUplinkEnabled: enabled,
	}
}

func (s stubMediaReader) HLSPlaylist(ctx context.Context, streamID string, profile string) ([]byte, error) {
	if s.hlsPlaylist != nil {
		return s.hlsPlaylist(ctx, streamID, profile)
	}
	return []byte("#EXTM3U\n"), nil
}

func (s stubMediaReader) HLSSegment(ctx context.Context, streamID string, profile string, segmentName string) ([]byte, string, error) {
	if s.hlsSegment != nil {
		return s.hlsSegment(ctx, streamID, profile, segmentName)
	}
	return []byte("segment"), "video/mp2t", nil
}

func (s stubMediaReader) StartClip(ctx context.Context, request mediaapi.ClipStartRequest) (mediaapi.ClipInfo, error) {
	if s.startClip != nil {
		return s.startClip(ctx, request)
	}
	return mediaapi.ClipInfo{
		ID:        "clip_test",
		StreamID:  request.StreamID,
		Profile:   request.ProfileName,
		Status:    mediaapi.ClipStatusRecording,
		StartedAt: time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC),
		Duration:  request.Duration,
		FileName:  "clip_test.mp4",
	}, nil
}

func (s stubMediaReader) StopClip(ctx context.Context, clipID string) (mediaapi.ClipInfo, error) {
	if s.stopClip != nil {
		return s.stopClip(ctx, clipID)
	}
	return mediaapi.ClipInfo{
		ID:        clipID,
		Status:    mediaapi.ClipStatusCompleted,
		StartedAt: time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 4, 29, 12, 0, 5, 0, time.UTC),
		FileName:  clipID + ".mp4",
	}, nil
}

func (s stubMediaReader) GetClip(clipID string) (mediaapi.ClipInfo, error) {
	if s.getClip != nil {
		return s.getClip(clipID)
	}
	return mediaapi.ClipInfo{
		ID:        clipID,
		Status:    mediaapi.ClipStatusCompleted,
		StartedAt: time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 4, 29, 12, 0, 5, 0, time.UTC),
		FileName:  clipID + ".mp4",
	}, nil
}

func (s stubMediaReader) FindClips(query mediaapi.ClipQuery) ([]mediaapi.ClipInfo, error) {
	if s.findClips != nil {
		return s.findClips(query)
	}
	return nil, nil
}

func (s stubMediaReader) ClipFilePath(clipID string) (string, error) {
	if s.clipFilePath != nil {
		return s.clipFilePath(clipID)
	}
	return "", mediaapi.ErrClipNotFound
}

func (s stubMediaReader) WebRTCAnswer(ctx context.Context, streamID string, profile string, offer mediaapi.WebRTCSessionDescription) (mediaapi.WebRTCSessionDescription, error) {
	if s.webrtcOffer != nil {
		return s.webrtcOffer(ctx, streamID, profile, offer)
	}
	return mediaapi.WebRTCSessionDescription{
		Type: "answer",
		SDP:  "v=0\r\n",
	}, nil
}

func (s stubActionReader) UnlockVTOLock(ctx context.Context, deviceID string, lockIndex int) error {
	if s.unlock == nil {
		return nil
	}
	return s.unlock(ctx, deviceID, lockIndex)
}

func (s stubActionReader) AnswerVTOCall(ctx context.Context, deviceID string) error {
	if s.answer == nil {
		return nil
	}
	return s.answer(ctx, deviceID)
}

func (s stubActionReader) HangupVTOCall(ctx context.Context, deviceID string) error {
	if s.hangup == nil {
		return nil
	}
	return s.hangup(ctx, deviceID)
}

func (s stubActionReader) VTOControlCapabilities(ctx context.Context, deviceID string) (dahua.VTOControlCapabilities, error) {
	if s.vtoControls == nil {
		return dahua.VTOControlCapabilities{}, nil
	}
	return s.vtoControls(ctx, deviceID)
}

func (s stubActionReader) SetVTOAudioOutputVolume(ctx context.Context, deviceID string, slot int, level int) error {
	if s.vtoOutputVol == nil {
		return nil
	}
	return s.vtoOutputVol(ctx, deviceID, slot, level)
}

func (s stubActionReader) SetVTOAudioInputVolume(ctx context.Context, deviceID string, slot int, level int) error {
	if s.vtoInputVol == nil {
		return nil
	}
	return s.vtoInputVol(ctx, deviceID, slot, level)
}

func (s stubActionReader) SetVTOMute(ctx context.Context, deviceID string, muted bool) error {
	if s.vtoMute == nil {
		return nil
	}
	return s.vtoMute(ctx, deviceID, muted)
}

func (s stubActionReader) SetVTORecordingEnabled(ctx context.Context, deviceID string, enabled bool) error {
	if s.vtoRecord == nil {
		return nil
	}
	return s.vtoRecord(ctx, deviceID, enabled)
}

func (s stubActionReader) NVRChannelControlCapabilities(ctx context.Context, deviceID string, channel int) (dahua.NVRChannelControlCapabilities, error) {
	if s.nvrControls == nil {
		return dahua.NVRChannelControlCapabilities{}, nil
	}
	return s.nvrControls(ctx, deviceID, channel)
}

func (s stubActionReader) ControlNVRPTZ(ctx context.Context, deviceID string, request dahua.NVRPTZRequest) error {
	if s.nvrPTZ == nil {
		return nil
	}
	return s.nvrPTZ(ctx, deviceID, request)
}

func (s stubActionReader) ControlNVRAux(ctx context.Context, deviceID string, request dahua.NVRAuxRequest) error {
	if s.nvrAux == nil {
		return nil
	}
	return s.nvrAux(ctx, deviceID, request)
}

func (s stubActionReader) ControlNVRAudio(ctx context.Context, deviceID string, request dahua.NVRAudioRequest) error {
	if s.nvrAudio == nil {
		return nil
	}
	return s.nvrAudio(ctx, deviceID, request)
}

func (s stubActionReader) ControlNVRRecording(ctx context.Context, deviceID string, request dahua.NVRRecordingRequest) error {
	if s.nvrRecording == nil {
		return nil
	}
	return s.nvrRecording(ctx, deviceID, request)
}

func (s stubActionReader) NVRDiagnosticAction(ctx context.Context, deviceID string, request dahua.NVRDiagnosticActionRequest) (dahua.NVRDiagnosticActionResult, error) {
	if s.nvrDiagnostic == nil {
		return dahua.NVRDiagnosticActionResult{}, nil
	}
	return s.nvrDiagnostic(ctx, deviceID, request)
}

func (s stubActionReader) ProbeDevice(ctx context.Context, deviceID string) (*dahua.ProbeResult, error) {
	if s.probe == nil {
		return nil, nil
	}
	return s.probe(ctx, deviceID)
}

func (s stubActionReader) ProbeAllDevices(ctx context.Context) []dahua.ProbeActionResult {
	if s.probeAll == nil {
		return nil
	}
	return s.probeAll(ctx)
}

func (s stubActionReader) RotateDeviceCredentials(ctx context.Context, deviceID string, update dahua.DeviceConfigUpdate) (*dahua.ProbeResult, error) {
	if s.rotate == nil {
		return nil, nil
	}
	return s.rotate(ctx, deviceID, update)
}

func (s stubActionReader) RefreshNVRInventory(ctx context.Context, deviceID string) (*dahua.ProbeResult, error) {
	if s.refreshNVR == nil {
		return nil, nil
	}
	return s.refreshNVR(ctx, deviceID)
}

func (s stubEventReader) ListEvents(deviceID string, childID string, deviceKind dahua.DeviceKind, code string, action string, limit int) []dahua.Event {
	if s.list == nil {
		return nil
	}
	return s.list(deviceID, childID, deviceKind, code, action, limit)
}

func (s stubEventReader) EventStats() map[string]any {
	if s.stats == nil {
		return map[string]any{"capacity": 0, "count": 0}
	}
	return s.stats()
}

func (s stubEventReader) ClearEvents() int {
	if s.clear == nil {
		return 0
	}
	return s.clear()
}

func newTestServer(actions ActionReader, events EventReader) *Server {
	return newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, nil, actions, events)
}

func newTestServerWithConfig(cfg config.HTTPConfig, snapshots SnapshotReader, media MediaReader, actions ActionReader, events EventReader) *Server {
	return newTestServerWithReaders(cfg, stubProbeReader{}, snapshots, media, actions, events)
}

func newTestServerWithReaders(cfg config.HTTPConfig, probes ProbeReader, snapshots SnapshotReader, media MediaReader, actions ActionReader, events EventReader) *Server {
	return New(
		cfg,
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		probes,
		snapshots,
		media,
		actions,
		events,
	)
}

func TestUnlockVTOLockEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		unlock: func(_ context.Context, deviceID string, lockIndex int) error {
			if deviceID != "front_vto" {
				t.Fatalf("unexpected device id: %q", deviceID)
			}
			if lockIndex != 0 {
				t.Fatalf("unexpected lock index: %d", lockIndex)
			}
			return nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/locks/0/unlock", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected status payload: %+v", payload)
	}
	if payload["device_id"] != "front_vto" {
		t.Fatalf("unexpected device_id payload: %+v", payload)
	}
	if payload["lock_index"] != float64(0) {
		t.Fatalf("unexpected lock_index payload: %+v", payload)
	}
}

func TestAPICORSPrefightRequest(t *testing.T) {
	server := newTestServer(stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/nvr/west20_nvr/channels/11/ptz", nil)
	req.Header.Set("Origin", "http://homeassistant.local:8123")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("unexpected allow origin header %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, DELETE, OPTIONS" {
		t.Fatalf("unexpected allow methods header %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "content-type" {
		t.Fatalf("unexpected allow headers header %q", got)
	}
}

func TestAPIResponsesIncludeCORSHeaders(t *testing.T) {
	server := newTestServer(stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Origin", "http://homeassistant.local:8123")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("unexpected allow origin header %q", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "Content-Length, Content-Type" {
		t.Fatalf("unexpected expose headers header %q", got)
	}
}

func TestUnlockVTOLockEndpointRejectsInvalidIndex(t *testing.T) {
	server := newTestServer(stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/locks/not-a-number/unlock", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnlockVTOLockEndpointReturnsBadGatewayForDeviceError(t *testing.T) {
	server := newTestServer(stubActionReader{
		unlock: func(_ context.Context, _ string, _ int) error {
			return errors.New("rpc failure")
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/locks/0/unlock", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload apiErrorPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ErrorCode != "device_failure" {
		t.Fatalf("unexpected error code %+v", payload)
	}
}

func TestUnlockVTOLockEndpointDetectsTypedNotFound(t *testing.T) {
	server := newTestServer(stubActionReader{
		unlock: func(_ context.Context, _ string, _ int) error {
			return fmt.Errorf("lookup failed: %w", dahua.ErrDeviceNotFound)
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/locks/0/unlock", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHangupVTOCallEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		hangup: func(_ context.Context, deviceID string) error {
			if deviceID != "front_vto" {
				t.Fatalf("unexpected device id: %q", deviceID)
			}
			return nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/call/hangup", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"action\":\"hangup_call\"") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestAnswerVTOCallEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		answer: func(_ context.Context, deviceID string) error {
			if deviceID != "front_vto" {
				t.Fatalf("unexpected device id: %q", deviceID)
			}
			return nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/call/answer", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"action\":\"answer_call\"") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestVTOControlsEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		vtoControls: func(_ context.Context, deviceID string) (dahua.VTOControlCapabilities, error) {
			if deviceID != "front_vto" {
				t.Fatalf("unexpected device id: %q", deviceID)
			}
			return dahua.VTOControlCapabilities{
				DeviceID: "front_vto",
				Call: dahua.VTOCallCapabilities{
					Answer: true,
					Hangup: true,
					State:  "Idle",
				},
				Locks: dahua.VTOLockCapabilities{
					Supported: true,
					Count:     1,
					Indexes:   []int{0},
				},
				Audio: dahua.VTOAudioCapabilities{
					OutputVolume: false,
					InputVolume:  false,
					Mute:         false,
					Codec:        "PCM",
				},
				Recording: dahua.VTORecordingCapabilities{
					Supported:          false,
					EventSnapshotLocal: false,
				},
				DirectTalkbackSupported:     false,
				FullCallAcceptanceSupported: false,
				ValidationNotes:             []string{"vto_audio_control_surface_not_exposed"},
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vto/front_vto/controls", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"device_id":"front_vto"`) ||
		!strings.Contains(rec.Body.String(), `"state":"Idle"`) ||
		!strings.Contains(rec.Body.String(), `"direct_talkback_supported":false`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestVTOAudioOutputVolumeEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		vtoOutputVol: func(_ context.Context, deviceID string, slot int, level int) error {
			if deviceID != "front_vto" || slot != 1 || level != 75 {
				t.Fatalf("unexpected request %q %d %d", deviceID, slot, level)
			}
			return nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/audio/output-volume", strings.NewReader(`{"slot":1,"level":75}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"target":"output_volume"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestVTOMuteEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		vtoMute: func(_ context.Context, deviceID string, muted bool) error {
			if deviceID != "front_vto" || !muted {
				t.Fatalf("unexpected request %q muted=%v", deviceID, muted)
			}
			return nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/audio/mute", strings.NewReader(`{"muted":true}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"muted":true`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestVTORecordingEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		vtoRecord: func(_ context.Context, deviceID string, enabled bool) error {
			if deviceID != "front_vto" || !enabled {
				t.Fatalf("unexpected request %q enabled=%v", deviceID, enabled)
			}
			return nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/recording", strings.NewReader(`{"auto_record_enabled":true}`))
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"auto_record_enabled":true`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestAnswerVTOCallEndpointDetectsTypedNotFound(t *testing.T) {
	server := newTestServer(stubActionReader{
		answer: func(_ context.Context, _ string) error {
			return fmt.Errorf("lookup failed: %w", dahua.ErrDeviceNotFound)
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/call/answer", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHangupVTOCallEndpointDetectsTypedNotFound(t *testing.T) {
	server := newTestServer(stubActionReader{
		hangup: func(_ context.Context, _ string) error {
			return fmt.Errorf("lookup failed: %w", dahua.ErrDeviceNotFound)
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/call/hangup", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProbeDeviceEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		probe: func(_ context.Context, deviceID string) (*dahua.ProbeResult, error) {
			if deviceID != "front_vto" {
				t.Fatalf("unexpected device id: %q", deviceID)
			}
			return &dahua.ProbeResult{
				Root: dahua.Device{
					ID:   "front_vto",
					Name: "Front Door",
					Kind: dahua.DeviceKindVTO,
				},
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/front_vto/probe", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := payload["result"]; !ok {
		t.Fatalf("missing result payload: %s", rec.Body.String())
	}
}

func TestProbeAllDevicesEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		probeAll: func(_ context.Context) []dahua.ProbeActionResult {
			return []dahua.ProbeActionResult{
				{
					DeviceID:   "front_vto",
					DeviceKind: dahua.DeviceKindVTO,
					Result: &dahua.ProbeResult{
						Root: dahua.Device{
							ID:   "front_vto",
							Name: "Front Door",
							Kind: dahua.DeviceKindVTO,
						},
					},
				},
				{
					DeviceID:   "yard_ipc",
					DeviceKind: dahua.DeviceKindIPC,
					Result: &dahua.ProbeResult{
						Root: dahua.Device{
							ID:   "yard_ipc",
							Name: "Yard Camera",
							Kind: dahua.DeviceKindIPC,
						},
					},
				},
			}
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/probe-all", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload["success_count"] != float64(2) {
		t.Fatalf("unexpected success_count: %+v", payload)
	}
	if payload["error_count"] != float64(0) {
		t.Fatalf("unexpected error_count: %+v", payload)
	}
}

func TestProbeAllDevicesEndpointPartialFailure(t *testing.T) {
	server := newTestServer(stubActionReader{
		probeAll: func(_ context.Context) []dahua.ProbeActionResult {
			return []dahua.ProbeActionResult{
				{
					DeviceID:   "front_vto",
					DeviceKind: dahua.DeviceKindVTO,
					Result: &dahua.ProbeResult{
						Root: dahua.Device{
							ID:   "front_vto",
							Name: "Front Door",
							Kind: dahua.DeviceKindVTO,
						},
					},
				},
				{
					DeviceID:   "west20_nvr",
					DeviceKind: dahua.DeviceKindNVR,
					Error:      "probe failed",
				},
			}
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/probe-all", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("expected status 207, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "partial_error" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload["success_count"] != float64(1) {
		t.Fatalf("unexpected success_count: %+v", payload)
	}
	if payload["error_count"] != float64(1) {
		t.Fatalf("unexpected error_count: %+v", payload)
	}
}

func TestProbeDeviceEndpointDetectsTypedNotFound(t *testing.T) {
	server := newTestServer(stubActionReader{
		probe: func(_ context.Context, _ string) (*dahua.ProbeResult, error) {
			return nil, fmt.Errorf("probe lookup: %w", dahua.ErrDeviceNotFound)
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/front_vto/probe", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRotateDeviceCredentialsEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		rotate: func(_ context.Context, deviceID string, update dahua.DeviceConfigUpdate) (*dahua.ProbeResult, error) {
			if deviceID != "yard_ipc" {
				t.Fatalf("unexpected device id: %q", deviceID)
			}
			if update.Username == nil || *update.Username != "service" {
				t.Fatalf("unexpected username update: %+v", update)
			}
			if update.Password == nil || *update.Password != "new-secret" {
				t.Fatalf("unexpected password update: %+v", update)
			}
			if update.OnvifEnabled == nil || !*update.OnvifEnabled {
				t.Fatalf("unexpected onvif flag: %+v", update)
			}
			return &dahua.ProbeResult{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard Camera",
					Kind: dahua.DeviceKindIPC,
				},
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/yard_ipc/credentials", strings.NewReader(`{"username":"service","password":"new-secret","onvif_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := payload["result"]; !ok {
		t.Fatalf("missing result payload: %s", rec.Body.String())
	}
}

func TestRotateDeviceCredentialsEndpointRejectsInvalidJSON(t *testing.T) {
	server := newTestServer(stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/yard_ipc/credentials", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRotateDeviceCredentialsEndpointDetectsTypedNotFound(t *testing.T) {
	server := newTestServer(stubActionReader{
		rotate: func(_ context.Context, _ string, _ dahua.DeviceConfigUpdate) (*dahua.ProbeResult, error) {
			return nil, fmt.Errorf("rotate lookup: %w", dahua.ErrDeviceNotFound)
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/missing_ipc/credentials", strings.NewReader(`{"password":"new-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshNVRInventoryEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		refreshNVR: func(_ context.Context, deviceID string) (*dahua.ProbeResult, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id: %q", deviceID)
			}
			return &dahua.ProbeResult{
				Root: dahua.Device{
					ID:   "west20_nvr",
					Name: "West 20 NVR",
					Kind: dahua.DeviceKindNVR,
				},
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/west20_nvr/inventory/refresh", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := payload["result"]; !ok {
		t.Fatalf("missing result payload: %s", rec.Body.String())
	}
}

func TestRefreshNVRInventoryEndpointDetectsTypedNotFound(t *testing.T) {
	server := newTestServer(stubActionReader{
		refreshNVR: func(_ context.Context, _ string) (*dahua.ProbeResult, error) {
			return nil, fmt.Errorf("refresh lookup: %w", dahua.ErrDeviceNotFound)
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/missing_nvr/inventory/refresh", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEventsEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{}, stubEventReader{
		list: func(deviceID string, childID string, deviceKind dahua.DeviceKind, code string, action string, limit int) []dahua.Event {
			if deviceID != "front_vto" {
				t.Fatalf("unexpected device id: %q", deviceID)
			}
			if childID != "" {
				t.Fatalf("unexpected child id: %q", childID)
			}
			if deviceKind != dahua.DeviceKindVTO {
				t.Fatalf("unexpected device kind: %q", deviceKind)
			}
			if code != "DoorBell" {
				t.Fatalf("unexpected code filter: %q", code)
			}
			if action != "start" {
				t.Fatalf("unexpected action filter: %q", action)
			}
			if limit != 1 {
				t.Fatalf("unexpected limit: %d", limit)
			}
			return []dahua.Event{
				{
					DeviceID:   "front_vto",
					DeviceKind: dahua.DeviceKindVTO,
					Code:       "DoorBell",
				},
			}
		},
		stats: func() map[string]any {
			return map[string]any{"capacity": 512, "count": 1}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?device_id=front_vto&device_kind=vto&code=DoorBell&action=start&limit=1", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := payload["events"]; !ok {
		t.Fatalf("missing events payload: %s", rec.Body.String())
	}
	if _, ok := payload["stats"]; !ok {
		t.Fatalf("missing stats payload: %s", rec.Body.String())
	}
}

func TestEventsEndpointRejectsInvalidLimit(t *testing.T) {
	server := newTestServer(stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=-1", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestClearEventsEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{}, stubEventReader{
		clear: func() int {
			return 3
		},
		stats: func() map[string]any {
			return map[string]any{"capacity": 512, "count": 0}
		},
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload["removed_count"] != float64(3) {
		t.Fatalf("unexpected removed_count: %+v", payload)
	}
}

func TestProbeDeviceEndpointRateLimited(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress:           ":0",
		MetricsPath:             "/metrics",
		HealthPath:              "/healthz",
		AdminRateLimitPerMinute: 1,
		AdminRateLimitBurst:     1,
	}, stubSnapshotReader{}, nil, stubActionReader{
		probe: func(_ context.Context, _ string) (*dahua.ProbeResult, error) {
			return &dahua.ProbeResult{Root: dahua.Device{ID: "front_vto"}}, nil
		},
	}, stubEventReader{})

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/devices/front_vto/probe", nil)
	req1.RemoteAddr = "192.168.1.10:1234"
	rec1 := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/devices/front_vto/probe", nil)
	req2.RemoteAddr = "192.168.1.10:1234"
	rec2 := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec2, req2)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d: %s", rec2.Code, rec2.Body.String())
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestServerClampsWriteTimeoutForLongAdminActions(t *testing.T) {
	server := New(
		config.HTTPConfig{
			ListenAddress: ":0",
			HealthPath:    "/healthz",
			MetricsPath:   "/metrics",
			WriteTimeout:  10 * time.Second,
		},
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		stubProbeReader{},
		stubSnapshotReader{},
		nil,
		stubActionReader{},
		nil,
	)

	if server.httpServer.WriteTimeout != 60*time.Second {
		t.Fatalf("unexpected write timeout %s", server.httpServer.WriteTimeout)
	}
}

func TestIPCSnapshotEndpointRateLimited(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress:              ":0",
		MetricsPath:                "/metrics",
		HealthPath:                 "/healthz",
		SnapshotRateLimitPerMinute: 1,
		SnapshotRateLimitBurst:     1,
	}, stubSnapshotReader{}, nil, stubActionReader{}, stubEventReader{})

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/ipc/yard_ipc/snapshot", nil)
	req1.RemoteAddr = "192.168.1.20:2222"
	rec1 := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/ipc/yard_ipc/snapshot", nil)
	req2.RemoteAddr = "192.168.1.20:2222"
	rec2 := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec2, req2)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestMediaMjpegEndpointRateLimited(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress:           ":0",
		MetricsPath:             "/metrics",
		HealthPath:              "/healthz",
		MediaRateLimitPerMinute: 1,
		MediaRateLimitBurst:     1,
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
	}, stubActionReader{}, stubEventReader{})

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/media/mjpeg/front_vto?profile=stable", nil)
	req1.RemoteAddr = "192.168.1.30:3333"
	rec1 := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/media/mjpeg/front_vto?profile=stable", nil)
	req2.RemoteAddr = "192.168.1.30:3333"
	rec2 := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec2, req2)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestMediaMjpegEndpointPassesRequestedWidth(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
		subscribeScaled: func(_ context.Context, streamID string, profile string, scaleWidth int) (<-chan []byte, func(), error) {
			if streamID != "front_vto" {
				t.Fatalf("unexpected stream id: %q", streamID)
			}
			if profile != "stable" {
				t.Fatalf("unexpected profile: %q", profile)
			}
			if scaleWidth != 498 {
				t.Fatalf("unexpected scale width: %d", scaleWidth)
			}
			ch := make(chan []byte, 1)
			ch <- []byte{0xFF, 0xD8, 0xFF, 0xD9}
			close(ch)
			return ch, func() {}, nil
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/mjpeg/front_vto?profile=stable&width=498&height=0", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMediaHLSPlaylistEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
		hlsPlaylist: func(_ context.Context, streamID string, profile string) ([]byte, error) {
			if streamID != "front_vto" {
				t.Fatalf("unexpected stream id: %q", streamID)
			}
			if profile != "stable" {
				t.Fatalf("unexpected profile: %q", profile)
			}
			return []byte("#EXTM3U\n#EXTINF:2.0,\nsegment_000.ts\n"), nil
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/hls/front_vto/stable/index.m3u8", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/vnd.apple.mpegurl" {
		t.Fatalf("unexpected content type %q", rec.Header().Get("Content-Type"))
	}
}

func TestMediaPreviewEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			if includeCredentials {
				t.Fatal("preview page should not request credentialed stream inventory")
			}
			return []streams.Entry{
				{
					ID:                 "front_vto",
					Name:               "Front Door",
					DeviceKind:         dahua.DeviceKindVTO,
					RecommendedProfile: "stable",
					SnapshotURL:        "/api/v1/vto/front_vto/snapshot",
					AudioCodec:         "PCM",
					MainCodec:          "H.264",
					MainResolution:     "1280x720",
					Profiles: map[string]streams.Profile{
						"stable": {
							Name:          "stable",
							LocalHLSURL:   "/api/v1/media/hls/front_vto/stable/index.m3u8",
							LocalMJPEGURL: "/api/v1/media/mjpeg/front_vto?profile=stable",
						},
						"quality": {
							Name:          "quality",
							LocalHLSURL:   "/api/v1/media/hls/front_vto/quality/index.m3u8",
							LocalMJPEGURL: "/api/v1/media/mjpeg/front_vto?profile=quality",
						},
					},
				},
			}
		},
	}, stubMediaReader{
		enabled: true,
		iceServers: func() []mediaapi.WebRTCICEServer {
			return []mediaapi.WebRTCICEServer{{
				URLs:       []string{"stun:stun.example.net:3478"},
				Username:   "user",
				Credential: "secret",
			}}
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/preview/front_vto?profile=stable", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content type %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Front Door Preview") {
		t.Fatalf("missing preview title:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/media/hls/front_vto/stable/index.m3u8") {
		t.Fatalf("missing hls url:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/media/mjpeg/front_vto?profile=stable") {
		t.Fatalf("missing mjpeg url:\n%s", body)
	}
	if !strings.Contains(body, `/api/v1/media/preview/front_vto?profile=quality`) {
		t.Fatalf("missing profile switch link:\n%s", body)
	}
}

func TestAdminPageEndpoint(t *testing.T) {
	now := time.Date(2026, 4, 27, 10, 15, 0, 0, time.UTC)
	server := newTestServerWithReaders(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubProbeReader{
		list: func() []*dahua.ProbeResult {
			return []*dahua.ProbeResult{{
				Root: dahua.Device{
					ID:    "front_vto",
					Name:  "Front Door",
					Kind:  dahua.DeviceKindVTO,
					Model: "VTO2202F-P",
				},
			}}
		},
		stats: func() store.Stats {
			return store.Stats{
				DeviceCount:   1,
				LastUpdatedAt: now,
			}
		},
	}, stubSnapshotReader{
		adminSettings: func() map[string]any {
			return map[string]any{
				"mqtt": map[string]any{
					"broker":   "tcp://mqtt.local:1883",
					"password": "[redacted]",
				},
				"home_assistant": map[string]any{
					"access_token": "[redacted]",
				},
				"devices": map[string]any{
					"vto": []map[string]any{{
						"id":       "front_vto",
						"base_url": "http://192.168.1.30",
						"password": "[redacted]",
					}},
				},
			}
		},
		listStreams: func(includeCredentials bool) []streams.Entry {
			if includeCredentials {
				t.Fatal("admin page should not request credentialed stream inventory")
			}
			return []streams.Entry{{
				ID:                 "front_vto",
				Name:               "Front Door",
				RootDeviceID:       "front_vto",
				DeviceKind:         dahua.DeviceKindVTO,
				SnapshotURL:        "/api/v1/vto/front_vto/snapshot",
				LocalPreviewURL:    "/api/v1/media/preview/front_vto?profile=stable",
				LocalIntercomURL:   "/api/v1/vto/front_vto/intercom?profile=stable",
				RecommendedProfile: "stable",
				MainCodec:          "H.264",
				MainResolution:     "1280x720",
				AudioCodec:         "PCM",
				Intercom: &streams.IntercomSummary{
					AnswerURL:                      "/api/v1/vto/front_vto/call/answer",
					HangupURL:                      "/api/v1/vto/front_vto/call/hangup",
					BridgeSessionResetURL:          "/api/v1/vto/front_vto/intercom/reset",
					LockURLs:                       []string{"/api/v1/vto/front_vto/locks/0/unlock"},
					OutputVolumeURL:                "/api/v1/vto/front_vto/audio/output-volume",
					InputVolumeURL:                 "/api/v1/vto/front_vto/audio/input-volume",
					MuteURL:                        "/api/v1/vto/front_vto/audio/mute",
					RecordingURL:                   "/api/v1/vto/front_vto/recording",
					SupportsVTOCallAnswer:          true,
					SupportsHangup:                 true,
					SupportsUnlock:                 true,
					SupportsVTOOutputVolumeControl: true,
					SupportsVTOInputVolumeControl:  true,
					SupportsVTOMuteControl:         true,
					SupportsVTORecordingControl:    true,
					ValidationNotes: []string{
						"AudioInputVolume writable",
						"AudioOutputVolume writable",
					},
				},
				Profiles: map[string]streams.Profile{
					"stable": {
						LocalWebRTCURL: "/api/v1/media/webrtc/front_vto/stable",
						LocalHLSURL:    "/api/v1/media/hls/front_vto/stable/index.m3u8",
						LocalMJPEGURL:  "/api/v1/media/mjpeg/front_vto?profile=stable",
					},
				},
			}}
		},
	}, stubMediaReader{
		enabled: true,
		listWorker: func() []mediaapi.WorkerStatus {
			return []mediaapi.WorkerStatus{{
				Key:      "front_vto:stable:webrtc",
				Format:   "webrtc",
				StreamID: "front_vto",
			}}
		},
	}, stubActionReader{}, stubEventReader{
		stats: func() map[string]any {
			return map[string]any{"count": 2, "capacity": 128}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content type %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "DahuaBridge Admin") || !strings.Contains(body, "Operator Surface") {
		t.Fatalf("missing admin page heading:\n%s", body)
	}
	if !strings.Contains(body, `/api/v1/devices/probe-all`) || !strings.Contains(body, "Clear Event Buffer") {
		t.Fatalf("missing admin action controls:\n%s", body)
	}
	if !strings.Contains(body, `/admin/test-bridge`) {
		t.Fatalf("missing bridge test page link:\n%s", body)
	}
	if !strings.Contains(body, `/api/v1/vto/front_vto/intercom?profile=stable`) || !strings.Contains(body, `/api/v1/media/webrtc/front_vto/stable`) {
		t.Fatalf("missing concrete stream links:\n%s", body)
	}
	if !strings.Contains(body, `/api/v1/vto/front_vto/controls`) || !strings.Contains(body, `/api/v1/vto/front_vto/audio/output-volume`) || !strings.Contains(body, `validated: AudioInputVolume writable | AudioOutputVolume writable`) {
		t.Fatalf("missing vto control detail:\n%s", body)
	}
	if !strings.Contains(body, `/admin/assets/logo.png`) || !strings.Contains(body, `/admin/assets/bootstrap.min.css`) || !strings.Contains(body, `data-bs-theme="dark"`) {
		t.Fatalf("missing embedded admin assets or dark theme marker:\n%s", body)
	}
	if !strings.Contains(body, `[redacted]`) {
		t.Fatalf("missing redacted settings marker:\n%s", body)
	}
	if strings.Contains(body, `"password":"secret"`) || strings.Contains(body, `"access_token":"token"`) {
		t.Fatalf("unexpected secret disclosure:\n%s", body)
	}
}

func TestAdminTestBridgePageEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			if includeCredentials {
				t.Fatal("test bridge page should not request credentialed stream inventory")
			}
			return []streams.Entry{{
				ID:                 "west20_nvr_channel_05",
				Name:               "Driveway",
				RootDeviceID:       "west20_nvr",
				DeviceKind:         dahua.DeviceKindNVRChannel,
				Channel:            5,
				SnapshotURL:        "/api/v1/nvr/west20_nvr/channels/5/snapshot",
				RecommendedProfile: "stable",
				MainCodec:          "H.265",
				MainResolution:     "2560x1440",
				SubCodec:           "H.264",
				SubResolution:      "704x576",
				Controls: &streams.ChannelControlSummary{
					Aux: &streams.AuxControlSummary{
						Supported: true,
						Outputs:   []string{"aux", "light"},
						Features:  []string{"siren", "warning_light"},
					},
				},
				Profiles: map[string]streams.Profile{
					"stable": {
						Name:         "stable",
						SourceWidth:  704,
						SourceHeight: 576,
						Recommended:  true,
					},
					"quality": {
						Name:         "quality",
						SourceWidth:  2560,
						SourceHeight: 1440,
					},
				},
			}}
		},
	}, stubMediaReader{enabled: true}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/admin/test-bridge", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Bridge Test Bench") || !strings.Contains(body, "west20_nvr_channel_05") {
		t.Fatalf("missing test bridge page content:\n%s", body)
	}
	if !strings.Contains(body, `/api/v1/nvr/west20_nvr/channels/5/snapshot`) || !strings.Contains(body, `/api/v1/media/webrtc/west20_nvr_channel_05/stable`) {
		t.Fatalf("missing local stream URLs:\n%s", body)
	}
	if !strings.Contains(body, `data-method="direct_ipc_lighting"`) || !strings.Contains(body, `data-method="nvr_lighting_config"`) || !strings.Contains(body, `data-method="direct_ipc_ptz_light_ch0"`) {
		t.Fatalf("missing diagnostic control buttons:\n%s", body)
	}
}

func TestAdminAssetEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, stubMediaReader{}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/admin/assets/bootstrap.min.css", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("unexpected content type %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "--bs-body-font-family") {
		t.Fatalf("expected bootstrap stylesheet body")
	}
}

func TestStreamsEndpointIncludesVTOIntercomSummary(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			if includeCredentials {
				t.Fatal("streams endpoint should not request credentialed inventory by default")
			}
			return []streams.Entry{
				{
					ID:                 "front_vto",
					Name:               "Front Door",
					DeviceKind:         dahua.DeviceKindVTO,
					LocalIntercomURL:   "/api/v1/vto/front_vto/intercom?profile=stable",
					RecommendedProfile: "stable",
					SnapshotURL:        "/api/v1/vto/front_vto/snapshot",
					Intercom: &streams.IntercomSummary{
						CallState:                           "ringing",
						LastCallSource:                      "villa_panel",
						LastCallDurationSeconds:             11,
						AnswerURL:                           "/api/v1/vto/front_vto/call/answer",
						HangupURL:                           "/api/v1/vto/front_vto/call/hangup",
						BridgeSessionResetURL:               "/api/v1/vto/front_vto/intercom/reset",
						LockURLs:                            []string{"/api/v1/vto/front_vto/locks/0/unlock"},
						SupportsVTOCallAnswer:               true,
						SupportsHangup:                      true,
						SupportsBridgeSessionReset:          true,
						SupportsUnlock:                      true,
						SupportsBrowserMicrophone:           true,
						SupportsBridgeAudioUplink:           true,
						SupportsBridgeAudioOutput:           true,
						SupportsExternalAudioExport:         true,
						ConfiguredExternalUplinkTargetCount: 1,
						BridgeSessionActive:                 true,
						BridgeSessionCount:                  2,
						ExternalUplinkEnabled:               true,
						BridgeUplinkActive:                  true,
						BridgeUplinkCodec:                   "audio/opus",
						BridgeForwardedPackets:              30,
						SupportsVTOTalkback:                 false,
						SupportsFullCallAcceptance:          false,
					},
					Profiles: map[string]streams.Profile{
						"stable": {
							Name:           "stable",
							LocalWebRTCURL: "/api/v1/media/webrtc/front_vto/stable",
						},
					},
				},
			}
		},
	}, stubMediaReader{enabled: true}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/streams", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"local_intercom_url":"/api/v1/vto/front_vto/intercom?profile=stable"`) {
		t.Fatalf("missing intercom page url:\n%s", body)
	}
	if !strings.Contains(body, `"intercom":{"call_state":"ringing"`) {
		t.Fatalf("missing intercom summary:\n%s", body)
	}
	if !strings.Contains(body, `"answer_url":"/api/v1/vto/front_vto/call/answer"`) {
		t.Fatalf("missing answer url:\n%s", body)
	}
	if !strings.Contains(body, `"hangup_url":"/api/v1/vto/front_vto/call/hangup"`) {
		t.Fatalf("missing hangup url:\n%s", body)
	}
	if !strings.Contains(body, `"bridge_session_reset_url":"/api/v1/vto/front_vto/intercom/reset"`) {
		t.Fatalf("missing bridge session reset url:\n%s", body)
	}
	if !strings.Contains(body, `"supports_vto_talkback":false`) {
		t.Fatalf("missing unsupported talkback flag:\n%s", body)
	}
	if !strings.Contains(body, `"configured_external_uplink_target_count":1`) {
		t.Fatalf("missing uplink target count:\n%s", body)
	}
	if !strings.Contains(body, `"bridge_session_count":2`) || !strings.Contains(body, `"external_uplink_enabled":true`) {
		t.Fatalf("missing bridge runtime intercom state:\n%s", body)
	}
}

func TestHomeAssistantNativeCatalogEndpoint(t *testing.T) {
	server := newTestServerWithReaders(
		config.HTTPConfig{
			ListenAddress: ":0",
			MetricsPath:   "/metrics",
			HealthPath:    "/healthz",
		},
		stubProbeReader{
			list: func() []*dahua.ProbeResult {
				return []*dahua.ProbeResult{
					{
						Root: dahua.Device{
							ID:   "west20_nvr",
							Name: "West 20 NVR",
							Kind: dahua.DeviceKindNVR,
						},
						Children: []dahua.Device{
							{
								ID:       "west20_nvr_channel_01",
								ParentID: "west20_nvr",
								Name:     "West 20 Channel 01",
								Kind:     dahua.DeviceKindNVRChannel,
							},
						},
						States: map[string]dahua.DeviceState{
							"west20_nvr_channel_01": {
								Available: true,
								Info: map[string]any{
									"motion": true,
								},
							},
						},
					},
				}
			},
			stats: func() store.Stats { return store.Stats{DeviceCount: 1} },
		},
		stubSnapshotReader{
			listStreams: func(bool) []streams.Entry {
				return []streams.Entry{
					{
						ID:                 "west20_nvr_channel_01",
						RootDeviceID:       "west20_nvr",
						SourceDeviceID:     "west20_nvr_channel_01",
						DeviceKind:         dahua.DeviceKindNVRChannel,
						Name:               "West 20 Channel 01",
						SnapshotURL:        "http://bridge.local/api/v1/nvr/west20_nvr/channels/1/snapshot",
						RecommendedProfile: "stable",
						Profiles: map[string]streams.Profile{
							"stable": {
								Name:          "stable",
								StreamURL:     "rtsp://bridge.local/stream",
								LocalMJPEGURL: "http://bridge.local/api/v1/media/mjpeg/west20_nvr_channel_01?profile=stable",
							},
						},
					},
				}
			},
		},
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home-assistant/native/catalog", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"device":{"id":"west20_nvr_channel_01"`) {
		t.Fatalf("expected channel device in native catalog:\n%s", body)
	}
	if !strings.Contains(body, `"motion":true`) {
		t.Fatalf("expected motion state in native catalog:\n%s", body)
	}
	if !strings.Contains(body, `"snapshot_url":"http://bridge.local/api/v1/nvr/west20_nvr/channels/1/snapshot"`) {
		t.Fatalf("expected snapshot url in native catalog:\n%s", body)
	}
}

func TestMediaWebRTCPageEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			return []streams.Entry{
				{
					ID:   "front_vto",
					Name: "Front Door",
					Profiles: map[string]streams.Profile{
						"stable": {
							Name:           "stable",
							LocalHLSURL:    "/api/v1/media/hls/front_vto/stable/index.m3u8",
							LocalMJPEGURL:  "/api/v1/media/mjpeg/front_vto?profile=stable",
							LocalWebRTCURL: "/api/v1/media/webrtc/front_vto/stable",
						},
					},
				},
			}
		},
	}, stubMediaReader{
		enabled: true,
		iceServers: func() []mediaapi.WebRTCICEServer {
			return []mediaapi.WebRTCICEServer{{
				URLs:       []string{"stun:stun.example.net:3478"},
				Username:   "user",
				Credential: "secret",
			}}
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/webrtc/front_vto/stable", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Bridge WebRTC") {
		t.Fatalf("missing webrtc heading:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/media/webrtc/front_vto/stable/offer") {
		t.Fatalf("missing offer endpoint reference:\n%s", body)
	}
	if !strings.Contains(body, `const iceServers = [{"urls":["stun:stun.example.net:3478"],"username":"user","credential":"secret"}]`) {
		t.Fatalf("missing configured ice servers:\n%s", body)
	}
	if !strings.Contains(body, `new RTCPeerConnection({ iceServers })`) {
		t.Fatalf("missing ice-aware peer connection:\n%s", body)
	}
	if !strings.Contains(body, "addTransceiver('audio'") {
		t.Fatalf("missing audio transceiver reference:\n%s", body)
	}
	if !strings.Contains(body, "scheduleReconnect(reason)") || !strings.Contains(body, "document.addEventListener('visibilitychange'") {
		t.Fatalf("missing webrtc auto-reconnect flow:\n%s", body)
	}
	if !strings.Contains(body, "start(true).catch(handleStartError)") {
		t.Fatalf("missing webrtc reconnect retry loop:\n%s", body)
	}
}

func TestVTOIntercomPageEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			if includeCredentials {
				t.Fatal("intercom page should not request credentialed stream inventory")
			}
			return []streams.Entry{
				{
					ID:                 "front_vto",
					Name:               "Front Door",
					DeviceKind:         dahua.DeviceKindVTO,
					LockCount:          2,
					AudioCodec:         "PCM",
					MainCodec:          "H.264",
					MainResolution:     "1280x720",
					SnapshotURL:        "/api/v1/vto/front_vto/snapshot",
					RecommendedProfile: "stable",
					Intercom: &streams.IntercomSummary{
						ConfiguredExternalUplinkTargetCount: 1,
						SupportsExternalAudioExport:         true,
						SupportsBridgeSessionReset:          true,
						SupportsVTOCallAnswer:               true,
						AnswerURL:                           "/api/v1/vto/front_vto/call/answer",
						BridgeSessionResetURL:               "/api/v1/vto/front_vto/intercom/reset",
						ExternalUplinkEnableURL:             "/api/v1/vto/front_vto/intercom/uplink/enable",
						ExternalUplinkDisableURL:            "/api/v1/vto/front_vto/intercom/uplink/disable",
					},
					Profiles: map[string]streams.Profile{
						"stable": {
							Name:           "stable",
							LocalHLSURL:    "/api/v1/media/hls/front_vto/stable/index.m3u8",
							LocalMJPEGURL:  "/api/v1/media/mjpeg/front_vto?profile=stable",
							LocalWebRTCURL: "/api/v1/media/webrtc/front_vto/stable",
						},
						"quality": {
							Name:           "quality",
							LocalHLSURL:    "/api/v1/media/hls/front_vto/quality/index.m3u8",
							LocalMJPEGURL:  "/api/v1/media/mjpeg/front_vto?profile=quality",
							LocalWebRTCURL: "/api/v1/media/webrtc/front_vto/quality",
						},
					},
				},
			}
		},
	}, stubMediaReader{
		enabled: true,
		iceServers: func() []mediaapi.WebRTCICEServer {
			return []mediaapi.WebRTCICEServer{{
				URLs: []string{"turn:turn.example.net:3478?transport=udp"},
			}}
		},
		intercomStatus: func(streamID string) mediaapi.IntercomStatus {
			return mediaapi.IntercomStatus{
				StreamID:               streamID,
				Active:                 true,
				SessionCount:           1,
				ExternalUplinkEnabled:  true,
				UplinkActive:           true,
				UplinkForwardedPackets: 12,
			}
		},
		setIntercomUplinkEnabled: func(streamID string, enabled bool) mediaapi.IntercomStatus {
			return mediaapi.IntercomStatus{
				StreamID:              streamID,
				ExternalUplinkEnabled: enabled,
			}
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vto/front_vto/intercom?profile=stable", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Bridge VTO Intercom") {
		t.Fatalf("missing intercom heading:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/vto/front_vto/call/answer") || !strings.Contains(body, "Answer Call") {
		t.Fatalf("missing answer action url:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/vto/front_vto/call/hangup") {
		t.Fatalf("missing hangup action url:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/vto/front_vto/intercom/reset") || !strings.Contains(body, "Reset Bridge Session") {
		t.Fatalf("missing bridge reset action:\n%s", body)
	}
	if !strings.Contains(body, `const lockURLBase = "/api/v1/vto/front_vto/locks"`) {
		t.Fatalf("missing lock action base url:\n%s", body)
	}
	if !strings.Contains(body, `data-lock-index="0"`) {
		t.Fatalf("missing first lock action button:\n%s", body)
	}
	if !strings.Contains(body, "Enable Microphone") {
		t.Fatalf("missing microphone action button:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/vto/front_vto/intercom/status") {
		t.Fatalf("missing intercom status url:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/vto/front_vto/intercom/uplink/enable") || !strings.Contains(body, "/api/v1/vto/front_vto/intercom/uplink/disable") {
		t.Fatalf("missing uplink toggle urls:\n%s", body)
	}
	if !strings.Contains(body, "Bridge Session") || !strings.Contains(body, "Forwarded RTP Packets") {
		t.Fatalf("missing bridge-side intercom status labels:\n%s", body)
	}
	if !strings.Contains(body, "Disable RTP Export") {
		t.Fatalf("missing external uplink control button:\n%s", body)
	}
	if !strings.Contains(body, "External RTP Targets") || !strings.Contains(body, "1 configured") {
		t.Fatalf("missing external uplink target summary:\n%s", body)
	}
	if !strings.Contains(body, `const iceServers = [{"urls":["turn:turn.example.net:3478?transport=udp"]}]`) {
		t.Fatalf("missing intercom ice servers:\n%s", body)
	}
	if !strings.Contains(body, `new RTCPeerConnection({ iceServers })`) {
		t.Fatalf("missing ice-aware intercom peer connection:\n%s", body)
	}
	if !strings.Contains(body, "getUserMedia({ audio: true })") {
		t.Fatalf("missing browser microphone uplink flow:\n%s", body)
	}
	if !strings.Contains(body, "/api/v1/devices/front_vto") {
		t.Fatalf("missing state polling url:\n%s", body)
	}
	if !strings.Contains(body, `/api/v1/vto/front_vto/intercom?profile=quality`) {
		t.Fatalf("missing profile switch link:\n%s", body)
	}
	if !strings.Contains(body, "addTransceiver('audio'") {
		t.Fatalf("missing audio transceiver:\n%s", body)
	}
	if !strings.Contains(body, "scheduleReconnect(reason)") || !strings.Contains(body, "connectMedia(micEnabled, true).catch(handleMediaError)") {
		t.Fatalf("missing intercom auto-reconnect flow:\n%s", body)
	}
	if !strings.Contains(body, "document.addEventListener('visibilitychange'") {
		t.Fatalf("missing intercom visibility recovery:\n%s", body)
	}
}

func TestVTOIntercomStatusEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			return []streams.Entry{{
				ID:         "front_vto",
				Name:       "Front Door",
				DeviceKind: dahua.DeviceKindVTO,
				Profiles: map[string]streams.Profile{
					"stable": {Name: "stable"},
				},
			}}
		},
	}, stubMediaReader{
		enabled: true,
		intercomStatus: func(streamID string) mediaapi.IntercomStatus {
			return mediaapi.IntercomStatus{
				StreamID:               streamID,
				Active:                 true,
				SessionCount:           2,
				Profiles:               []string{"quality", "stable"},
				ExternalUplinkEnabled:  true,
				UplinkActive:           true,
				UplinkCodec:            "audio/opus",
				UplinkPackets:          44,
				UplinkTargetCount:      1,
				UplinkForwardedPackets: 40,
				UplinkForwardErrors:    1,
			}
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vto/front_vto/intercom/status", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"stream_id":"front_vto"`) {
		t.Fatalf("missing stream id:\n%s", body)
	}
	if !strings.Contains(body, `"session_count":2`) {
		t.Fatalf("missing session count:\n%s", body)
	}
	if !strings.Contains(body, `"uplink_forwarded_packets":40`) {
		t.Fatalf("missing forwarded packet count:\n%s", body)
	}
	if !strings.Contains(body, `"external_uplink_enabled":true`) {
		t.Fatalf("missing external uplink enabled flag:\n%s", body)
	}
}

func TestVTOIntercomResetEndpoint(t *testing.T) {
	var requestedStreamID string
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			return []streams.Entry{{
				ID:         "front_vto",
				Name:       "Front Door",
				DeviceKind: dahua.DeviceKindVTO,
				Profiles: map[string]streams.Profile{
					"stable": {Name: "stable"},
				},
			}}
		},
	}, stubMediaReader{
		enabled: true,
		stopIntercomSessions: func(streamID string) mediaapi.IntercomStatus {
			requestedStreamID = streamID
			return mediaapi.IntercomStatus{StreamID: streamID}
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/intercom/reset", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if requestedStreamID != "front_vto" {
		t.Fatalf("unexpected stream id %q", requestedStreamID)
	}
	if !strings.Contains(rec.Body.String(), `"stream_id":"front_vto"`) {
		t.Fatalf("unexpected response body %s", rec.Body.String())
	}
}

func TestVTOIntercomUplinkToggleEndpoints(t *testing.T) {
	var toggles []bool
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			return []streams.Entry{{
				ID:         "front_vto",
				Name:       "Front Door",
				DeviceKind: dahua.DeviceKindVTO,
				Intercom: &streams.IntercomSummary{
					SupportsExternalAudioExport: true,
				},
				Profiles: map[string]streams.Profile{
					"stable": {Name: "stable"},
				},
			}}
		},
	}, stubMediaReader{
		enabled: true,
		setIntercomUplinkEnabled: func(streamID string, enabled bool) mediaapi.IntercomStatus {
			if streamID != "front_vto" {
				t.Fatalf("unexpected stream id %q", streamID)
			}
			toggles = append(toggles, enabled)
			return mediaapi.IntercomStatus{
				StreamID:              streamID,
				ExternalUplinkEnabled: enabled,
			}
		},
	}, stubActionReader{}, stubEventReader{})

	enableReq := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/intercom/uplink/enable", nil)
	enableRec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(enableRec, enableReq)

	disableReq := httptest.NewRequest(http.MethodPost, "/api/v1/vto/front_vto/intercom/uplink/disable", nil)
	disableRec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(disableRec, disableReq)

	if enableRec.Code != http.StatusOK || disableRec.Code != http.StatusOK {
		t.Fatalf("unexpected status codes enable=%d disable=%d", enableRec.Code, disableRec.Code)
	}
	if len(toggles) != 2 || !toggles[0] || toggles[1] {
		t.Fatalf("unexpected toggle calls %+v", toggles)
	}
	if !strings.Contains(enableRec.Body.String(), `"external_uplink_enabled":true`) {
		t.Fatalf("unexpected enable response %s", enableRec.Body.String())
	}
	if !strings.Contains(disableRec.Body.String(), `"external_uplink_enabled":false`) {
		t.Fatalf("unexpected disable response %s", disableRec.Body.String())
	}
}

func TestMediaWebRTCOfferEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
		webrtcOffer: func(_ context.Context, streamID string, profile string, offer mediaapi.WebRTCSessionDescription) (mediaapi.WebRTCSessionDescription, error) {
			if streamID != "front_vto" {
				t.Fatalf("unexpected stream id: %q", streamID)
			}
			if profile != "stable" {
				t.Fatalf("unexpected profile: %q", profile)
			}
			if offer.Type != "offer" || !strings.Contains(offer.SDP, "m=video") {
				t.Fatalf("unexpected offer: %+v", offer)
			}
			return mediaapi.WebRTCSessionDescription{
				Type: "answer",
				SDP:  "v=0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
			}, nil
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/webrtc/front_vto/stable/offer", strings.NewReader(`{"type":"offer","sdp":"v=0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload mediaapi.WebRTCSessionDescription
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Type != "answer" {
		t.Fatalf("unexpected answer payload: %+v", payload)
	}
}

func TestMediaHLSSegmentEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
		hlsSegment: func(_ context.Context, streamID string, profile string, segmentName string) ([]byte, string, error) {
			if streamID != "front_vto" {
				t.Fatalf("unexpected stream id: %q", streamID)
			}
			if profile != "stable" {
				t.Fatalf("unexpected profile: %q", profile)
			}
			if segmentName != "segment_000.ts" {
				t.Fatalf("unexpected segment name: %q", segmentName)
			}
			return []byte("fake-ts"), "video/mp2t", nil
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/hls/front_vto/stable/segment_000.ts", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "video/mp2t" {
		t.Fatalf("unexpected content type %q", rec.Header().Get("Content-Type"))
	}
}

func TestMediaHLSEndpointRateLimited(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress:           ":0",
		MetricsPath:             "/metrics",
		HealthPath:              "/healthz",
		MediaRateLimitPerMinute: 1,
		MediaRateLimitBurst:     1,
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
	}, stubActionReader{}, stubEventReader{})

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/media/hls/front_vto/stable/index.m3u8", nil)
	req1.RemoteAddr = "192.168.1.40:4444"
	rec1 := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/media/hls/front_vto/stable/index.m3u8", nil)
	req2.RemoteAddr = "192.168.1.40:4444"
	rec2 := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec2, req2)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestMediaSnapshotEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
		captureFrame: func(_ context.Context, streamID string, profile string, scaleWidth int) ([]byte, string, error) {
			if streamID != "front_vto" {
				t.Fatalf("unexpected stream id %q", streamID)
			}
			if profile != "stable" {
				t.Fatalf("unexpected profile %q", profile)
			}
			if scaleWidth != 640 {
				t.Fatalf("unexpected width %d", scaleWidth)
			}
			return []byte("jpeg-bytes"), "image/jpeg", nil
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/snapshot/front_vto?profile=stable&width=640", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("unexpected content type %q", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "jpeg-bytes" {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
}

func TestMediaRecordingEndpoints(t *testing.T) {
	startedAt := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
		startClip: func(_ context.Context, request mediaapi.ClipStartRequest) (mediaapi.ClipInfo, error) {
			if request.StreamID != "west20_nvr_channel_05" {
				t.Fatalf("unexpected stream id %q", request.StreamID)
			}
			if request.ProfileName != "stable" {
				t.Fatalf("unexpected profile %q", request.ProfileName)
			}
			if request.Duration != 15*time.Second {
				t.Fatalf("unexpected duration %s", request.Duration)
			}
			return mediaapi.ClipInfo{
				ID:           "clip_live",
				StreamID:     request.StreamID,
				RootDeviceID: "west20_nvr",
				DeviceKind:   dahua.DeviceKindNVRChannel,
				Channel:      5,
				Profile:      request.ProfileName,
				Status:       mediaapi.ClipStatusRecording,
				StartedAt:    startedAt,
				Duration:     request.Duration,
				FileName:     "clip_live.mp4",
			}, nil
		},
		findClips: func(query mediaapi.ClipQuery) ([]mediaapi.ClipInfo, error) {
			if query.StreamID != "west20_nvr_channel_05" {
				t.Fatalf("unexpected clip query %+v", query)
			}
			return []mediaapi.ClipInfo{{
				ID:        "clip_live",
				StreamID:  query.StreamID,
				Status:    mediaapi.ClipStatusRecording,
				StartedAt: startedAt,
				FileName:  "clip_live.mp4",
			}}, nil
		},
		getClip: func(clipID string) (mediaapi.ClipInfo, error) {
			if clipID != "clip_live" {
				t.Fatalf("unexpected clip id %q", clipID)
			}
			return mediaapi.ClipInfo{
				ID:        clipID,
				StreamID:  "west20_nvr_channel_05",
				Status:    mediaapi.ClipStatusRecording,
				StartedAt: startedAt,
				FileName:  "clip_live.mp4",
			}, nil
		},
		stopClip: func(_ context.Context, clipID string) (mediaapi.ClipInfo, error) {
			if clipID != "clip_live" {
				t.Fatalf("unexpected clip id %q", clipID)
			}
			return mediaapi.ClipInfo{
				ID:        clipID,
				StreamID:  "west20_nvr_channel_05",
				Status:    mediaapi.ClipStatusCompleted,
				StartedAt: startedAt,
				EndedAt:   startedAt.Add(15 * time.Second),
				Duration:  15 * time.Second,
				FileName:  "clip_live.mp4",
			}, nil
		},
	}, stubActionReader{}, stubEventReader{})

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/streams/west20_nvr_channel_05/recordings", strings.NewReader(`{"profile":"stable","duration_seconds":15}`))
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("expected start status 200, got %d: %s", startRec.Code, startRec.Body.String())
	}
	if !strings.Contains(startRec.Body.String(), `"status":"recording"`) || !strings.Contains(startRec.Body.String(), `/api/v1/media/recordings/clip_live/stop`) {
		t.Fatalf("unexpected start response %s", startRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/media/recordings?stream_id=west20_nvr_channel_05", nil)
	listRec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), `"returned_count":1`) {
		t.Fatalf("unexpected list response %s", listRec.Body.String())
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/recordings/clip_live/stop", nil)
	stopRec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(stopRec, stopReq)

	if stopRec.Code != http.StatusOK {
		t.Fatalf("expected stop status 200, got %d: %s", stopRec.Code, stopRec.Body.String())
	}
	if !strings.Contains(stopRec.Body.String(), `"status":"completed"`) {
		t.Fatalf("unexpected stop response %s", stopRec.Body.String())
	}
}

func TestParseClipStartRequestUsesQueryDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/streams/west20_nvr_channel_05/recordings?profile=quality&duration_seconds=30", nil)

	got, err := parseClipStartRequest(req)
	if err != nil {
		t.Fatalf("parseClipStartRequest returned error: %v", err)
	}
	if got.ProfileName != "quality" {
		t.Fatalf("unexpected profile %q", got.ProfileName)
	}
	if got.Duration != 30*time.Second {
		t.Fatalf("unexpected duration %s", got.Duration)
	}
}

func TestParseClipStartRequestBodyOverridesQueryDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/streams/west20_nvr_channel_05/recordings?profile=quality&duration_seconds=30", strings.NewReader(`{"profile":"default","duration_ms":1500}`))
	req.Header.Set("Content-Type", "application/json")

	got, err := parseClipStartRequest(req)
	if err != nil {
		t.Fatalf("parseClipStartRequest returned error: %v", err)
	}
	if got.ProfileName != "default" {
		t.Fatalf("unexpected profile %q", got.ProfileName)
	}
	if got.Duration != 1500*time.Millisecond {
		t.Fatalf("unexpected duration %s", got.Duration)
	}
}

func TestMediaRecordingDownloadEndpoint(t *testing.T) {
	dir := t.TempDir()
	clipPath := filepath.Join(dir, "clip_test.mp4")
	if err := os.WriteFile(clipPath, []byte("mp4-bytes"), 0o644); err != nil {
		t.Fatalf("write clip file: %v", err)
	}

	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, stubMediaReader{
		enabled: true,
		clipFilePath: func(clipID string) (string, error) {
			if clipID != "clip_test" {
				t.Fatalf("unexpected clip id %q", clipID)
			}
			return clipPath, nil
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/recordings/clip_test/download", nil)
	rec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("unexpected content type %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), `filename="clip_test.mp4"`) {
		t.Fatalf("unexpected content disposition %q", rec.Header().Get("Content-Disposition"))
	}
	if rec.Body.String() != "mp4-bytes" {
		t.Fatalf("unexpected download body %q", rec.Body.String())
	}
}

func TestNVRRecordingsEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		nvrRecordings: func(_ context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if query.Channel != 1 {
				t.Fatalf("unexpected channel %d", query.Channel)
			}
			if query.Limit != 5 {
				t.Fatalf("unexpected limit %d", query.Limit)
			}
			if query.EventCode != "VideoMotion" {
				t.Fatalf("unexpected event code %q", query.EventCode)
			}
			if query.EventOnly {
				t.Fatal("did not expect event-only search")
			}
			return dahua.NVRRecordingSearchResult{
				DeviceID:      deviceID,
				Channel:       query.Channel,
				StartTime:     "2026-04-28 00:00:00",
				EndTime:       "2026-04-28 01:00:00",
				Limit:         query.Limit,
				ReturnedCount: 1,
				Items: []dahua.NVRRecording{{
					Channel:        1,
					StartTime:      "2026-04-28 00:00:00",
					EndTime:        "2026-04-28 00:30:00",
					Type:           "dav",
					VideoStream:    "Main",
					LengthBytes:    1234,
					CutLengthBytes: 1234,
				}},
			}, nil
		},
	}, nil, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/west20_nvr/recordings?channel=1&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=5&event=VideoMotion", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"device_id":"west20_nvr"`) {
		t.Fatalf("expected device id in response: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"returned_count":1`) {
		t.Fatalf("expected returned_count in response: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"source":"nvr"`) ||
		!strings.Contains(rec.Body.String(), `"export_url":"http://example.com/api/v1/nvr/west20_nvr/recordings/export?`) {
		t.Fatalf("expected native recording export URL in response: %s", rec.Body.String())
	}
}

func TestNVRRecordingsEndpointParsesEventOnlyQuery(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		nvrRecordings: func(_ context.Context, _ string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			if !query.EventOnly {
				t.Fatal("expected event-only search")
			}
			if query.EventCode != "all" {
				t.Fatalf("unexpected event code %q", query.EventCode)
			}
			return dahua.NVRRecordingSearchResult{
				DeviceID:      "west20_nvr",
				Channel:       query.Channel,
				StartTime:     "2026-04-28 00:00:00",
				EndTime:       "2026-04-28 01:00:00",
				Limit:         query.Limit,
				ReturnedCount: 1,
				Items: []dahua.NVRRecording{{
					Source:    "nvr_event",
					Channel:   query.Channel,
					StartTime: "2026-04-28 00:10:00",
					EndTime:   "2026-04-28 00:10:20",
					Type:      "Event.smdTypeHuman",
					Flags:     []string{"Event", "smdTypeHuman"},
				}},
			}, nil
		},
	}, nil, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/west20_nvr/recordings?channel=1&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&event_only=true&event=all", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"source":"nvr_event"`) ||
		!strings.Contains(rec.Body.String(), `"type":"Event.smdTypeHuman"`) {
		t.Fatalf("expected nvr event response: %s", rec.Body.String())
	}
}

func TestNVRRecordingsEndpointSerializesEmptyItemsArray(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		nvrRecordings: func(_ context.Context, _ string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			return dahua.NVRRecordingSearchResult{
				DeviceID:      "west20_nvr",
				Channel:       query.Channel,
				StartTime:     "2026-04-28 00:00:00",
				EndTime:       "2026-04-28 01:00:00",
				Limit:         query.Limit,
				ReturnedCount: 0,
				Items:         nil,
			}, nil
		},
	}, nil, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/west20_nvr/recordings?channel=1&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&event_only=true&event=all", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("expected empty items array in response: %s", rec.Body.String())
	}
}

func TestNVREventSummaryEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		listStreams: func(bool) []streams.Entry {
			return []streams.Entry{
				{RootDeviceID: "west20_nvr", DeviceKind: dahua.DeviceKindNVRChannel, Channel: 2},
				{RootDeviceID: "west20_nvr", DeviceKind: dahua.DeviceKindNVRChannel, Channel: 1},
				{RootDeviceID: "west20_nvr", DeviceKind: dahua.DeviceKindNVRChannel, Channel: 1},
				{RootDeviceID: "other_nvr", DeviceKind: dahua.DeviceKindNVRChannel, Channel: 9},
			}
		},
		nvrRecordings: func(_ context.Context, deviceID string, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if !query.EventOnly {
				t.Fatal("expected event-only summary search")
			}
			if query.EventCode != "all" {
				t.Fatalf("unexpected event code %q", query.EventCode)
			}

			switch query.Channel {
			case 1:
				return dahua.NVRRecordingSearchResult{
					Items: []dahua.NVRRecording{
						{Type: "Event.smdTypeHuman", Flags: []string{"Event", "smdTypeHuman"}},
						{Type: "Event.CrossLineDetection", Flags: []string{"Event", "CrossLineDetection"}},
					},
				}, nil
			case 2:
				return dahua.NVRRecordingSearchResult{
					Items: []dahua.NVRRecording{
						{Type: "Event.smdTypeVehicle", Flags: []string{"Event", "smdTypeVehicle"}},
					},
				}, nil
			default:
				t.Fatalf("unexpected channel %d", query.Channel)
				return dahua.NVRRecordingSearchResult{}, nil
			}
		},
	}, nil, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/west20_nvr/events/summary?start=2026-05-01T00:00:00Z&end=2026-05-02T00:00:00Z&event=all", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var summary dahua.NVREventSummary
	if err := json.NewDecoder(rec.Body).Decode(&summary); err != nil {
		t.Fatalf("decode summary response: %v", err)
	}

	if summary.DeviceID != "west20_nvr" {
		t.Fatalf("unexpected device id %q", summary.DeviceID)
	}
	if summary.TotalCount != 3 {
		t.Fatalf("unexpected total count %d", summary.TotalCount)
	}
	if len(summary.Channels) != 2 {
		t.Fatalf("unexpected channel summary count %d", len(summary.Channels))
	}
	if summary.Channels[0].Channel != 1 || summary.Channels[0].TotalCount != 2 {
		t.Fatalf("unexpected first channel summary %+v", summary.Channels[0])
	}
	if summary.Channels[1].Channel != 2 || summary.Channels[1].TotalCount != 1 {
		t.Fatalf("unexpected second channel summary %+v", summary.Channels[1])
	}

	gotCounts := map[string]int{}
	gotLabels := map[string]string{}
	for _, item := range summary.Items {
		gotCounts[item.Code] = item.Count
		gotLabels[item.Code] = item.Label
	}

	expectedCounts := map[string]int{
		"smdTypeHuman":       1,
		"CrossLineDetection": 1,
		"smdTypeVehicle":     1,
	}
	expectedLabels := map[string]string{
		"smdTypeHuman":       "Human",
		"CrossLineDetection": "Cross Line",
		"smdTypeVehicle":     "Vehicle",
	}
	if !reflect.DeepEqual(gotCounts, expectedCounts) {
		t.Fatalf("unexpected summary counts %+v", gotCounts)
	}
	if !reflect.DeepEqual(gotLabels, expectedLabels) {
		t.Fatalf("unexpected summary labels %+v", gotLabels)
	}
}

func TestNVRRecordingsEndpointRejectsInvalidQuery(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, nil, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/west20_nvr/recordings?channel=0&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNVRRecordingExportEndpointCreatesPlaybackClip(t *testing.T) {
	start := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	end := start.Add(4 * time.Second)

	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		createNVRPlaybackSession: func(_ context.Context, deviceID string, request dahua.NVRPlaybackSessionRequest) (dahua.NVRPlaybackSession, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if request.Channel != 5 || !request.StartTime.Equal(start) || !request.EndTime.Equal(end) {
				t.Fatalf("unexpected playback request %+v", request)
			}
			return dahua.NVRPlaybackSession{
				ID:                 "nvrpb_test",
				StreamID:           "nvrpb_test",
				DeviceID:           deviceID,
				Name:               "Channel 5",
				Channel:            request.Channel,
				StartTime:          request.StartTime.Format(time.RFC3339),
				EndTime:            request.EndTime.Format(time.RFC3339),
				SeekTime:           request.StartTime.Format(time.RFC3339),
				RecommendedProfile: "quality",
				CreatedAt:          start.Format(time.RFC3339),
				ExpiresAt:          start.Add(30 * time.Minute).Format(time.RFC3339),
				Profiles:           map[string]dahua.NVRPlaybackProfile{},
			}, nil
		},
	}, stubMediaReader{
		enabled: true,
		startClip: func(_ context.Context, request mediaapi.ClipStartRequest) (mediaapi.ClipInfo, error) {
			if request.StreamID != "nvrpb_test" {
				t.Fatalf("unexpected stream id %q", request.StreamID)
			}
			if request.ProfileName != "quality" {
				t.Fatalf("unexpected profile %q", request.ProfileName)
			}
			if request.Duration != 4*time.Second {
				t.Fatalf("unexpected duration %s", request.Duration)
			}
			return mediaapi.ClipInfo{
				ID:        "clip_archive",
				StreamID:  request.StreamID,
				Profile:   request.ProfileName,
				Status:    mediaapi.ClipStatusRecording,
				StartedAt: start,
				Duration:  request.Duration,
				FileName:  "clip_archive.mp4",
			}, nil
		},
	}, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/west20_nvr/recordings/export?channel=5&start_time=2026-04-29T00:00:00Z&end_time=2026-04-29T00:00:04Z", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) ||
		!strings.Contains(rec.Body.String(), `"id":"clip_archive"`) ||
		!strings.Contains(rec.Body.String(), `"download_url":"http://example.com/api/v1/media/recordings/clip_archive/download"`) {
		t.Fatalf("unexpected export response: %s", rec.Body.String())
	}
}

func TestNVRChannelControlsEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, nil, stubActionReader{
		nvrControls: func(_ context.Context, deviceID string, channel int) (dahua.NVRChannelControlCapabilities, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if channel != 5 {
				t.Fatalf("unexpected channel %d", channel)
			}
			return dahua.NVRChannelControlCapabilities{
				DeviceID: deviceID,
				Channel:  channel,
				PTZ: dahua.NVRPTZCapabilities{
					Supported:    true,
					Pan:          true,
					Tilt:         true,
					Zoom:         true,
					Aux:          true,
					AuxFunctions: []string{"Light", "Wiper"},
					Commands:     []string{"up", "down", "left", "right"},
				},
				Aux: dahua.NVRAuxCapabilities{
					Supported: true,
					Outputs:   []string{"aux", "light", "wiper"},
					Features:  []string{"siren", "warning_light", "wiper"},
				},
				Audio: dahua.NVRChannelAudioCapabilities{
					Supported: false,
					Mute:      false,
					Volume:    false,
				},
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/west20_nvr/channels/5/controls", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"channel":5`) || !strings.Contains(rec.Body.String(), `"outputs":["aux","light","wiper"]`) || !strings.Contains(rec.Body.String(), `"audio":{"supported":false,"mute":false,"volume":false,`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestNVRChannelPTZEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, nil, stubActionReader{
		nvrPTZ: func(_ context.Context, deviceID string, request dahua.NVRPTZRequest) error {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if request.Channel != 11 {
				t.Fatalf("unexpected channel %d", request.Channel)
			}
			if request.Action != dahua.NVRPTZActionPulse {
				t.Fatalf("unexpected action %q", request.Action)
			}
			if request.Command != dahua.NVRPTZCommandLeft {
				t.Fatalf("unexpected command %q", request.Command)
			}
			if request.Speed != 3 {
				t.Fatalf("unexpected speed %d", request.Speed)
			}
			if request.Duration != 250*time.Millisecond {
				t.Fatalf("unexpected duration %s", request.Duration)
			}
			return nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/west20_nvr/channels/11/ptz", strings.NewReader(`{"action":"pulse","command":"left","speed":3,"duration_ms":250}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) || !strings.Contains(rec.Body.String(), `"channel":11`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestNVRChannelPTZEndpointReturnsUnsupportedOperationErrorCode(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, nil, stubActionReader{
		nvrPTZ: func(_ context.Context, _ string, _ dahua.NVRPTZRequest) error {
			return fmt.Errorf("ptz unsupported: %w", dahua.ErrUnsupportedOperation)
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/west20_nvr/channels/11/ptz", strings.NewReader(`{"action":"pulse","command":"left","speed":3,"duration_ms":250}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload apiErrorPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ErrorCode != "unsupported_operation" {
		t.Fatalf("unexpected error code %+v", payload)
	}
}

func TestNVRChannelAuxEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, nil, stubActionReader{
		nvrAux: func(_ context.Context, deviceID string, request dahua.NVRAuxRequest) error {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if request.Channel != 5 {
				t.Fatalf("unexpected channel %d", request.Channel)
			}
			if request.Action != dahua.NVRAuxActionPulse {
				t.Fatalf("unexpected action %q", request.Action)
			}
			if request.Output != "warning_light" {
				t.Fatalf("unexpected output %q", request.Output)
			}
			if request.Duration != 400*time.Millisecond {
				t.Fatalf("unexpected duration %s", request.Duration)
			}
			return nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/west20_nvr/channels/5/aux", strings.NewReader(`{"action":"pulse","output":"warning_light","duration_ms":400}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output":"warning_light"`) || !strings.Contains(rec.Body.String(), `"duration_ms":400`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestNVRChannelRecordingEndpoint(t *testing.T) {
	actions := make([]dahua.NVRRecordingAction, 0, 2)
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, nil, stubActionReader{
		nvrRecording: func(_ context.Context, deviceID string, request dahua.NVRRecordingRequest) error {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if request.Channel != 11 {
				t.Fatalf("unexpected channel %d", request.Channel)
			}
			actions = append(actions, request.Action)
			return nil
		},
	}, stubEventReader{})

	for _, action := range []dahua.NVRRecordingAction{dahua.NVRRecordingActionStart, dahua.NVRRecordingActionAuto} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/west20_nvr/channels/11/recording", strings.NewReader(fmt.Sprintf(`{"action":%q}`, action)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		server.httpServer.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), fmt.Sprintf(`"action":%q`, action)) || !strings.Contains(rec.Body.String(), `"channel":11`) {
			t.Fatalf("unexpected response body: %s", rec.Body.String())
		}
	}
	if !reflect.DeepEqual(actions, []dahua.NVRRecordingAction{dahua.NVRRecordingActionStart, dahua.NVRRecordingActionAuto}) {
		t.Fatalf("unexpected actions %+v", actions)
	}
}

func TestNVRDiagnosticEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{}, nil, stubActionReader{
		nvrDiagnostic: func(_ context.Context, deviceID string, request dahua.NVRDiagnosticActionRequest) (dahua.NVRDiagnosticActionResult, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if request.Channel != 5 {
				t.Fatalf("unexpected channel %d", request.Channel)
			}
			if request.Method != "direct_ipc_lighting" {
				t.Fatalf("unexpected method %q", request.Method)
			}
			if request.Action != "start" {
				t.Fatalf("unexpected action %q", request.Action)
			}
			if request.Duration != 500*time.Millisecond {
				t.Fatalf("unexpected duration %s", request.Duration)
			}
			return dahua.NVRDiagnosticActionResult{
				Status:      "ok",
				DeviceID:    deviceID,
				Channel:     request.Channel,
				Method:      request.Method,
				Action:      request.Action,
				DurationMS:  request.Duration.Milliseconds(),
				Endpoint:    "/cgi-bin/configManager.cgi?action=setConfig&Lighting_V2...",
				Description: "Direct IPC Lighting_V2 configManager setConfig path.",
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/west20_nvr/channels/5/diagnostics", strings.NewReader(`{"method":"direct_ipc_lighting","action":"start","duration_ms":500}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"method":"direct_ipc_lighting"`) || !strings.Contains(rec.Body.String(), `"duration_ms":500`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestNVRPlaybackSessionCreateEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		createNVRPlaybackSession: func(_ context.Context, deviceID string, request dahua.NVRPlaybackSessionRequest) (dahua.NVRPlaybackSession, error) {
			if deviceID != "west20_nvr" {
				t.Fatalf("unexpected device id %q", deviceID)
			}
			if request.Channel != 2 {
				t.Fatalf("unexpected channel %d", request.Channel)
			}
			if request.SeekTime.IsZero() {
				t.Fatal("expected non-zero seek time")
			}
			return dahua.NVRPlaybackSession{
				ID:                 "nvrpb_test",
				StreamID:           "nvrpb_test",
				DeviceID:           deviceID,
				SourceStreamID:     "west20_nvr_channel_02",
				Name:               "Lobby",
				Channel:            2,
				StartTime:          "2026-04-28T00:00:00Z",
				EndTime:            "2026-04-28T01:00:00Z",
				SeekTime:           "2026-04-28T00:20:00Z",
				RecommendedProfile: "quality",
				Profiles: map[string]dahua.NVRPlaybackProfile{
					"quality": {
						Name:           "quality",
						HLSURL:         "/api/v1/media/hls/nvrpb_test/quality/index.m3u8",
						MJPEGURL:       "/api/v1/media/mjpeg/nvrpb_test?profile=quality",
						WebRTCOfferURL: "/api/v1/media/webrtc/nvrpb_test/quality/offer",
					},
				},
			}, nil
		},
	}, nil, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/west20_nvr/playback/sessions", strings.NewReader(`{"channel":2,"start_time":"2026-04-28T00:00:00Z","end_time":"2026-04-28T01:00:00Z","seek_time":"2026-04-28T00:20:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"stream_id":"nvrpb_test"`) {
		t.Fatalf("expected stream id in response: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `/api/v1/media/hls/nvrpb_test/quality/index.m3u8`) {
		t.Fatalf("expected hls url in response: %s", rec.Body.String())
	}
}

func TestNVRPlaybackSessionGetEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		getNVRPlaybackSession: func(sessionID string) (dahua.NVRPlaybackSession, error) {
			if sessionID != "nvrpb_test" {
				t.Fatalf("unexpected session id %q", sessionID)
			}
			return dahua.NVRPlaybackSession{
				ID:       sessionID,
				StreamID: sessionID,
				DeviceID: "west20_nvr",
				Channel:  1,
				Name:     "Entrance",
				Profiles: map[string]dahua.NVRPlaybackProfile{
					"stable": {Name: "stable"},
				},
			}, nil
		},
	}, nil, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nvr/playback/sessions/nvrpb_test", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"device_id":"west20_nvr"`) {
		t.Fatalf("expected device id in response: %s", rec.Body.String())
	}
}

func TestNVRPlaybackSessionSeekEndpoint(t *testing.T) {
	server := newTestServerWithConfig(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubSnapshotReader{
		seekNVRPlaybackSession: func(_ context.Context, sessionID string, seekTime time.Time) (dahua.NVRPlaybackSession, error) {
			if sessionID != "nvrpb_test" {
				t.Fatalf("unexpected session id %q", sessionID)
			}
			if seekTime.Format(time.RFC3339) != "2026-04-28T00:45:00Z" {
				t.Fatalf("unexpected seek time %s", seekTime.Format(time.RFC3339))
			}
			return dahua.NVRPlaybackSession{
				ID:       "nvrpb_seeked",
				StreamID: "nvrpb_seeked",
				DeviceID: "west20_nvr",
				Channel:  1,
				Name:     "Entrance",
				SeekTime: "2026-04-28T00:45:00Z",
				Profiles: map[string]dahua.NVRPlaybackProfile{
					"stable": {
						Name:           "stable",
						HLSURL:         "/api/v1/media/hls/nvrpb_seeked/stable/index.m3u8",
						WebRTCOfferURL: "/api/v1/media/webrtc/nvrpb_seeked/stable/offer",
					},
				},
			}, nil
		},
	}, nil, stubActionReader{}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nvr/playback/sessions/nvrpb_test/seek", strings.NewReader(`{"seek_time":"2026-04-28T00:45:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"nvrpb_seeked"`) {
		t.Fatalf("expected new session id in response: %s", rec.Body.String())
	}
}
