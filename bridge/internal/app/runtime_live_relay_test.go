package app

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/ha"
)

func TestHomeAssistantLiveURLIsStableAndHidesUpstreamRoutes(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	runtime.cfg.Media.Enabled = true
	runtime.cfg.Media.RTSPListenAddress = ":8554"
	runtime.cfg.HomeAssistant.PublicBaseURL = "https://bridge.example/proxy"
	runtime.cfg.HTTP.AuthToken = "bridge-token:@"
	runtime.probes.Update("nvr", func(probe *dahua.ProbeResult) {
		probe.States = map[string]dahua.DeviceState{"nvr_channel_05": {Info: map[string]any{
			"onvif_stream_url": "rtsp://nvr-user:nvr-pass@nvr.local/cam/realmonitor",
		}}, "nvr": {Info: map[string]any{
			"onvif_profiles": []map[string]any{{"stream_uri": "rtsp://nvr-user:nvr-pass@nvr.local/cam/realmonitor", "name": "main"}},
		}}}
	})
	before := runtime.ListHomeAssistantStreams(true)[0]
	if _, err := runtime.SetStreamLiveSource(context.Background(), before.ID, "camera"); err != nil {
		t.Fatal(err)
	}
	after := runtime.ListHomeAssistantStreams(true)[0]
	for name, first := range before.Profiles {
		second := after.Profiles[name]
		if first.StreamURL != second.StreamURL {
			t.Fatal("HA live URL changed with upstream preference")
		}
		target, _ := url.Parse(second.StreamURL)
		password, _ := target.User.Password()
		if target.Host != "bridge.example:8554" || target.Path != "/api/v1/rtsp/live/nvr_channel_05/"+name || password != runtime.cfg.HTTP.AuthToken {
			t.Fatalf("invalid proxy URL: %s", target.Redacted())
		}
		if second.RecorderStreamURL != "" || second.AlternativeStreamURL != "" {
			t.Fatal("HA received duplicate upstream URLs")
		}
	}
	catalog := ha.BuildNativeCatalog(runtime.probes.List(), runtime.ListHomeAssistantStreams(true))
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"nvr-pass", "camera-pass", "rtsp://nvr-user", "rtsp://camera-user", "recorder_stream_url", "alternative_stream_url"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("native catalog exposed %s", secret)
		}
	}
	public := runtime.ListHomeAssistantStreams(false)[0]
	for _, profile := range public.Profiles {
		target, _ := url.Parse(profile.StreamURL)
		if target.User != nil {
			t.Fatal("redacted catalog exposed relay credentials")
		}
	}
	private, profile, ok := runtime.GetStream(before.ID, "stable", true)
	if !ok || private.LiveSource.Source != "camera" || !strings.Contains(profile.StreamURL, "camera-user:camera-pass@camera.local") {
		t.Fatal("relay resolver lost private camera route")
	}
}

func TestPreferenceInvalidatesBothMediaAndRelay(t *testing.T) {
	runtime, _ := liveSourceRuntime(t)
	media := &invalidatingRuntimeMedia{}
	relay := &invalidatingRuntimeMedia{}
	runtime.AttachMedia(media)
	runtime.AttachLiveRelay(relay)
	if _, err := runtime.SetStreamLiveSource(context.Background(), "nvr_channel_05", "camera"); err != nil {
		t.Fatal(err)
	}
	if len(media.invalidated) != 1 || len(relay.invalidated) != 1 {
		t.Fatal("source switch did not invalidate both bridge transports")
	}
}
