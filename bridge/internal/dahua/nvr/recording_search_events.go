package nvr

import (
	"context"
	"fmt"
	"sort"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

func (d *Driver) findEventRecordings(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	result := emptyNVRRecordingSearchResult(d.ID(), query)
	if d.rpc == nil {
		return result, nil
	}

	var firstErr error
	appendResult := func(next dahua.NVRRecordingSearchResult, err error) {
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		result.Items = append(result.Items, next.Items...)
	}

	if smdTypes := recordingSMDTypesForEvent(query.EventCode); len(smdTypes) > 0 {
		next, err := d.findEventRecordingsViaSMDRPC(ctx, query, smdTypes)
		appendResult(next, err)
	}

	for _, ivsCode := range recordingIVSEventCodesForEvent(query.EventCode) {
		ivsQuery := query
		ivsQuery.EventCode = ivsCode
		next, err := d.findEventRecordingsViaIVSRPC(ctx, ivsQuery, ivsCode)
		appendResult(next, err)
	}

	if len(result.Items) == 0 {
		if firstErr != nil {
			return dahua.NVRRecordingSearchResult{}, firstErr
		}
		result.ReturnedCount = 0
		return result, nil
	}

	result.Items = dedupeNVRRecordings(result.Items)
	sort.SliceStable(result.Items, func(i, j int) bool {
		return nvrRecordingStartSortKey(result.Items[i]).After(nvrRecordingStartSortKey(result.Items[j]))
	})
	if query.Limit > 0 && len(result.Items) > query.Limit {
		result.Items = result.Items[:query.Limit]
	}
	result.ReturnedCount = len(result.Items)
	return result, nil
}

func (d *Driver) findEventRecordingsViaSMDRPC(ctx context.Context, query dahua.NVRRecordingQuery, smdTypes []string) (dahua.NVRRecordingSearchResult, error) {
	result := emptyNVRRecordingSearchResult(d.ID(), query)

	var startResp nvrSMDStartFindResponse
	if err := d.rpc.Call(ctx, "SmdDataFinder.startFind", map[string]any{
		"Condition": map[string]any{
			"StartTime": formatRecordingWallTime(query.StartTime),
			"EndTime":   formatRecordingWallTime(query.EndTime),
			"Orders": []map[string]any{{
				"Field": "SystemTime",
				"Type":  "Descent",
			}},
			"Channels": []int{query.Channel - 1},
			"Order":    "descOrder",
			"SmdType":  smdTypes,
		},
	}, &startResp); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start smd event search: %w", err)
	}
	if startResp.Token == 0 || startResp.Count == 0 {
		return result, nil
	}

	pageSize := recordingEventSearchPageSize(query.Limit)
	for offset := 0; offset < startResp.Count && len(result.Items) < query.Limit; offset += pageSize {
		count := pageSize
		if remaining := query.Limit - len(result.Items); remaining > 0 && remaining < count {
			count = remaining
		}
		if count <= 0 {
			break
		}

		var findResp nvrSMDFindResponse
		if err := d.rpc.Call(ctx, "SmdDataFinder.doFind", map[string]any{
			"Token":  startResp.Token,
			"Offset": offset,
			"Count":  count,
		}, &findResp); err != nil {
			return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch smd event results: %w", err)
		}
		if len(findResp.SMDInfo) == 0 {
			break
		}

		for _, info := range findResp.SMDInfo {
			recording, ok := smdInfoToRecording(info, query)
			if !ok {
				continue
			}
			result.Items = append(result.Items, recording)
			if len(result.Items) >= query.Limit {
				break
			}
		}
		if len(findResp.SMDInfo) < count {
			break
		}
	}

	result.ReturnedCount = len(result.Items)
	return result, nil
}

