package vto

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
)

func TestParseEventStreamCapturedSession(t *testing.T) {
	body, err := os.ReadFile("testdata/event_stream_session.txt")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	driver := &Driver{
		cfg: config.DeviceConfig{ID: "west20_vto"},
	}
	sink := make(chan dahua.Event, 8)
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{`multipart/x-mixed-replace; boundary=myboundary`},
		},
		Body: io.NopCloser(strings.NewReader(string(body))),
	}

	err = driver.parseEventStream(context.Background(), resp, sink)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	close(sink)

	events := make([]dahua.Event, 0)
	for event := range sink {
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Code != "DoorBell" || events[0].Action != dahua.EventActionStart || events[0].ChildID != "" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Code != "AlarmLocal" || events[1].ChildID != "west20_vto_alarm_03" || events[1].Channel != 4 {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if events[2].Code != "AccessCtl" || events[2].Action != dahua.EventActionPulse {
		t.Fatalf("unexpected third event: %+v", events[2])
	}
}
