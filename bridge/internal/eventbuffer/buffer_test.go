package eventbuffer

import (
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

func TestBufferListNewestFirstWithFilters(t *testing.T) {
	buffer := New(3)

	buffer.Add(dahua.Event{
		DeviceID:   "nvr_1",
		DeviceKind: dahua.DeviceKindNVR,
		Code:       "VideoMotion",
		OccurredAt: time.Unix(1, 0),
	})
	buffer.Add(dahua.Event{
		DeviceID:   "vto_1",
		DeviceKind: dahua.DeviceKindVTO,
		ChildID:    "vto_1_alarm_00",
		Code:       "AlarmLocal",
		OccurredAt: time.Unix(2, 0),
	})
	buffer.Add(dahua.Event{
		DeviceID:   "vto_1",
		DeviceKind: dahua.DeviceKindVTO,
		Code:       "DoorBell",
		OccurredAt: time.Unix(3, 0),
	})
	buffer.Add(dahua.Event{
		DeviceID:   "vto_1",
		DeviceKind: dahua.DeviceKindVTO,
		Code:       "Call",
		OccurredAt: time.Unix(4, 0),
	})

	events := buffer.ListEvents("vto_1", "", dahua.DeviceKindVTO, "", "", 2)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Code != "Call" {
		t.Fatalf("expected newest event first, got %+v", events[0])
	}
	if events[1].Code != "DoorBell" {
		t.Fatalf("expected second newest event, got %+v", events[1])
	}

	childEvents := buffer.ListEvents("", "vto_1_alarm_00", "", "", "", 10)
	if len(childEvents) != 1 {
		t.Fatalf("expected 1 child event, got %d", len(childEvents))
	}
	if childEvents[0].Code != "AlarmLocal" {
		t.Fatalf("unexpected child event: %+v", childEvents[0])
	}
}

func TestBufferClear(t *testing.T) {
	buffer := New(2)
	buffer.Add(dahua.Event{DeviceID: "a", Code: "One"})
	buffer.Add(dahua.Event{DeviceID: "b", Code: "Two"})

	removed := buffer.ClearEvents()
	if removed != 2 {
		t.Fatalf("expected 2 removed events, got %d", removed)
	}

	events := buffer.ListEvents("", "", "", "", "", 10)
	if len(events) != 0 {
		t.Fatalf("expected cleared buffer, got %d events", len(events))
	}

	stats := buffer.EventStats()
	if stats["count"] != 0 {
		t.Fatalf("expected count 0 after clear, got %+v", stats)
	}
}