func (d *Driver) findEventRecordingsViaIVSRPC(ctx context.Context, query dahua.NVRRecordingQuery, eventCode string) (dahua.NVRRecordingSearchResult, error) {
	result := emptyNVRRecordingSearchResult(d.ID(), query)

	var handle int64
	if err := d.rpc.Call(ctx, "mediaFileFind.factory.create", nil, &handle); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("create ivs event search handle: %w", err)
	}
	if handle == 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("ivs event search handle is empty")
	}
	defer d.closeRecordingSearchHandle(handle)

	findParams := map[string]any{
		"condition": map[string]any{
			"StartTime": formatRecordingWallTime(query.StartTime),
			"EndTime":   formatRecordingWallTime(query.EndTime),
			"Channel":   []int{query.Channel - 1},
			"DB": map[string]any{
				"IVS": map[string]any{
					"Order":      "Descent",
					"Events":     []string{eventCode},
					"Rule":       eventCode,
					"ObjectType": []string{"Vehicle", "Human"},
				},
			},
			"Events": []string{eventCode},
		},
	}
	if err := d.rpc.CallObject(ctx, "mediaFileFind.findFile", findParams, handle, nil); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start ivs event search: %w", err)
	}

	var countResp nvrMediaFileCountResponse
	if err := d.rpc.CallObject(ctx, "mediaFileFind.getCount", nil, handle, &countResp); err != nil {
		d.logger.Debug().Err(err).Int64("handle", handle).Msg("ivs event count failed")
	} else if countResp.Count == 0 && countResp.Found == 0 {
		return result, nil
	}

	if err := d.rpc.CallObject(ctx, "mediaFileFind.setQueryResultOptions", map[string]any{
		"options": map[string]any{"offset": 0},
	}, handle, nil); err != nil {
		d.logger.Debug().Err(err).Int64("handle", handle).Msg("ivs event result option setup failed")
	}

	var rawResult map[string]any
	if err := d.rpc.CallObject(ctx, "mediaFileFind.findNextFile", map[string]any{
		"count": query.Limit,
	}, handle, &rawResult); err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch ivs event results: %w", err)
	}

	result = parseRPCRecordingSearchResult(rawResult)
	result.DeviceID = d.ID()
	result.Channel = query.Channel
	result.StartTime = query.StartTime.In(time.Local).Format(recordingTimeLayout)
	result.EndTime = query.EndTime.In(time.Local).Format(recordingTimeLayout)
	result.Limit = query.Limit
	for i := range result.Items {
		result.Items[i] = normalizeIVSEventRecording(result.Items[i], query, eventCode)
	}
	result.ReturnedCount = len(result.Items)
	return result, nil
}

func (d *Driver) findEventRecordingsViaLogRPC(ctx context.Context, query dahua.NVRRecordingQuery) (dahua.NVRRecordingSearchResult, error) {
	eventCode := recordingEventCondition(query.EventCode)
	if eventCode == "*" {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("event log search requires a specific event filter")
	}

	logTypes := recordingLogTypesForEvent(eventCode)
	if len(logTypes) == 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("event log type list is empty")
	}

	result := dahua.NVRRecordingSearchResult{
		DeviceID:  d.ID(),
		Channel:   query.Channel,
		StartTime: query.StartTime.In(time.Local).Format(recordingTimeLayout),
		EndTime:   query.EndTime.In(time.Local).Format(recordingTimeLayout),
		Limit:     query.Limit,
	}

	var startResp nvrLogStartFindResponse
	err := d.rpc.Call(ctx, "log.startFind", map[string]any{
		"condition": map[string]any{
			"Types":     logTypes,
			"StartTime": query.StartTime.In(time.Local).Format(recordingTimeLayout),
			"EndTime":   query.EndTime.In(time.Local).Format(recordingTimeLayout),
			"Translate": true,
			"Order":     "Descent",
		},
	}, &startResp)
	if err != nil {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("start event log search: %w", err)
	}
	if startResp.Token == 0 {
		return dahua.NVRRecordingSearchResult{}, fmt.Errorf("event log search token is empty")
	}

	countKnown := false
	totalCount := 0
	var countResp nvrLogCountResponse
	if err := d.rpc.Call(ctx, "log.getCount", map[string]any{"token": startResp.Token}, &countResp); err != nil {
		d.logger.Debug().Err(err).Int64("token", startResp.Token).Msg("event log count failed")
	} else {
		countKnown = true
		totalCount = countResp.Count
	}
	if countKnown && totalCount == 0 {
		return result, nil
	}

	pageSize := recordingEventLogPageSize(query.Limit)
	maxPages := 10
	if countKnown {
		maxPages = (totalCount + pageSize - 1) / pageSize
		if maxPages > 10 {
			maxPages = 10
		}
	}
	seen := make(map[string]struct{})
	for page := 0; page < maxPages && len(result.Items) < query.Limit; page++ {
		offset := page * pageSize
		var seekResp nvrLogSeekResponse
		err := d.rpc.Call(ctx, "log.doSeekFind", map[string]any{
			"token":  startResp.Token,
			"offset": offset,
			"count":  pageSize,
		}, &seekResp)
		if err != nil {
			if page == 0 {
				return dahua.NVRRecordingSearchResult{}, fmt.Errorf("fetch event log results: %w", err)
			}
			d.logger.Debug().Err(err).Int("offset", offset).Msg("event log page failed")
			break
		}
		if len(seekResp.Items) == 0 {
			break
		}

		for _, logItem := range seekResp.Items {
			recording, eventTime, ok := nvrLogItemToRecording(logItem, query, eventCode)
			if !ok {
				continue
			}
			key := nvrLogRecordingDedupeKey(recording, eventCode, eventTime)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result.Items = append(result.Items, recording)
			if len(result.Items) >= query.Limit {
				break
			}
		}
		if countKnown && offset+pageSize >= totalCount {
			break
		}
		if len(seekResp.Items) < pageSize {
			break
		}
	}

	result.ReturnedCount = len(result.Items)
	return result, nil
}
