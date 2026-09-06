package app

import (
	"net"
	"net/url"

	"RCooLeR/DahuaBridge/internal/streams"
)

func (r *runtimeServices) AttachLiveRelay(relay interface{ InvalidateStream(string) }) {
	r.mu.Lock()
	r.liveRelay = relay
	r.mu.Unlock()
}

// LiveMediaInput shares the relay's upstream between native HA and browser media.
// Public catalog URLs remain independent of this container-local connection.
func (r *runtimeServices) LiveMediaInput(id, profile string) string {
	r.mu.RLock()
	relay, ok := r.liveRelay.(interface{ LocalStreamURL(string, string) string })
	r.mu.RUnlock()
	if !ok {
		return ""
	}
	return relay.LocalStreamURL(id, profile)
}

// ListHomeAssistantStreams publishes a stable bridge URL, never the selected
// camera/NVR route. The media resolver continues to use the private catalog.
func (r *runtimeServices) ListHomeAssistantStreams(includeCredentials bool) []streams.Entry {
	entries := r.ListStreams(false)
	parsedBase, _ := url.Parse(r.cfg.HomeAssistant.PublicBaseURL)
	host := "localhost"
	if parsedBase != nil && parsedBase.Hostname() != "" {
		host = parsedBase.Hostname()
	}
	_, port, err := net.SplitHostPort(r.cfg.Media.RTSPListenAddress)
	if err != nil || port == "" {
		port = "8554"
	}
	for index := range entries {
		entry := &entries[index]
		entry.ONVIFH264Available = false
		entry.ONVIFStreamURL = ""
		entry.ONVIFSnapshotURL = ""
		entry.ONVIFProfileName = ""
		entry.ONVIFProfileToken = ""
		entry.RecommendedHAIntegration = "bridge_media"
		entry.RecommendedHAReason = "bridge_rtsp_relay"
		for name, profile := range entry.Profiles {
			profile.StreamURL = ""
			profile.RecorderStreamURL = ""
			profile.AlternativeStreamURL = ""
			if r.cfg.Media.Enabled {
				target := &url.URL{
					Scheme: "rtsp", Host: net.JoinHostPort(host, port),
					Path: "/api/v1/rtsp/live/" + entry.ID + "/" + name,
				}
				if includeCredentials && r.cfg.HTTP.AuthToken != "" {
					target.User = url.UserPassword("dahuabridge", r.cfg.HTTP.AuthToken)
				}
				profile.StreamURL = target.String()
			}
			entry.Profiles[name] = profile
		}
	}
	return entries
}
