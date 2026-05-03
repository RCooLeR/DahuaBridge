# 🔌 API Reference

This page documents the bridge HTTP surface by function.

Unless noted otherwise, endpoints return JSON and are safe to call from browser-based clients because the server adds permissive CORS headers.

## 🧭 Conventions

- Timestamp inputs accept:
  - RFC3339, for example `2026-04-28T00:00:00Z`
  - `YYYY-MM-DD HH:MM:SS`
  - `YYYY-MM-DDTHH:MM:SS`
- Timestamp inputs without an explicit timezone are parsed in the bridge host's local timezone.
- `include_credentials=true` on catalog and stream endpoints can expose RTSP URLs with embedded usernames and passwords. Treat that as an operator-only mode.
- Action endpoints usually return a small `"status":"ok"` payload plus echoed identifiers.
- HTML helper pages are meant for human/browser use. JSON control endpoints are the automation surface.

## 🩺 Health And Operations

### `GET /healthz`

- Liveness probe for the process itself.
- Returns plain text `ok`.
- Does not verify that devices were probed successfully.

### `GET /readyz`

- Readiness probe for deployments and reverse proxies.
- Returns plain text `ready` when the bridge has at least one probed device.
- Returns `503` with the same JSON shape as `/api/v1/status` when the bridge is alive but not ready.

### `GET /api/v1/status`

- Returns compact bridge readiness state.
- Main fields:
  - `ready`
  - `device_count`
  - `last_updated_at`
- Use this for automation and integration connectivity checks.

### `GET /metrics`

- Prometheus-style metrics endpoint.
- Intended for monitoring systems, not the Home Assistant integration.

### `GET /admin`

- Built-in HTML admin page.
- Shows probe results, stream catalog, runtime settings, event stats, and media worker state.
- Good first-stop diagnostic surface before looking at Home Assistant.

### `GET /admin/test-bridge`

- Built-in NVR channel test bench.
- Provides channel/profile selection, snapshot/MJPEG/HLS/preview/WebRTC stream switching, normal bridge controls, and diagnostic control buttons.
- Uses bridge-side API calls so device credentials never need to be exposed to the browser.

## 🗂️ Device Inventory

### `GET /api/v1/devices`

- Returns all current probe results.
- Each item contains:
  - `root`
  - `children`
  - `states`
  - `raw`

### `GET /api/v1/devices/{deviceID}`

- Returns one current probe result by configured device ID.
- `404` if the device ID is unknown to the bridge.

### `POST /api/v1/devices/probe-all`

- Re-runs probing for every configured device.
- Intended for operators, not high-frequency polling.
- Returns per-device results plus:
  - `device_count`
  - `success_count`
  - `error_count`
  - `results`
- Returns:
  - `200` when every probe succeeded
  - `207` when at least one device failed and at least one succeeded

### `POST /api/v1/devices/{deviceID}/probe`

- Re-runs probing for one device.
- No request body.
- Returns the fresh probe result in `result`.

### `POST /api/v1/devices/{deviceID}/credentials`

- Updates bridge-side connection settings for one device and immediately re-probes it.
- Request body is partial JSON. Supported fields:

```json
{
  "base_url": "https://camera.local",
  "username": "admin",
  "password": "secret",
  "onvif_enabled": true,
  "onvif_username": "admin",
  "onvif_password": "secret",
  "onvif_service_url": "http://camera.local/onvif/device_service",
  "insecure_skip_tls": true
}
```

- Only the provided fields are updated.
- Returns the refreshed probe result in `result`.

## 📡 Stream And Catalog Surfaces

### `GET /api/v1/streams`

- Returns the bridge stream catalog.
- This is the best raw view of stream-oriented bridge capabilities.
- Each item describes one streamable unit such as:
  - an NVR channel
  - an IPC camera
  - a VTO root stream
- Optional query params:
  - `device_id`: keep only entries whose root device, source device, or stream ID matches the supplied value
  - `include_credentials=true`: include RTSP URLs with credentials
- Main response fields per item:
  - identity: `id`, `root_device_id`, `source_device_id`, `device_kind`, `name`, `channel`
  - access URLs: `snapshot_url`, `local_preview_url`, `local_intercom_url`
  - media selection: `recommended_profile`, `profiles`
  - optional control summary: `controls`
  - optional feature summary: `features`
  - optional bridge capture summary: `capture`
  - optional VTO intercom summary: `intercom`
- Typical `profiles` entries include:
  - `stream_url`
  - `local_mjpeg_url`
  - `local_hls_url`
  - `local_webrtc_url`
