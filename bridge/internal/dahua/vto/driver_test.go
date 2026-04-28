package vto

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
	"RCooLeR/DahuaBridge/internal/metrics"
	"github.com/rs/zerolog"
)

func TestParseVTOEncode(t *testing.T) {
	values := map[string]string{
		"table.Encode[0].MainFormat[0].Video.resolution":   "1280x720",
		"table.Encode[0].MainFormat[0].Video.Compression":  "H.265",
		"table.Encode[0].ExtraFormat[0].Video.resolution":  "1920x1080",
		"table.Encode[0].ExtraFormat[0].Video.Compression": "H.265",
		"table.Encode[0].MainFormat[0].Audio.Compression":  "PCM",
	}

	mainRes, mainCodec, subRes, subCodec, audioCodec := parseVTOEncode(values)

	if mainRes != "1280x720" || mainCodec != "H.265" {
		t.Fatalf("unexpected main format: %s %s", mainRes, mainCodec)
	}
	if subRes != "1920x1080" || subCodec != "H.265" {
		t.Fatalf("unexpected sub format: %s %s", subRes, subCodec)
	}
	if audioCodec != "PCM" {
		t.Fatalf("unexpected audio codec: %s", audioCodec)
	}
}

func TestParseVTOLocks(t *testing.T) {
	values := map[string]string{
		"table.AccessControl[0].Name":               "Door1",
		"table.AccessControl[0].State":              "Normal",
		"table.AccessControl[0].SensorEnable":       "false",
		"table.AccessControl[0].LockMode":           "2",
		"table.AccessControl[0].UnlockHoldInterval": "2",
	}

	locks := parseVTOLocks(values)
	if len(locks) != 1 {
		t.Fatalf("expected 1 lock, got %d", len(locks))
	}
	if locks[0].Name != "Door1" || locks[0].State != "Normal" {
		t.Fatalf("unexpected lock inventory: %+v", locks[0])
	}
}

func TestParseVTOAlarms(t *testing.T) {
	values := map[string]string{
		"table.Alarm[3].Name":        "Nonamed",
		"table.Alarm[3].SenseMethod": "Button",
		"table.Alarm[3].Enable":      "true",
	}

	alarms := parseVTOAlarms(values)
	if len(alarms) != 1 {
		t.Fatalf("expected 1 alarm, got %d", len(alarms))
	}
	if alarms[0].SenseMethod != "Button" || !alarms[0].Enabled {
		t.Fatalf("unexpected alarm inventory: %+v", alarms[0])
	}
}

func TestFilterVTOLocks(t *testing.T) {
	cfg := config.DeviceConfig{LockAllowlist: []int{1}}
	locks := []lockInventory{
		{Index: 0, Name: "Door1"},
		{Index: 1, Name: "Door2"},
		{Index: 2, Name: "Door3"},
	}

	filtered := filterVTOLocks(cfg, locks)
	if len(filtered) != 1 || filtered[0].Index != 0 {
		t.Fatalf("unexpected filtered locks %+v", filtered)
	}
}

func TestFilterVTOAlarms(t *testing.T) {
	cfg := config.DeviceConfig{AlarmAllowlist: []int{1, 4}}
	alarms := []alarmInventory{
		{Index: 0, Name: "Alarm1"},
		{Index: 1, Name: "Alarm2"},
		{Index: 3, Name: "Alarm4"},
	}

	filtered := filterVTOAlarms(cfg, alarms)
	if len(filtered) != 2 || filtered[0].Index != 0 || filtered[1].Index != 3 {
		t.Fatalf("unexpected filtered alarms %+v", filtered)
	}
}

func TestNormalizeEvent(t *testing.T) {
	event, ok := normalizeEvent("west20_vto", map[string]string{
		"Code":   "AlarmLocal",
		"action": "Start",
		"index":  "3",
	})
	if !ok {
		t.Fatal("expected event to normalize")
	}
	if event.ChildID != "west20_vto_alarm_03" {
		t.Fatalf("child id mismatch: %q", event.ChildID)
	}
	if event.Channel != 4 {
		t.Fatalf("channel mismatch: %d", event.Channel)
	}
}

