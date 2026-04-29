# DahuaBridge

`DahuaBridge` is the Go backend for this repo. It talks to Dahua devices, keeps the normalized runtime model, exposes the HTTP/admin surface, serves bridge-hosted media, and provides the native Home Assistant catalog consumed by the custom integration.

<p align="center" style="text-align: center;">
  <img src="./assets/overview.png" alt="DahuaBridge diagram" width="70%">
</p>

## Supported Home Assistant Path

The supported Home Assistant path is:

1. Run the Go bridge.
2. Install `integration/custom_components/dahuabridge`.
3. Add the `DahuaBridge` integration in Home Assistant.
4. Let the integration poll `GET /api/v1/home-assistant/native/catalog`.

The bridge no longer exposes legacy Home Assistant helper/package endpoints from its public HTTP surface.

## What The Bridge Does

- probes Dahua `NVR`, `VTO`, and standalone `IPC` devices
- collects inventory such as firmware, channels, locks, alarms, disks, and encode metadata
- attaches to supported Dahua event streams and normalizes device state
- exposes status, devices, events, streams, snapshots, and admin APIs over HTTP
- serves bridge-hosted `MJPEG`, `HLS`, and playback WebRTC
- exposes a browser intercom page and bridge-side `VTO` control APIs
- persists last known state to disk when `state_store.enabled=true`

## Current Limits

- full end-to-end `VTO` talkback is not implemented
- browser microphone uplink can reach the bridge and be exported as RTP, but that is still not direct `VTO` talkback
- the Home Assistant custom integration is polling-based today
- wider real-device validation is still needed across firmware variants

## Requirements

- network access from the bridge host to your Dahua devices
- `ffmpeg` available on the host if media is enabled
- Home Assistant is optional, but the repo is built around it

## Quick Start

If you want the shortest path, use [../docs/install.md](../docs/install.md).

1. Copy `config.example.yaml` to `config.yaml`.
2. Set device `base_url`, `username`, and `password`.
3. Set `home_assistant.public_base_url` to the URL Home Assistant and browsers can actually reach.
4. Keep the default native integration settings:

```yaml
mqtt:
  enabled: false

home_assistant:
  entity_mode: native
```

5. Start the bridge:

```bash
go run ./cmd/dahuabridge --config config.yaml
```

6. Verify:

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/status`
- `GET /api/v1/devices`
- `GET /api/v1/home-assistant/native/catalog`
- `GET /admin`

## Primary HTTP Surfaces

- `GET /api/v1/status`
- `GET /api/v1/devices`
- `GET /api/v1/events`
- `GET /api/v1/streams`
- `GET /api/v1/home-assistant/native/catalog`
- snapshot endpoints under `/api/v1/nvr/...`, `/api/v1/ipc/...`, and `/api/v1/vto/...`
- media endpoints under `/api/v1/media/...`

## Configuration Notes

### `mqtt.enabled`

Defaults to `false`. Leave it disabled for the supported bridge + native integration setup.

### `home_assistant.public_base_url`

This must be the bridge URL that Home Assistant and browsers can actually reach. It is used in generated stream URLs and media pages.

### `home_assistant.camera_snapshot_source`

Controls the snapshot source used when the bridge prepares camera metadata during probe refresh:

- `device`: fetch a real snapshot from the device
- `logo`: use a built-in placeholder image instead

### `media.webrtc_ice_servers`

Use this when bridge-hosted WebRTC pages must work across NAT, remote networks, or the public Internet.

### `media.webrtc_uplink_targets`

Optional UDP targets for exporting incoming browser microphone audio from WebRTC intercom sessions.

## More Docs

- install guide: [../docs/install.md](../docs/install.md)
- system overview: [../docs/how-it-works.md](../docs/how-it-works.md)
- device and naming model: [../docs/device-model.md](../docs/device-model.md)
- control surface: [../docs/controls.md](../docs/controls.md)
- project status: [../docs/project-status.md](../docs/project-status.md)
