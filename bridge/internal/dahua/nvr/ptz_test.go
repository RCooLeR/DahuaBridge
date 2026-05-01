package nvr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
	"RCooLeR/DahuaBridge/internal/metrics"
	"github.com/rs/zerolog"
)

func TestParsePTZCapabilities(t *testing.T) {
	capabilities := parsePTZCapabilities(map[string]string{
		"caps.Pan":                               "true",
		"caps.Tile":                              "true",
		"caps.Zoom":                              "false",
		"caps.Focus":                             "true",
		"caps.MoveRelatively":                    "true",
		"caps.AutoScan":                          "true",
		"caps.Preset":                            "true",
		"caps.Pattern":                           "false",
		"caps.Tour":                              "true",
		"caps.Aux":                               "true",
		"caps.Auxs[0]":                           "Light",
		"caps.Auxs[1]":                           "Wiper",
		"caps.PanSpeedMin":                       "1",
		"caps.PanSpeedMax":                       "8",
		"caps.TileSpeedMin":                      "1",
		"caps.TileSpeedMax":                      "8",
		"caps.PresetMin":                         "1",
		"caps.PresetMax":                         "300",
		"caps.PtzMotionRange.HorizontalAngle[0]": "0",
		"caps.PtzMotionRange.HorizontalAngle[1]": "360",
		"caps.PtzMotionRange.VerticalAngle[0]":   "-15",
		"caps.PtzMotionRange.VerticalAngle[1]":   "100",
	})

	if !capabilities.Supported || !capabilities.Pan || !capabilities.Tilt || capabilities.Zoom {
		t.Fatalf("unexpected capabilities %+v", capabilities)
	}
	if len(capabilities.AuxFunctions) != 2 || capabilities.AuxFunctions[0] != "Light" {
		t.Fatalf("unexpected aux functions %+v", capabilities.AuxFunctions)
	}
	if len(capabilities.Commands) == 0 {
		t.Fatalf("expected commands to be populated: %+v", capabilities)
	}
}

func TestParsePTZCapabilitiesAuxOnlySurfaceIsNotPTZSupported(t *testing.T) {
	capabilities := parsePTZCapabilities(map[string]string{
		"caps.Pan":      "false",
		"caps.Tile":     "false",
		"caps.Zoom":     "false",
		"caps.Focus":    "false",
		"caps.Aux":      "true",
		"caps.Auxs[0]":  "Light",
		"caps.Auxs[1]":  "Wiper",
		"caps.AutoScan": "false",
	})

	if capabilities.Supported {
		t.Fatalf("expected aux-only ptz surface to stay unsupported, got %+v", capabilities)
	}
	if !capabilities.Aux || len(capabilities.AuxFunctions) != 2 {
		t.Fatalf("expected aux metadata to be preserved, got %+v", capabilities)
	}
}

func TestAttachChannelControlState(t *testing.T) {
	state := dahua.DeviceState{Available: true, Info: map[string]any{}}
	attachChannelControlState(&state, dahua.NVRChannelControlCapabilities{
		DeviceID: "west20_nvr",
		Channel:  5,
		PTZ: dahua.NVRPTZCapabilities{
			Supported:    true,
			Pan:          true,
			Tilt:         true,
			Zoom:         true,
			Focus:        true,
			Commands:     []string{"left", "right", "up"},
			AuxFunctions: []string{"Light", "Wiper"},
		},
		Aux: dahua.NVRAuxCapabilities{
			Supported: true,
			Outputs:   []string{"aux", "light", "wiper"},
			Features:  []string{"siren", "warning_light", "wiper"},
		},
	})

	if supported, _ := state.Info["control_ptz_supported"].(bool); !supported {
		t.Fatalf("expected ptz support in state %+v", state.Info)
	}
	outputs, ok := state.Info["control_aux_outputs"].([]string)
	if !ok || len(outputs) != 3 || outputs[0] != "aux" || outputs[1] != "light" || outputs[2] != "wiper" {
		t.Fatalf("unexpected aux outputs %#v", state.Info["control_aux_outputs"])
	}
	features, ok := state.Info["control_aux_features"].([]string)
	if !ok || len(features) != 3 || features[0] != "siren" {
		t.Fatalf("unexpected aux features %#v", state.Info["control_aux_features"])
	}
}

