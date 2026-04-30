# Media And Recording

This page focuses on the bridge media subsystem.

## Media Profiles

The bridge exposes stream helpers around named profiles such as:

- `quality`
- `default`
- `stable`
- `substream`

The exact profile set depends on the stream and discovered metadata.

## Live Media Outputs

The bridge can provide:

- MJPEG
- HLS
- preview pages
- WebRTC helper pages and answers

These are derived from the stream catalog and generated URLs.

## Stream-Backed Snapshots

The bridge supports generic stream-backed snapshots:

- `GET /api/v1/media/snapshot/{streamID}`

Behavior:

- if a compatible worker already exists, the bridge can reuse it
- if no worker is active, the bridge can start one, capture a frame, and stop it when idle

Device-specific snapshot endpoints now use the stream path first where possible:

- NVR channel snapshot
- VTO snapshot
- IPC snapshot

## Bridge-Owned Clip Recording

The bridge can record clips directly from streams into its own storage.

This is designed for:

- NVR channels
- IPC cameras
- VTO streams
- other stream-backed sources known to the bridge

It is intentionally separate from device-side NVR manual recording mode control.

### Clip Lifecycle

1. client starts a clip for a `stream_id`
2. the bridge resolves the stream and profile
3. ffmpeg records an MP4 into the configured clip path
4. the bridge persists clip metadata
5. clients can query, stop, and download the clip

### Clip APIs

- `POST /api/v1/media/streams/{streamID}/recordings`
- `GET /api/v1/media/recordings`
- `GET /api/v1/media/recordings/{clipID}`
- `POST /api/v1/media/recordings/{clipID}/stop`
- `GET /api/v1/media/recordings/{clipID}/download`

### Storage

The output path is controlled by:

- `media.clip_path`

Clips are stored on the bridge side, typically under the mounted `/data` volume in container deployments.

### Relationship To NVR Recordings Search

Bridge-owned clips are merged into:

- `GET /api/v1/nvr/{deviceID}/recordings`

That final result can include:

- native NVR archive items
- bridge MP4 clip items

## Playback Sessions

For NVR archive playback, the bridge supports:

- session creation
- session lookup
- session seek
- playback stream helpers

These are for archive playback, not for bridge clip capture.

## Worker Visibility

The bridge exposes current media workers at:

- `GET /api/v1/media/workers`

Use this for diagnostics and operational visibility.

## Practical Guidance

- use bridge clip recording when you want ad-hoc MP4 capture owned by the bridge
- use NVR archive search when you want historical recorder footage
- use the generic stream snapshot endpoint when you want a frame from the actual stream path

## Related Docs

- [features.md](features.md)
- [api-reference.md](api-reference.md)