- Root NVR records, disks, locks, and alarms are not returned here unless they are streamable units.

### `GET /api/v1/streams/{streamID}`

- Returns one stream catalog entry by stream ID.
- Optional query param:
  - `include_credentials=true`
- `404` if the stream is unknown.
- Response shape is the same as one item from `GET /api/v1/streams`.

### `GET /api/v1/home-assistant/native/catalog`

- Returns the Home Assistant-oriented native catalog consumed by the custom integration.
- Optional query param:
  - `include_credentials=true`
- Use this when you want the same device, stream, control, and capture model that Home Assistant will see.
- Top-level response fields:
  - `generated_at`
  - `devices`
- Each `devices` item contains:
  - `device`: the normalized device object
  - `state`: latest availability plus `info`
  - `stream`: optional stream metadata when that device is streamable
- The `stream` object uses the same shape as `GET /api/v1/streams/{streamID}`.
- Non-streamable records such as NVR roots, disks, VTO locks, and VTO alarms still appear in the catalog, but usually without a `stream` object.
- VTO intercom runtime fields are exposed in two places:
  - `stream.intercom` for direct control and UI use
  - `state.info` as flattened values such as `call_state`, `bridge_session_active`, and `external_uplink_enabled`

Example catalog skeleton:

```json
{
  "generated_at": "2026-04-29T19:30:00Z",
  "devices": [
    {
      "device": {
        "id": "west20_nvr_channel_01",
        "parent_id": "west20_nvr",
        "kind": "nvr_channel",
        "name": "West 20 Channel 01"
      },
      "state": {
        "available": true,
        "info": {
          "motion": false
        }
      },
      "stream": {
        "id": "west20_nvr_channel_01",
        "recommended_profile": "quality",
        "profiles": {
          "quality": {
            "local_hls_url": "/api/v1/media/hls/west20_nvr_channel_01/quality/index.m3u8"
          }
        }
      }
    }
  ]
}
```

## 🎥 Generic Media

### `GET /api/v1/media/workers`

- Returns current media worker state.
- Use this for diagnostics when MJPEG, HLS, WebRTC, or clip recording behavior looks wrong.
- A listed worker does not always mean an FFmpeg child process is still running.
- For finite playback HLS sessions, retained playlists and segments can keep the worker visible until idle-timeout cleanup even after FFmpeg exited at end-of-file.

### `GET /api/v1/media/snapshot/{streamID}`

- Captures a still frame from a bridge-known stream.
- Optional query params:
  - `profile`: profile name such as `quality`, `stable`, or `substream`
  - `width`: positive scaled output width, or `0` to keep source width
- Returns image bytes, usually `image/jpeg`.
- This is the preferred snapshot endpoint when you want the actual stream path rather than device snapshot CGI behavior.

### `GET /api/v1/media/preview/{streamID}`

- Returns an HTML preview page for a stream.
- Optional query param:
  - `profile`
- If `profile` is omitted, the bridge uses the stream's recommended profile and then falls back to `stable`.

### `GET /api/v1/media/mjpeg/{streamID}`

- Returns a multipart MJPEG stream for browser or lightweight client use.
- Optional query params:
  - `profile`, default `stable`
  - `width`, non-negative integer scaling target
- Returns `multipart/x-mixed-replace`.

### `GET /api/v1/media/hls/{streamID}/{profile}/index.m3u8`

- Returns the HLS playlist for one live or playback stream/profile pair.
- Returns `application/vnd.apple.mpegurl`.

### `GET /api/v1/media/hls/{streamID}/{profile}/{segmentName}`

- Returns one HLS segment referenced by the playlist.
- Intended to be consumed by the player, not called directly by operators.

### `GET /api/v1/media/webrtc/{streamID}/{profile}`

- Returns an HTML WebRTC helper page for one stream/profile pair.
- Intended for direct browser viewing and troubleshooting.

### `POST /api/v1/media/webrtc/{streamID}/{profile}/offer`

- Completes WebRTC SDP offer/answer negotiation for a stream/profile pair.
- Request body must be a normal WebRTC session description:

```json
{
  "type": "offer",
  "sdp": "v=0\r\n..."
}
```

- Returns a WebRTC SDP answer.
- Use this when building your own player instead of the bridge-hosted HTML helper page.

## ⏺️ Bridge Clip Recording

### `POST /api/v1/media/streams/{streamID}/recordings`

- Starts a bridge-owned MP4 clip capture for one stream.
- Request body is optional. Supported fields:

```json
{
  "profile": "stable",
  "duration_seconds": 15
}
```

