package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/ha"
	"RCooLeR/DahuaBridge/internal/haapi"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/mqtt"
	"RCooLeR/DahuaBridge/internal/store"
	"github.com/rs/zerolog"
)

type stubDriver struct {
	id           string
	kind         dahua.DeviceKind
	probeFn      func(context.Context) (*dahua.ProbeResult, error)
	unlockFn     func(context.Context, int) error
	answerFn     func(context.Context) error
	hangupFn     func(context.Context) error
	invalidateFn func()
	updateCfgFn  func(config.DeviceConfig) error
}

func (s stubDriver) ID() string { return s.id }

func (s stubDriver) Kind() dahua.DeviceKind { return s.kind }

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
var _ dahua.NVRInventoryRefresher = stubDriver{}
var _ dahua.ConfigurableDriver = stubDriver{}

type mockMQTTClient struct {
	published        []publishedMessage
	subscribedTopic  string
	subscribedQoS    byte
	subscribeHandler func(string, []byte)
}

type publishedMessage struct {
	topic   string
	payload []byte
}

type stubDeviceConfigStore struct {
	items        map[string]config.DeviceConfig
	onvifTargets []haapi.ONVIFProvisionTarget
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

func (s *stubDeviceConfigStore) ListONVIFProvisionTargets(deviceIDs []string, force bool) []haapi.ONVIFProvisionTarget {
	return append([]haapi.ONVIFProvisionTarget(nil), s.onvifTargets...)
}

type stubHAProvisioner struct {
	enabled     bool
	provisionFn func(context.Context, haapi.ONVIFProvisionTarget) (haapi.ONVIFProvisionResult, error)
}

func (s stubHAProvisioner) Enabled() bool {
	return s.enabled
}

func (s stubHAProvisioner) ProvisionONVIF(ctx context.Context, target haapi.ONVIFProvisionTarget) (haapi.ONVIFProvisionResult, error) {
	if s.provisionFn == nil {
		return haapi.ONVIFProvisionResult{}, nil
	}
	return s.provisionFn(ctx, target)
}

func (m *mockMQTTClient) Connect(context.Context) error { return nil }

func (m *mockMQTTClient) Publish(_ context.Context, topic string, _ byte, _ bool, payload []byte) error {
	cloned := make([]byte, len(payload))
	copy(cloned, payload)
	m.published = append(m.published, publishedMessage{topic: topic, payload: cloned})
	return nil
}

func (m *mockMQTTClient) PublishJSON(ctx context.Context, topic string, qos byte, retain bool, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return m.Publish(ctx, topic, qos, retain, data)
}

func (m *mockMQTTClient) Subscribe(_ context.Context, topic string, qos byte, handler func(string, []byte)) error {
	m.subscribedTopic = topic
	m.subscribedQoS = qos
	m.subscribeHandler = handler
	return nil
}

func (m *mockMQTTClient) Close() {}

var _ mqtt.Client = (*mockMQTTClient)(nil)

func TestAdminActionsProbeDeviceStoresAndPublishes(t *testing.T) {
	store := store.NewProbeStore()
	metricsRegistry := metrics.New(buildinfo.Info())
	mqttClient := &mockMQTTClient{}
	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}

	driver := stubDriver{
		id:   "front_vto",
		kind: dahua.DeviceKindVTO,
		probeFn: func(context.Context) (*dahua.ProbeResult, error) {
			return &dahua.ProbeResult{
				Root: dahua.Device{
					ID:   "front_vto",
					Name: "Front Door",
					Kind: dahua.DeviceKindVTO,
				},
				States: map[string]dahua.DeviceState{
					"front_vto": {Available: true},
				},
			}, nil
		},
	}

	actions := newAdminActions(
		zerolog.Nop(),
		metricsRegistry,
		ha.NewDiscoveryPublisher(cfg, mqttClient, zerolog.Nop()),
		store,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"front_vto": {ID: "front_vto"}}},
		nil,
		[]dahua.Driver{driver},
	)

	result, err := actions.ProbeDevice(context.Background(), "front_vto")
	if err != nil {
		t.Fatalf("ProbeDevice returned error: %v", err)
	}
	if result == nil || result.Root.ID != "front_vto" {
		t.Fatalf("unexpected result: %+v", result)
	}

	stored, ok := store.Get("front_vto")
	if !ok || stored == nil {
		t.Fatal("expected probe result to be stored")
	}
	if stored.Root.Name != "Front Door" {
		t.Fatalf("unexpected stored result: %+v", stored.Root)
	}

	if !hasPublishedMessage(mqttClient.published, "dahuabridge/devices/front_vto/availability", "online") {
		t.Fatalf("expected online availability publish, got %+v", mqttClient.published)
	}
}

