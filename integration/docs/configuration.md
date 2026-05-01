# Configuration

The integration is configured through the Home Assistant UI.

## Config Entry Input

The main required value is:

- bridge URL

This should point to the running bridge, for example:

```text
http://bridge-host:9205
```

The setup flow validates this URL by calling the bridge status endpoint before it creates the config entry.

If Home Assistant reaches the bridge through a reverse proxy path such as `https://ha.example.com/dahuabridge`, use that full public URL. The integration preserves that base path when it rewrites bridge-hosted HTTP links from the catalog.

## Options

The integration currently exposes runtime options for:

- poll interval
- preferred video profile
- preferred video source

Current defaults:

- poll interval: `15` seconds
- preferred video profile: `quality`
- preferred video source: `hls`
- allowed poll interval range: `5` to `300` seconds

## Polling Model

The integration is polling-based.

It periodically refreshes the bridge-native catalog and updates entities from that data.

This means:

- device behavior is ultimately owned by the bridge
- entity availability follows the most recent successful catalog refresh
- repeated bridge snapshot and archive-search requests are still reduced on the bridge side by short caching and in-flight request coalescing

## Preferred Video Profile

This controls which bridge-provided profile the camera entity prefers when choosing stream-related URLs.

Examples:

- `auto`
- `quality`
- `stable`
- `substream`

Behavior:

- `auto` follows the bridge `recommended_profile`
- `quality` prioritizes the bridge main-stream style profiles
- `stable` and `substream` prefer lower-bandwidth bridge-generated variants when available

## Preferred Video Source

This controls which generated stream URL type the integration prefers.

Typical choices:

- auto
- HLS
- MJPEG
- RTSP where relevant

Current option values:

- `auto`
- `hls`
- `mjpeg`
- `rtsp`

Behavior:

- `hls` is the default and prefers bridge HLS, then falls back to RTSP or MJPEG as needed
- `mjpeg` prefers bridge MJPEG
- `rtsp` prefers direct RTSP URLs from the bridge catalog
- `auto` currently uses the same source ordering as the default path

Current source order is:

1. bridge HLS
2. direct RTSP
3. bridge MJPEG

RTSP selections keep the direct `rtsp://` URL from the catalog. They are not rewritten into bridge HTTP URLs.

If Home Assistant cannot play the chosen source well in your environment, change this option before changing bridge-side stream modeling.

## Diagnostics

The integration supports diagnostics export for support and troubleshooting.

Diagnostics redact sensitive bridge and stream URLs where appropriate.

## Next Step

- [features.md](features.md)
- [entities-and-controls.md](entities-and-controls.md)
