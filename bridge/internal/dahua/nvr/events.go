package nvr

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

var supportedEventCodes = []string{
	"VideoMotion",
	"AlarmLocal",
	"SmartMotionHuman",
	"SmartMotionVehicle",
	"CrossLineDetection",
	"CrossRegionDetection",
}

func (d *Driver) StreamEvents(ctx context.Context, sink chan<- dahua.Event) error {
	overrides := d.imouEventOverrides()
	if len(overrides) == 0 {
		return d.streamLocalEvents(ctx, sink)
	}

	merged := make(chan dahua.Event, 64)
	go d.runLocalEventSource(ctx, merged)
	for _, override := range overrides {
		go d.runImouEventSource(ctx, override, merged)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-merged:
			select {
			case <-ctx.Done():
				return ctx.Err()
			case sink <- event:
			}
		}
	}
}

func (d *Driver) streamLocalEvents(ctx context.Context, sink chan<- dahua.Event) error {
	query := url.Values{
		"action": []string{"attach"},
		"codes":  []string{"[" + strings.Join(supportedEventCodes, ",") + "]"},
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
		if parseErr == nil || context.Canceled == parseErr || context.DeadlineExceeded == parseErr {
			return parseErr
		}

		d.logger.Warn().Err(parseErr).Msg("event stream disconnected, retrying")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (d *Driver) parseEventStream(ctx context.Context, resp *http.Response, sink chan<- dahua.Event) error {
	rootID := d.currentConfig().ID
	contentType := resp.Header.Get("Content-Type")
	boundary := parseBoundary(contentType)
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

		if d.shouldSuppressLocalEvent(event) {
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
	channel := 0
	childID := ""
	if index >= 0 {
		channel = index + 1
		childID = channelDeviceID(rootID, index)
	}

	return dahua.Event{
		DeviceID:   rootID,
		DeviceKind: dahua.DeviceKindNVR,
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
	switch event.Code {
	case "VideoMotion":
		return "motion", isActiveAction(event.Action), true
	case "SmartMotionHuman":
		return "human", isActiveAction(event.Action), true
	case "SmartMotionVehicle":
		return "vehicle", isActiveAction(event.Action), true
	case "CrossLineDetection":
		return "tripwire", isActiveAction(event.Action), true
	case "CrossRegionDetection":
		return "intrusion", isActiveAction(event.Action), true
	default:
		return "", false, false
	}
}

func BoolStateFromEventForApp(event dahua.Event) (string, bool, bool) {
	return boolStateFromEvent(event)
}

func eventTypeFromEvent(event dahua.Event) string {
	code := strings.ToLower(strings.TrimSpace(event.Code))
	action := string(event.Action)
	if action == "" {
		action = "state"
	}
	return fmt.Sprintf("%s_%s", code, action)
}

func EventTypeForApp(event dahua.Event) string {
	return eventTypeFromEvent(event)
}

func isActiveAction(action dahua.EventAction) bool {
	switch action {
	case dahua.EventActionStart, dahua.EventActionPulse, dahua.EventActionState:
		return true
	default:
		return false
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