func TestAuxCapabilitiesPreferDirectLightOverAuxOnlyPTZSurface(t *testing.T) {
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID: "west20_nvr",
			DirectIPCCredentials: []config.ChannelDirectIPCCredential{
				{
					NVRChannel:        9,
					DirectIPCIP:       "192.168.150.110",
					DirectIPCUser:     "admin",
					DirectIPCPassword: "secret",
				},
			},
		},
		cachedInventory: &inventorySnapshot{
			Channels: []channelInventory{
				{
					Index: 8,
					RemoteDevice: remoteDeviceInventory{
						Address:    "192.168.150.110",
						DeviceType: "DH-IPC-HFW2849S-S-IL-BE",
					},
				},
			},
		},
		inventoryExpires: time.Now().Add(time.Minute),
	}

	capabilities, err := driver.auxCapabilities(context.Background(), 9, dahua.NVRPTZCapabilities{
		Aux:          true,
		AuxFunctions: []string{"Light", "Wiper"},
	})
	if err != nil {
		t.Fatalf("auxCapabilities returned error: %v", err)
	}
	if !capabilities.Supported {
		t.Fatalf("expected aux support, got %+v", capabilities)
	}
	if len(capabilities.Outputs) != 1 || capabilities.Outputs[0] != "light" {
		t.Fatalf("expected direct light only, got %+v", capabilities.Outputs)
	}
	if len(capabilities.Features) != 1 || capabilities.Features[0] != "light" {
		t.Fatalf("expected direct light feature only, got %+v", capabilities.Features)
	}
}

func TestParseChannelStreamsIncludesAudioConfig(t *testing.T) {
	streams := parseChannelStreams(map[string]string{
		"table.Encode[4].MainFormat[0].Video.resolution":   "2560x1440",
		"table.Encode[4].MainFormat[0].Video.Compression":  "H.265",
		"table.Encode[4].MainFormat[0].Audio.Compression":  "AAC",
		"table.Encode[4].MainFormat[0].AudioEnable":        "true",
		"table.Encode[4].ExtraFormat[0].Video.resolution":  "704x576",
		"table.Encode[4].ExtraFormat[0].Video.Compression": "H.264",
	})

	channel := streams[4]
	if channel.MainResolution != "2560x1440" || channel.MainCodec != "H.265" {
		t.Fatalf("unexpected parsed channel stream %+v", channel)
	}
	if channel.AudioCodec != "AAC" || !channel.AudioKnown || !channel.AudioEnabled {
		t.Fatalf("expected audio config to be parsed, got %+v", channel)
	}
}

