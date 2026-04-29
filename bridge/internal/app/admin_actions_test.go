package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

type stubDriver struct {
	id            string
	kind          dahua.DeviceKind
	probeFn       func(context.Context) (*dahua.ProbeResult, error)
	unlockFn      func(context.Context, int) error
	answerFn      func(context.Context) error
	hangupFn      func(context.Context) error
	vtoControlsFn func(context.Context) (dahua.VTOControlCapabilities, error)
	vtoOutputFn   func(context.Context, int, int) error
	vtoInputFn    func(context.Context, int, int) error
	vtoMuteFn     func(context.Context, bool) error
	vtoRecordFn   func(context.Context, bool) error
	auxFn         func(context.Context, dahua.NVRAuxRequest) error
	recordingFn   func(context.Context, dahua.NVRRecordingRequest) error
	invalidateFn  func()
	updateCfgFn   func(config.DeviceConfig) error
}

func (s stubDriver) ID() string                  { return s.id }
func (s stubDriver) Kind() dahua.DeviceKind      { return s.kind }
func (s stubDriver) PollInterval() time.Duration { return 30 * time.Second }
func (s stubDriver) Probe(ctx context.Context) (*dahua.ProbeResult, error) {
	if s.probeFn == nil {
		return nil, nil
	}
	return s.probeFn(ctx)
}
func (s stubDriver) Unlock(ctx context.Context, index int) error {
	if s.unlockFn == nil {
		return nil
	}
	return s.unlockFn(ctx, index)
}
func (s stubDriver) HangupCall(ctx context.Context) error {
	if s.hangupFn == nil {
		return nil
	}
	return s.hangupFn(ctx)
}
func (s stubDriver) AnswerCall(ctx context.Context) error {
	if s.answerFn == nil {
		return nil
	}
	return s.answerFn(ctx)
}
func (s stubDriver) ControlCapabilities(ctx context.Context) (dahua.VTOControlCapabilities, error) {
	if s.vtoControlsFn == nil {
		return dahua.VTOControlCapabilities{}, nil
	}
	return s.vtoControlsFn(ctx)
}
func (s stubDriver) SetAudioOutputVolume(ctx context.Context, slot int, level int) error {
	if s.vtoOutputFn == nil {
		return nil
	}
	return s.vtoOutputFn(ctx, slot, level)
}
func (s stubDriver) SetAudioInputVolume(ctx context.Context, slot int, level int) error {
	if s.vtoInputFn == nil {
		return nil
	}
	return s.vtoInputFn(ctx, slot, level)
}
func (s stubDriver) SetAudioMute(ctx context.Context, muted bool) error {
	if s.vtoMuteFn == nil {
		return nil
	}
	return s.vtoMuteFn(ctx, muted)
}
func (s stubDriver) SetRecordingEnabled(ctx context.Context, enabled bool) error {
	if s.vtoRecordFn == nil {
		return nil
	}
	return s.vtoRecordFn(ctx, enabled)
}
func (s stubDriver) Aux(ctx context.Context, request dahua.NVRAuxRequest) error {
	if s.auxFn == nil {
		return nil
	}
	return s.auxFn(ctx, request)
}
func (s stubDriver) Recording(ctx context.Context, request dahua.NVRRecordingRequest) error {
	if s.recordingFn == nil {
		return nil
	}
	return s.recordingFn(ctx, request)
}
func (s stubDriver) InvalidateInventoryCache() {
	if s.invalidateFn != nil {
		s.invalidateFn()
	}
}
func (s stubDriver) UpdateConfig(cfg config.DeviceConfig) error {
	if s.updateCfgFn != nil {
		return s.updateCfgFn(cfg)
	}
	return nil
}

