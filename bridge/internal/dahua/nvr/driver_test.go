package nvr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/onvif"
	"github.com/rs/zerolog"
)

func TestParseSoftwareVersion(t *testing.T) {
	firmware, buildDate := parseSoftwareVersion("version=5.001.0000000.2.R,build:2026-03-31\n")

	if firmware != "5.001.0000000.2.R" {
		t.Fatalf("firmware mismatch: got %q", firmware)
	}

	if buildDate != "2026-03-31" {
		t.Fatalf("buildDate mismatch: got %q", buildDate)
	}
}

func TestParseChannelTitles(t *testing.T) {
	values := map[string]string{
		"table.ChannelTitle[0].Name": "Front Gate",
		"table.ChannelTitle[1].Name": "Garage",
	}

	got := parseChannelTitles(values)

	if got[0] != "Front Gate" {
		t.Fatalf("channel 0 mismatch: got %q", got[0])
	}

	if got[1] != "Garage" {
		t.Fatalf("channel 1 mismatch: got %q", got[1])
	}
}

func TestParseChannelStreams(t *testing.T) {
	values := map[string]string{
		"table.Encode[0].MainFormat[0].Video.resolution":   "3840x2160",
		"table.Encode[0].MainFormat[0].Video.Compression":  "H.265",
		"table.Encode[0].ExtraFormat[0].Video.resolution":  "704x576",
		"table.Encode[0].ExtraFormat[0].Video.Compression": "H.264",
	}

	got := parseChannelStreams(values)
	item := got[0]

	if item.MainResolution != "3840x2160" {
		t.Fatalf("main resolution mismatch: got %q", item.MainResolution)
	}
	if item.MainCodec != "H.265" {
		t.Fatalf("main codec mismatch: got %q", item.MainCodec)
	}
	if item.SubResolution != "704x576" {
		t.Fatalf("sub resolution mismatch: got %q", item.SubResolution)
	}
	if item.SubCodec != "H.264" {
		t.Fatalf("sub codec mismatch: got %q", item.SubCodec)
	}
}

func TestParseDiskInventory(t *testing.T) {
	values := map[string]string{
		"list.info[0].Name":                 "/dev/sda",
		"list.info[0].State":                "Success",
		"list.info[0].Detail[0].TotalBytes": "100.0",
		"list.info[0].Detail[0].UsedBytes":  "40.0",
		"list.info[0].Detail[0].IsError":    "false",
		"list.info[0].Detail[1].TotalBytes": "50.0",
		"list.info[0].Detail[1].UsedBytes":  "20.0",
		"list.info[0].Detail[1].IsError":    "false",
	}

	got := parseDiskInventory(values)
	if len(got) != 1 {
		t.Fatalf("expected 1 disk, got %d", len(got))
	}

	if got[0].Name != "/dev/sda" {
		t.Fatalf("disk name mismatch: got %q", got[0].Name)
	}
	if got[0].State != "Success" {
		t.Fatalf("disk state mismatch: got %q", got[0].State)
	}
	if got[0].TotalBytes != 150 {
		t.Fatalf("disk total mismatch: got %v", got[0].TotalBytes)
	}
	if got[0].UsedBytes != 60 {
		t.Fatalf("disk used mismatch: got %v", got[0].UsedBytes)
	}
}

