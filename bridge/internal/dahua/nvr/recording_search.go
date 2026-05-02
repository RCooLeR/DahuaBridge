package nvr

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

func (d *Driver) FindRecordings(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	if query.Channel <= 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("channel must be >= 1")
	}
	if query.StartTime.IsZero() || query.EndTime.IsZero() {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start and end time are required")
	}
	if query.EndTime.Before(query.StartTime) {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("end time must not be before start time")
	}
	if query.Limit <= 0 {
		query.Limit = 25
	}

	if query.EventOnly {
		return d.findEventRecordings(ctx, query)
	}

	eventCode := recordingEventCondition(query.EventCode)
	if d.rpc != nil && eventCode != "*" {
		result, err := d.findEventRecordingsViaLogRPC(ctx, query)
		if err == nil && len(result.Items) > 0 {
			return result, nil
		}
		if err != nil {
			d.logger.Debug().Err(err).Int("channel", query.Channel).Str("event_code", eventCode).Msg("rpc event log recording search failed, falling back to event/media file search")
		}

		result, err = d.findEventRecordings(ctx, query)
		if err == nil && len(result.Items) > 0 {
			return result, nil
		}
		if err != nil {
			d.logger.Debug().Err(err).Int("channel", query.Channel).Str("event_code", eventCode).Msg("rpc event recording search failed, falling back to media file search")
		}
	}

	if d.rpc != nil {
		result, err := d.findRecordingsViaRPC(ctx, query)
		if err == nil {
			return filterRecordingSearchResultForEvent(result, query.EventCode), nil
		}
		d.logger.Debug().Err(err).Int("channel", query.Channel).Msg("rpc recording search failed, falling back to cgi")
	}

	result, err := d.findRecordingsViaCGI(ctx, query)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, err
	}
	return filterRecordingSearchResultForEvent(result, query.EventCode), nil
}

func (d *Driver) findRecordingsViaCGI(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	handleKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/mediaFileFind.cgi", url.Values{
		"action": []string{"factory.create"},
	})
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("create recording search handle: %w", err)
	}
	handle := strings.TrimSpace(handleKV["result"])
	if handle == "" {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("recording search handle is empty")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = d.client.GetText(closeCtx, "/cgi-bin/mediaFileFind.cgi", url.Values{
			"action": []string{"close"},
			"object": []string{handle},
		})
	}()

	findQuery := url.Values{
		"action":                []string{"findFile"},
		"object":                []string{handle},
		"condition.Channel":     []string{strconv.Itoa(query.Channel)},
		"condition.StartTime":   []string{formatRecordingWallTime(query.StartTime)},
		"condition.EndTime":     []string{formatRecordingWallTime(query.EndTime)},
		"condition.Types[0]":    []string{"dav"},
		"condition.VideoStream": []string{"Main"},
	}
	if eventCode := recordingEventCondition(query.EventCode); eventCode != "*" {
		findQuery.Set("condition.Flag[0]", "Event")
		findQuery.Set("condition.Events[0]", eventCode)
	}
	findBody, err := d.client.GetText(ctx, "/cgi-bin/mediaFileFind.cgi", findQuery)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start recording search: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(findBody), "OK") {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start recording search returned %q", strings.TrimSpace(findBody))
	}

	itemsKV, err := d.client.GetKeyValues(ctx, "/cgi-bin/mediaFileFind.cgi", url.Values{
		"action": []string{"findNextFile"},
		"object": []string{handle},
		"count":  []string{strconv.Itoa(query.Limit)},
	})
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch recording search results: %w", err)
	}

	result := parseRecordingSearchResult(itemsKV)
	result.DeviceID = d.ID()
	result.Channel = query.Channel
	result.StartTime = query.StartTime.In(time.Local).Format(recordingTimeLayout)
	result.EndTime = query.EndTime.In(time.Local).Format(recordingTimeLayout)
	result.Limit = query.Limit
	return result, nil
}

func (d *Driver) findRecordingsViaRPC(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	var handle int64
	if err := d.rpc.Call(ctx, "mediaFileFind.factory.create", nil, &handle); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("create recording search handle: %w", err)
	}
	if handle == 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("recording search handle is empty")
	}

	defer d.closeRecordingSearchHandle(handle)

	eventCode := recordingEventCondition(query.EventCode)
	flags := any(nil)
	if eventCode != "*" {
		flags = []string{"Event"}
	}
	findParams := map[string]any{
		"condition": map[string]any{
			"StartTime":   formatRecordingRPCTime(query.StartTime),
			"EndTime":     formatRecordingRPCTime(query.EndTime),
			"Events":      []string{eventCode},
			"Flags":       flags,
			"Types":       []string{"dav"},
			"Channel":     query.Channel - 1,
			"VideoStream": "Main",
		},
	}
	if err := d.rpc.CallObject(ctx, "mediaFileFind.findFile", findParams, handle, nil); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start recording search: %w", err)
	}

	var rawResult map[string]any
	if err := d.rpc.CallObject(ctx, "mediaFileFind.findNextFile", map[string]any{
		"count": query.Limit,
	}, handle, &rawResult); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch recording search results: %w", err)
	}

	result := parseRPCRecordingSearchResult(rawResult)
	result.DeviceID = d.ID()
	result.Channel = query.Channel
	result.StartTime = query.StartTime.In(time.Local).Format(recordingTimeLayout)
	result.EndTime = query.EndTime.In(time.Local).Format(recordingTimeLayout)
	result.Limit = query.Limit
	return result, nil
}