func TestBuildVTOStreamURL(t *testing.T) {
	got := buildVTOStreamURL("http://vto.example.local", 1)
	want := "rtsp://vto.example.local:554/cam/realmonitor?channel=1&subtype=1"
	if got != want {
		t.Fatalf("rtsp url mismatch:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestEventTypeForAppCanonicalizesAccessCtl(t *testing.T) {
	eventType := EventTypeForApp(dahua.Event{
		Code:   "AccessCtl",
		Action: dahua.EventActionStart,
	})

	if eventType != "accesscontrol_start" {
		t.Fatalf("unexpected event type: %q", eventType)
	}
}

func TestSessionStateUpdatesForAppTracksCallLifecycle(t *testing.T) {
	info := map[string]any{
		"call_state": "idle",
	}
	startedAt := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(45 * time.Second)

	startUpdates := SessionStateUpdatesForApp(info, dahua.Event{
		Code:       "Call",
		Action:     dahua.EventActionStart,
		OccurredAt: startedAt,
		Data: map[string]string{
			"CallSrc": "Villa-01",
		},
	})
	if startUpdates["call_state"] != "ringing" {
		t.Fatalf("unexpected call start state: %+v", startUpdates)
	}
	if startUpdates["last_call_started_at"] != startedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected call start timestamp: %+v", startUpdates)
	}
	if startUpdates["last_call_source"] != "Villa-01" {
		t.Fatalf("unexpected call source: %+v", startUpdates)
	}

	stopUpdates := SessionStateUpdatesForApp(info, dahua.Event{
		Code:       "Call",
		Action:     dahua.EventActionStop,
		OccurredAt: endedAt,
	})
	if stopUpdates["call_state"] != "idle" {
		t.Fatalf("unexpected call stop state: %+v", stopUpdates)
	}
	if stopUpdates["last_call_ended_at"] != endedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected call stop timestamp: %+v", stopUpdates)
	}
	if stopUpdates["last_call_duration_seconds"] != "45" {
		t.Fatalf("unexpected call duration update: %+v", stopUpdates)
	}
	if _, ok := info["call_started_at"]; ok {
		t.Fatalf("expected call_started_at to be cleared, got %+v", info)
	}
	if info["call_state"] != "idle" {
		t.Fatalf("expected stored call state to be idle, got %+v", info)
	}
}

func TestSessionStateUpdatesForAppTracksDoorbellRing(t *testing.T) {
	info := map[string]any{
		"call_state": "idle",
	}
	occurredAt := time.Date(2026, 4, 27, 12, 1, 0, 0, time.UTC)

	updates := SessionStateUpdatesForApp(info, dahua.Event{
		Code:       "DoorBell",
		Action:     dahua.EventActionStart,
		OccurredAt: occurredAt,
		Data: map[string]string{
			"Source": "Front Gate",
		},
	})
	if updates["last_ring_at"] != occurredAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected ring timestamp: %+v", updates)
	}
	if updates["last_call_source"] != "Front Gate" {
		t.Fatalf("unexpected ring source: %+v", updates)
	}
}

func TestDriverHangupCallUsesConsoleRunCmd(t *testing.T) {
	var command string
	var loginPassword string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch r.URL.Path {
		case "/RPC2_Login":
			session, _ := payload["session"].(float64)
			if session == 0 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  false,
					"session": 123,
					"params": map[string]any{
						"realm":      "Login to Dahua",
						"random":     "abc123",
						"encryption": "Default",
					},
					"error": map[string]any{
						"code":    401,
						"message": "challenge",
					},
				})
				return
			}
			params, ok := payload["params"].(map[string]any)
			if !ok {
				t.Fatalf("unexpected login params payload: %+v", payload)
			}
			loginPassword, _ = params["password"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      payload["id"],
				"result":  true,
				"session": 123,
			})
		case "/RPC2":
			switch payload["method"] {
			case "VideoTalkPhone.factory.instance":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  20737192,
					"session": 123,
				})
			case "VideoTalkPhone.getCallState":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  true,
					"session": 123,
					"params": map[string]any{
						"callState": "Idle",
					},
				})
			case "console.runCmd":
				params, ok := payload["params"].(map[string]any)
				if !ok {
					t.Fatalf("unexpected params payload: %+v", payload)
				}
				command, _ = params["command"].(string)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  true,
					"session": 123,
				})
			default:
				t.Fatalf("unexpected rpc method: %+v", payload)
			}
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:             "front_vto",
		BaseURL:        server.URL,
		Username:       "admin",
		Password:       "secret",
		RequestTimeout: 5 * time.Second,
	}
	rpc, err := newRPCClient(cfg)
	if err != nil {
		t.Fatalf("newRPCClient returned error: %v", err)
	}

	driver := &Driver{
		cfg:    cfg,
		rpc:    rpc,
		logger: zerolog.Nop(),
	}

	if err := driver.HangupCall(context.Background()); err != nil {
		t.Fatalf("HangupCall returned error: %v", err)
	}
	if command != "hc" {
		t.Fatalf("unexpected console command %q", command)
	}
	if loginPassword != uppercaseMD5("admin:abc123:"+uppercaseMD5("admin:Login to Dahua:secret")) {
		t.Fatalf("unexpected rpc login password %q", loginPassword)
	}
}