func TestParseRecordingSearchResult(t *testing.T) {
	values := map[string]string{
		"found":                "2",
		"items[0].Channel":     "0",
		"items[0].StartTime":   "2026-04-28 01:58:16",
		"items[0].EndTime":     "2026-04-28 02:00:02",
		"items[0].FilePath":    "/mnt/dvr/2026-04-28/0/dav/01/1/0/2129/01.58.16-02.00.02[R][0@0][0].dav",
		"items[0].Type":        "dav",
		"items[0].VideoStream": "Main",
		"items[0].Disk":        "8",
		"items[0].Partition":   "0",
		"items[0].Cluster":     "2129",
		"items[0].Length":      "37617664",
		"items[0].CutLength":   "37617664",
		"items[0].Flags[0]":    "Timing",
		"items[0].Flags[1]":    "UnMarked",
		"items[1].Channel":     "0",
		"items[1].StartTime":   "2026-04-28 02:00:02",
		"items[1].EndTime":     "2026-04-28 02:30:00",
		"items[1].FilePath":    "/mnt/dvr/2026-04-28/0/dav/02/1/0/2296/02.00.02-02.30.00[R][0@0][0].dav",
		"items[1].Type":        "dav",
		"items[1].VideoStream": "Main",
		"items[1].Disk":        "8",
		"items[1].Partition":   "0",
		"items[1].Cluster":     "2296",
		"items[1].Length":      "611844096",
		"items[1].CutLength":   "611844096",
		"items[1].Flags[0]":    "Timing",
	}

	got := parseRecordingSearchResult(values)

	if got.ReturnedCount != 2 {
		t.Fatalf("unexpected returned count %d", got.ReturnedCount)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 recording items, got %d", len(got.Items))
	}
	if got.Items[0].Channel != 1 {
		t.Fatalf("expected channel 1, got %d", got.Items[0].Channel)
	}
	if got.Items[0].Disk != 8 || got.Items[0].Cluster != 2129 {
		t.Fatalf("unexpected first item disk/cluster: %+v", got.Items[0])
	}
	if got.Items[0].LengthBytes != 37617664 || got.Items[0].CutLengthBytes != 37617664 {
		t.Fatalf("unexpected first item lengths: %+v", got.Items[0])
	}
	if len(got.Items[0].Flags) != 2 {
		t.Fatalf("expected 2 flags, got %+v", got.Items[0].Flags)
	}
	if got.Items[1].StartTime != "2026-04-28 02:00:02" || got.Items[1].EndTime != "2026-04-28 02:30:00" {
		t.Fatalf("unexpected second item times: %+v", got.Items[1])
	}
}

func TestParseRPCRecordingSearchResult(t *testing.T) {
	got := parseRPCRecordingSearchResult(map[string]any{
		"found": float64(2),
		"items": []any{
			map[string]any{
				"Channel":     float64(0),
				"StartTime":   "2026-04-28 01:58:16",
				"EndTime":     "2026-04-28 02:00:02",
				"FilePath":    "/mnt/dvr/2026-04-28/0/dav/01/1/0/2129/01.58.16-02.00.02[R][0@0][0].dav",
				"Type":        "dav",
				"VideoStream": "Main",
				"Disk":        float64(8),
				"Partition":   float64(0),
				"Cluster":     float64(2129),
				"Length":      float64(37617664),
				"CutLength":   float64(37617664),
				"Flags":       []any{"Timing", "UnMarked"},
			},
			map[string]any{
				"Channel":     float64(0),
				"StartTime":   "2026-04-28 02:00:02",
				"EndTime":     "2026-04-28 02:30:00",
				"FilePath":    "/mnt/dvr/2026-04-28/0/dav/02/1/0/2296/02.00.02-02.30.00[R][0@0][0].dav",
				"Type":        "dav",
				"VideoStream": "Main",
				"Disk":        float64(8),
				"Partition":   float64(0),
				"Cluster":     float64(2296),
				"Length":      float64(611844096),
				"CutLength":   float64(611844096),
				"Flags":       []any{"Timing"},
			},
		},
	})

	if got.ReturnedCount != 2 {
		t.Fatalf("unexpected returned count %d", got.ReturnedCount)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 recording items, got %d", len(got.Items))
	}
	if got.Items[0].Channel != 1 {
		t.Fatalf("expected channel 1, got %d", got.Items[0].Channel)
	}
	if got.Items[0].Disk != 8 || got.Items[0].Cluster != 2129 {
		t.Fatalf("unexpected first item disk/cluster: %+v", got.Items[0])
	}
	if len(got.Items[0].Flags) != 2 {
		t.Fatalf("expected 2 flags, got %+v", got.Items[0].Flags)
	}
}

func TestParseRPCRecordingSearchResultSupportsInfos(t *testing.T) {
	got := parseRPCRecordingSearchResult(map[string]any{
		"found": float64(1),
		"infos": []any{
			map[string]any{
				"Channel":     float64(7),
				"StartTime":   "2026-04-30 00:00:00",
				"EndTime":     "2026-04-30 00:10:00",
				"FilePath":    "/mnt/dvr/2026-04-30/7/dav/00.00.00-00.10.00.dav",
				"Type":        "dav",
				"VideoStream": "Main",
				"Flags":       []any{"Timing"},
			},
		},
	})

	if got.ReturnedCount != 1 {
		t.Fatalf("unexpected returned count %d", got.ReturnedCount)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected 1 recording item, got %d", len(got.Items))
	}
	if got.Items[0].Channel != 8 {
		t.Fatalf("expected channel 8, got %d", got.Items[0].Channel)
	}
	if got.Items[0].FilePath == "" || got.Items[0].StartTime == "" {
		t.Fatalf("unexpected item %+v", got.Items[0])
	}
}