func TestAudioCapabilitiesHideReadOnlyNVRMuteControl(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getConfig":
			switch r.URL.Query().Get("name") {
			case "RecordMode":
				_, _ = w.Write([]byte("table.RecordMode[4].Mode=0\ntable.RecordMode[4].ModeExtra1=2\ntable.RecordMode[4].ModeExtra2=2\n"))
			default:
				http.Error(w, "unexpected config", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{5},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.cachedInventory = &inventorySnapshot{
		Channels: []channelInventory{
			{
				Index:        4,
				AudioCodec:   "G.711A",
				AudioEnabled: true,
				AudioKnown:   true,
			},
		},
	}
	driver.inventoryExpires = time.Now().Add(time.Minute)
	driver.rpc = nil

	capabilities, notes := driver.audioCapabilities(context.Background(), 5)
	if capabilities.Mute {
		t.Fatalf("expected mute control to stay hidden, got %+v", capabilities)
	}
	if !capabilities.Supported || !capabilities.StreamEnabled || capabilities.Muted {
		t.Fatalf("expected stream audio state to remain visible, got %+v", capabilities)
	}
	if authority := driver.audioControlAuthority(context.Background(), 5); authority != "bridge_transcode" {
		t.Fatalf("expected bridge_transcode authority, got %q", authority)
	}
	if !strings.Contains(strings.Join(notes, ","), "channel_audio_transcode_managed_by_bridge") {
		t.Fatalf("expected bridge-transcode note, got %+v", notes)
	}
}

func TestAudioCapabilitiesHideNVRMuteWhenEncodeWriteDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getConfig":
			switch r.URL.Query().Get("name") {
			case "ChannelTitle":
				_, _ = w.Write([]byte("table.ChannelTitle[4].Name=Boiler Room\n"))
			case "Encode":
				_, _ = w.Write([]byte("table.Encode[4].MainFormat[0].Video.resolution=2560x1440\ntable.Encode[4].MainFormat[0].Audio.Compression=AAC\ntable.Encode[4].MainFormat[0].AudioEnable=true\n"))
			case "RecordMode":
				_, _ = w.Write([]byte("table.RecordMode[4].Mode=0\ntable.RecordMode[4].ModeExtra1=2\ntable.RecordMode[4].ModeExtra2=2\n"))
			case "RemoteDevice":
				_, _ = w.Write([]byte(""))
			default:
				http.Error(w, "unexpected config", http.StatusBadRequest)
			}
		case "setConfig":
			http.Error(w, "Authority:check failure.", http.StatusForbidden)
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:                "west20_nvr",
		BaseURL:           server.URL,
		Username:          "assistant",
		Password:          "secret",
		ChannelAllowlist:  []int{5},
		AllowConfigWrites: true,
		RequestTimeout:    5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	capabilities, notes := driver.audioCapabilities(context.Background(), 5)
	if capabilities.Mute || !capabilities.Supported || !capabilities.StreamEnabled || capabilities.Muted {
		t.Fatalf("expected denied encode write to keep bridge-managed audio metadata only, got %+v", capabilities)
	}
	if authority := driver.audioControlAuthority(context.Background(), 5); authority != "bridge_transcode" {
		t.Fatalf("expected bridge_transcode authority, got %q", authority)
	}
	if !strings.Contains(strings.Join(notes, ","), "channel_audio_transcode_managed_by_bridge") {
		t.Fatalf("expected bridge-transcode note, got %+v", notes)
	}
}

func TestRecordingCapabilitiesHideReadOnlyNVRWrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getConfig":
			switch r.URL.Query().Get("name") {
			case "RecordMode":
				_, _ = w.Write([]byte("table.RecordMode[4].Mode=0\ntable.RecordMode[4].ModeExtra1=2\ntable.RecordMode[4].ModeExtra2=2\n"))
			default:
				http.Error(w, "unexpected config", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{5},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	capabilities, err := driver.recordingCapabilities(context.Background(), 5)
	if err != nil {
		t.Fatalf("recordingCapabilities returned error: %v", err)
	}
	if capabilities.Supported {
		t.Fatalf("expected read-only recording control to stay hidden, got %+v", capabilities)
	}
	if capabilities.Mode != "auto" {
		t.Fatalf("expected recording mode to remain readable, got %+v", capabilities)
	}
}

func TestDriverPTZSendsExpectedQueries(t *testing.T) {
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())
		switch r.URL.Query().Get("action") {
		case "getCurrentProtocolCaps":
			_, _ = w.Write([]byte("caps.Pan=true\ncaps.Tile=true\ncaps.Zoom=true\ncaps.Focus=true\ncaps.Aux=true\ncaps.Auxs[0]=Light\ncaps.Auxs[1]=Wiper\ncaps.PanSpeedMin=1\ncaps.PanSpeedMax=8\ncaps.TileSpeedMin=1\ncaps.TileSpeedMax=8\n"))
		case "getConfig":
			_, _ = w.Write([]byte("table.RecordMode[4].Mode=0\ntable.RecordMode[4].ModeExtra1=2\ntable.RecordMode[4].ModeExtra2=2\n"))
		case "start", "stop":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{5},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	err := driver.PTZ(context.Background(), dahua.NVRPTZRequest{
		Channel:  5,
		Action:   dahua.NVRPTZActionPulse,
		Command:  dahua.NVRPTZCommandLeft,
		Speed:    2,
		Duration: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("PTZ returned error: %v", err)
	}

	if len(requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requests))
	}
	startRequest := requests[len(requests)-2]
	stopRequest := requests[len(requests)-1]
	if startRequest.Get("action") != "start" || startRequest.Get("code") != "Left" || startRequest.Get("arg2") != "2" {
		t.Fatalf("unexpected start request: %+v", startRequest)
	}
	if stopRequest.Get("action") != "stop" || stopRequest.Get("code") != "Left" {
		t.Fatalf("unexpected stop request: %+v", stopRequest)
	}
}

func TestDriverAuxSendsExpectedQueries(t *testing.T) {
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())
		switch r.URL.Query().Get("action") {
		case "getCurrentProtocolCaps":
			_, _ = w.Write([]byte("caps.Aux=true\ncaps.Auxs[0]=Light\ncaps.Auxs[1]=Wiper\n"))
		case "start", "stop":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:                "west20_nvr",
		BaseURL:           server.URL,
		Username:          "assistant",
		Password:          "secret",
		ChannelAllowlist:  []int{11},
		AllowConfigWrites: true,
		RequestTimeout:    5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	err := driver.Aux(context.Background(), dahua.NVRAuxRequest{
		Channel:  11,
		Action:   dahua.NVRAuxActionPulse,
		Output:   "warning_light",
		Duration: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Aux returned error: %v", err)
	}

	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}
	if requests[1].Get("action") != "start" || requests[1].Get("code") != "Light" {
		t.Fatalf("unexpected start request: %+v", requests[1])
	}
	if requests[2].Get("action") != "stop" || requests[2].Get("code") != "Light" {
		t.Fatalf("unexpected stop request: %+v", requests[2])
	}
}

func TestDriverAuxLightUsesRPCModeSwitch(t *testing.T) {
	var ptzRequests []url.Values
	var rpcMethods []string
	var rpcParams []any
	loginRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/configManager.cgi":
			switch r.URL.Query().Get("action") {
			case "getConfig":
				if r.URL.Query().Get("name") == "RecordMode" {
					_, _ = w.Write([]byte("table.RecordMode[10].Mode=0\ntable.RecordMode[10].ModeExtra1=2\ntable.RecordMode[10].ModeExtra2=2\n"))
					return
				}
				http.Error(w, "unexpected config", http.StatusBadRequest)
			case "setConfig":
				_, _ = w.Write([]byte("OK"))
			default:
				http.Error(w, "unexpected action", http.StatusBadRequest)
			}
		case "/cgi-bin/ptz.cgi":
			ptzRequests = append(ptzRequests, r.URL.Query())
			switch r.URL.Query().Get("action") {
			case "getCurrentProtocolCaps":
				_, _ = w.Write([]byte("caps.Aux=true\ncaps.Auxs[0]=Light\n"))
			default:
				http.Error(w, "unexpected ptz action", http.StatusBadRequest)
			}
		case "/RPC2_Login":
			loginRequests++
			if loginRequests == 1 {
				_, _ = w.Write([]byte(`{"id":1,"params":{"realm":"Login to Test","random":"12345","encryption":"Default"},"result":false,"session":"sess1","error":{"code":268632079,"message":"challenge"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":2,"params":{},"result":true,"session":"sess2"}`))
		case "/RPC2":
			var payload struct {
				Method string `json:"method"`
				Params any    `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode rpc request: %v", err)
			}
			rpcMethods = append(rpcMethods, payload.Method)
			rpcParams = append(rpcParams, payload.Params)
			switch payload.Method {
			case "configManager.getConfig":
				params, ok := payload.Params.(map[string]any)
				if !ok {
					t.Fatalf("expected getConfig params map, got %#v", payload.Params)
				}
				name, _ := params["name"].(string)
				switch name {
				case "Lighting_V2":
					_, _ = w.Write([]byte(`{"id":10,"params":{"table":[[{"LightType":"InfraredLight","Mode":"Auto","PercentOfMaxBrightness":0},{"LightType":"WhiteLight","Mode":"Auto","PercentOfMaxBrightness":30},{"LightType":"AIMixLight","Mode":"Auto","PercentOfMaxBrightness":40}]],"channel":10,"name":"Lighting_V2"},"result":true,"session":"sess2"}`))
				case "LightingScheme":
					_, _ = w.Write([]byte(`{"id":11,"params":{"table":[{"LightingMode":"AIMode"},{"LightingMode":"AIMode"},{"LightingMode":"AIMode"}],"channel":10,"name":"LightingScheme"},"result":true,"session":"sess2"}`))
				default:
					http.Error(w, "unexpected config name", http.StatusBadRequest)
				}
			case "system.multicall":
				_, _ = w.Write([]byte(`{"id":11,"result":true,"session":"sess2"}`))
			default:
				http.Error(w, "unexpected rpc method", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:                "west20_nvr",
		BaseURL:           server.URL,
		Username:          "assistant",
		Password:          "secret",
		ChannelAllowlist:  []int{11},
		AllowConfigWrites: true,
		RequestTimeout:    5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))

	err := driver.Aux(context.Background(), dahua.NVRAuxRequest{
		Channel: 11,
		Action:  dahua.NVRAuxActionStart,
		Output:  "light",
	})
	if err != nil {
		t.Fatalf("Aux returned error: %v", err)
	}

	if len(ptzRequests) != 1 || ptzRequests[0].Get("action") != "getCurrentProtocolCaps" {
		t.Fatalf("unexpected ptz capability probe requests: %+v", ptzRequests)
	}
	if len(rpcMethods) != 3 {
		t.Fatalf("expected 3 rpc calls, got %d (%+v)", len(rpcMethods), rpcMethods)
	}
	if rpcMethods[0] != "configManager.getConfig" || rpcMethods[1] != "configManager.getConfig" || rpcMethods[2] != "system.multicall" {
		t.Fatalf("unexpected rpc methods %+v", rpcMethods)
	}
	firstParams, ok := rpcParams[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first getConfig params map, got %#v", rpcParams[0])
	}
	if channel, _ := firstParams["channel"].(float64); channel != 10 {
		t.Fatalf("unexpected first getConfig params %+v", firstParams)
	}
	multiParams, ok := rpcParams[2].([]any)
	if !ok || len(multiParams) != 2 {
		t.Fatalf("expected multicall params, got %#v", rpcParams[2])
	}
	firstCall, ok := multiParams[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first multicall entry map, got %#v", multiParams[0])
	}
	firstCallParams, ok := firstCall["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected first multicall params map, got %#v", firstCall["params"])
	}
	if name, _ := firstCallParams["name"].(string); name != "Lighting_V2" {
		t.Fatalf("unexpected first multicall name %+v", firstCallParams)
	}
	secondCall, ok := multiParams[1].(map[string]any)
	if !ok {
		t.Fatalf("expected second multicall entry map, got %#v", multiParams[1])
	}
	secondCallParams, ok := secondCall["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected second multicall params map, got %#v", secondCall["params"])
	}
	table, ok := secondCallParams["table"].([]any)
	if !ok || len(table) != 3 {
		t.Fatalf("unexpected lighting scheme table %#v", secondCallParams["table"])
	}
	firstScheme, ok := table[0].(map[string]any)
	if !ok || firstScheme["LightingMode"] != "WhiteMode" {
		t.Fatalf("unexpected first lighting scheme entry %#v", table[0])
	}
	secondScheme, ok := table[1].(map[string]any)
	if !ok || secondScheme["LightingMode"] != "AIMode" {
		t.Fatalf("unexpected second lighting scheme entry %#v", table[1])
	}
}

func TestApplyPTZOverrideDisablesAdvertisedPTZ(t *testing.T) {
	disabled := false
	driver := &Driver{
		cfg: config.DeviceConfig{
			ChannelPTZControlOverrides: []config.ChannelPTZControlOverride{
				{Channel: 9, Enabled: &disabled},
			},
		},
	}

	capabilities := driver.applyPTZOverride(9, dahua.NVRPTZCapabilities{
		Supported: true,
		Pan:       true,
		Tilt:      true,
		Commands:  []string{"left", "right"},
	})
	if capabilities.Supported || capabilities.Pan || capabilities.Tilt || len(capabilities.Commands) != 0 {
		t.Fatalf("expected ptz override to clear capabilities, got %+v", capabilities)
	}
}

func TestParseRecordModes(t *testing.T) {
	modes := parseRecordModes(map[string]string{
		"table.RecordMode[4].Mode":       "1",
		"table.RecordMode[4].ModeExtra1": "2",
		"table.RecordMode[4].ModeExtra2": "2",
		"table.RecordMode[10].Mode":      "2",
	})

	if modes[4].Mode != 1 || modes[4].ModeExtra1 != "2" || modes[4].ModeExtra2 != "2" {
		t.Fatalf("unexpected mode state %+v", modes[4])
	}

	capabilities := recordingCapabilitiesForChannel(5, modes)
	if !capabilities.Supported || !capabilities.Active || capabilities.Mode != "manual" {
		t.Fatalf("unexpected recording capabilities %+v", capabilities)
	}
}

func TestDriverRecordingSendsExpectedQueries(t *testing.T) {
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())
		switch r.URL.Query().Get("action") {
		case "getConfig":
			_, _ = w.Write([]byte("table.RecordMode[4].Mode=0\ntable.RecordMode[4].ModeExtra1=2\ntable.RecordMode[4].ModeExtra2=2\n"))
		case "setConfig":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:                "west20_nvr",
		BaseURL:           server.URL,
		Username:          "assistant",
		Password:          "secret",
		ChannelAllowlist:  []int{5},
		AllowConfigWrites: true,
		RequestTimeout:    5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	err := driver.Recording(context.Background(), dahua.NVRRecordingRequest{
		Channel: 5,
		Action:  dahua.NVRRecordingActionStart,
	})
	if err != nil {
		t.Fatalf("Recording returned error: %v", err)
	}

	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}
	if requests[2].Get("RecordMode[4].Mode") != "1" || requests[2].Get("RecordMode[4].ModeExtra1") != "2" || requests[2].Get("RecordMode[4].ModeExtra2") != "2" {
		t.Fatalf("unexpected setConfig request: %+v", requests[2])
	}
}

func TestDriverRecordingFallsBackToTablePrefixedQueries(t *testing.T) {
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())
		switch r.URL.Query().Get("action") {
		case "getConfig":
			_, _ = w.Write([]byte("table.RecordMode[4].Mode=0\ntable.RecordMode[4].ModeExtra1=2\ntable.RecordMode[4].ModeExtra2=2\n"))
		case "setConfig":
			if r.URL.Query().Get("RecordMode[4].Mode") != "" {
				http.Error(w, "Bad Request!", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:                "west20_nvr",
		BaseURL:           server.URL,
		Username:          "assistant",
		Password:          "secret",
		ChannelAllowlist:  []int{5},
		AllowConfigWrites: true,
		RequestTimeout:    5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	err := driver.Recording(context.Background(), dahua.NVRRecordingRequest{
		Channel: 5,
		Action:  dahua.NVRRecordingActionStart,
	})
	if err != nil {
		t.Fatalf("Recording returned error: %v", err)
	}

	if len(requests) != 4 {
		t.Fatalf("expected 4 requests, got %d", len(requests))
	}
	if requests[3].Get("table.RecordMode[4].Mode") != "1" || requests[3].Get("table.RecordMode[4].ModeExtra1") != "2" || requests[3].Get("table.RecordMode[4].ModeExtra2") != "2" {
		t.Fatalf("unexpected fallback setConfig request: %+v", requests[3])
	}
}

func TestDriverSetAudioMuteUsesEncodeAudioEnable(t *testing.T) {
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())
		switch r.URL.Query().Get("action") {
		case "getConfig":
			switch r.URL.Query().Get("name") {
			case "Encode":
				_, _ = w.Write([]byte("table.Encode[4].MainFormat[0].Video.resolution=2560x1440\ntable.Encode[4].MainFormat[0].Audio.Compression=AAC\ntable.Encode[4].MainFormat[0].AudioEnable=true\ntable.Encode[4].ExtraFormat[0].AudioEnable=true\n"))
			case "RecordMode":
				_, _ = w.Write([]byte("table.RecordMode[4].Mode=0\ntable.RecordMode[4].ModeExtra1=2\ntable.RecordMode[4].ModeExtra2=2\n"))
			default:
				http.Error(w, "unexpected config", http.StatusBadRequest)
			}
		case "setConfig":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:                "west20_nvr",
		BaseURL:           server.URL,
		Username:          "assistant",
		Password:          "secret",
		ChannelAllowlist:  []int{5},
		AllowConfigWrites: true,
		RequestTimeout:    5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.cachedInventory = &inventorySnapshot{
		Channels: []channelInventory{
			{
				Index:        4,
				AudioCodec:   "AAC",
				AudioEnabled: true,
				AudioKnown:   true,
			},
		},
	}
	driver.inventoryExpires = time.Now().Add(time.Minute)
	driver.rpc = nil

	capabilities, notes := driver.audioCapabilities(context.Background(), 5)
	if capabilities.Mute || !capabilities.Supported || !capabilities.StreamEnabled || capabilities.Muted {
		t.Fatalf("unexpected audio capabilities %+v", capabilities)
	}
	if len(notes) == 0 {
		t.Fatalf("expected audio notes, got %+v", notes)
	}

	if err := driver.SetAudioMute(context.Background(), dahua.NVRAudioRequest{
		Channel: 5,
		Muted:   true,
	}); !errors.Is(err, dahua.ErrUnsupportedOperation) {
		t.Fatalf("expected unsupported operation, got %v", err)
	}
	for _, request := range requests {
		if request.Get("action") == "setConfig" {
			t.Fatalf("did not expect setConfig audio writes %+v", request)
		}
	}
}

func TestDriverSetAudioMuteDoesNotWriteDirectIPCWhenConfigured(t *testing.T) {
	var nvrRequests []url.Values
	var directRequests []url.Values

	directServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directRequests = append(directRequests, r.URL.Query())
		switch r.URL.Query().Get("action") {
		case "getConfig":
			_, _ = w.Write([]byte("table.Encode[0].MainFormat[0].Video.resolution=2560x1440\ntable.Encode[0].MainFormat[0].Audio.Compression=AAC\ntable.Encode[0].MainFormat[0].AudioEnable=true\ntable.Encode[0].ExtraFormat[0].AudioEnable=true\n"))
		case "setConfig":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected direct action", http.StatusBadRequest)
		}
	}))
	defer directServer.Close()

	directHost := strings.TrimPrefix(directServer.URL, "http://")
	nvrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nvrRequests = append(nvrRequests, r.URL.Query())
		switch r.URL.Query().Get("name") {
		case "ChannelTitle":
			_, _ = w.Write([]byte("table.ChannelTitle[4].Name=Boiler Room\n"))
		case "Encode":
			_, _ = w.Write([]byte("table.Encode[4].MainFormat[0].Video.resolution=2560x1440\ntable.Encode[4].MainFormat[0].Audio.Compression=AAC\ntable.Encode[4].MainFormat[0].AudioEnable=true\n"))
		case "RemoteDevice":
			_, _ = w.Write([]byte(
				"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_4.Address=" + directHost + "\n" +
					"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_4.DeviceType=DH-T4A-PV\n" +
					"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_4.HttpPort=80\n",
			))
		default:
			http.Error(w, "unexpected nvr config query", http.StatusBadRequest)
		}
	}))
	defer nvrServer.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          nvrServer.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{5},
		RequestTimeout:   5 * time.Second,
		DirectIPCCredentials: []config.ChannelDirectIPCCredential{
			{
				NVRChannel:        5,
				DirectIPCIP:       directHost,
				DirectIPCUser:     "admin",
				DirectIPCPassword: "secret",
			},
		},
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	if err := driver.SetAudioMute(context.Background(), dahua.NVRAudioRequest{
		Channel: 5,
		Muted:   true,
	}); !errors.Is(err, dahua.ErrUnsupportedOperation) {
		t.Fatalf("expected unsupported operation, got %v", err)
	}

	for _, request := range nvrRequests {
		if request.Get("action") == "setConfig" {
			t.Fatalf("did not expect nvr setConfig request %+v", request)
		}
	}
	for _, request := range directRequests {
		if request.Get("action") == "setConfig" {
			t.Fatalf("did not expect direct ipc setConfig request %+v", request)
		}
	}
}

func TestSetDirectIPCLightingModeSetsDirectLightingScheme(t *testing.T) {
	var directRequests []url.Values
	directServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directRequests = append(directRequests, r.URL.Query())
		switch r.URL.Query().Get("action") {
		case "getConfig":
			switch r.URL.Query().Get("name") {
			case "Lighting_V2":
				_, _ = w.Write([]byte(
					"table.Lighting_V2[0][0][0].LightType=InfraredLight\n" +
						"table.Lighting_V2[0][0][0].Mode=Auto\n" +
						"table.Lighting_V2[0][0][1].LightType=WhiteLight\n" +
						"table.Lighting_V2[0][0][1].Mode=Auto\n" +
						"table.Lighting_V2[0][0][2].LightType=AIMixLight\n" +
						"table.Lighting_V2[0][0][2].Mode=Auto\n",
				))
			case "LightingScheme":
				_, _ = w.Write([]byte(
					"table.LightingScheme[0][0].LightingMode=AIMode\n" +
						"table.LightingScheme[0][1].LightingMode=AIMode\n" +
						"table.LightingScheme[0][2].LightingMode=AIMode\n",
				))
			default:
				http.Error(w, "unexpected direct config query", http.StatusBadRequest)
			}
		case "setConfig":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected direct action", http.StatusBadRequest)
		}
	}))
	defer directServer.Close()

	directHost := strings.TrimPrefix(directServer.URL, "http://")
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID:             "west20_nvr",
			RequestTimeout: 5 * time.Second,
			DirectIPCCredentials: []config.ChannelDirectIPCCredential{
				{
					NVRChannel:        8,
					DirectIPCIP:       directHost,
					DirectIPCUser:     "admin",
					DirectIPCPassword: "secret",
				},
			},
		},
		metrics: metricsRegistry,
		logger:  zerolog.Nop(),
		cachedInventory: &inventorySnapshot{
			Channels: []channelInventory{
				{
					Index: 7,
					RemoteDevice: remoteDeviceInventory{
						Address:    directHost,
						DeviceType: "DH-T4A-PV",
						HTTPPort:   80,
					},
				},
			},
		},
		inventoryExpires: time.Now().Add(time.Minute),
	}

	if err := driver.setDirectIPCLightingMode(context.Background(), 8, dahua.NVRAuxActionStart); err != nil {
		t.Fatalf("setDirectIPCLightingMode start returned error: %v", err)
	}
	if err := driver.setDirectIPCLightingMode(context.Background(), 8, dahua.NVRAuxActionStop); err != nil {
		t.Fatalf("setDirectIPCLightingMode stop returned error: %v", err)
	}

	var setRequests []url.Values
	for _, request := range directRequests {
		if request.Get("action") == "setConfig" {
			setRequests = append(setRequests, request)
		}
	}
	if len(setRequests) != 4 {
		t.Fatalf("expected 4 direct setConfig requests, got %d (%+v)", len(setRequests), setRequests)
	}

	if setRequests[0].Get("Lighting_V2[0][0][1].Mode") != "Auto" {
		t.Fatalf("expected white-light profile to stay Auto, got %+v", setRequests[0])
	}
	if setRequests[1].Get("LightingScheme[0][0].LightingMode") != "WhiteMode" ||
		setRequests[1].Get("LightingScheme[0][1].LightingMode") != "AIMode" ||
		setRequests[1].Get("LightingScheme[0][2].LightingMode") != "AIMode" {
		t.Fatalf("unexpected white-light scheme request: %+v", setRequests[1])
	}
	if setRequests[2].Get("Lighting_V2[0][0][2].Mode") != "Auto" {
		t.Fatalf("expected smart-light profile to stay Auto, got %+v", setRequests[2])
	}
	if setRequests[3].Get("LightingScheme[0][0].LightingMode") != "AIMode" ||
		setRequests[3].Get("LightingScheme[0][1].LightingMode") != "AIMode" ||
		setRequests[3].Get("LightingScheme[0][2].LightingMode") != "AIMode" {
		t.Fatalf("unexpected smart-light scheme request: %+v", setRequests[3])
	}
}

func TestChannelControlCapabilitiesUsesConfiguredAuxOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getCurrentProtocolCaps":
			http.Error(w, "Bad Request!", http.StatusBadRequest)
		case "getConfig":
			_, _ = w.Write([]byte("table.RecordMode[7].Mode=0\ntable.RecordMode[7].ModeExtra1=2\ntable.RecordMode[7].ModeExtra2=2\n"))
		case "stop":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{8},
		ChannelAuxControlOverrides: []config.ChannelAuxControlOverride{
			{Channel: 8, Features: []string{"siren"}},
		},
		RequestTimeout: 5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	capabilities, err := driver.ChannelControlCapabilities(context.Background(), 8)
	if err != nil {
		t.Fatalf("ChannelControlCapabilities returned error: %v", err)
	}
	if capabilities.PTZ.Supported {
		t.Fatalf("expected no ptz support, got %+v", capabilities.PTZ)
	}
	if !capabilities.Aux.Supported {
		t.Fatalf("expected aux override support, got %+v", capabilities.Aux)
	}
	if len(capabilities.Aux.Outputs) != 1 || capabilities.Aux.Outputs[0] != "aux" {
		t.Fatalf("unexpected aux outputs %+v", capabilities.Aux.Outputs)
	}
	if len(capabilities.Aux.Features) != 1 || capabilities.Aux.Features[0] != "siren" {
		t.Fatalf("unexpected aux features %+v", capabilities.Aux.Features)
	}
}

func TestChannelControlCapabilitiesDoesNotInferAuxFromBlindProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getCurrentProtocolCaps":
			http.Error(w, "Bad Request!", http.StatusBadRequest)
		case "getConfig":
			_, _ = w.Write([]byte("table.RecordMode[7].Mode=0\ntable.RecordMode[7].ModeExtra1=2\ntable.RecordMode[7].ModeExtra2=2\n"))
		case "stop":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{8},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	capabilities, err := driver.ChannelControlCapabilities(context.Background(), 8)
	if err != nil {
		t.Fatalf("ChannelControlCapabilities returned error: %v", err)
	}
	if capabilities.Aux.Supported {
		t.Fatalf("expected aux to stay unsupported without override, got %+v", capabilities.Aux)
	}
}

func TestPTZCapabilitiesClassifiesBadRequestAsUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Request!", http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:             "west20_nvr",
		BaseURL:        server.URL,
		Username:       "assistant",
		Password:       "secret",
		RequestTimeout: 5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))

	_, err := driver.ptzCapabilities(context.Background(), 8)
	if !errors.Is(err, dahua.ErrUnsupportedOperation) {
		t.Fatalf("expected unsupported operation, got %v", err)
	}
}

func TestChannelControlCapabilitiesIncludesRemoteSpeakPlayback(t *testing.T) {
	loginRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/ptz.cgi":
			switch r.URL.Query().Get("action") {
			case "getCurrentProtocolCaps":
				http.Error(w, "Bad Request!", http.StatusBadRequest)
			case "stop":
				_, _ = w.Write([]byte("Error\nNot Implemented!"))
			default:
				http.Error(w, "unexpected ptz action", http.StatusBadRequest)
			}
		case "/cgi-bin/configManager.cgi":
			switch r.URL.Query().Get("action") {
			case "getConfig":
				_, _ = w.Write([]byte("table.RecordMode[6].Mode=0\ntable.RecordMode[6].ModeExtra1=2\ntable.RecordMode[6].ModeExtra2=2\n"))
			default:
				http.Error(w, "unexpected config action", http.StatusBadRequest)
			}
		case "/RPC2_Login":
			loginRequests++
			if loginRequests == 1 {
				_, _ = w.Write([]byte(`{"id":1,"params":{"realm":"Login to Test","random":"12345","encryption":"Default"},"result":false,"session":"sess1","error":{"code":268632079,"message":"challenge"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":2,"params":{},"result":true,"session":"sess2"}`))
		case "/RPC2":
			var payload struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode rpc request: %v", err)
			}
			switch payload.Method {
			case "RemoteSpeak.getCaps":
				_, _ = w.Write([]byte(`{"id":10,"params":{"Caps":[{"AudioPlayPath":[{"Path":"/usr/data/audiofiles/siren/","SupportUpload":false}],"SupportAudioPlay":true,"SupportQuickReply":false,"SupportSiren":true,"SupportedAudioFormat":[{"Format":"aac"},{"Format":"wav"}]}]},"result":true,"session":"sess2"}`))
			case "RemoteFileManager.listCache":
				_, _ = w.Write([]byte(`{"id":10,"params":{"FileInfo":[{"Path":"/usr/data/audiofiles/siren/alarm.wav","Size":110012}]},"result":true,"session":"sess2"}`))
			case "RemoteFileManager.GetVolume":
				_, _ = w.Write([]byte(`{"id":10,"result":false,"session":"sess2","error":{"code":285278249,"message":"Authority:check failure."}}`))
			default:
				http.Error(w, "unexpected rpc method", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{7},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))

	capabilities, err := driver.ChannelControlCapabilities(context.Background(), 7)
	if err != nil {
		t.Fatalf("ChannelControlCapabilities returned error: %v", err)
	}
	if !capabilities.Audio.Supported {
		t.Fatalf("expected audio support, got %+v", capabilities.Audio)
	}
	if !capabilities.Audio.Playback.Supported || !capabilities.Audio.Playback.Siren {
		t.Fatalf("expected playback siren support, got %+v", capabilities.Audio.Playback)
	}
	if !capabilities.Audio.VolumePermissionDenied {
		t.Fatalf("expected volume permission denied, got %+v", capabilities.Audio)
	}
	if capabilities.Audio.Playback.FileCount != 1 || len(capabilities.Audio.Playback.Files) != 1 || capabilities.Audio.Playback.Files[0].Name != "alarm.wav" {
		t.Fatalf("unexpected playback files %+v", capabilities.Audio.Playback.Files)
	}
}