func TestDriverHangupCallUsesVideoTalkPhoneDisconnectWhenActive(t *testing.T) {
	var usedConsole bool
	var disconnectObjectID float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch r.URL.Path {
		case "/RPC2_Login":
			session, _ := payload["session"].(float64)
			if session == 0 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  false,
					"session": 123,
					"params": map[string]any{
						"realm":      "Login to Dahua",
						"random":     "abc123",
						"encryption": "Default",
					},
					"error": map[string]any{
						"code":    401,
						"message": "challenge",
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      payload["id"],
				"result":  true,
				"session": 123,
			})
		case "/RPC2":
			switch payload["method"] {
			case "VideoTalkPhone.factory.instance":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  20737192,
					"session": 123,
				})
			case "VideoTalkPhone.getCallState":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  true,
					"session": 123,
					"params": map[string]any{
						"callState": "Talking",
					},
				})
			case "VideoTalkPhone.disconnect":
				disconnectObjectID, _ = payload["object"].(float64)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  true,
					"session": 123,
				})
			case "console.runCmd":
				usedConsole = true
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  true,
					"session": 123,
				})
			default:
				t.Fatalf("unexpected rpc method: %+v", payload)
			}
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:             "front_vto",
		BaseURL:        server.URL,
		Username:       "admin",
		Password:       "secret",
		RequestTimeout: 5 * time.Second,
	}
	rpc, err := newRPCClient(cfg)
	if err != nil {
		t.Fatalf("newRPCClient returned error: %v", err)
	}

	driver := &Driver{
		cfg:    cfg,
		rpc:    rpc,
		logger: zerolog.Nop(),
	}

	if err := driver.HangupCall(context.Background()); err != nil {
		t.Fatalf("HangupCall returned error: %v", err)
	}
	if disconnectObjectID != 20737192 {
		t.Fatalf("unexpected VideoTalkPhone object id %.0f", disconnectObjectID)
	}
	if usedConsole {
		t.Fatal("expected hangup to avoid console fallback when disconnect succeeds")
	}
}

func TestDriverAnswerCallUsesVideoTalkPhoneService(t *testing.T) {
	var answerObjectID float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch r.URL.Path {
		case "/RPC2_Login":
			session, _ := payload["session"].(float64)
			if session == 0 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  false,
					"session": 123,
					"params": map[string]any{
						"realm":      "Login to Dahua",
						"random":     "abc123",
						"encryption": "Default",
					},
					"error": map[string]any{
						"code":    401,
						"message": "challenge",
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      payload["id"],
				"result":  true,
				"session": 123,
			})
		case "/RPC2":
			switch payload["method"] {
			case "VideoTalkPhone.factory.instance":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  20737192,
					"session": 123,
				})
			case "VideoTalkPhone.answer":
				answerObjectID, _ = payload["object"].(float64)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      payload["id"],
					"result":  true,
					"session": 123,
				})
			default:
				t.Fatalf("unexpected rpc method: %+v", payload)
			}
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:             "front_vto",
		BaseURL:        server.URL,
		Username:       "admin",
		Password:       "secret",
		RequestTimeout: 5 * time.Second,
	}
	rpc, err := newRPCClient(cfg)
	if err != nil {
		t.Fatalf("newRPCClient returned error: %v", err)
	}

	driver := &Driver{
		cfg:    cfg,
		rpc:    rpc,
		logger: zerolog.Nop(),
	}

	if err := driver.AnswerCall(context.Background()); err != nil {
		t.Fatalf("AnswerCall returned error: %v", err)
	}
	if answerObjectID != 20737192 {
		t.Fatalf("unexpected VideoTalkPhone object id %.0f", answerObjectID)
	}
}

