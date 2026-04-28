package nvr

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"RCooLeR/DahuaBridge/internal/dahua"
)

var (
	recordModePattern       = regexp.MustCompile(`^table\.RecordMode\[(\d+)\]\.Mode$`)
	recordModeExtra1Pattern = regexp.MustCompile(`^table\.RecordMode\[(\d+)\]\.ModeExtra1$`)
	recordModeExtra2Pattern = regexp.MustCompile(`^table\.RecordMode\[(\d+)\]\.ModeExtra2$`)
)

type recordModeState struct {
	Mode       int
	ModeExtra1 string
	ModeExtra2 string
}

func (d *Driver) Recording(ctx context.Context, request dahua.NVRRecordingRequest) error {
	cfg := d.currentConfig()
	if request.Channel <= 0 {
		return fmt.Errorf("channel must be >= 1")
	}
	if !cfg.AllowsChannel(request.Channel) {
		return fmt.Errorf("%w: channel %d is not allowed", dahua.ErrUnsupportedOperation, request.Channel)
	}

	recordModes, err := d.loadRecordModes(ctx)
	if err != nil {
		return err
	}

	state, ok := recordModes[request.Channel-1]
	if !ok {
		return fmt.Errorf("%w: recording controls are not supported on channel %d", dahua.ErrUnsupportedOperation, request.Channel)
	}

	switch request.Action {
	case dahua.NVRRecordingActionStart:
		return d.setChannelRecordMode(ctx, request.Channel, 1, state)
	case dahua.NVRRecordingActionStop:
		return d.setChannelRecordMode(ctx, request.Channel, 2, state)
	default:
		return fmt.Errorf("unsupported recording action %q", request.Action)
	}
}

func (d *Driver) recordingCapabilities(ctx context.Context, channel int) (dahua.NVRRecordingCapabilities, error) {
	recordModes, err := d.loadRecordModes(ctx)
	if err != nil {
		return dahua.NVRRecordingCapabilities{}, err
	}
	return recordingCapabilitiesForChannel(channel, recordModes), nil
}

func (d *Driver) loadRecordModes(ctx context.Context) (map[int]recordModeState, error) {
	values, err := d.client.GetKeyValues(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"getConfig"},
		"name":   []string{"RecordMode"},
	})
	if err != nil {
		return nil, err
	}
	return parseRecordModes(values), nil
}

func (d *Driver) setChannelRecordMode(ctx context.Context, channel int, mode int, current recordModeState) error {
	body, err := d.client.GetText(ctx, "/cgi-bin/configManager.cgi", url.Values{
		"action": []string{"setConfig"},
		fmt.Sprintf("RecordMode[%d].Mode", channel-1):       []string{strconv.Itoa(mode)},
		fmt.Sprintf("RecordMode[%d].ModeExtra1", channel-1): []string{firstNonEmpty(current.ModeExtra1, "2")},
		fmt.Sprintf("RecordMode[%d].ModeExtra2", channel-1): []string{firstNonEmpty(current.ModeExtra2, "2")},
	})
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(body), "OK") {
		return fmt.Errorf("recording action returned %q", strings.TrimSpace(body))
	}
	return nil
}

func parseRecordModes(values map[string]string) map[int]recordModeState {
	modes := make(map[int]recordModeState)
	for key, value := range values {
		switch {
		case recordModePattern.MatchString(key):
			match := recordModePattern.FindStringSubmatch(key)
			index, _ := parseInt(match[1])
			item := modes[index]
			item.Mode, _ = parseInt(value)
			modes[index] = item
		case recordModeExtra1Pattern.MatchString(key):
			match := recordModeExtra1Pattern.FindStringSubmatch(key)
			index, _ := parseInt(match[1])
			item := modes[index]
			item.ModeExtra1 = strings.TrimSpace(value)
			modes[index] = item
		case recordModeExtra2Pattern.MatchString(key):
			match := recordModeExtra2Pattern.FindStringSubmatch(key)
			index, _ := parseInt(match[1])
			item := modes[index]
			item.ModeExtra2 = strings.TrimSpace(value)
			modes[index] = item
		}
	}
	return modes
}

func recordingCapabilitiesForChannel(channel int, modes map[int]recordModeState) dahua.NVRRecordingCapabilities {
	state, ok := modes[channel-1]
	if !ok {
		return dahua.NVRRecordingCapabilities{}
	}

	mode := "unknown"
	active := false
	switch state.Mode {
	case 0:
		mode = "auto"
	case 1:
		mode = "manual"
		active = true
	case 2:
		mode = "stop"
	}

	return dahua.NVRRecordingCapabilities{
		Supported: true,
		Active:    active,
		Mode:      mode,
	}
}

var _ dahua.NVRRecordingController = (*Driver)(nil)