func TestAdminActionsProbeDeviceFailurePublishesUnavailable(t *testing.T) {
	store := store.NewProbeStore()
	metricsRegistry := metrics.New(buildinfo.Info())
	mqttClient := &mockMQTTClient{}
	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}

	driver := stubDriver{
		id:   "front_vto",
		kind: dahua.DeviceKindVTO,
		probeFn: func(context.Context) (*dahua.ProbeResult, error) {
			return nil, errors.New("probe failed")
		},
	}

	actions := newAdminActions(
		zerolog.Nop(),
		metricsRegistry,
		ha.NewDiscoveryPublisher(cfg, mqttClient, zerolog.Nop()),
		store,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"front_vto": {ID: "front_vto"}}},
		nil,
		[]dahua.Driver{driver},
	)

	if _, err := actions.ProbeDevice(context.Background(), "front_vto"); err == nil {
		t.Fatal("expected ProbeDevice to fail")
	}

	if _, ok := store.Get("front_vto"); ok {
		t.Fatal("did not expect failed probe to be stored")
	}
	if !hasPublishedMessage(mqttClient.published, "dahuabridge/devices/front_vto/availability", "offline") {
		t.Fatalf("expected offline availability publish, got %+v", mqttClient.published)
	}
}

func TestAdminActionsProbeAllDevices(t *testing.T) {
	store := store.NewProbeStore()
	metricsRegistry := metrics.New(buildinfo.Info())
	mqttClient := &mockMQTTClient{}
	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}

	actions := newAdminActions(
		zerolog.Nop(),
		metricsRegistry,
		ha.NewDiscoveryPublisher(cfg, mqttClient, zerolog.Nop()),
		store,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{
			"yard_ipc":  {ID: "yard_ipc"},
			"front_vto": {ID: "front_vto"},
		}},
		nil,
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

	gotOrder := []string{results[0].DeviceID, results[1].DeviceID}
	wantOrder := []string{"front_vto", "yard_ipc"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("unexpected result order: got %v want %v", gotOrder, wantOrder)
	}

	if results[0].Error == "" {
		t.Fatalf("expected first result to carry error: %+v", results[0])
	}
	if results[1].Result == nil || results[1].Result.Root.ID != "yard_ipc" {
		t.Fatalf("expected second result to carry probe result: %+v", results[1])
	}
}