func TestDriverProbeCachesStaticMetadata(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[requestKey(r)]++
		mu.Unlock()

		switch r.URL.Path {
		case "/cgi-bin/magicBox.cgi":
			switch r.URL.Query().Get("action") {
			case "getSystemInfo":
				fmt.Fprint(w, "deviceType=VTO\nprocessor=ARM\nserialNumber=SN123\nupdateSerial=VTO2311\n")
			case "getMachineName":
				fmt.Fprint(w, "name=Front Gate\n")
			case "getSoftwareVersion":
				fmt.Fprint(w, "version=1.2.3, build:2026-04-27\n")
			default:
				t.Fatalf("unexpected magicBox action: %s", r.URL.RawQuery)
			}
		case "/cgi-bin/configManager.cgi":
			switch r.URL.Query().Get("name") {
			case "AccessControl":
				fmt.Fprint(w, "table.AccessControl[0].Name=Door1\ntable.AccessControl[0].State=Normal\ntable.AccessControl[0].SensorEnable=false\ntable.AccessControl[0].LockMode=2\ntable.AccessControl[0].UnlockHoldInterval=2\n")
			case "CommGlobal":
				fmt.Fprint(w, "table.CommGlobal.CurrentProfile=Villa\ntable.CommGlobal.AlarmEnable=true\n")
			case "Alarm":
				fmt.Fprint(w, "table.Alarm[0].Name=Alarm1\ntable.Alarm[0].SenseMethod=Button\ntable.Alarm[0].Enable=true\n")
			case "Encode":
				fmt.Fprint(w, "table.Encode[0].MainFormat[0].Video.resolution=1280x720\ntable.Encode[0].MainFormat[0].Video.Compression=H.264\ntable.Encode[0].ExtraFormat[0].Video.resolution=640x480\ntable.Encode[0].ExtraFormat[0].Video.Compression=H.264\ntable.Encode[0].MainFormat[0].Audio.Compression=PCM\n")
			default:
				t.Fatalf("unexpected configManager name: %s", r.URL.RawQuery)
			}
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:             "front_vto",
		BaseURL:        server.URL,
		Username:       "admin",
		Password:       "secret",
		RequestTimeout: 2 * time.Second,
		PollInterval:   30 * time.Second,
	}
	driver := New(cfg, zerolog.Nop(), cgi.New(cfg, metrics.New(buildinfo.BuildInfo{})))

	result1, err := driver.Probe(context.Background())
	if err != nil {
		t.Fatalf("first probe returned error: %v", err)
	}
	result2, err := driver.Probe(context.Background())
	if err != nil {
		t.Fatalf("second probe returned error: %v", err)
	}

	if result1.Root.Name != "Front Gate" || result2.Root.Name != "Front Gate" {
		t.Fatalf("unexpected probe names: %q / %q", result1.Root.Name, result2.Root.Name)
	}
	if got := len(result2.Children); got != 2 {
		t.Fatalf("expected cached probe to preserve children, got %d", got)
	}

	for _, key := range []string{
		"magicBox:getSystemInfo",
		"magicBox:getMachineName",
		"magicBox:getSoftwareVersion",
		"config:AccessControl",
		"config:CommGlobal",
		"config:Alarm",
		"config:Encode",
	} {
		mu.Lock()
		got := counts[key]
		mu.Unlock()
		if got != 1 {
			t.Fatalf("expected %s to be fetched once, got %d", key, got)
		}
	}
}

