package ha

import (
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/streams"
)

func TestRenderDashboardCameraPackage(t *testing.T) {
	result, err := RenderDashboardCameraPackage([]streams.Entry{
		{
			ID:                 "west20_nvr_channel_01",
			Name:               "Front Gate",
			RecommendedProfile: "stable",
			Profiles: map[string]streams.Profile{
				"stable": {
					LocalMJPEGURL: "http://bridge.local:8080/api/v1/media/mjpeg/west20_nvr_channel_01?profile=stable",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderDashboardCameraPackage returned error: %v", err)
	}

	if !strings.Contains(result, "platform: mjpeg") {
		t.Fatalf("missing mjpeg platform in package:\n%s", result)
	}
	if !strings.Contains(result, "mjpeg_url: http://bridge.local:8080/api/v1/media/mjpeg/west20_nvr_channel_01?profile=stable") {
		t.Fatalf("missing dashboard mjpeg url in package:\n%s", result)
	}
}
