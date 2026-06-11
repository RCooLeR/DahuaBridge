# Bridge API Reference

This file documents the HTTP API implemented by the current bridge code. JSON endpoints return JSON unless noted otherwise. Timestamp query fields accept RFC3339 or `YYYY-MM-DD HH:MM:SS`; values without a timezone are interpreted in the bridge host timezone.

## Health

- `GET /healthz`: process liveness, plain text `ok`.
- `GET /readyz`: readiness once at least one configured device has been probed.
- `GET /api/v1/status`: JSON readiness and probe summary.
- `GET /metrics`: Prometheus metrics.
- `GET /admin`: built-in diagnostic UI.

## Authentication

Bridge-level HTTP auth is optional. When `http.auth_token` or the configured
`http.auth_token_env` environment variable is set, protected routes accept:

- `Authorization: Bearer <token>`
- `X-DahuaBridge-Token: <token>`
- `?auth_token=<token>` or `?token=<token>` when `http.auth_query_token` is true

Health, readiness, status, metrics, CORS preflight, and static admin assets stay
open so monitors and browser startup can still work. Query tokens are intended
for Home Assistant camera/media URLs where headers are not available; use them
only over HTTPS.

## Devices And Streams

- `GET /api/v1/devices`: current probe result for all configured devices.
- `GET /api/v1/devices/{deviceID}`: one probe result.
- `POST /api/v1/devices/probe-all`: reprobe all configured devices.
- `POST /api/v1/devices/{deviceID}/probe`: reprobe one device.
- `GET /api/v1/streams`: stream catalog used by the bridge and cards.
- `GET /api/v1/streams/{streamID}`: one stream catalog entry.
- `GET /api/v1/home-assistant/native/catalog`: Home Assistant integration catalog.

NVR channel catalog entries expose archive features:

- `archive_smd_ivs`: `/api/v1/nvr/{deviceID}/smd-ivs`
- `archive_recording_chunks`: `/api/v1/nvr/{deviceID}/recording-chunks`
- `archive_search`: compatibility alias for recording chunks
- `archive_playback`: `/api/v1/nvr/{deviceID}/playback/sessions`
- `archive_coverage`: chunk coverage from `nvr_recording_chunks`

Use `include_credentials=true` only for operator-only diagnostics. It can include RTSP URLs with embedded credentials.

## Live Media

- `GET /api/v1/media/snapshot/{streamID}`: JPEG snapshot from a bridge stream.
- `GET /api/v1/nvr/{deviceID}/channels/{channel}/snapshot`: NVR channel snapshot.
- `GET /api/v1/vto/{deviceID}/snapshot`: VTO snapshot.
- `GET /api/v1/ipc/{deviceID}/snapshot`: standalone IPC snapshot.
- `GET /api/v1/media/preview/{streamID}`: HTML preview.
- `GET /api/v1/media/mjpeg/{streamID}`: MJPEG stream.
- `GET /api/v1/media/hls/{streamID}/{profile}/index.m3u8`: HLS playlist.
- `GET /api/v1/media/webrtc/{streamID}/{profile}`: WebRTC helper page.
- `POST /api/v1/media/webrtc/{streamID}/{profile}/offer`: WebRTC SDP answer.
- `GET /api/v1/media/workers`: current media worker state.

The only published profile names are `quality` and `stable`. Legacy aliases are accepted and normalized.
Snapshot endpoints return the bundled DahuaBridge logo as a `200 image/png` placeholder when the device or stream snapshot path cannot produce an image.

## Bridge MP4 Clips

- `POST /api/v1/media/streams/{streamID}/recordings`: start a bridge-owned MP4 clip.
- `GET /api/v1/media/recordings`: list bridge-owned MP4 clips.
- `GET /api/v1/media/recordings/{clipID}`: clip metadata.
- `GET /api/v1/media/recordings/{clipID}/play`: stream the MP4.
- `GET /api/v1/media/recordings/{clipID}/download`: download the MP4.
- `POST /api/v1/media/recordings/{clipID}/stop`: stop an active clip.
- `DELETE /api/v1/media/recordings/{clipID}`: delete a completed/stopped clip.

Archive exports also create bridge MP4 clips. SMD/IVS export state is stored back on `smd_ivs_events`.
MP4 clips keep audio when the selected source stream advertises an audio track; the bridge no longer toggles device-side audio settings to force that.

## NVR Archive APIs

### `GET /api/v1/nvr/{deviceID}/smd-ivs`