- You can also supply `duration_ms` instead of `duration_seconds`.
- If no duration is supplied, the clip keeps recording until stopped.
- Returns clip metadata including:
  - `id`
  - `status`
  - `download_url`
  - `self_url`
  - `stop_url` while recording
- `409 clip_already_active` if that stream already has a live clip.

### `GET /api/v1/media/recordings`

- Lists bridge-owned clips.
- Optional filters:
  - `stream_id`
  - `root_device_id`
  - `channel`
  - `start`
  - `end`
  - `limit`
- `limit` is capped at `200`.
- Returns:
  - `items`
  - `returned_count`

### `GET /api/v1/media/recordings/{clipID}`

- Returns one bridge-owned clip by ID.
- Use this to refresh clip status after starting or stopping a recording.

### `POST /api/v1/media/recordings/{clipID}/stop`

- Stops an active clip.
- No request body.
- Returns the final clip metadata.
- If the clip is already finished, the bridge returns its current metadata.

### `GET /api/v1/media/recordings/{clipID}/download`

- Downloads the resulting MP4 file.
- Returns `Content-Disposition: attachment`.

## 🗃️ NVR Surfaces

### `POST /api/v1/nvr/{deviceID}/inventory/refresh`

- Forces the bridge to refresh NVR-specific inventory such as channels and disks.
- Use this after channel changes or storage changes on the recorder.
- Returns the refreshed probe result in `result`.

### `GET /api/v1/nvr/{deviceID}/recordings`

- Searches recorder-side archive footage for one NVR channel and time range.
- Required query params:
  - `channel`
  - `start`
  - `end`
- Optional query param:
  - `limit`, default `25`, max `200`
  - `event` or `event_type`, optional Dahua event filter. Friendly values such as `motion`, `human`, `vehicle`, `tripwire`, `intrusion`, and `access` are normalized to recorder event codes.
  - When an event filter is supplied, the bridge resolves matching archive windows from recorder event metadata, including recorder log RPC, SMD finder RPC, and IVS media-file RPC as needed.
- Example:

```text
/api/v1/nvr/west20_nvr/recordings?channel=1&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=25&event=motion
```

- Returns a normalized search result with `items`.
- Every item now carries a stable bridge `id` plus `record_kind` (`file` or `event`).
- Native non-event NVR archive items can also include `download_url` when the recorder returned a usable `file_path`.
- The bridge caches identical archive-search queries briefly and coalesces concurrent misses so repeated UI polling does not duplicate recorder searches.
- Event-backed items such as SMD and IVS results are intended for bridge playback/export workflows and do not expose raw direct-download URLs.
- Native NVR archive items expose `export_url` as the supported bridge MP4 export path.
- Archive items can also include persisted asset metadata:
  - `asset_status`: `indexed`, `transcoding`, `ready`, `failed`, or `missing`
  - `asset_clip_id`: bridge clip ID when an export/transcode job already exists
  - `asset_self_url`: bridge clip status endpoint when a clip exists
  - `asset_playback_url` and `asset_download_url`: present only when the asset is `ready`
  - `asset_stop_url`: present when the asset is still `transcoding`

### `POST /api/v1/nvr/{deviceID}/recordings/export`

- Exports a native NVR archive window as a bridge-owned MP4 clip.
- When `file_path` is supplied, the bridge downloads the recorder `.dav` file first and transcodes from that file.
- When `file_path` is absent, the bridge creates an archive playback session and records that playback stream into a bridge-owned MP4 clip.
- Accepts query params or JSON body fields:

```json
{
  "channel": 5,
  "start_time": "2026-04-30 10:04:12",
  "end_time": "2026-04-30 10:30:03",
  "seek_time": "2026-04-30 10:04:12",
  "profile": "stable",
  "duration_seconds": 4
}
```

- `seek_time`, `profile`, `duration_seconds`, and `duration_ms` are optional.
- If no duration is supplied, the bridge records from `seek_time` or `start_time` until `end_time`.
- For finite archive exports, the bridge derives the playback duration from the archive RTSP window and lets FFmpeg terminate at end-of-file.
- Live validation on May 2, 2026 confirmed a 22-second SMD export on channel 1 completed cleanly as a video-only MP4 on the tested recorder.
- The same export flow is the supported path for SMD and IVS event-backed archive items.
- Returns:
  - `session`: playback session metadata when the playback-session path was used
  - `clip`: bridge MP4 clip metadata with `self_url` and `download_url`
- Poll `clip.self_url` until `clip.status` is `completed`, then download from `clip.download_url`.