var _ dahua.Driver = stubDriver{}
var _ dahua.VTOLockController = stubDriver{}
var _ dahua.VTOCallController = stubDriver{}
var _ dahua.VTOControlReader = stubDriver{}
var _ dahua.VTOAudioController = stubDriver{}
var _ dahua.VTORecordingController = stubDriver{}
var _ dahua.NVRAuxController = stubDriver{}
var _ dahua.NVRRecordingController = stubDriver{}
var _ dahua.NVRInventoryRefresher = stubDriver{}
var _ dahua.ConfigurableDriver = stubDriver{}

type stubDeviceConfigStore struct {
	items map[string]config.DeviceConfig
}

func (s *stubDeviceConfigStore) GetDeviceConfig(deviceID string) (config.DeviceConfig, bool) {
	cfg, ok := s.items[deviceID]
	return cfg, ok
}

func (s *stubDeviceConfigStore) UpdateDeviceConfig(deviceID string, cfg config.DeviceConfig) bool {
	if _, ok := s.items[deviceID]; !ok {
		return false
	}
	s.items[deviceID] = cfg
	return true
}

func TestAdminActionsProbeDeviceStoresResult(t *testing.T) {
	probes := store.NewProbeStore()
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		probes,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"front_vto": {ID: "front_vto"}}},
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				probeFn: func(context.Context) (*dahua.ProbeResult, error) {
					return &dahua.ProbeResult{
						Root: dahua.Device{ID: "front_vto", Name: "Front Door", Kind: dahua.DeviceKindVTO},
						States: map[string]dahua.DeviceState{
							"front_vto": {Available: true},
						},
					}, nil
				},
			},
		},
	)

	result, err := actions.ProbeDevice(context.Background(), "front_vto")
	if err != nil {
		t.Fatalf("ProbeDevice returned error: %v", err)
	}
	if result == nil || result.Root.ID != "front_vto" {
		t.Fatalf("unexpected result: %+v", result)
	}

	stored, ok := probes.Get("front_vto")
	if !ok || stored == nil || stored.Root.Name != "Front Door" {
		t.Fatalf("unexpected stored result: %+v ok=%v", stored, ok)
	}
}

func TestAdminActionsProbeDeviceFailureDoesNotStore(t *testing.T) {
	probes := store.NewProbeStore()
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		probes,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"front_vto": {ID: "front_vto"}}},
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				probeFn: func(context.Context) (*dahua.ProbeResult, error) {
					return nil, errors.New("probe failed")
				},
			},
		},
	)

	if _, err := actions.ProbeDevice(context.Background(), "front_vto"); err == nil {
		t.Fatal("expected ProbeDevice to fail")
	}
	if _, ok := probes.Get("front_vto"); ok {
		t.Fatal("did not expect failed probe to be stored")
	}
}

func TestAdminActionsProbeAllDevices(t *testing.T) {
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		store.NewProbeStore(),
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{
			"yard_ipc":  {ID: "yard_ipc"},
			"front_vto": {ID: "front_vto"},
		}},
		[]dahua.Driver{
			stubDriver{
				id:   "yard_ipc",
				kind: dahua.DeviceKindIPC,
				probeFn: func(context.Context) (*dahua.ProbeResult, error) {
					return &dahua.ProbeResult{Root: dahua.Device{ID: "yard_ipc", Kind: dahua.DeviceKindIPC}}, nil
				},
			},
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				probeFn: func(context.Context) (*dahua.ProbeResult, error) {
					return nil, errors.New("probe failed")
				},
			},
		},
	)

	results := actions.ProbeAllDevices(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 probe results, got %d", len(results))
	}
	if !reflect.DeepEqual([]string{results[0].DeviceID, results[1].DeviceID}, []string{"front_vto", "yard_ipc"}) {
		t.Fatalf("unexpected device order: %+v", results)
	}
	if results[0].Error == "" || results[1].Result == nil || results[1].Result.Root.ID != "yard_ipc" {
		t.Fatalf("unexpected probe-all result: %+v", results)
	}
}

