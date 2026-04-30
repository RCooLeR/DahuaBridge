package imou

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
)

func TestClientCachesAccessTokenAndFetchesCameraStatus(t *testing.T) {
	var mu sync.Mutex
	accessTokenCalls := 0
	statusCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/openapi/accessToken":
			accessTokenCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"code": "0",
					"msg":  "ok",
					"data": map[string]any{
						"accessToken": "At_test",
						"expireTime":  3600,
					},
				},
				"id": "1",
			})
		case "/openapi/getDeviceCameraStatus":
			statusCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"code": "0",
					"msg":  "ok",
					"data": map[string]any{
						"enableType": "whiteLight",
						"status":     "on",
					},
				},
				"id": "2",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(config.ImouConfig{
		Enabled:        true,
		AppID:          "app",
		AppSecret:      "secret",
		Endpoint:       server.URL,
		RequestTimeout: 5 * time.Second,
	})

	for i := 0; i < 2; i++ {
		status, err := client.GetCameraStatus(context.Background(), CameraStatusRequest{
			DeviceID:   "serial",
			ChannelID:  "0",
			EnableType: "whiteLight",
		})
		if err != nil {
			t.Fatalf("GetCameraStatus returned error: %v", err)
		}
		if !status.Enabled {
			t.Fatalf("expected enabled status, got %+v", status)
		}
	}

	if accessTokenCalls != 1 {
		t.Fatalf("expected 1 access token call, got %d", accessTokenCalls)
	}
	if statusCalls != 2 {
		t.Fatalf("expected 2 status calls, got %d", statusCalls)
	}
}

func TestClientListsAlarmPagesInChronologicalOrder(t *testing.T) {
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/accessToken":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"code": "0",
					"msg":  "ok",
					"data": map[string]any{
						"accessToken": "At_test",
						"expireTime":  3600,
					},
				},
				"id": "1",
			})
		case "/openapi/getAlarmMessage":
			call++
			if call == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"result": map[string]any{
						"code": "0",
						"msg":  "ok",
						"data": map[string]any{
							"count":       2,
							"nextAlarmId": "100",
							"alarms": []map[string]any{
								{"alarmId": "200", "time": 20, "channelId": "1", "type": "1", "deviceId": "serial", "localDate": "2026-04-30 10:00:20"},
								{"alarmId": "100", "time": 10, "channelId": "1", "type": "0", "deviceId": "serial", "localDate": "2026-04-30 10:00:10"},
							},
						},
					},
					"id": "2",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"code": "0",
					"msg":  "ok",
					"data": map[string]any{
						"count":       1,
						"nextAlarmId": "-1",
						"alarms": []map[string]any{
							{"alarmId": "300", "time": 30, "channelId": "1", "type": "1", "deviceId": "serial", "localDate": "2026-04-30 10:00:30"},
						},
					},
				},
				"id": "3",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(config.ImouConfig{
		Enabled:        true,
		AppID:          "app",
		AppSecret:      "secret",
		Endpoint:       server.URL,
		RequestTimeout: 5 * time.Second,
	})

	alarms, err := client.ListAlarms(context.Background(), AlarmQuery{
		DeviceID:  "serial",
		ChannelID: "1",
		BeginTime: time.Unix(0, 0),
		EndTime:   time.Unix(40, 0),
		Count:     2,
	})
	if err != nil {
		t.Fatalf("ListAlarms returned error: %v", err)
	}
	if len(alarms) != 3 {
		t.Fatalf("expected 3 alarms, got %d", len(alarms))
	}
	if alarms[0].AlarmID != "100" || alarms[1].AlarmID != "200" || alarms[2].AlarmID != "300" {
		t.Fatalf("unexpected alarm order %+v", alarms)
	}
}

func TestClientGetsAndSetsNightVisionMode(t *testing.T) {
	getCalls := 0
	setCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/accessToken":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"code": "0",
					"msg":  "ok",
					"data": map[string]any{
						"accessToken": "At_test",
						"expireTime":  3600,
					},
				},
				"id": "1",
			})
		case "/openapi/getNightVisionMode":
			getCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"code": "0",
					"msg":  "ok",
					"data": map[string]any{
						"mode":  "SmartLowLight",
						"modes": []string{"SmartLowLight", "FullColor", "Infrared"},
					},
				},
				"id": "2",
			})
		case "/openapi/setNightVisionMode":
			setCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"code": "0",
					"msg":  "ok",
				},
				"id": "3",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(config.ImouConfig{
		Enabled:        true,
		AppID:          "app",
		AppSecret:      "secret",
		Endpoint:       server.URL,
		RequestTimeout: 5 * time.Second,
	})

	mode, err := client.GetNightVisionMode(context.Background(), NightVisionModeRequest{
		DeviceID:  "serial",
		ChannelID: "0",
	})
	if err != nil {
		t.Fatalf("GetNightVisionMode returned error: %v", err)
	}
	if mode.Mode != "SmartLowLight" || len(mode.Modes) != 3 {
		t.Fatalf("unexpected night mode %+v", mode)
	}

	if err := client.SetNightVisionMode(context.Background(), NightVisionModeChange{
		DeviceID:  "serial",
		ChannelID: "0",
		Mode:      "FullColor",
	}); err != nil {
		t.Fatalf("SetNightVisionMode returned error: %v", err)
	}
	if getCalls != 1 || setCalls != 1 {
		t.Fatalf("unexpected get/set counts %d/%d", getCalls, setCalls)
	}
}