### `GET /api/v1/nvr/{deviceID}/recordings/download`

- Downloads a native recorder file through the bridge.
- Required query param:
  - `file_path`
- Intended for native non-event archive items that resolved to a recorder `file_path`.
- Returns the raw recorder payload, typically a `.dav` file.

### `POST /api/v1/nvr/{deviceID}/playback/sessions`

- Creates an archive playback session for one NVR channel and time range.
- Request body:

```json
{
  "channel": 2,
  "start_time": "2026-04-28T00:00:00Z",
  "end_time": "2026-04-28T01:00:00Z",
  "seek_time": "2026-04-28T00:20:00Z"
}
```

- `seek_time` is optional. If present, playback starts near that point.
- Returns session metadata plus generated HLS, MJPEG, and WebRTC URLs under `profiles`.
- Live validation on May 2, 2026 confirmed a seek to `2026-04-28T02:59:50+03:00` inside a `2026-04-28T02:30:00+03:00` to `2026-04-28T03:00:00+03:00` playback window exited FFmpeg at EOF while the retained HLS playlist remained fetchable.

### `GET /api/v1/nvr/playback/sessions/{sessionID}`

- Returns current metadata for one previously created playback session.
- Use this if a client persists a session ID and later needs to re-resolve media URLs.

### `POST /api/v1/nvr/playback/sessions/{sessionID}/seek`

- Seeks an existing archive playback session to a new point in time.
- Request body:

```json
{
  "seek_time": "2026-04-28T00:45:00Z"
}
```

- Returns updated session metadata.
- Clients should replace any cached playback URLs with the returned session payload.

### `GET /api/v1/nvr/{deviceID}/channels/{channel}/controls`

- Returns discovered control capabilities for one NVR channel.
- Main capability groups:
  - `ptz`
  - `aux`
  - `audio`
  - `recording`
- `audio` is capability metadata only for NVR channels. The bridge no longer exposes a recorder mute toggle there.
- Use this before attempting PTZ or aux actions from a custom UI.

### `POST /api/v1/nvr/{deviceID}/channels/{channel}/ptz`

- Sends a PTZ command to one NVR channel.
- Request body:

```json
{
  "action": "pulse",
  "command": "left",
  "speed": 3,
  "duration_ms": 250
}
```

- Supported `action` values:
  - `start`
  - `stop`
  - `pulse`
- Supported `command` values:
  - `up`
  - `down`
  - `left`
  - `right`
  - `left_up`
  - `right_up`
  - `left_down`
  - `right_down`
  - `zoom_in`
  - `zoom_out`
  - `focus_near`
  - `focus_far`
- `duration_ms` matters for `pulse`.

### `POST /api/v1/nvr/{deviceID}/channels/{channel}/aux`

- Controls aux-style outputs for one NVR channel.
- Request body:

```json
{
  "action": "pulse",
  "output": "warning_light",
  "duration_ms": 400
}
```

- Supported `action` values:
  - `start`
  - `stop`
  - `pulse`
- Supported `output` aliases:
  - `aux` or `siren`
  - `light` or `warning_light`
  - `wiper`
- The bridge normalizes aliases to device-facing output names before sending the action.

### `POST /api/v1/nvr/{deviceID}/channels/{channel}/recording`

- Sets recorder-side recording mode for one NVR channel.
- Request body:

```json
{
  "action": "start"
}
```

- Supported `action` values:
  - `start`
  - `stop`
  - `auto`
- This is device-side recording control, not bridge-owned MP4 clip capture.
- Use `auto` to return the channel to schedule-controlled recording after a manual start/stop action.

### `POST /api/v1/nvr/{deviceID}/channels/{channel}/diagnostics`

- Runs one explicit diagnostic control strategy against an NVR channel.
- This endpoint is intended for `/admin/test-bridge` and field diagnostics, not routine automations.
- Request body:

```json
{
  "method": "direct_ipc_lighting",
  "action": "start",
  "duration_ms": 500
}
```

- Supported method groups include:
  - bridge-selected strategies: `bridge_light`, `bridge_warning_light`, `bridge_siren`, `bridge_wiper`, `bridge_audio`
  - raw NVR PTZ CGI: `nvr_ptz_aux`, `nvr_ptz_light`, `nvr_ptz_wiper`
  - NVR config/RPC lighting: `nvr_lighting_config`, `nvr_video_input_light_param`
  - direct IPC lighting/PTZ/audio: `direct_ipc_lighting`, `direct_ipc_ptz_light`, `direct_ipc_ptz_light_ch0`, `direct_ipc_ptz_aux`, `direct_ipc_ptz_aux_ch0`, `direct_ipc_ptz_wiper`, `direct_ipc_ptz_wiper_ch0`, `direct_ipc_audio`
  - recorder mode: `record_mode`
