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

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"
)

type livePreconnectSnapshotReader struct {
	stubSnapshotReader
	current streams.LivePreconnectSettings
	reads   int
	writes  int
	setErr  error
}

func (s *livePreconnectSnapshotReader) GetLivePreconnect() streams.LivePreconnectSettings {
	s.reads++
	return s.current
}

func (s *livePreconnectSnapshotReader) SetLivePreconnect(ctx context.Context, settings streams.LivePreconnectSettings) (streams.LivePreconnectSettings, error) {
	s.writes++
	if err := ctx.Err(); err != nil {
		return s.current, err
	}
	if !settings.Valid() {
		return s.current, streams.ErrInvalidLivePreconnect
	}
	if s.setErr != nil {
		return s.current, s.setErr
	}
	s.current = settings
	return s.current, nil
}

func livePreconnectHTTPConfig() config.HTTPConfig {
	return config.HTTPConfig{
		ListenAddress: ":0", HealthPath: "/healthz", MetricsPath: "/metrics",
		AuthToken: "api-secret", AuthQueryToken: true,
	}
}

func preconnectRequest(server *Server, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	return response
}

func TestLivePreconnectAuthenticationAndRoundTrip(t *testing.T) {
	reader := &livePreconnectSnapshotReader{current: streams.LivePreconnectSettings{Mode: "off", Profile: "auto"}}
	server := newTestServerWithConfig(livePreconnectHTTPConfig(), reader, nil, nil, nil)
	const path = "/api/v1/settings/live-preconnect"
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, token := range []string{"", "wrong-token"} {
			response := preconnectRequest(server, method, path, `{"mode":"always","profile":"stable"}`, token)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated %s returned %d", method, response.Code)
			}
		}
	}
	if reader.reads != 0 || reader.writes != 0 {
		t.Fatal("unauthenticated request reached runtime settings")
	}
	for _, mode := range []string{"off", "recent", "always"} {
		for _, profile := range []string{"auto", "quality", "stable"} {
			want := streams.LivePreconnectSettings{Mode: mode, Profile: profile}
			response := preconnectRequest(server, http.MethodPut, path, fmt.Sprintf(`{"mode":%q,"profile":%q}`, mode, profile), "api-secret")
			assertPreconnectResponse(t, response, want)
			// Exercise query authentication used by configured HA bridge URLs.
			response = preconnectRequest(server, http.MethodGet, path+"?auth_token=api-secret", "", "")
			assertPreconnectResponse(t, response, want)
		}
	}
	if reader.reads != 9 || reader.writes != 9 {
		t.Fatalf("unexpected runtime access: %d reads, %d writes", reader.reads, reader.writes)
	}
}

func assertPreconnectResponse(t *testing.T, response *httptest.ResponseRecorder, want streams.LivePreconnectSettings) {
	t.Helper()
	var got streams.LivePreconnectSettings
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &got) != nil || got != want {
		t.Fatalf("response %d %s, want %+v", response.Code, response.Body.String(), want)
	}
}

