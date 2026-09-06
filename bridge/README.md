# DahuaBridge Go Service

This is the bridge backend. It talks to Dahua NVR, IPC, and VTO devices, builds the normalized runtime model, exposes HTTP APIs, indexes archive metadata, and serves bridge-hosted media.

Start with [docs/README.md](docs/README.md).

The bridge owns live upstream selection. A persisted NVR/Camera default applies
to channels without an override; resetting an override to `default` restores
inheritance. Camera address, credentials, RTSP port, and any explicit input-channel
mapping come from the existing device configuration.

Home Assistant receives one stable bridge RTSP relay URL per live profile. The
bridge chooses the camera or recorder upstream and reconnects the relay when
needed. Upstream alternatives stay private to the bridge. Existing HLS, DASH,
MJPEG, and WebRTC outputs retain their URLs and continue to use the same routing
decision; recorded playback remains separate.

The health loop uses authenticated RTSP DESCRIBE checks with four concurrent
workers, checks every 15 seconds, and waits 60 seconds before retrying a failed
route. It validates all advertised live profiles before switching to an alternate.
Terminal media failures also trigger validation. If both routes fail, the bridge
keeps its current selection; it returns to the preferred source once that route
recovers. Effective source and fallback reason are reported in the catalog.
Recorded playback URLs and running clip recordings remain on their original input.

Common references:

- [Getting Started](docs/getting-started.md)
- [Configuration](docs/configuration.md)
- [Media And Recording](docs/media-and-recording.md)
- [API Reference](docs/api-reference.md)

Related docs:

- [Root architecture docs](../docs/README.md)
- [Home Assistant integration docs](../integration/docs/README.md)
