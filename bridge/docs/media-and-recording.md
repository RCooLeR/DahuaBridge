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

### Audio Handling

- The bridge no longer toggles NVR or camera stream-audio settings for normal viewing.
- Source audio is detected per stream/profile at transcode start.
- If source audio is present, the bridge can include transcoded audio in outputs that support it.
- If source audio is absent, the bridge emits video-only output instead of forcing a broken audio track.
- NVR channel catalog entries still publish discovered source-audio state for higher layers.

## Stream-Backed Snapshots

The bridge supports generic stream-backed snapshots:

- `GET /api/v1/media/snapshot/{streamID}`

Behavior:

- if a compatible worker already exists, the bridge can reuse it
- if no worker is active, the bridge can start one, capture a frame, and stop it when idle
- identical snapshot requests are cached briefly and coalesced in-flight so bursty callers do not stampede the upstream NVR, VTO, or IPC device

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

For finite archive playback clips, the bridge now derives the playback duration from the archive RTSP window and lets FFmpeg exit naturally at end-of-file.

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

Native NVR archive items are playable through archive playback sessions. Bridge-owned MP4 clips can include direct download URLs. Native NVR archive items do not expose direct download URLs because the generic Dahua HTTP download path was not reliable on tested firmware.

Native NVR archive items now expose `export_url`. Calling that URL with `POST` makes the bridge create an archive playback session, capture the playback stream with FFmpeg, and save the result as a bridge-owned MP4 clip. Clients should poll the returned clip `self_url` until the clip is `completed`, then open its `download_url`.

The bridge also caches identical archive-search queries briefly and coalesces concurrent misses so repeated UI polling does not issue duplicate recorder searches.

For event-backed archive items such as SMD and IVS hits, the supported bridge workflow is the same:

1. search archive items
2. use the returned `export_url` or playback session flow
3. record the playback stream into a bridge-owned MP4 clip when a file export is needed

Direct recorder file transfer is not required for this workflow.

## Playback Sessions

For NVR archive playback, the bridge supports:

- session creation
- session lookup
- session seek
- playback stream helpers
- MP4 export by recording an archive playback stream

These are for archive playback, not for bridge clip capture.

Playback-specific worker behavior:

- playback HLS, MJPEG, and WebRTC workers enforce the requested archive window duration
- playback RTSP inputs use wallclock timestamps so finite archive windows terminate cleanly
- playback HLS output can remain addressable after FFmpeg exits, while the worker entry stays visible until idle-timeout cleanup runs
- live validation on May 2, 2026 confirmed near-end seek playback on a 24/7 archive window exited FFmpeg at EOF while the retained HLS playlist stayed fetchable

## Worker Visibility

The bridge exposes current media workers at:

- `GET /api/v1/media/workers`

Use this for diagnostics and operational visibility.

Important detail:

- a worker entry represents bridge runtime state, not only a live FFmpeg child process
- retained playback HLS workers can still appear in `/api/v1/media/workers` after FFmpeg has already exited, until idle-timeout cleanup runs

## Practical Guidance

- use bridge clip recording when you want ad-hoc MP4 capture owned by the bridge
- use NVR archive search when you want historical recorder footage
- use the generic stream snapshot endpoint when you want a frame from the actual stream path

## Related Docs

- [features.md](features.md)
- [api-reference.md](api-reference.md)
