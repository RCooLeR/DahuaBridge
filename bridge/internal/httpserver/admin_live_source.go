package httpserver

import (
	"fmt"
	"net/url"

	"RCooLeR/DahuaBridge/internal/streams"
)

func buildAdminLiveSourceControl(entry streams.Entry) string {
	if entry.LiveSource == nil {
		return ""
	}
	source := entry.LiveSource.OverrideSource
	defaultSelected, nvrSelected, cameraSelected := "", "", ""
	if source == "camera" {
		cameraSelected = " selected"
	} else if source == "nvr" {
		nvrSelected = " selected"
	} else {
		source = "default"
		defaultSelected = " selected"
	}
	markup := fmt.Sprintf(
		`<label class="chip">Live source <select aria-label="Live source for %s" data-live-source-url="%s" data-current-source="%s"><option value="default"%s>Default (%s)</option><option value="nvr"%s>NVR</option><option value="camera"%s>Camera</option></select></label>`,
		htmlEscape(firstNonEmpty(entry.Name, entry.ID)),
		htmlEscape("/api/v1/streams/"+url.PathEscape(entry.ID)+"/live-source"),
		htmlEscape(source), defaultSelected, htmlEscape(firstNonEmpty(entry.LiveSource.DefaultSource, "nvr")), nvrSelected, cameraSelected,
	)
	if reason := entry.LiveSource.CameraUnavailableReason; reason != "" {
		markup += `<span class="muted-note">` + htmlEscape(reason) + `</span>`
	}
	return markup
}