- Config-writing methods require `allow_config_writes: true` on the NVR.
- Direct IPC methods require `direct_ipc` credentials for the selected NVR channel.

### `GET /api/v1/nvr/{deviceID}/channels/{channel}/snapshot`

- Returns a channel snapshot image.
- This is the stable NVR channel snapshot URL exposed in catalogs and Home Assistant.
- Internally, the bridge can satisfy this from the configured stream path first and use device-specific snapshot behavior as fallback.

## 🚪 VTO Surfaces

### `GET /api/v1/vto/{deviceID}/controls`

- Returns VTO control capabilities.
- Main capability groups:
  - `call`
  - `locks`
  - `audio`
  - `recording`

### `POST /api/v1/vto/{deviceID}/locks/{lockIndex}/unlock`

- Fires one VTO lock output.
- No request body.
- `lockIndex` is zero-based in the URL.

### `POST /api/v1/vto/{deviceID}/call/answer`

- Attempts to answer the current VTO call session.
- No request body.

### `POST /api/v1/vto/{deviceID}/call/hangup`

- Attempts to hang up the current VTO call session.
- No request body.

### `POST /api/v1/vto/{deviceID}/audio/output-volume`

- Sets VTO speaker/output volume.
- Request body:

```json
{
  "level": 35,
  "slot": 0
}
```

- `level` must be between `0` and `100`.
- `slot` must be zero or positive.

### `POST /api/v1/vto/{deviceID}/audio/input-volume`

- Sets VTO microphone/input volume.
- Uses the same request body shape as output volume.

### `POST /api/v1/vto/{deviceID}/audio/mute`

- Sets VTO mute state.
- Request body:

```json
{
  "muted": true
}
```

### `POST /api/v1/vto/{deviceID}/recording`

- Enables or disables VTO auto-record behavior.
- Request body:

```json
{
  "auto_record_enabled": true
}
```

- This is device-side VTO recording configuration, not bridge MP4 clip recording.

### `GET /api/v1/vto/{deviceID}/intercom`

- Returns the bridge-hosted HTML intercom page for one VTO.
- Optional query param:
  - `profile`
- If `profile` is omitted, the bridge uses the VTO's recommended profile and then falls back to `stable`.

### `GET /api/v1/vto/{deviceID}/intercom/status`

- Returns bridge intercom runtime status for one VTO stream.
- Main uses:
  - inspect active bridge media sessions
  - inspect browser/uplink state
  - inspect forwarded RTP packet counters

### `POST /api/v1/vto/{deviceID}/intercom/reset`

- Stops bridge-managed intercom media sessions for the VTO stream.
- Use this when the browser helper page or bridge media state gets stuck.
- Returns the updated intercom status.

### `POST /api/v1/vto/{deviceID}/intercom/uplink/enable`

- Enables external RTP export for the VTO bridge intercom stream.
- Requires `media.webrtc_uplink_targets` to be configured.
- Returns the updated intercom status.

### `POST /api/v1/vto/{deviceID}/intercom/uplink/disable`

- Disables external RTP export for the VTO bridge intercom stream.
- Returns the updated intercom status.

### `GET /api/v1/vto/{deviceID}/snapshot`

- Returns the VTO snapshot image used by the normalized stream model and Home Assistant camera entity.

## 📷 IPC Surfaces

### `GET /api/v1/ipc/{deviceID}/snapshot`

- Returns the IPC snapshot image used by the normalized stream model and Home Assistant camera entity.

## ⚠️ Error Model

Most action and media failures use:

```json
{
  "error": "human-readable message",
  "error_code": "machine_code"
}
```

Common `error_code` values:

- `invalid_request`
- `service_unavailable`
- `unsupported_operation`
- `device_not_found`
- `playback_session_not_found`
- `clip_not_found`
- `clip_already_active`
- `transport_failure`
- `device_failure`

In practice:

- `400` usually means request validation failed or the device does not support that operation.
- `404` usually means the device, playback session, stream, or clip does not exist.
- `409` is used when a new clip is requested for a stream that is already recording.
- `502` usually means the bridge could talk to the route handler but the downstream device or media backend failed.
- `503` usually means the relevant action or media layer is disabled or not configured.

## 📚 Related Docs

- [features.md](features.md)
- [media-and-recording.md](media-and-recording.md)
