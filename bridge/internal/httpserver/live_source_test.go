package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/streams"
	"github.com/go-chi/chi/v5"
)

type liveSourceSnapshotReader struct {
	stubSnapshotReader
	set func(context.Context, string, string) (streams.Entry, error)
}

func (s liveSourceSnapshotReader) SetStreamLiveSource(ctx context.Context, id, source string) (streams.Entry, error) {
	return s.set(ctx, id, source)
}

func TestSetLiveSourceEndpoint(t *testing.T) {
	for _, item := range []struct {
		name, body string
		err        error
		status     int
	}{
		{"success", `{"source":"camera"}`, nil, http.StatusOK},
		{"restore default", `{"source":"default"}`, nil, http.StatusOK},
		{"invalid source", `{"source":"auto"}`, streams.ErrInvalidLiveSource, http.StatusBadRequest},
		{"invalid override", `{"source":"other"}`, streams.ErrInvalidLiveSourceOverride, http.StatusBadRequest},
		{"unavailable", `{"source":"camera"}`, streams.ErrLiveSourceUnavailable, http.StatusConflict},
		{"no persistence", `{"source":"camera"}`, streams.ErrLiveSourcePersistence, http.StatusServiceUnavailable},
		{"unknown stream", `{"source":"camera"}`, dahua.ErrDeviceNotFound, http.StatusNotFound},
		{"bad json", `{"source":`, nil, http.StatusBadRequest},
		{"trailing json", `{"source":"camera"}{}`, nil, http.StatusBadRequest},
		{"unknown field", `{"source":"camera","password":"secret"}`, nil, http.StatusBadRequest},
	} {
		t.Run(item.name, func(t *testing.T) {
			controller := &controller{snapshots: liveSourceSnapshotReader{set: func(_ context.Context, id, source string) (streams.Entry, error) {
				if id != "nvr_channel_05" {
					t.Fatalf("unexpected stream %s", id)
				}
				return streams.Entry{ID: id, LiveSource: &streams.LiveSourceSummary{Source: source}}, item.err
			}}}
			router := chi.NewRouter()
			controller.registerCatalogRoutes(router)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/v1/streams/nvr_channel_05/live-source", strings.NewReader(item.body)))
			if recorder.Code != item.status {
				t.Fatalf("got %d, want %d: %s", recorder.Code, item.status, recorder.Body.String())
			}
		})
	}
}

type defaultLiveSourceSnapshotReader struct {
	stubSnapshotReader
	get func() streams.LiveSourceSettings
	set func(context.Context, string) (streams.LiveSourceSettings, error)
}

func (s defaultLiveSourceSnapshotReader) GetDefaultLiveSource() streams.LiveSourceSettings {
	return s.get()
}
func (s defaultLiveSourceSnapshotReader) SetDefaultLiveSource(ctx context.Context, source string) (streams.LiveSourceSettings, error) {
	return s.set(ctx, source)
}

func TestDefaultLiveSourceEndpointsRequireAuthentication(t *testing.T) {
	current, reads, writes := "nvr", 0, 0
	controller := &controller{snapshots: defaultLiveSourceSnapshotReader{
		get: func() streams.LiveSourceSettings { reads++; return streams.LiveSourceSettings{Source: current} },
		set: func(_ context.Context, source string) (streams.LiveSourceSettings, error) {
			writes++
			current = source
			return streams.LiveSourceSettings{Source: source}, nil
		},
	}}
	router := chi.NewRouter()
	router.Use(authMiddleware(config.HTTPConfig{AuthToken: "api-secret", AuthQueryToken: true}))
	controller.registerCatalogRoutes(router)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/api/v1/settings/live-source", strings.NewReader(`{"source":"camera"}`)))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s allowed: %d", method, response.Code)
		}
	}
	if reads != 0 || writes != 0 {
		t.Fatal("unauthenticated request reached setting store")
	}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/settings/live-source", strings.NewReader(`{"source":"camera"}`))
	request.Header.Set("Authorization", "Bearer api-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"source":"camera"}` || writes != 1 {
		t.Fatalf("authenticated update failed: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings/live-source?auth_token=api-secret", nil))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"source":"camera"}` || reads != 1 {
		t.Fatalf("authenticated read failed: %d %s", response.Code, response.Body.String())
	}
}

func TestDefaultLiveSourceEndpointErrors(t *testing.T) {
	for _, item := range []struct {
		name, body string
		err        error
		status     int
	}{
		{"invalid source", `{"source":"default"}`, streams.ErrInvalidLiveSource, http.StatusBadRequest},
		{"no persistence", `{"source":"camera"}`, streams.ErrLiveSourcePersistence, http.StatusServiceUnavailable},
		{"invalid json", `{"source":`, nil, http.StatusBadRequest},
		{"extra json", `{"source":"camera"}{}`, nil, http.StatusBadRequest},
		{"unknown fields", `{"source":"camera","password":"x"}`, nil, http.StatusBadRequest},
	} {
		t.Run(item.name, func(t *testing.T) {
			controller := &controller{snapshots: defaultLiveSourceSnapshotReader{set: func(_ context.Context, source string) (streams.LiveSourceSettings, error) {
				return streams.LiveSourceSettings{Source: source}, item.err
			}}}
			router := chi.NewRouter()
			controller.registerCatalogRoutes(router)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/v1/settings/live-source", strings.NewReader(item.body)))
			if response.Code != item.status {
				t.Fatalf("got %d, want %d: %s", response.Code, item.status, response.Body.String())
			}
		})
	}
}

func TestDefaultLiveSourceEndpointsWithoutOptionalSupport(t *testing.T) {
	controller := &controller{snapshots: stubSnapshotReader{}}
	router := chi.NewRouter()
	controller.registerCatalogRoutes(router)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/api/v1/settings/live-source", strings.NewReader(`{"source":"nvr"}`)))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("unsupported %s returned %d", method, response.Code)
		}
	}
}