func TestDriverFindRecordingsUsesRPCMediaFileFind(t *testing.T) {
	loginRequests := 0
	var rpcCalls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/RPC2_Login":
			loginRequests++
			if loginRequests == 1 {
				_, _ = w.Write([]byte(`{"id":1,"params":{"realm":"Login to Test","random":"12345","encryption":"Default"},"result":false,"session":"sess1","error":{"code":268632079,"message":"challenge"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":2,"params":{},"result":true,"session":"sess2"}`))
		case "/RPC2":
			var payload struct {
				Method string `json:"method"`
				Object any    `json:"object"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode rpc request: %v", err)
			}
			rpcCalls = append(rpcCalls, payload.Method)
			switch payload.Method {
			case "mediaFileFind.factory.create":
				_, _ = w.Write([]byte(`{"id":10,"result":758262256,"session":"sess2"}`))
			case "mediaFileFind.findFile":
				if payload.Object != float64(758262256) {
					t.Fatalf("unexpected findFile object %#v", payload.Object)
				}
				_, _ = w.Write([]byte(`{"id":11,"result":true,"session":"sess2"}`))
			case "mediaFileFind.findNextFile":
				if payload.Object != float64(758262256) {
					t.Fatalf("unexpected findNextFile object %#v", payload.Object)
				}
				_, _ = w.Write([]byte(`{"id":12,"params":{"found":1,"items":[{"Channel":0,"StartTime":"2026-04-30 00:00:00","EndTime":"2026-04-30 00:10:00","FilePath":"/mnt/dvr/2026-04-30/0/dav/00.00.00-00.10.00.dav","Type":"dav","VideoStream":"Main","Disk":8,"Partition":0,"Cluster":100,"Length":1234,"CutLength":1234,"Flags":["Timing"]}]},"result":true,"session":"sess2"}`))
			case "mediaFileFind.close":
				_, _ = w.Write([]byte(`{"id":13,"result":true,"session":"sess2"}`))
			case "mediaFileFind.destroy":
				_, _ = w.Write([]byte(`{"id":14,"result":true,"session":"sess2"}`))
			default:
				http.Error(w, "unexpected rpc method", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{1},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))

	got, err := driver.FindRecordings(context.Background(), dahua.NVRRecordingQuery{
		Channel:   1,
		StartTime: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 30, 0, 10, 0, 0, time.UTC),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("FindRecordings returned error: %v", err)
	}
	if got.DeviceID != "west20_nvr" || got.Channel != 1 || got.ReturnedCount != 1 {
		t.Fatalf("unexpected search result %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].FilePath == "" || got.Items[0].Channel != 1 {
		t.Fatalf("unexpected recording items %+v", got.Items)
	}
	if len(rpcCalls) != 5 {
		t.Fatalf("expected 5 rpc calls, got %d (%+v)", len(rpcCalls), rpcCalls)
	}
	if rpcCalls[0] != "mediaFileFind.factory.create" || rpcCalls[1] != "mediaFileFind.findFile" || rpcCalls[2] != "mediaFileFind.findNextFile" || rpcCalls[3] != "mediaFileFind.close" || rpcCalls[4] != "mediaFileFind.destroy" {
		t.Fatalf("unexpected rpc sequence %+v", rpcCalls)
	}
}

func TestDriverFindRecordingsEventFilterUsesRPCEventLog(t *testing.T) {
	loginRequests := 0
	var rpcCalls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/RPC2_Login":
			loginRequests++
			if loginRequests == 1 {
				_, _ = w.Write([]byte(`{"id":1,"params":{"realm":"Login to Test","random":"12345","encryption":"Default"},"result":false,"session":"sess1","error":{"code":268632079,"message":"challenge"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":2,"params":{},"result":true,"session":"sess2"}`))
		case "/RPC2":
			var payload struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode rpc request: %v", err)
			}
			rpcCalls = append(rpcCalls, payload.Method)
			switch payload.Method {
			case "log.startFind":
				condition, ok := payload.Params["condition"].(map[string]any)
				if !ok {
					t.Fatalf("missing log condition: %#v", payload.Params)
				}
				rawTypes, ok := condition["Types"].([]any)
				if !ok {
					t.Fatalf("missing log Types: %#v", condition)
				}
				types := make([]string, 0, len(rawTypes))
				for _, rawType := range rawTypes {
					value, ok := rawType.(string)
					if !ok {
						t.Fatalf("unexpected log type %#v", rawType)
					}
					types = append(types, value)
				}
				if containsString(types, "Event") {
					t.Fatalf("event log filter must not include broad Event type: %+v", types)
				}
				if !containsString(types, "Event.VideoMotion.Start") || !containsString(types, "Event.VideoMotion.Stop") {
					t.Fatalf("expected VideoMotion log types, got %+v", types)
				}
				if condition["Order"] != "Descent" || condition["Translate"] != true {
					t.Fatalf("unexpected log condition %+v", condition)
				}
				_, _ = w.Write([]byte(`{"id":10,"params":{"token":67108871},"result":true,"session":"sess2"}`))
			case "log.getCount":
				if payload.Params["token"] != float64(67108871) {
					t.Fatalf("unexpected count token %#v", payload.Params["token"])
				}
				_, _ = w.Write([]byte(`{"id":11,"params":{"count":3},"result":true,"session":"sess2"}`))
			case "log.doSeekFind":
				if payload.Params["token"] != float64(67108871) {
					t.Fatalf("unexpected seek token %#v", payload.Params["token"])
				}
				_, _ = w.Write([]byte(`{"id":12,"params":{"found":3,"items":[{"Channel":1,"Code":"VideoMotion","Time":"2026-04-30 12:00:30","Type":"Motion Detect","Detail":"motion started"},{"Channel":2,"Code":"VideoMotion","Time":"2026-04-30 12:00:40","Type":"Motion Detect"},{"Channel":1,"Code":"NetMonitorAbort","Time":"2026-04-30 12:01:00","Type":"Network"}]},"result":true,"session":"sess2"}`))
			default:
				http.Error(w, "unexpected rpc method", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{1},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))

	got, err := driver.FindRecordings(context.Background(), dahua.NVRRecordingQuery{
		Channel:   1,
		StartTime: time.Date(2026, 4, 30, 11, 59, 0, 0, time.Local),
		EndTime:   time.Date(2026, 4, 30, 12, 5, 0, 0, time.Local),
		Limit:     10,
		EventCode: "motion",
	})
	if err != nil {
		t.Fatalf("FindRecordings returned error: %v", err)
	}
	if got.DeviceID != "west20_nvr" || got.Channel != 1 || got.ReturnedCount != 1 {
		t.Fatalf("unexpected search result %+v", got)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected one event item, got %+v", got.Items)
	}
	item := got.Items[0]
	if item.Source != "nvr_event" || item.FilePath != "" || item.Type != "Event.VideoMotion" {
		t.Fatalf("unexpected event recording %+v", item)
	}
	if item.StartTime != "2026-04-30 12:00:15" || item.EndTime != "2026-04-30 12:01:15" {
		t.Fatalf("unexpected event playback window %+v", item)
	}
	if !containsString(item.Flags, "Event") || !containsString(item.Flags, "VideoMotion") {
		t.Fatalf("expected event flags, got %+v", item.Flags)
	}
	if len(rpcCalls) != 3 || rpcCalls[0] != "log.startFind" || rpcCalls[1] != "log.getCount" || rpcCalls[2] != "log.doSeekFind" {
		t.Fatalf("unexpected rpc sequence %+v", rpcCalls)
	}
}

func TestDriverFindRecordingsEventOnlyUsesSMDFinder(t *testing.T) {
	loginRequests := 0
	var rpcCalls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/RPC2_Login":
			loginRequests++
			if loginRequests == 1 {
				_, _ = w.Write([]byte(`{"id":1,"params":{"realm":"Login to Test","random":"12345","encryption":"Default"},"result":false,"session":"sess1","error":{"code":268632079,"message":"challenge"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":2,"params":{},"result":true,"session":"sess2"}`))
		case "/RPC2":
			var payload struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode rpc request: %v", err)
			}
			rpcCalls = append(rpcCalls, payload.Method)
			switch payload.Method {
			case "SmdDataFinder.startFind":
				condition, ok := payload.Params["Condition"].(map[string]any)
				if !ok {
					t.Fatalf("missing smd condition: %#v", payload.Params)
				}
				if channels, ok := condition["Channels"].([]any); !ok || len(channels) != 1 || channels[0] != float64(0) {
					t.Fatalf("unexpected smd channels: %#v", condition["Channels"])
				}
				rawTypes, ok := condition["SmdType"].([]any)
				if !ok || len(rawTypes) != 1 || rawTypes[0] != "smdTypeHuman" {
					t.Fatalf("unexpected smd types: %#v", condition["SmdType"])
				}
				_, _ = w.Write([]byte(`{"id":10,"params":{"Count":2,"Token":10},"result":true,"session":"sess2"}`))
			case "SmdDataFinder.doFind":
				if payload.Params["Token"] != float64(10) || payload.Params["Offset"] != float64(0) {
					t.Fatalf("unexpected smd doFind params: %+v", payload.Params)
				}
				_, _ = w.Write([]byte(`{"id":11,"params":{"SmdInfo":[{"Channel":0,"StartTime":"2026-05-01 15:09:07","EndTime":"2026-05-01 15:09:28","Type":"smdTypeHuman"},{"Channel":1,"StartTime":"2026-05-01 15:10:00","EndTime":"2026-05-01 15:10:20","Type":"smdTypeHuman"}]},"result":true,"session":"sess2"}`))
			default:
				http.Error(w, "unexpected rpc method", http.StatusBadRequest)
			}
		default:
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{1},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))

	got, err := driver.FindRecordings(context.Background(), dahua.NVRRecordingQuery{
		Channel:   1,
		StartTime: time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local),
		EndTime:   time.Date(2026, 5, 1, 23, 59, 59, 0, time.Local),
		Limit:     10,
		EventCode: "smdTypeHuman",
		EventOnly: true,
	})
	if err != nil {
		t.Fatalf("FindRecordings returned error: %v", err)
	}
	if got.DeviceID != "west20_nvr" || got.Channel != 1 || got.ReturnedCount != 1 {
		t.Fatalf("unexpected search result %+v", got)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected one smd event item, got %+v", got.Items)
	}
	item := got.Items[0]
	if item.Source != "nvr_event" || item.FilePath != "" || item.Type != "Event.smdTypeHuman" {
		t.Fatalf("unexpected smd event recording %+v", item)
	}
	if item.StartTime != "2026-05-01 15:09:07" || item.EndTime != "2026-05-01 15:09:28" {
		t.Fatalf("unexpected smd event time window %+v", item)
	}
	if !containsString(item.Flags, "Event") || !containsString(item.Flags, "smdTypeHuman") {
		t.Fatalf("expected smd event flags, got %+v", item.Flags)
	}
	if len(rpcCalls) != 2 || rpcCalls[0] != "SmdDataFinder.startFind" || rpcCalls[1] != "SmdDataFinder.doFind" {
		t.Fatalf("unexpected rpc sequence %+v", rpcCalls)
	}
}

