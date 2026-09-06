package streams

import "errors"

const (
	LivePreconnectOff    = "off"
	LivePreconnectRecent = "recent"
	LivePreconnectAlways = "always"
)

var ErrInvalidLivePreconnect = errors.New("live preconnect requires mode off, recent or always and profile auto, quality or stable")

// LivePreconnectSettings controls only shared live RTSP inputs. It does not
// start a player, an encoder, or a recording.
type LivePreconnectSettings struct {
	Mode    string `json:"mode"`
	Profile string `json:"profile"`
}

type LivePreconnectTarget struct {
	StreamID string
	Profile  string
}

func (s LivePreconnectSettings) Valid() bool {
	return (s.Mode == LivePreconnectOff || s.Mode == LivePreconnectRecent || s.Mode == LivePreconnectAlways) &&
		(s.Profile == "auto" || s.Profile == "quality" || s.Profile == "stable")
}

// Defaulted is for saved state from older versions, never for validating API
// writes. Unknown saved values keep the existing on-demand behavior.
func (s LivePreconnectSettings) Defaulted() LivePreconnectSettings {
	if !s.Valid() {
		return LivePreconnectSettings{Mode: LivePreconnectOff, Profile: "auto"}
	}
	return s
}
