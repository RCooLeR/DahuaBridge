package nvr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/dahua/cgi"
	"RCooLeR/DahuaBridge/internal/metrics"
	"github.com/rs/zerolog"
)

func TestCompleteCGIScanUsesCursorAndPreservesTimestampTies(t *testing.T) {
	var offset int
	var closed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "factory.create":
			fmt.Fprintln(w, "result=handle")
		case "findFile":
			fmt.Fprint(w, "OK")
		case "close":
			closed.Store(true)
			fmt.Fprint(w, "OK")
		case "findNextFile":
			count := min(32, 300-offset) // Simulate firmware returning short pages.
			fmt.Fprintf(w, "found=%d\n", count)
			for i := range count {
				fmt.Fprintf(w, "items[%d].Channel=0\nitems[%d].FilePath=/recordings/%d.dav\nitems[%d].StartTime=2026-09-06 12:00:00\nitems[%d].EndTime=2026-09-06 12:00:20\n", i, i, offset+i, i, i)
			}
			offset += count
		default:
			http.Error(w, "bad action", 400)
		}
	}))
	defer server.Close()
	driver := newPaginationDriver(server.URL)
	driver.rpc = nil
	query := paginationQuery()
	query.ScanAll = true
	result, err := driver.FindRecordings(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 300 || result.ReturnedCount != 300 {
		t.Fatalf("incomplete result: %d", len(result.Items))
	}
	if !closed.Load() {
		t.Fatal("recorder cursor was not closed")
	}
	if result.Items[0].StartTime != result.Items[299].StartTime || result.Items[0].FilePath == result.Items[299].FilePath {
		t.Fatal("timestamp ties were not preserved")
	}
}

func TestCompleteRPCMediaAndIVSScansReadEveryCursorPage(t *testing.T) {
	for _, event := range []bool{false, true} {
		t.Run(fmt.Sprint(event), func(t *testing.T) {
			var offset int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Method string `json:"method"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				response := map[string]any{"id": 1, "session": "test", "result": true}
				switch req.Method {
				case "global.login", "mediaFileFind.findFile", "mediaFileFind.close", "mediaFileFind.destroy", "mediaFileFind.setQueryResultOptions":
				case "mediaFileFind.factory.create":
					response["result"] = 123
				case "mediaFileFind.getCount":
					response["params"] = map[string]any{"count": 300}
				case "mediaFileFind.findNextFile":
					count := min(32, 300-offset)
					items := make([]map[string]any, 0, count)
					for i := range count {
						items = append(items, map[string]any{"Channel": 0, "StartTime": "2026-09-06 12:00:00", "EndTime": "2026-09-06 12:00:20", "FilePath": fmt.Sprintf("/recordings/%d.dav", offset+i), "Type": "dav"})
					}
					offset += count
					response["params"] = map[string]any{"found": count, "items": items}
				default:
					t.Errorf("unexpected method %s", req.Method)
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			query := paginationQuery()
			query.ScanAll = true
			query.EventOnly = event
			if event {
				query.EventCode = "tripwire"
			}
			result, err := newPaginationDriver(server.URL).FindRecordings(t.Context(), query)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 300 {
				t.Fatalf("got %d records", len(result.Items))
			}
		})
	}
}

func TestCompleteSMDScanAdvancesByActualShortPageLength(t *testing.T) {
	var offsets []int
	start := time.Date(2026, 9, 6, 12, 0, 0, 0, time.Local)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		response := map[string]any{"id": 1, "session": "test", "result": true}
		switch req.Method {
		case "global.login":
		case "SmdDataFinder.startFind":
			response["params"] = map[string]any{"Token": 1, "Count": 300}
		case "SmdDataFinder.doFind":
			offset := int(req.Params["Offset"].(float64))
			offsets = append(offsets, offset)
			items := make([]nvrSMDInfo, 0, 32)
			for i := offset; i < min(offset+32, 300); i++ {
				items = append(items, nvrSMDInfo{Channel: 0, StartTime: start.Format(recordingTimeLayout), EndTime: start.Add(time.Duration(i+1) * time.Second).Format(recordingTimeLayout), SMDType: "smdTypeHuman"})
			}
			response["params"] = map[string]any{"SmdInfo": items}
		case "mediaFileFind.factory.create":
			response["result"] = 0 // File resolution is optional.
		default:
			t.Errorf("unexpected method %s", req.Method)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	query := paginationQuery()
	query.ScanAll = true
	query.EventOnly = true
	query.EventCode = "human"
	result, err := newPaginationDriver(server.URL).FindRecordings(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 300 || len(offsets) != 10 || offsets[1] != 32 || offsets[9] != 288 {
		t.Fatalf("result=%d offsets=%v", len(result.Items), offsets)
	}
}

func TestCompleteScanRejectsRepeatingCursorAndCancellation(t *testing.T) {
	query := paginationQuery()
	query.ScanAll = true
	calls := 0
	fetch := func(int) (dahua.NVRRecordingSearchResult, error) {
		calls++
		return dahua.NVRRecordingSearchResult{Items: []dahua.NVRRecording{{FilePath: "same"}}}, nil
	}
	if _, err := readRecordingPages(t.Context(), query, fetch); err == nil || calls != 2 {
		t.Fatal("repeating cursor did not fail promptly")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls = 0
	if _, err := readRecordingPages(ctx, query, fetch); err == nil || calls != 0 {
		t.Fatal("canceled scan fetched another page")
	}
	query.ScanAll = false
	calls = 0
	if _, err := readRecordingPages(t.Context(), query, fetch); err != nil || calls != 1 {
		t.Fatal("public bounded query changed behavior")
	}
}

func TestCompleteSMDScanRejectsInvalidCursorWithoutFetching(t *testing.T) {
	for _, tc := range []struct {
		name         string
		token, count int
		fail         bool
	}{
		{"missing_cursor", 0, 12, true},
		{"negative_count", 1, -1, true},
		{"empty_result", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Method string `json:"method"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				response := map[string]any{"id": 1, "session": "test", "result": true}
				switch req.Method {
				case "global.login":
				case "SmdDataFinder.startFind":
					response["params"] = map[string]any{"Token": tc.token, "Count": tc.count}
				default:
					t.Errorf("invalid cursor issued request %s", req.Method)
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			query := paginationQuery()
			query.ScanAll, query.EventOnly, query.EventCode = true, true, "human"
			result, err := newPaginationDriver(server.URL).FindRecordings(t.Context(), query)
			if (err != nil) != tc.fail || len(result.Items) != 0 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func newPaginationDriver(base string) *Driver {
	cfg := config.DeviceConfig{ID: "nvr", BaseURL: base, Username: "viewer", Password: "test", RequestTimeout: time.Second}
	registry := metrics.New(buildinfo.Info())
	return New(cfg, config.ImouConfig{}, nil, nil, zerolog.Nop(), registry, cgi.New(cfg, registry))
}
func paginationQuery() dahua.NVRRecordingQuery {
	start := time.Date(2026, 9, 6, 12, 0, 0, 0, time.Local)
	return dahua.NVRRecordingQuery{Channel: 1, StartTime: start, EndTime: start.Add(time.Hour), Limit: 128}
}
