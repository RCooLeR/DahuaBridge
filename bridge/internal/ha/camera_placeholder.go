package ha

import _ "embed"

//go:embed assets/logo.png
var cameraSnapshotPlaceholder []byte

func CameraSnapshotPlaceholder() []byte {
	return append([]byte(nil), cameraSnapshotPlaceholder...)
}

func (p *DiscoveryPublisher) LogoCameraSnapshots() bool {
	if p == nil {
		return false
	}
	return p.cfg.HomeAssistant.LogoCameraSnapshots()
}