func TestDriverFindRecordingsEventFilterUsesEventFlagAndDropsTimingResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "factory.create":
			_, _ = w.Write([]byte("result=handle1\n"))
		case "findFile":
			query := r.URL.Query()
			if query.Get("condition.Events[0]") != "VideoMotion" {
				t.Fatalf("expected VideoMotion event condition, got %+v", query)
			}
			if query.Get("condition.Flag[0]") != "Event" {
				t.Fatalf("expected Event flag condition, got %+v", query)
			}
			_, _ = w.Write([]byte("OK"))
		case "findNextFile":
			_, _ = w.Write([]byte(
				"found=1\n" +
					"items[0].Channel=0\n" +
					"items[0].StartTime=2026-04-30 00:00:00\n" +
					"items[0].EndTime=2026-04-30 00:10:00\n" +
					"items[0].FilePath=/mnt/dvr/2026-04-30/0/dav/00.00.00-00.10.00[R][0@0][0].dav\n" +
					"items[0].Type=dav\n" +
					"items[0].VideoStream=Main\n" +
					"items[0].Flags[0]=Timing\n",
			))
		case "close":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := config.DeviceConfig{
		ID:               "west20_nvr",
		BaseURL:          server.URL,
		Username:         "assistant",
		Password:         "secret",
		ChannelAllowlist: []int{1},
		RequestTimeout:   5 * time.Second,
	}
	metricsRegistry := metrics.New(buildinfo.Info())
	driver := New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), metricsRegistry, cgi.New(cfg, metricsRegistry))
	driver.rpc = nil

	got, err := driver.FindRecordings(context.Background(), dahua.NVRRecordingQuery{
		Channel:   1,
		StartTime: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 30, 0, 10, 0, 0, time.UTC),
		Limit:     10,
		EventCode: "motion",
	})
	if err != nil {
		t.Fatalf("FindRecordings returned error: %v", err)
	}
	if got.ReturnedCount != 0 || len(got.Items) != 0 {
		t.Fatalf("expected regular timing result to be hidden for event filter, got %+v", got)
	}
}

