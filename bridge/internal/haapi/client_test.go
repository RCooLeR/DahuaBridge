package haapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

func TestProvisionONVIFCreatesEntry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		requests++

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/config/config_entries/flow":
			writeJSON(t, w, map[string]any{
				"type":    "form",
				"flow_id": "flow-1",
				"step_id": "user",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/config/config_entries/flow/flow-1":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if _, ok := payload["auto"]; ok {
				writeJSON(t, w, map[string]any{
					"type":    "form",
					"flow_id": "flow-1",
					"step_id": "configure",
				})
				return
			}
			if payload["host"] != "192.168.1.20" || payload["port"] != float64(8999) {
				t.Fatalf("unexpected configure payload: %+v", payload)
			}
			writeJSON(t, w, map[string]any{
				"type": "create_entry",
				"result": map[string]any{
					"entry_id": "entry-123",
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(config.HomeAssistantConfig{
		APIBaseURL:     server.URL,
		AccessToken:    "token",
		RequestTimeout: 5 * time.Second,
	})

	result, err := client.ProvisionONVIF(context.Background(), ONVIFProvisionTarget{
		DeviceID:   "yard_ipc",
		DeviceKind: dahua.DeviceKindIPC,
		Name:       "Yard Camera",
		Host:       "192.168.1.20",
		Port:       8999,
		Username:   "admin",
		Password:   "secret",
	})
	if err != nil {
		t.Fatalf("ProvisionONVIF returned error: %v", err)
	}
	if result.Status != "created" {
		t.Fatalf("unexpected status %+v", result)
	}
	if result.EntryID != "entry-123" {
		t.Fatalf("unexpected entry id %+v", result)
	}
	if requests != 3 {
		t.Fatalf("expected 3 requests, got %d", requests)
	}
}

func TestProvisionONVIFHandlesAlreadyConfiguredAbort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config/config_entries/flow":
			writeJSON(t, w, map[string]any{
				"type":    "form",
				"flow_id": "flow-1",
				"step_id": "user",
			})
		case "/api/config/config_entries/flow/flow-1":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if _, ok := payload["auto"]; ok {
				writeJSON(t, w, map[string]any{
					"type":    "form",
					"flow_id": "flow-1",
					"step_id": "configure",
				})
				return
			}
			writeJSON(t, w, map[string]any{
				"type":   "abort",
				"reason": "already_configured",
			})
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(config.HomeAssistantConfig{
		APIBaseURL:     server.URL,
		AccessToken:    "token",
		RequestTimeout: 5 * time.Second,
	})

	result, err := client.ProvisionONVIF(context.Background(), ONVIFProvisionTarget{
		DeviceID:   "west20_nvr",
		DeviceKind: dahua.DeviceKindNVR,
		Name:       "West 20 NVR",
		Host:       "192.168.1.10",
		Port:       80,
	})
	if err != nil {
		t.Fatalf("ProvisionONVIF returned error: %v", err)
	}
	if result.Status != "already_configured" {
		t.Fatalf("unexpected result %+v", result)
	}
	if result.Reason != "already_configured" {
		t.Fatalf("unexpected reason %+v", result)
	}
}

func TestProvisionONVIFReturnsFlowError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config/config_entries/flow":
			writeJSON(t, w, map[string]any{
				"type":    "form",
				"flow_id": "flow-1",
				"step_id": "user",
			})
		case "/api/config/config_entries/flow/flow-1":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if _, ok := payload["auto"]; ok {
				writeJSON(t, w, map[string]any{
					"type":    "form",
					"flow_id": "flow-1",
					"step_id": "configure",
				})
				return
			}
			writeJSON(t, w, map[string]any{
				"type":    "form",
				"step_id": "configure",
				"errors": map[string]any{
					"password": "auth_failed",
				},
				"description_placeholders": map[string]any{
					"error": "401 Unauthorized",
				},
			})
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(config.HomeAssistantConfig{
		APIBaseURL:     server.URL,
		AccessToken:    "token",
		RequestTimeout: 5 * time.Second,
	})

	result, err := client.ProvisionONVIF(context.Background(), ONVIFProvisionTarget{
		DeviceID:   "west20_vto",
		DeviceKind: dahua.DeviceKindVTO,
		Name:       "Front Door",
		Host:       "192.168.1.30",
		Port:       80,
	})
	if err == nil {
		t.Fatal("expected ProvisionONVIF to fail")
	}
	if result.Status != "error" {
		t.Fatalf("unexpected result %+v", result)
	}
	if result.Error == "" {
		t.Fatalf("expected detailed error %+v", result)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
