package vto

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

func (d *Driver) StreamEvents(ctx context.Context, sink chan<- dahua.Event) error {
	query := url.Values{
		"action": []string{"attach"},
		"codes":  []string{"[All]"},
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := d.client.OpenStream(ctx, "/cgi-bin/eventManager.cgi", query)
		if err != nil {
			return err
		}

		parseErr := d.parseEventStream(ctx, resp, sink)
		_ = resp.Body.Close()
		if parseErr == nil || parseErr == context.Canceled || parseErr == context.DeadlineExceeded {
			return parseErr
		}

		d.logger.Warn().Err(parseErr).Msg("vto event stream disconnected, retrying")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (d *Driver) parseEventStream(ctx context.Context, resp *http.Response, sink chan<- dahua.Event) error {
	rootID := d.currentConfig().ID
	boundary := parseBoundary(resp.Header.Get("Content-Type"))
	reader := bufio.NewReader(resp.Body)

	var (
		inPayload bool
		payload   []string
	)

	flush := func() error {
		if len(payload) == 0 {
			return nil
		}

		values := parseEventPayload(strings.Join(payload, "\n"))
		payload = payload[:0]
		if len(values) == 0 {
			return nil
		}

		event, ok := normalizeEvent(rootID, values)
		if !ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case sink <- event:
			return nil
		}
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return flush()
			}
			if flushErr := flush(); flushErr != nil {
				return flushErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		line = strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(line)

		if boundary != "" && (trimmed == boundary || trimmed == "--"+boundary || trimmed == "--"+boundary+"--") {
			if err := flush(); err != nil {
				return err
			}
			inPayload = false
			continue
		}

		if strings.HasPrefix(strings.ToLower(trimmed), "content-") {
			continue
		}

		if trimmed == "" {
			if inPayload {
				if err := flush(); err != nil {
					return err
				}
				inPayload = false
				continue
			}
			inPayload = true
			continue
		}

		if !inPayload && strings.Contains(trimmed, "Code=") {
			payload = append(payload, trimmed)
			if err := flush(); err != nil {
				return err
			}
			continue
		}

		if inPayload || strings.Contains(trimmed, "=") {
			inPayload = true
			payload = append(payload, trimmed)
		}
	}
}

func parseBoundary(contentType string) string {
	for _, part := range strings.Split(contentType, ";") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(strings.ToLower(part), "boundary=") {
			continue
		}
		return strings.Trim(strings.TrimPrefix(part, "boundary="), `"`)
	}
	return ""
}

func parseEventPayload(payload string) map[string]string {
	result := make(map[string]string)
	if strings.Contains(payload, ";") {
		for _, part := range strings.Split(payload, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			result[key] = strings.TrimSpace(value)
		}
		if len(result) > 0 {
			return result
		}
	}

	for _, line := range strings.Split(payload, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		result[key] = strings.TrimSpace(value)
	}

	return result
}

func normalizeEvent(rootID string, values map[string]string) (dahua.Event, bool) {
	code := firstNonEmpty(values["Code"], values["code"])
	if code == "" {
		return dahua.Event{}, false
	}

	action := normalizeEventAction(firstNonEmpty(values["action"], values["Action"]))
	index := parseIndex(firstNonEmpty(values["index"], values["Index"]), -1)
	childID := ""
	channel := 0
	if index >= 0 {
		childID = alarmDeviceID(rootID, index)
		channel = index + 1
	}

	return dahua.Event{
		DeviceID:   rootID,
		DeviceKind: dahua.DeviceKindVTO,
		ChildID:    childID,
		Code:       code,
		Action:     action,
		Index:      index,
		Channel:    channel,
		OccurredAt: time.Now().UTC(),
		Data:       values,
	}, true
}

func normalizeEventAction(value string) dahua.EventAction {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "start":
		return dahua.EventActionStart
	case "stop":
		return dahua.EventActionStop
	case "pulse":
		return dahua.EventActionPulse
	default:
		return dahua.EventActionState
	}
}

