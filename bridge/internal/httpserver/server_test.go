package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/ha"
	"RCooLeR/DahuaBridge/internal/haapi"
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
	listStreams   func(bool) []streams.Entry
	adminSettings func() map[string]any
}

func (stubSnapshotReader) NVRSnapshot(context.Context, string, int) ([]byte, string, error) {
	return nil, "", nil
}
func (stubSnapshotReader) VTOSnapshot(context.Context, string) ([]byte, string, error) {
	return nil, "", nil
}
func (stubSnapshotReader) IPCSnapshot(context.Context, string) ([]byte, string, error) {
	return nil, "", nil
}
func (stubSnapshotReader) RenderHomeAssistantCameraPackage(ha.CameraPackageOptions) (string, error) {
	return "", nil
}
func (stubSnapshotReader) RenderHomeAssistantDashboardPackage() (string, error) {
	return "", nil
}
func (stubSnapshotReader) RenderHomeAssistantLovelaceDashboard() (string, error) {
	return "", nil
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
	hlsPlaylist              func(context.Context, string, string) ([]byte, error)
	hlsSegment               func(context.Context, string, string, string) ([]byte, string, error)
	webrtcOffer              func(context.Context, string, string, mediaapi.WebRTCSessionDescription) (mediaapi.WebRTCSessionDescription, error)
	iceServers               func() []mediaapi.WebRTCICEServer
	intercomStatus           func(string) mediaapi.IntercomStatus
	stopIntercomSessions     func(string) mediaapi.IntercomStatus
	setIntercomUplinkEnabled func(string, bool) mediaapi.IntercomStatus
	listWorker               func() []mediaapi.WorkerStatus
}

