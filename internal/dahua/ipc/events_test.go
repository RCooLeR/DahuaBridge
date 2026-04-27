package ipc

import (
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
)

func TestNormalizeEventDefaultsToRootDevice(t *testing.T) {
	event, ok := normalizeEvent("yard_ipc", map[string]string{
		"Code":   "VideoMotion",
		"action": "Start",
	})
	if !ok {
		t.Fatal("expected event to normalize")
	}

	if event.ChildID != "yard_ipc" {
		t.Fatalf("unexpected child id %q", event.ChildID)
	}
	if event.Channel != 1 {
		t.Fatalf("unexpected channel %d", event.Channel)
	}
	if event.Action != dahua.EventActionStart {
		t.Fatalf("unexpected action %q", event.Action)
	}
}

func TestBoolStateFromEvent(t *testing.T) {
	stateKey, active, ok := boolStateFromEvent(dahua.Event{
		Code:   "SmartMotionHuman",
		Action: dahua.EventActionStart,
	})
	if !ok {
		t.Fatal("expected smart human motion to map")
	}
	if stateKey != "human" || !active {
		t.Fatalf("unexpected mapping %q %v", stateKey, active)
	}
}