func TestAdminActionsRefreshNVRInventory(t *testing.T) {
	probes := store.NewProbeStore()
	invalidated := false
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		probes,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"west20_nvr": {ID: "west20_nvr"}}},
		[]dahua.Driver{
			stubDriver{
				id:           "west20_nvr",
				kind:         dahua.DeviceKindNVR,
				invalidateFn: func() { invalidated = true },
				probeFn: func(context.Context) (*dahua.ProbeResult, error) {
					return &dahua.ProbeResult{
						Root: dahua.Device{ID: "west20_nvr", Kind: dahua.DeviceKindNVR},
						States: map[string]dahua.DeviceState{
							"west20_nvr": {Available: true},
						},
					}, nil
				},
			},
		},
	)

	result, err := actions.RefreshNVRInventory(context.Background(), "west20_nvr")
	if err != nil {
		t.Fatalf("RefreshNVRInventory returned error: %v", err)
	}
	if !invalidated || result == nil || result.Root.ID != "west20_nvr" {
		t.Fatalf("unexpected refresh result: invalidated=%v result=%+v", invalidated, result)
	}
}

func TestAdminActionsRotateDeviceCredentials(t *testing.T) {
	probes := store.NewProbeStore()
	configStore := &stubDeviceConfigStore{items: map[string]config.DeviceConfig{
		"yard_ipc": {
			ID:             "yard_ipc",
			Name:           "Yard",
			BaseURL:        "http://192.168.1.20",
			Username:       "admin",
			Password:       "old-secret",
			RequestTimeout: 10 * time.Second,
			Enabled:        boolPtr(true),
		},
	}}

	var updated config.DeviceConfig
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		probes,
		configStore,
		[]dahua.Driver{
			stubDriver{
				id:   "yard_ipc",
				kind: dahua.DeviceKindIPC,
				updateCfgFn: func(cfg config.DeviceConfig) error {
					updated = cfg
					return nil
				},
				probeFn: func(context.Context) (*dahua.ProbeResult, error) {
					return &dahua.ProbeResult{
						Root: dahua.Device{ID: "yard_ipc", Kind: dahua.DeviceKindIPC},
						States: map[string]dahua.DeviceState{
							"yard_ipc": {Available: true},
						},
					}, nil
				},
			},
		},
	)

	baseURL := "https://192.168.1.21"
	username := "service"
	password := "new-secret"
	onvifEnabled := true
	onvifUsername := "onvif-user"
	onvifPassword := "onvif-secret"
	result, err := actions.RotateDeviceCredentials(context.Background(), "yard_ipc", dahua.DeviceConfigUpdate{
		BaseURL:       &baseURL,
		Username:      &username,
		Password:      &password,
		OnvifEnabled:  &onvifEnabled,
		OnvifUsername: &onvifUsername,
		OnvifPassword: &onvifPassword,
	})
	if err != nil {
		t.Fatalf("RotateDeviceCredentials returned error: %v", err)
	}
	if result == nil || result.Root.ID != "yard_ipc" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if updated.BaseURL != "https://192.168.1.21" || updated.Username != "service" || updated.Password != "new-secret" {
		t.Fatalf("unexpected updated config: %+v", updated)
	}
	if !updated.ONVIFEnabledValue() || updated.OnvifUsername != "onvif-user" || updated.OnvifPassword != "onvif-secret" {
		t.Fatalf("expected updated onvif config, got %+v", updated)
	}
	stored, ok := configStore.GetDeviceConfig("yard_ipc")
	if !ok || stored.Username != "service" || stored.Password != "new-secret" {
		t.Fatalf("unexpected stored config: %+v ok=%v", stored, ok)
	}
}