func TestRecordingEventConditionSupportsArchiveAliases(t *testing.T) {
	testCases := map[string]string{
		"com.All":                  "*",
		"com.Human":                "SmartMotionHuman",
		"smdTypeHuman":             "SmartMotionHuman",
		"ivs.MotorVehicle":         "SmartMotionVehicle",
		"smdTypeVehicle":           "SmartMotionVehicle",
		"com.Animal":               "AnimalDetection",
		"smdTypeAnimal":            "AnimalDetection",
		"CrossLineDetection":       "CrossLineDetection",
		"com.CrossRegionDetection": "CrossRegionDetection",
		"ivs.LeftDetection":        "LeftDetection",
		"com.MoveDetection":        "MoveDetection",
	}

	for input, want := range testCases {
		if got := recordingEventCondition(input); got != want {
			t.Fatalf("recordingEventCondition(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNVRLogItemMatchesRecordingEventSupportsAliasCodes(t *testing.T) {
	testCases := []struct {
		name     string
		filter   string
		itemCode string
	}{
		{name: "human com", filter: "human", itemCode: "com.Human"},
		{name: "human smd", filter: "human", itemCode: "smdTypeHuman"},
		{name: "vehicle ivs", filter: "vehicle", itemCode: "ivs.MotorVehicle"},
		{name: "vehicle smd", filter: "vehicle", itemCode: "smdTypeVehicle"},
		{name: "animal com", filter: "com.Animal", itemCode: "AnimalDetection"},
		{name: "left ivs", filter: "LeftDetection", itemCode: "ivs.LeftDetection"},
		{name: "move com", filter: "MoveDetection", itemCode: "com.MoveDetection"},
	}

	for _, testCase := range testCases {
		item := nvrLogItem{Code: testCase.itemCode}
		if !nvrLogItemMatchesRecordingEvent(item, testCase.filter) {
			t.Fatalf("%s: expected %q to match filter %q", testCase.name, testCase.itemCode, testCase.filter)
		}
	}
}

func TestSummarizeDisks(t *testing.T) {
	summary := summarizeDisks([]diskInventory{
		{
			Index:      0,
			State:      "Success",
			TotalBytes: 200,
			UsedBytes:  50,
		},
		{
			Index:      1,
			State:      "Error",
			TotalBytes: 100,
			UsedBytes:  80,
			IsError:    true,
		},
	})

	if !summary.DiskFault {
		t.Fatal("expected disk fault")
	}
	if summary.DiskErrorCount != 1 {
		t.Fatalf("unexpected disk error count %d", summary.DiskErrorCount)
	}
	if summary.DiskHealthyCount != 1 {
		t.Fatalf("unexpected healthy disk count %d", summary.DiskHealthyCount)
	}
	if summary.TotalBytes != 300 {
		t.Fatalf("unexpected total bytes %v", summary.TotalBytes)
	}
	if summary.UsedBytes != 130 {
		t.Fatalf("unexpected used bytes %v", summary.UsedBytes)
	}
	if summary.FreeBytes != 170 {
		t.Fatalf("unexpected free bytes %v", summary.FreeBytes)
	}
	if summary.UsedPercent < 43.3 || summary.UsedPercent > 43.4 {
		t.Fatalf("unexpected used percent %v", summary.UsedPercent)
	}
}

func TestParseEventPayload(t *testing.T) {
	values := parseEventPayload("Code=VideoMotion;action=Start;index=0")

	if values["Code"] != "VideoMotion" {
		t.Fatalf("code mismatch: got %q", values["Code"])
	}
	if values["action"] != "Start" {
		t.Fatalf("action mismatch: got %q", values["action"])
	}
	if values["index"] != "0" {
		t.Fatalf("index mismatch: got %q", values["index"])
	}
}

func TestNormalizeEvent(t *testing.T) {
	event, ok := normalizeEvent("west20_nvr", map[string]string{
		"Code":   "VideoMotion",
		"action": "Start",
		"index":  "1",
	})
	if !ok {
		t.Fatal("expected event to normalize")
	}

	if event.ChildID != "west20_nvr_channel_02" {
		t.Fatalf("child id mismatch: got %q", event.ChildID)
	}
	if event.Channel != 2 {
		t.Fatalf("channel mismatch: got %d", event.Channel)
	}
	if event.Action != "start" {
		t.Fatalf("action mismatch: got %q", event.Action)
	}
}

func TestParseBoundary(t *testing.T) {
	got := parseBoundary("multipart/x-mixed-replace; boundary=myboundary")
	if got != "myboundary" {
		t.Fatalf("boundary mismatch: got %q", got)
	}
}

func TestBuildRTSPURL(t *testing.T) {
	got := buildRTSPURL("http://nvr.example.local", 3, 1)
	want := "rtsp://nvr.example.local:554/cam/realmonitor?channel=3&subtype=1"
	if got != want {
		t.Fatalf("rtsp url mismatch:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestInvalidateInventoryCache(t *testing.T) {
	driver := &Driver{
		cachedInventory: &inventorySnapshot{
			Channels: []channelInventory{{Index: 0, Name: "Front Gate"}},
		},
		inventoryExpires: time.Now().Add(time.Minute),
	}

	driver.InvalidateInventoryCache()

	if driver.cachedInventory != nil {
		t.Fatalf("expected cached inventory to be cleared: %+v", driver.cachedInventory)
	}
	if !driver.inventoryExpires.IsZero() {
		t.Fatalf("expected inventory expiry to be cleared: %v", driver.inventoryExpires)
	}
}

func TestAttachONVIFChannelStateIncludesSnapshotURI(t *testing.T) {
	device := dahua.Device{Attributes: map[string]string{}}
	state := dahua.DeviceState{Info: map[string]any{}}
	discovery := onvif.Discovery{
		Profiles: []onvif.Profile{
			{
				Token:       "Profile_1",
				Name:        "MainStream-H264",
				Encoding:    "H264",
				Width:       1920,
				Height:      1080,
				Channel:     1,
				Subtype:     0,
				StreamURI:   "rtsp://nvr.example.local:554/cam/realmonitor?channel=1&subtype=0",
				SnapshotURI: "http://nvr.example.local/onvif/snapshot1.jpg",
				IsH264:      true,
			},
		},
	}

	attachONVIFChannelState(&device, &state, discovery, 1)

	if device.Attributes["onvif_snapshot_url"] != "http://nvr.example.local/onvif/snapshot1.jpg" {
		t.Fatalf("unexpected device snapshot url %q", device.Attributes["onvif_snapshot_url"])
	}
	if state.Info["onvif_snapshot_url"] != "http://nvr.example.local/onvif/snapshot1.jpg" {
		t.Fatalf("unexpected state snapshot url %#v", state.Info["onvif_snapshot_url"])
	}
}

func TestApplyRecordingOverrideForcesChannelState(t *testing.T) {
	supported := true
	active := true
	driver := &Driver{
		cfg: config.DeviceConfig{
			ChannelRecordingOverrides: []config.ChannelRecordingControlOverride{
				{Channel: 9, Supported: &supported, Active: &active, Mode: "auto"},
			},
		},
	}

	capabilities := driver.applyRecordingOverride(9, dahua.NVRRecordingCapabilities{})
	if !capabilities.Supported || !capabilities.Active || capabilities.Mode != "auto" {
		t.Fatalf("unexpected overridden recording capabilities %+v", capabilities)
	}
}

func TestChannelInventoryLooksLikePlaceholder(t *testing.T) {
	tests := []struct {
		name string
		item channelInventory
		want bool
	}{
		{
			name: "ukrainian placeholder",
			item: channelInventory{Index: 11, Name: "Канал12"},
			want: true,
		},
		{
			name: "english placeholder",
			item: channelInventory{Index: 6, Name: "Channel 07"},
			want: true,
		},
		{
			name: "real named channel",
			item: channelInventory{Index: 11, Name: "Garage"},
			want: false,
		},
		{
			name: "mismatched number",
			item: channelInventory{Index: 11, Name: "Канал13"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := channelInventoryLooksLikePlaceholder(tt.item); got != tt.want {
				t.Fatalf("channelInventoryLooksLikePlaceholder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelInventoryWanted(t *testing.T) {
	cfg := config.DeviceConfig{ChannelAllowlist: []int{1, 2, 3}}

	if channelInventoryWanted(cfg, channelInventory{
		Index:          3,
		Name:           "Front Gate",
		MainResolution: "2560x1440",
		MainCodec:      "H.265",
	}) {
		t.Fatal("expected channel 4 to be rejected by allowlist")
	}

	if channelInventoryWanted(config.DeviceConfig{}, channelInventory{
		Index:          11,
		Name:           "Канал12",
		MainResolution: "2880x1620",
		MainCodec:      "H.265",
		SubResolution:  "640x480",
		SubCodec:       "H.264",
	}) {
		t.Fatal("expected placeholder channel to be rejected")
	}

	if !channelInventoryWanted(config.DeviceConfig{}, channelInventory{
		Index:          0,
		Name:           "Вхід",
		MainResolution: "3840x2160",
		MainCodec:      "H.265",
		SubResolution:  "704x576",
		SubCodec:       "H.265",
	}) {
		t.Fatal("expected real channel to be accepted")
	}
}

func TestParseRemoteDevices(t *testing.T) {
	got := parseRemoteDevices(map[string]string{
		"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_7.Address":             "192.168.150.120",
		"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_7.DeviceType":          "DH-T4A-PV",
		"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_7.HttpPort":            "80",
		"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_7.HttpsPort":           "443",
		"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_7.Port":                "37777",
		"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_7.RtspPort":            "554",
		"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_7.UserName":            "admin",
		"table.RemoteDevice.uuid:System_CONFIG_NETCAMERA_INFO_7.VideoInputs[0].Name": "Boiler Room",
	})

	item, ok := got[7]
	if !ok {
		t.Fatalf("expected remote device 7 in %+v", got)
	}
	if item.Address != "192.168.150.120" || item.DeviceType != "DH-T4A-PV" {
		t.Fatalf("unexpected remote device identity %+v", item)
	}
	if item.HTTPPort != 80 || item.HTTPSPort != 443 || item.SDKPort != 37777 || item.RTSPPort != 554 {
		t.Fatalf("unexpected remote device ports %+v", item)
	}
	if item.UserName != "admin" || item.Name != "Boiler Room" {
		t.Fatalf("unexpected remote device metadata %+v", item)
	}
}

func TestDirectIPCTargetForChannelUsesMatchingIPCConfig(t *testing.T) {
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID: "west20_nvr",
			DirectIPCCredentials: []config.ChannelDirectIPCCredential{
				{
					NVRChannel:        11,
					DirectIPCIP:       "192.168.150.20",
					DirectIPCUser:     "admin",
					DirectIPCPassword: "secret",
				},
			},
			RequestTimeout: 5 * time.Second,
		},
		ipcCfgs: []config.DeviceConfig{
			{
				ID:              "hut_ipc",
				BaseURL:         "https://192.168.150.20",
				Username:        "admin",
				Password:        "secret",
				InsecureSkipTLS: true,
				RequestTimeout:  5 * time.Second,
			},
		},
		cachedInventory: &inventorySnapshot{
			Channels: []channelInventory{
				{
					Index: 10,
					Name:  "Hut",
					RemoteDevice: remoteDeviceInventory{
						Address:    "192.168.150.20",
						DeviceType: "DH-H4C-GE",
						HTTPSPort:  443,
					},
				},
			},
		},
		inventoryExpires: time.Now().Add(time.Minute),
	}

	target, err := driver.directIPCTargetForChannel(context.Background(), 11)
	if err != nil {
		t.Fatalf("directIPCTargetForChannel returned error: %v", err)
	}
	if target == nil {
		t.Fatal("expected direct ipc target")
	}
	if target.BaseURL != "https://192.168.150.20" {
		t.Fatalf("unexpected direct ipc base url %q", target.BaseURL)
	}
	if !target.InsecureTLS {
		t.Fatal("expected direct ipc target to inherit insecure tls setting")
	}
	if target.DeviceType != "DH-H4C-GE" {
		t.Fatalf("unexpected direct ipc device type %+v", target)
	}
}

func TestDirectIPCTargetForChannelUsesCredentialBaseURL(t *testing.T) {
	driver := &Driver{
		cfg: config.DeviceConfig{
			ID: "west20_nvr",
			DirectIPCCredentials: []config.ChannelDirectIPCCredential{
				{
					NVRChannel:        11,
					DirectIPCIP:       "192.168.150.20",
					DirectIPCBaseURL:  "https://192.168.150.20",
					DirectIPCUser:     "admin",
					DirectIPCPassword: "secret",
				},
			},
			RequestTimeout: 5 * time.Second,
		},
		cachedInventory: &inventorySnapshot{
			Channels: []channelInventory{
				{
					Index: 10,
					Name:  "Hut",
					RemoteDevice: remoteDeviceInventory{
						Address:    "192.168.150.20",
						DeviceType: "DH-H4C-GE",
						HTTPPort:   80,
						HTTPSPort:  37777,
					},
				},
			},
		},
		inventoryExpires: time.Now().Add(time.Minute),
	}

	target, err := driver.directIPCTargetForChannel(context.Background(), 11)
	if err != nil {
		t.Fatalf("directIPCTargetForChannel returned error: %v", err)
	}
	if target == nil {
		t.Fatal("expected direct ipc target")
	}
	if target.BaseURL != "https://192.168.150.20" {
		t.Fatalf("unexpected direct ipc base url %q", target.BaseURL)
	}
}