type stubActionReader struct {
	unlock     func(context.Context, string, int) error
	answer     func(context.Context, string) error
	hangup     func(context.Context, string) error
	probe      func(context.Context, string) (*dahua.ProbeResult, error)
	probeAll   func(context.Context) []dahua.ProbeActionResult
	rotate     func(context.Context, string, dahua.DeviceConfigUpdate) (*dahua.ProbeResult, error)
	refreshNVR func(context.Context, string) (*dahua.ProbeResult, error)
	provision  func(context.Context, haapi.ONVIFProvisionRequest) ([]haapi.ONVIFProvisionResult, error)
	cleanup    func(context.Context) (ha.LegacyDiscoveryCleanupResult, error)
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

func (s stubActionReader) ProvisionHomeAssistantONVIF(ctx context.Context, request haapi.ONVIFProvisionRequest) ([]haapi.ONVIFProvisionResult, error) {
	if s.provision == nil {
		return nil, nil
	}
	return s.provision(ctx, request)
}

func (s stubActionReader) RemoveLegacyHomeAssistantMQTTDiscovery(ctx context.Context) (ha.LegacyDiscoveryCleanupResult, error) {
	if s.cleanup == nil {
		return ha.LegacyDiscoveryCleanupResult{}, nil
	}
	return s.cleanup(ctx)
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

func TestProvisionHomeAssistantONVIFEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		provision: func(_ context.Context, request haapi.ONVIFProvisionRequest) ([]haapi.ONVIFProvisionResult, error) {
			if !request.Force {
				t.Fatalf("expected force request, got %+v", request)
			}
			if len(request.DeviceIDs) != 1 || request.DeviceIDs[0] != "yard_ipc" {
				t.Fatalf("unexpected request %+v", request)
			}
			return []haapi.ONVIFProvisionResult{
				{
					DeviceID:   "yard_ipc",
					DeviceKind: dahua.DeviceKindIPC,
					Name:       "Yard Camera",
					Host:       "192.168.1.20",
					Port:       8999,
					Status:     "created",
					EntryID:    "entry-123",
				},
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/home-assistant/onvif/provision", strings.NewReader(`{"device_ids":["yard_ipc"],"force":true}`))
	req.Header.Set("Content-Type", "application/json")
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
		t.Fatalf("unexpected payload %+v", payload)
	}
	if payload["created_count"] != float64(1) {
		t.Fatalf("unexpected payload %+v", payload)
	}
}

func TestProvisionHomeAssistantONVIFEndpointPartialFailure(t *testing.T) {
	server := newTestServer(stubActionReader{
		provision: func(_ context.Context, _ haapi.ONVIFProvisionRequest) ([]haapi.ONVIFProvisionResult, error) {
			return []haapi.ONVIFProvisionResult{
				{
					DeviceID:   "yard_ipc",
					DeviceKind: dahua.DeviceKindIPC,
					Name:       "Yard Camera",
					Host:       "192.168.1.20",
					Port:       8999,
					Status:     "created",
				},
				{
					DeviceID:   "west20_vto",
					DeviceKind: dahua.DeviceKindVTO,
					Name:       "Front Door",
					Host:       "192.168.1.30",
					Port:       80,
					Status:     "error",
					Error:      "auth_failed",
				},
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/home-assistant/onvif/provision", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("expected status 207, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProvisionHomeAssistantONVIFEndpointReturnsUnavailableWhenActionFails(t *testing.T) {
	server := newTestServer(stubActionReader{
		provision: func(_ context.Context, _ haapi.ONVIFProvisionRequest) ([]haapi.ONVIFProvisionResult, error) {
			return nil, errors.New("home assistant api is not configured")
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/home-assistant/onvif/provision", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d: %s", rec.Code, rec.Body.String())
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
	if !strings.Contains(body, `/api/v1/home-assistant/onvif/provision`) || !strings.Contains(body, "Force ONVIF Provisioning") {
		t.Fatalf("missing onvif provisioning controls:\n%s", body)
	}
	if !strings.Contains(body, `/api/v1/devices/probe-all`) || !strings.Contains(body, "Clear Event Buffer") || !strings.Contains(body, "Remove Legacy MQTT Discovery") {
		t.Fatalf("missing admin action controls:\n%s", body)
	}
	if !strings.Contains(body, `/api/v1/vto/front_vto/intercom?profile=stable`) || !strings.Contains(body, `/api/v1/media/webrtc/front_vto/stable`) {
		t.Fatalf("missing concrete stream links:\n%s", body)
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

func TestHomeAssistantMigrationPlanEndpoint(t *testing.T) {
	server := newTestServerWithReaders(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubProbeReader{
		list: func() []*dahua.ProbeResult {
			return []*dahua.ProbeResult{{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard IPC",
					Kind: dahua.DeviceKindIPC,
				},
				States: map[string]dahua.DeviceState{
					"yard_ipc": {Available: true},
				},
			}}
		},
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			return []streams.Entry{
				{
					ID:                 "yard_ipc",
					RootDeviceID:       "yard_ipc",
					Name:               "Yard IPC",
					DeviceKind:         dahua.DeviceKindIPC,
					ONVIFH264Available: true,
				},
			}
		},
	}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home-assistant/migration/plan", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"strategy":"native_integration_primary"`) {
		t.Fatalf("expected native strategy in response:\n%s", body)
	}
	if !strings.Contains(body, `"duplicate_paths_if_present":["generic_camera_package","mqtt_discovery","onvif_config_entry"]`) {
		t.Fatalf("expected duplicate path guidance in response:\n%s", body)
	}
}

func TestHomeAssistantMigrationGuideEndpoint(t *testing.T) {
	server := newTestServerWithReaders(config.HTTPConfig{
		ListenAddress: ":0",
		MetricsPath:   "/metrics",
		HealthPath:    "/healthz",
	}, stubProbeReader{
		list: func() []*dahua.ProbeResult {
			return []*dahua.ProbeResult{{
				Root: dahua.Device{
					ID:   "yard_ipc",
					Name: "Yard IPC",
					Kind: dahua.DeviceKindIPC,
				},
				States: map[string]dahua.DeviceState{
					"yard_ipc": {Available: true},
				},
			}}
		},
	}, stubSnapshotReader{
		listStreams: func(includeCredentials bool) []streams.Entry {
			return []streams.Entry{
				{
					ID:           "yard_ipc",
					RootDeviceID: "yard_ipc",
					Name:         "Yard IPC",
					DeviceKind:   dahua.DeviceKindIPC,
				},
			}
		},
	}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home-assistant/migration/guide.md", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("unexpected content type %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "POST /api/v1/home-assistant/mqtt/discovery/remove") {
		t.Fatalf("expected cleanup endpoint in guide:\n%s", body)
	}
}

func TestRemoveLegacyMQTTDiscoveryEndpoint(t *testing.T) {
	server := newTestServer(stubActionReader{
		cleanup: func(_ context.Context) (ha.LegacyDiscoveryCleanupResult, error) {
			return ha.LegacyDiscoveryCleanupResult{
				RemovedTopics: 12,
				DeviceCount:   2,
				DeviceIDs:     []string{"west20_nvr", "west20_nvr_channel_01"},
			}, nil
		},
	}, stubEventReader{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/home-assistant/mqtt/discovery/remove", nil)
	rec := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"removed_topics":12`) || !strings.Contains(body, `"device_count":2`) {
		t.Fatalf("expected cleanup result in response:\n%s", body)
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