func TestAdminActionsHangupVTOCallUpdatesStoredState(t *testing.T) {
	probes := store.NewProbeStore()
	probes.Set("front_vto", &dahua.ProbeResult{
		Root: dahua.Device{ID: "front_vto", Kind: dahua.DeviceKindVTO},
		States: map[string]dahua.DeviceState{
			"front_vto": {
				Available: true,
				Info: map[string]any{
					"call":            true,
					"call_state":      "ringing",
					"call_started_at": "2026-04-27T12:00:00Z",
				},
			},
		},
	})

	called := false
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		probes,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"front_vto": {ID: "front_vto"}}},
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				hangupFn: func(context.Context) error {
					called = true
					return nil
				},
			},
		},
	)

	if err := actions.HangupVTOCall(context.Background(), "front_vto"); err != nil {
		t.Fatalf("HangupVTOCall returned error: %v", err)
	}
	if !called {
		t.Fatal("expected hangup controller to be called")
	}

	stored, ok := probes.Get("front_vto")
	if !ok || stored == nil {
		t.Fatal("expected updated probe result")
	}
	info := stored.States["front_vto"].Info
	if info["call"] != false || info["call_state"] != "idle" {
		t.Fatalf("unexpected call state after hangup: %+v", info)
	}
	if _, ok := info["last_call_ended_at"]; !ok {
		t.Fatalf("expected last_call_ended_at to be set, got %+v", info)
	}
}

func TestAdminActionsAnswerVTOCall(t *testing.T) {
	called := false
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.Info()),
		store.NewProbeStore(),
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"front_vto": {ID: "front_vto"}}},
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				answerFn: func(context.Context) error {
					called = true
					return nil
				},
			},
		},
	)

	if err := actions.AnswerVTOCall(context.Background(), "front_vto"); err != nil {
		t.Fatalf("AnswerVTOCall returned error: %v", err)
	}
	if !called {
		t.Fatal("expected answer controller to be called")
	}
}

func TestAdminActionsVTOControlCapabilities(t *testing.T) {
	expected := dahua.VTOControlCapabilities{
		DeviceID: "front_vto",
		Call: dahua.VTOCallCapabilities{
			Answer: true,
			Hangup: true,
			State:  "Idle",
		},
	}

	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.BuildInfo{}),
		store.NewProbeStore(),
		&stubDeviceConfigStore{},
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				vtoControlsFn: func(context.Context) (dahua.VTOControlCapabilities, error) {
					return expected, nil
				},
			},
		},
	)

	result, err := actions.VTOControlCapabilities(context.Background(), "front_vto")
	if err != nil {
		t.Fatalf("VTOControlCapabilities returned error: %v", err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("unexpected capabilities %+v", result)
	}
}

func TestAdminActionsSetVTOAudioOutputVolume(t *testing.T) {
	called := false
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.BuildInfo{}),
		store.NewProbeStore(),
		&stubDeviceConfigStore{},
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				vtoOutputFn: func(_ context.Context, slot int, level int) error {
					called = true
					if slot != 1 || level != 80 {
						t.Fatalf("unexpected slot/level %d/%d", slot, level)
					}
					return nil
				},
			},
		},
	)

	if err := actions.SetVTOAudioOutputVolume(context.Background(), "front_vto", 1, 80); err != nil {
		t.Fatalf("SetVTOAudioOutputVolume returned error: %v", err)
	}
	if !called {
		t.Fatal("expected output volume controller to be called")
	}
}

func TestAdminActionsSetVTORecordingEnabled(t *testing.T) {
	called := false
	actions := newAdminActions(
		zerolog.Nop(),
		metrics.New(buildinfo.BuildInfo{}),
		store.NewProbeStore(),
		&stubDeviceConfigStore{},
		[]dahua.Driver{
			stubDriver{
				id:   "front_vto",
				kind: dahua.DeviceKindVTO,
				vtoRecordFn: func(_ context.Context, enabled bool) error {
					called = true
					if !enabled {
						t.Fatal("expected enabled=true")
					}
					return nil
				},
			},
		},
	)

	if err := actions.SetVTORecordingEnabled(context.Background(), "front_vto", true); err != nil {
		t.Fatalf("SetVTORecordingEnabled returned error: %v", err)
	}
	if !called {
		t.Fatal("expected recording controller to be called")
	}
}