func TestDriverProbeUsesStaleMetadataOnRefreshFailure(t *testing.T) {
	var mu sync.Mutex
	failAccess := false
	counts := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := requestKey(r)
		mu.Lock()
		counts[key]++
		shouldFailAccess := failAccess
		mu.Unlock()

		switch r.URL.Path {
		case "/cgi-bin/magicBox.cgi":
			switch r.URL.Query().Get("action") {
			case "getSystemInfo":
				fmt.Fprint(w, "deviceType=VTO\nprocessor=ARM\nserialNumber=SN123\nupdateSerial=VTO2311\n")
			case "getMachineName":
				fmt.Fprint(w, "name=Front Gate\n")
			case "getSoftwareVersion":
				fmt.Fprint(w, "version=1.2.3, build:2026-04-27\n")
			default:
				t.Fatalf("unexpected magicBox action: %s", r.URL.RawQuery)
			}
		case "/cgi-bin/configManager.cgi":
			switch r.URL.Query().Get("name") {
			case "AccessControl":
				if shouldFailAccess {
					http.Error(w, "timeout-ish failure", http.StatusGatewayTimeout)
					return
				}
				fmt.Fprint(w, "table.AccessControl[0].Name=Door1\ntable.AccessControl[0].State=Normal\ntable.AccessControl[0].SensorEnable=false\ntable.AccessControl[0].LockMode=2\ntable.AccessControl[0].UnlockHoldInterval=2\n")
			case "CommGlobal":
				fmt.Fprint(w, "table.CommGlobal.CurrentProfile=Villa\ntable.CommGlobal.AlarmEnable=true\n")
			case "Alarm":
				fmt.Fprint(w, "table.Alarm[0].Name=Alarm1\ntable.Alarm[0].SenseMethod=Button\ntable.Alarm[0].Enable=true\n")
			case "Encode":
				fmt.Fprint(w, "table.Encode[0].MainFormat[0].Video.resolution=1280x720\ntable.Encode[0].MainFormat[0].Video.Compression=H.264\ntable.Encode[0].ExtraFormat[0].Video.resolution=640x480\ntable.Encode[0].ExtraFormat[0].Video.Compression=H.264\ntable.Encode[0].MainFormat[0].Audio.Compression=PCM\n")
			default:
				t.Fatalf("unexpected configManager name: %s", r.URL.RawQuery)
			}
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:             "front_vto",
		BaseURL:        server.URL,
		Username:       "admin",
		Password:       "secret",
		RequestTimeout: 2 * time.Second,
		PollInterval:   30 * time.Second,
	}
	driver := New(cfg, zerolog.Nop(), cgi.New(cfg, metrics.New(buildinfo.BuildInfo{})))

	first, err := driver.Probe(context.Background())
	if err != nil {
		t.Fatalf("first probe returned error: %v", err)
	}
	if first.Root.Attributes["lock_count"] != "1" {
		t.Fatalf("expected initial lock count 1, got %q", first.Root.Attributes["lock_count"])
	}

	driver.mu.Lock()
	driver.probeCacheExpiry = time.Now().Add(-time.Second)
	driver.mu.Unlock()

	mu.Lock()
	failAccess = true
	mu.Unlock()

	second, err := driver.Probe(context.Background())
	if err != nil {
		t.Fatalf("probe with stale metadata fallback returned error: %v", err)
	}
	if second.Root.Attributes["lock_count"] != "1" {
		t.Fatalf("expected stale lock count 1, got %q", second.Root.Attributes["lock_count"])
	}
	if got := second.States["front_vto_lock_00"].Info["state"]; got != "Normal" {
		t.Fatalf("expected stale lock state to survive refresh failure, got %+v", got)
	}

	mu.Lock()
	got := counts["config:AccessControl"]
	mu.Unlock()
	if got != 2 {
		t.Fatalf("expected AccessControl refresh attempt count 2, got %d", got)
	}
}

func uppercaseMD5(value string) string {
	sum := md5.Sum([]byte(value))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func requestKey(r *http.Request) string {
	if r.URL.Path == "/cgi-bin/magicBox.cgi" {
		return "magicBox:" + r.URL.Query().Get("action")
	}
	if r.URL.Path == "/cgi-bin/configManager.cgi" {
		return "config:" + r.URL.Query().Get("name")
	}
	return r.URL.Path
}
