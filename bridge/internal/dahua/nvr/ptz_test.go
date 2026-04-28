package nvr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	driver := New(cfg, zerolog.Nop(), cgi.New(cfg, metrics.New(buildinfo.Info())))
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

	if len(requests) != 4 {
		t.Fatalf("expected 4 requests, got %d", len(requests))
	}
	if requests[2].Get("action") != "start" || requests[2].Get("code") != "Left" || requests[2].Get("arg2") != "2" {
		t.Fatalf("unexpected start request: %+v", requests[2])
	}
	if requests[3].Get("action") != "stop" || requests[3].Get("code") != "Left" {
		t.Fatalf("unexpected stop request: %+v", requests[3])
	}
}

func TestDriverAuxSendsExpectedQueries(t *testing.T) {
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())
		switch r.URL.Query().Get("action") {
		case "getCurrentProtocolCaps":
			_, _ = w.Write([]byte("caps.Aux=true\ncaps.Auxs[0]=Light\ncaps.Auxs[1]=Wiper\n"))
		case "getConfig":
			_, _ = w.Write([]byte("table.RecordMode[10].Mode=0\ntable.RecordMode[10].ModeExtra1=2\ntable.RecordMode[10].ModeExtra2=2\n"))
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
		ChannelAllowlist: []int{11},
		RequestTimeout:   5 * time.Second,
	}
	driver := New(cfg, zerolog.Nop(), cgi.New(cfg, metrics.New(buildinfo.Info())))
	driver.rpc = nil

	err := driver.Aux(context.Background(), dahua.NVRAuxRequest{
		Channel:  11,
		Action:   dahua.NVRAuxActionPulse,
		Output:   "light",
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
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{5},
		RequestTimeout:   5 * time.Second,
	}
	driver := New(cfg, zerolog.Nop(), cgi.New(cfg, metrics.New(buildinfo.Info())))
	driver.rpc = nil

	err := driver.Recording(context.Background(), dahua.NVRRecordingRequest{
		Channel: 5,
		Action:  dahua.NVRRecordingActionStart,
	})
	if err != nil {
		t.Fatalf("Recording returned error: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if requests[1].Get("RecordMode[4].Mode") != "1" || requests[1].Get("RecordMode[4].ModeExtra1") != "2" || requests[1].Get("RecordMode[4].ModeExtra2") != "2" {
		t.Fatalf("unexpected setConfig request: %+v", requests[1])
	}
}

func TestChannelControlCapabilitiesFallsBackToAuxProbe(t *testing.T) {
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())
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
	driver := New(cfg, zerolog.Nop(), cgi.New(cfg, metrics.New(buildinfo.Info())))
	driver.rpc = nil

	capabilities, err := driver.ChannelControlCapabilities(context.Background(), 8)
	if err != nil {
		t.Fatalf("ChannelControlCapabilities returned error: %v", err)
	}
	if capabilities.PTZ.Supported {
		t.Fatalf("expected no ptz support, got %+v", capabilities.PTZ)
	}
	if !capabilities.Aux.Supported {
		t.Fatalf("expected aux fallback support, got %+v", capabilities.Aux)
	}
	if len(capabilities.Aux.Outputs) != 3 || capabilities.Aux.Outputs[0] != "aux" {
		t.Fatalf("unexpected aux outputs %+v", capabilities.Aux.Outputs)
	}
	if len(capabilities.Aux.Features) != 3 || capabilities.Aux.Features[0] != "siren" {
		t.Fatalf("unexpected aux features %+v", capabilities.Aux.Features)
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
	driver := New(cfg, zerolog.Nop(), cgi.New(cfg, metrics.New(buildinfo.Info())))

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