func parseIndex(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func boolStateFromEvent(event dahua.Event) (string, bool, bool) {
	code := strings.ToLower(strings.TrimSpace(event.Code))
	switch code {
	case "alarmlocal":
		return "active", isActiveAction(event.Action), true
	case "doorbell":
		return "doorbell", isActiveAction(event.Action), true
	case "accessctl", "accesscontrol":
		return "access", isActiveAction(event.Action), true
	case "tamper":
		return "tamper", isActiveAction(event.Action), true
	case "call":
		return "call", isActiveAction(event.Action), true
	default:
		return "", false, false
	}
}

func eventTypeFromEvent(event dahua.Event) string {
	code := strings.ToLower(strings.TrimSpace(event.Code))
	switch code {
	case "accessctl":
		code = "accesscontrol"
	}
	action := string(event.Action)
	if action == "" {
		action = "state"
	}
	return fmt.Sprintf("%s_%s", code, action)
}

func isActiveAction(action dahua.EventAction) bool {
	switch action {
	case dahua.EventActionStart, dahua.EventActionPulse, dahua.EventActionState:
		return true
	default:
		return false
	}
}

func BoolStateFromEventForApp(event dahua.Event) (string, bool, bool) {
	return boolStateFromEvent(event)
}

func EventTypeForApp(event dahua.Event) string {
	return eventTypeFromEvent(event)
}

func SessionStateUpdatesForApp(info map[string]any, event dahua.Event) map[string]string {
	if info == nil {
		return nil
	}

	updates := map[string]string{}
	code := strings.ToLower(strings.TrimSpace(event.Code))
	occurredAt := event.OccurredAt.Format(time.RFC3339Nano)
	source := callSourceFromEventData(event.Data)

	switch code {
	case "doorbell":
		if !isActiveAction(event.Action) {
			return nil
		}
		info["last_ring_at"] = occurredAt
		updates["last_ring_at"] = occurredAt
		if source != "" {
			info["last_call_source"] = source
			updates["last_call_source"] = source
		}
	case "call":
		if isActiveAction(event.Action) {
			info["call_state"] = "ringing"
			updates["call_state"] = "ringing"

			if currentStartedAt := infoStringValue(info["call_started_at"]); currentStartedAt == "" {
				info["call_started_at"] = occurredAt
				info["last_call_started_at"] = occurredAt
				updates["last_call_started_at"] = occurredAt
			}
			if source != "" {
				info["last_call_source"] = source
				updates["last_call_source"] = source
			}
			return updates
		}

		info["call_state"] = "idle"
		updates["call_state"] = "idle"
		info["last_call_ended_at"] = occurredAt
		updates["last_call_ended_at"] = occurredAt
		if source != "" {
			info["last_call_source"] = source
			updates["last_call_source"] = source
		}

		if currentStartedAt := infoStringValue(info["call_started_at"]); currentStartedAt != "" {
			if startedAt, err := time.Parse(time.RFC3339Nano, currentStartedAt); err == nil && !event.OccurredAt.Before(startedAt) {
				durationSeconds := int(event.OccurredAt.Sub(startedAt).Seconds())
				info["last_call_duration_seconds"] = durationSeconds
				updates["last_call_duration_seconds"] = strconv.Itoa(durationSeconds)
			}
		}
		delete(info, "call_started_at")
	}

	if len(updates) == 0 {
		return nil
	}
	return updates
}

var _ dahua.EventSource = (*Driver)(nil)

func callSourceFromEventData(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}

	candidates := []string{
		"CallSrc",
		"CallSource",
		"Source",
		"Src",
		"Caller",
		"From",
		"FromNo",
		"RoomNo",
		"Room",
		"VillaNo",
		"UnitNo",
		"FloorNo",
		"UserID",
		"Name",
	}

	for _, candidate := range candidates {
		if value := strings.TrimSpace(values[candidate]); value != "" {
			return value
		}
		for key, candidateValue := range values {
			if strings.EqualFold(strings.TrimSpace(key), candidate) {
				if value := strings.TrimSpace(candidateValue); value != "" {
					return value
				}
			}
		}
	}

	return ""
}

func infoStringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