func TestAdminActionsRefreshNVRInventory(t *testing.T) {
	store := store.NewProbeStore()
	metricsRegistry := metrics.New(buildinfo.Info())
	mqttClient := &mockMQTTClient{}
	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}

	invalidated := false
	actions := newAdminActions(
		zerolog.Nop(),
		metricsRegistry,
		ha.NewDiscoveryPublisher(cfg, mqttClient, zerolog.Nop()),
		store,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"west20_nvr": {ID: "west20_nvr"}}},
		nil,
		[]dahua.Driver{
			stubDriver{
				id:   "west20_nvr",
				kind: dahua.DeviceKindNVR,
				invalidateFn: func() {
					invalidated = true
				},
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
	if !invalidated {
		t.Fatal("expected inventory cache invalidation before reprobe")
	}
	if result == nil || result.Root.ID != "west20_nvr" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAdminActionsRotateDeviceCredentials(t *testing.T) {
	store := store.NewProbeStore()
	metricsRegistry := metrics.New(buildinfo.Info())
	mqttClient := &mockMQTTClient{}
	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}

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
		metricsRegistry,
		ha.NewDiscoveryPublisher(cfg, mqttClient, zerolog.Nop()),
		store,
		configStore,
		nil,
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
	if updated.BaseURL != "https://192.168.1.21" {
		t.Fatalf("expected updated base url, got %+v", updated)
	}
	if updated.Username != "service" || updated.Password != "new-secret" {
		t.Fatalf("expected rotated credentials, got %+v", updated)
	}
	if !updated.ONVIFEnabledValue() || updated.OnvifUsername != "onvif-user" || updated.OnvifPassword != "onvif-secret" {
		t.Fatalf("expected updated onvif config, got %+v", updated)
	}
	stored, ok := configStore.GetDeviceConfig("yard_ipc")
	if !ok || stored.Username != "service" || stored.Password != "new-secret" {
		t.Fatalf("expected updated stored config, got %+v ok=%v", stored, ok)
	}
}

func TestAdminActionsProvisionHomeAssistantONVIF(t *testing.T) {
	store := store.NewProbeStore()
	metricsRegistry := metrics.New(buildinfo.Info())
	mqttClient := &mockMQTTClient{}
	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}

	configStore := &stubDeviceConfigStore{
		items: map[string]config.DeviceConfig{"yard_ipc": {ID: "yard_ipc"}},
		onvifTargets: []haapi.ONVIFProvisionTarget{
			{
				DeviceID:   "yard_ipc",
				DeviceKind: dahua.DeviceKindIPC,
				Name:       "Yard Camera",
				Host:       "192.168.1.20",
				Port:       8999,
			},
		},
	}

	actions := newAdminActions(
		zerolog.Nop(),
		metricsRegistry,
		ha.NewDiscoveryPublisher(cfg, mqttClient, zerolog.Nop()),
		store,
		configStore,
		stubHAProvisioner{
			enabled: true,
			provisionFn: func(_ context.Context, target haapi.ONVIFProvisionTarget) (haapi.ONVIFProvisionResult, error) {
				if target.DeviceID != "yard_ipc" {
					t.Fatalf("unexpected target %+v", target)
				}
				return haapi.ONVIFProvisionResult{
					DeviceID:   target.DeviceID,
					DeviceKind: target.DeviceKind,
					Name:       target.Name,
					Host:       target.Host,
					Port:       target.Port,
					Status:     "created",
					EntryID:    "entry-123",
				}, nil
			},
		},
		nil,
	)

	results, err := actions.ProvisionHomeAssistantONVIF(context.Background(), haapi.ONVIFProvisionRequest{})
	if err != nil {
		t.Fatalf("ProvisionHomeAssistantONVIF returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "created" || results[0].EntryID != "entry-123" {
		t.Fatalf("unexpected results %+v", results)
	}
}

func TestAdminActionsHangupVTOCallPublishesIdleState(t *testing.T) {
	store := store.NewProbeStore()
	store.Set("front_vto", &dahua.ProbeResult{
		Root: dahua.Device{
			ID:   "front_vto",
			Kind: dahua.DeviceKindVTO,
		},
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

	metricsRegistry := metrics.New(buildinfo.Info())
	mqttClient := &mockMQTTClient{}
	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}

	called := false
	actions := newAdminActions(
		zerolog.Nop(),
		metricsRegistry,
		ha.NewDiscoveryPublisher(cfg, mqttClient, zerolog.Nop()),
		store,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"front_vto": {ID: "front_vto"}}},
		nil,
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

	stored, ok := store.Get("front_vto")
	if !ok || stored == nil {
		t.Fatal("expected updated probe result")
	}
	info := stored.States["front_vto"].Info
	if info["call_state"] != "idle" {
		t.Fatalf("expected idle call state, got %+v", info)
	}
	if info["call"] != false {
		t.Fatalf("expected call=false, got %+v", info)
	}
	if _, ok := info["last_call_ended_at"]; !ok {
		t.Fatalf("expected last_call_ended_at to be set, got %+v", info)
	}

	if !hasPublishedMessage(mqttClient.published, "dahuabridge/devices/front_vto/state/call", "OFF") {
		t.Fatalf("expected call OFF publish, got %+v", mqttClient.published)
	}
	if !hasPublishedMessage(mqttClient.published, "dahuabridge/devices/front_vto/state/call_state", "idle") {
		t.Fatalf("expected call_state idle publish, got %+v", mqttClient.published)
	}
}

func TestAdminActionsAnswerVTOCall(t *testing.T) {
	store := store.NewProbeStore()
	metricsRegistry := metrics.New(buildinfo.Info())
	mqttClient := &mockMQTTClient{}
	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix:     "dahuabridge",
			DiscoveryPrefix: "homeassistant",
			QoS:             1,
			Retain:          true,
		},
		HomeAssistant: config.HomeAssistantConfig{
			Enabled: true,
			NodeID:  "dahuabridge",
		},
	}

	called := false
	actions := newAdminActions(
		zerolog.Nop(),
		metricsRegistry,
		ha.NewDiscoveryPublisher(cfg, mqttClient, zerolog.Nop()),
		store,
		&stubDeviceConfigStore{items: map[string]config.DeviceConfig{"front_vto": {ID: "front_vto"}}},
		nil,
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

func hasPublishedMessage(messages []publishedMessage, topic string, payload string) bool {
	for _, message := range messages {
		if message.topic == topic && string(message.payload) == payload {
			return true
		}
	}
	return false
}