Returns indexed SMD/IVS event rows. These are served from SQLite; the request does not query the NVR live.

Query fields:

- `channel`: required positive channel number.
- `start` or `start_time`: required start time.
- `end` or `end_time`: required end time.
- `limit`: optional, default from the card/domain model.
- `event`: optional event filter. Supported normalized values are `all`, `human`, `vehicle`, `animal`, `tripwire`, and `intrusion`. `transport` is accepted as an alias for Dahua vehicle SMD events. Dahua names such as `smdTypeHuman`, `smdTypeVehicle`, `CrossLineDetection`, and `CrossRegionDetection` are accepted. SQLite filtering also matches stored `event_type` values such as `Event.smdTypeHuman`.
- `db_only`, `skip_assets`, or `skip_asset_enrichment`: compatibility flags that skip MP4 asset enrichment work for list responses.
- `include_assets` or `with_assets`: enables asset enrichment for event list responses.
- `include_credentials` or `with_credentials`: includes raw RTSP playback URLs. Leave this off for normal clients.

Returned rows have:

- `record_kind: "smd_ivs"`
- `source: "smd_ivs"`
- `export_url` for MP4 export
- `rtsp_main_url` and `rtsp_sub_url` only when `include_credentials=true`
- no raw DAV `download_url`

`GET /api/v1/nvr/{deviceID}/smd_ivs` is an underscore alias.

### `GET /api/v1/nvr/{deviceID}/recording-chunks`

Returns NVR recording chunk rows from SQLite only. These rows represent normal DAV archive chunks, not SMD/IVS events.

Query fields:

- `channel`: required positive channel number.
- `start` or `start_time`: required start time.
- `end` or `end_time`: required end time.
- `limit`: optional.

Returned rows have:

- `record_kind: "recording_chunk"`
- `source: "nvr"`
- `download_url` for direct original DAV download when `file_path` is present
- `export_url` for bridge MP4 export

### `GET /api/v1/nvr/{deviceID}/recordings`

Compatibility endpoint. New clients should use `/smd-ivs` for SMD/IVS and `/recording-chunks` for normal chunks. The endpoint also redacts raw RTSP playback URLs unless `include_credentials=true` is supplied.

### `POST /api/v1/nvr/{deviceID}/recordings/export`

Creates a bridge MP4 clip from an NVR archive window.

For SMD/IVS rows, the bridge records the Dahua RTSP playback stream. For recording chunks with a `file_path`, the bridge can download the original DAV first and transcode from that file.

### `GET /api/v1/nvr/{deviceID}/recordings/download`

Downloads the original DAV file by `file_path`. This is intended for recording chunks only.

### `GET /api/v1/nvr/{deviceID}/recordings/coverage`

Returns coverage generated from `nvr_recording_chunks`. The current cards do not use this endpoint for seek playback; native seek builds a direct RTSP playback URL.

## Archive Query Checks

For event rows, compare the unfiltered and filtered views first:

```text
GET /api/v1/nvr/{deviceID}/events/summary?channel=1&start=...&end=...&event=all
GET /api/v1/nvr/{deviceID}/smd-ivs?channel=1&start=...&end=...&event=all
GET /api/v1/nvr/{deviceID}/smd-ivs?channel=1&start=...&end=...&event=smdTypeHuman
```

If `event=all` has rows and `event=smdTypeHuman` has none, the indexed rows in that window are not human detections. Check the summary codes before debugging the NVR query path.

## NVR Playback Sessions

- `POST /api/v1/nvr/{deviceID}/playback/sessions`: create a finite archive playback session.
- `GET /api/v1/nvr/playback/sessions/{sessionID}`: session metadata.
- `POST /api/v1/nvr/playback/sessions/{sessionID}/seek`: recreate/seek a session.

Native Home Assistant playback in the cards does not require a bridge session. The card builds Dahua RTSP playback URLs directly:

```text
rtsp://user:pass@host:554/cam/playback?channel=1&subtype=0&starttime=2026_05_03_14_11_22&endtime=2026_05_03_14_11_48
```

The query order is always `channel`, `subtype`, `starttime`, then optional `endtime`.

## NVR Controls

The bridge exposes channel controls under `/api/v1/nvr/{deviceID}/channels/{channel}` for snapshot, PTZ, aux/light/siren, and recorder-mode operations. Browser audio is controlled by the player; the bridge does not expose device-side mute or volume controls. The exact supported controls depend on probe results and are reflected in the stream catalog `controls` and `features` fields.
