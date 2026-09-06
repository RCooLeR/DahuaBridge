package nvr

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"RCooLeR/DahuaBridge/internal/dahua"
)

const maxRecordingScanRows = 100000

// Cursor pagination preserves records sharing a timestamp. Do not seek by the
// last timestamp, or treat a firmware-capped short page as the end of a scan.
func readRecordingPages(ctx context.Context, query dahua.NVRRecordingQuery, fetch func(int) (dahua.NVRRecordingSearchResult, error)) (dahua.NVRRecordingSearchResult, error) {
	if !query.ScanAll {
		return fetch(query.Limit)
	}
	result := dahua.NVRRecordingSearchResult{}
	seen := make(map[[32]byte]bool)
	for {
		if err := ctx.Err(); err != nil {
			return dahua.NVRRecordingSearchResult{}, err
		}
		page, err := fetch(128)
		if err != nil {
			return dahua.NVRRecordingSearchResult{}, err
		}
		if len(page.Items) == 0 {
			result.ReturnedCount = len(result.Items)
			return result, nil
		}
		fingerprint, err := recordingPageFingerprint(page.Items)
		if err != nil {
			return dahua.NVRRecordingSearchResult{}, err
		}
		if seen[fingerprint] || len(result.Items)+len(page.Items) > maxRecordingScanRows {
			return dahua.NVRRecordingSearchResult{}, fmt.Errorf("recording cursor did not finish within the bounded scan; archive checkpoint was not advanced")
		}
		seen[fingerprint] = true
		result.Items = append(result.Items, page.Items...)
	}
}

func recordingPageFingerprint(items any) ([32]byte, error) {
	encoded, err := json.Marshal(items)
	return sha256.Sum256(encoded), err
}
