# Device And Stream Model

The bridge publishes a normalized model of devices and streams.

This model drives:

- `/api/v1/devices`
- `/api/v1/streams`
- `/api/v1/home-assistant/native/catalog`

## Device Kinds

Current device kinds include:

- `nvr`
- `nvr_channel`
- `nvr_disk`
- `ipc`
- `vto`
- `vto_lock`
- `vto_alarm`

## Root Device IDs

Root IDs come from bridge configuration.

Examples:

- `west20_nvr`
- `front_vto`
- `yard_ipc`

These IDs should be stable because they flow into URLs, catalog records, and Home Assistant unique IDs.

## Important Modeling Rules

### NVR Root

Represents the recorder itself.

### NVR Channel

Represents the actual camera-like child streamable unit.

This is the most important Home Assistant-facing camera model for NVR-connected cameras.

### IPC

Represents a single standalone camera.

### VTO

Represents the door station root plus related call/intercom semantics.

## Stream Catalog Entries

Each stream entry can include:

- identity fields
- device linkage
- stream profiles
- preview/snapshot URLs
- control summaries
- feature summaries
- intercom summary for VTO
- capture summary for bridge-owned snapshot/recording helpers

### Live Routing

NVR channel entries retain their IDs, parent NVR, channel number, controls, and
archive URLs when their live source changes. The Home Assistant catalog exposes
one stable bridge RTSP relay URL per live profile. The bridge chooses the upstream
and performs failover; upstream alternatives are private runtime fields.

The `live_source` summary reports:

- `source`: the effective source, `nvr` or `camera`
- `preferred_source`: the resolved preference
- `default_source`: the bridge default preference
- `override_source`: the camera override, or an empty string for inheritance
- `fallback_reason`: why an alternate is active, when applicable
- `camera_available` and `camera_unavailable_reason`: direct-camera configuration availability

Recorded playback remains attached to the NVR and uses `recorder_stream_url`,
independently of the live relay source. Selecting HLS, DASH, MJPEG, WebRTC, or
RTSP remains a separate output-format choice.

## Why This Matters

The integration relies on this normalized model so it does not need to understand raw Dahua protocol details.

For the Home Assistant-facing interpretation of this model, see:

- [../../integration/docs/entities-and-controls.md](../../integration/docs/entities-and-controls.md)