func TestLivePreconnectRejectsInvalidBodiesWithoutChangingSettings(t *testing.T) {
	valid := `{"mode":"recent","profile":"stable"}`
	for _, item := range []struct {
		name, body string
		writes     int
	}{
		{"invalid mode", `{"mode":"sometimes","profile":"auto"}`, 1},
		{"invalid profile", `{"mode":"recent","profile":"main"}`, 1},
		{"missing mode", `{"profile":"auto"}`, 1},
		{"missing profile", `{"mode":"recent"}`, 1},
		{"empty object", `{}`, 1},
		{"null", `null`, 1},
		{"empty body", ``, 0},
		{"malformed json", `{"mode":`, 0},
		{"wrong field type", `{"mode":true,"profile":"auto"}`, 0},
		{"unknown field", `{"mode":"recent","profile":"auto","extra":true}`, 0},
		{"trailing json", valid + `{}`, 0},
		{"trailing null", valid + `null`, 0},
		{"oversized field", `{"mode":"` + strings.Repeat("x", 4096) + `","profile":"auto"}`, 0},
		{"oversized trailing whitespace", valid + strings.Repeat(" ", 4097-len(valid)), 0},
	} {
		t.Run(item.name, func(t *testing.T) {
			initial := streams.LivePreconnectSettings{Mode: "off", Profile: "auto"}
			reader := &livePreconnectSnapshotReader{current: initial}
			server := newTestServerWithConfig(livePreconnectHTTPConfig(), reader, nil, nil, nil)
			response := preconnectRequest(server, http.MethodPut, "/api/v1/settings/live-preconnect", item.body, "api-secret")
			if response.Code != http.StatusBadRequest || reader.current != initial || reader.writes != item.writes {
				t.Fatalf("response %d %s, writes=%d, current=%+v", response.Code, response.Body.String(), reader.writes, reader.current)
			}
		})
	}
	reader := &livePreconnectSnapshotReader{}
	server := newTestServerWithConfig(livePreconnectHTTPConfig(), reader, nil, nil, nil)
	response := preconnectRequest(server, http.MethodPut, "/api/v1/settings/live-preconnect", valid+strings.Repeat(" ", 4096-len(valid)), "api-secret")
	assertPreconnectResponse(t, response, streams.LivePreconnectSettings{Mode: "recent", Profile: "stable"})
}

func TestLivePreconnectRuntimeErrors(t *testing.T) {
	for _, item := range []struct {
		name   string
		err    error
		status int
	}{
		{"validation", fmt.Errorf("validate settings: %w", streams.ErrInvalidLivePreconnect), http.StatusBadRequest},
		{"persistence", fmt.Errorf("save settings: %w", streams.ErrLiveSourcePersistence), http.StatusServiceUnavailable},
		{"unexpected", errors.New("runtime failed"), http.StatusInternalServerError},
	} {
		t.Run(item.name, func(t *testing.T) {
			initial := streams.LivePreconnectSettings{Mode: "off", Profile: "auto"}
			reader := &livePreconnectSnapshotReader{current: initial, setErr: item.err}
			server := newTestServerWithConfig(livePreconnectHTTPConfig(), reader, nil, nil, nil)
			response := preconnectRequest(server, http.MethodPut, "/api/v1/settings/live-preconnect", `{"mode":"always","profile":"quality"}`, "api-secret")
			if response.Code != item.status || reader.current != initial || reader.writes != 1 {
				t.Fatalf("response %d %s, current=%+v", response.Code, response.Body.String(), reader.current)
			}
		})
	}
}

func TestLivePreconnectUnavailableRuntime(t *testing.T) {
	server := newTestServerWithConfig(livePreconnectHTTPConfig(), stubSnapshotReader{}, nil, nil, nil)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := preconnectRequest(server, method, "/api/v1/settings/live-preconnect", `{"mode":"off","profile":"auto"}`, "api-secret")
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("unsupported %s returned %d: %s", method, response.Code, response.Body.String())
		}
	}
}

func TestLivePreconnectWriteUsesAdminRateLimit(t *testing.T) {
	cfg := livePreconnectHTTPConfig()
	cfg.AdminRateLimitPerMinute, cfg.AdminRateLimitBurst = 1, 1
	reader := &livePreconnectSnapshotReader{}
	server := newTestServerWithConfig(cfg, reader, nil, nil, nil)
	const path = "/api/v1/settings/live-preconnect"
	first := preconnectRequest(server, http.MethodPut, path, `{"mode":"always","profile":"quality"}`, "api-secret")
	assertPreconnectResponse(t, first, streams.LivePreconnectSettings{Mode: "always", Profile: "quality"})
	second := preconnectRequest(server, http.MethodPut, path, `{"mode":"off","profile":"auto"}`, "api-secret")
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") == "" || reader.writes != 1 {
		t.Fatalf("second write bypassed admin limit: %d %s, writes=%d", second.Code, second.Body.String(), reader.writes)
	}
	read := preconnectRequest(server, http.MethodGet, path, "", "api-secret")
	assertPreconnectResponse(t, read, streams.LivePreconnectSettings{Mode: "always", Profile: "quality"})
}
