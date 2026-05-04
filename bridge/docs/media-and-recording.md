# Media And Recording

This file describes the current media and archive model.

## Profiles

The bridge publishes two user-facing profiles:

- `quality`: main stream, normally `subtype=0`
- `stable`: low-bandwidth stream, normally `subtype=1`

Legacy names such as `default` and `substream` are accepted and normalized, but new code should use `quality` and `stable`.

## Live Outputs

The bridge can expose stream-backed:

- snapshots
- MJPEG
- HLS
- WebRTC helper pages and answers
- HTML previews

These outputs are generated from the stream catalog. The bridge does not toggle camera or NVR audio settings for normal viewing.

## Bridge MP4 Clips

Bridge-owned MP4 clips are separate from the NVR archive. They are stored under `media.clip_path` and are controlled through:

- `POST /api/v1/media/streams/{streamID}/recordings`
- `GET /api/v1/media/recordings`
- `GET /api/v1/media/recordings/{clipID}`
- `GET /api/v1/media/recordings/{clipID}/play`
- `GET /api/v1/media/recordings/{clipID}/download`
- `POST /api/v1/media/recordings/{clipID}/stop`
- `DELETE /api/v1/media/recordings/{clipID}`

The archive database has a separate `bridge_mp4_clips` table for MP4 assets produced from SMD/IVS exports.

## Archive Tables

The archive service intentionally keeps SMD/IVS and normal recorder chunks separate.

### `smd_ivs_events`

Stores SMD/IVS detections:

- device ID
- channel
- event type
- start and end time
- generated main/sub RTSP playback URLs for export and diagnostics
- source DAV path when the NVR reports one
- MP4 backup clip ID, file path, status, and error

The card uses these rows for the SMD/IVS list. Play uses direct RTSP native Home Assistant playback. Export creates or reuses an MP4 clip.
HTTP list responses redact the raw RTSP playback URLs by default. Use `include_credentials=true` only for operator diagnostics.

### `nvr_recording_chunks`

Stores normal NVR recording chunks:

- device ID
- channel
- start and end time
- original DAV `file_path`
- NVR file metadata and flags

The card uses these rows for the recordings list. Chunks expose raw DAV download when `file_path` is available.

### `bridge_mp4_clips`

Stores archive MP4 export metadata owned by the bridge. SMD/IVS rows point at these clips when export backup exists.

The bridge drops and recreates the archive schema for this model; no migration is kept for the old mixed tables.

## Background Archive Jobs

There are two archive indexing flows:

- recording chunk sync fills `nvr_recording_chunks`
- SMD/IVS sync fills `smd_ivs_events` and queues MP4 backup export when enabled

API reads use SQLite only. SMD/IVS list requests do not call the NVR, and recording chunk list requests do not call the NVR. Event filters are applied in SQLite before `LIMIT`, then checked again in Go against the normalized Dahua event names.

## Native Historical Playback

The cards build Dahua RTSP playback URLs directly for native Home Assistant playback. No coverage lookup is required for seek.

Event playback uses the selected SMD/IVS row:

```text
rtsp://user:pass@host:554/cam/playback?channel=1&subtype=0&starttime=2026_05_03_14_11_22&endtime=2026_05_03_14_11_48
```

Seek playback uses the selected date and second-of-day:

```text
rtsp://user:pass@host:554/cam/playback?channel=9&subtype=0&starttime=2026_05_01_09_59_30
```

The required Dahua query order is:

1. `channel`
2. `subtype`
3. `starttime`
4. `endtime` when an end time is known

## Export Paths

SMD/IVS export:

1. read row from `smd_ivs_events`
2. build/play the NVR archive RTSP URL
3. record the finite window with FFmpeg
4. store MP4 metadata in `bridge_mp4_clips`
5. update the SMD/IVS row with MP4 backup fields

Recording chunk export:

1. read row from `nvr_recording_chunks`
2. use the original DAV path when available
3. transcode to MP4 through the bridge media layer

Raw DAV download is for recording chunks only.
