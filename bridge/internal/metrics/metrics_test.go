package metrics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/buildinfo"
)

func TestEventStreamMetricsAreExposed(t *testing.T) {
	registry := New(buildinfo.Info())

	registry.SetEventStreamUp("west20_nvr", "nvr", true)
	registry.ObserveEventStreamRestart("west20_nvr", "nvr", errors.New("connect failed"))
	registry.ObserveEvent("west20_nvr", "nvr", "VideoMotion", "start", "1")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	registry.Handler().ServeHTTP(recorder, request)

	body := recorder.Body.String()
	for _, metric := range []string{
		`dahuabridge_event_stream_up{device_id="west20_nvr",device_type="nvr"} 1`,
		`dahuabridge_event_stream_restarts_total{device_id="west20_nvr",device_type="nvr",status="error"} 1`,
		`dahuabridge_event_last_seen_timestamp_seconds{device_id="west20_nvr",device_type="nvr"}`,
		`promhttp_metric_handler_errors_total{cause="encoding"} 0`,
		`promhttp_metric_handler_errors_total{cause="gathering"} 0`,
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("expected metrics output to contain %q\n%s", metric, body)
		}
	}
}
