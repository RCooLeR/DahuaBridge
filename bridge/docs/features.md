# Bridge Features

This page is the bridge feature inventory with drill-down by area.

## 1. Device Discovery And Inventory

The bridge can probe:

- NVR devices
- NVR channels
- NVR disks
- VTO devices
- VTO child objects such as locks and alarm inputs
- standalone IPC cameras

What it discovers includes:

- identity and model data
- firmware and serial data
- channel inventory
- disk inventory
- lock/alarm inventory
- stream and codec metadata
- control capability metadata

Related APIs:

- `GET /api/v1/devices`
- `GET /api/v1/devices/{deviceID}`
- `POST /api/v1/devices/{deviceID}/probe`
- `POST /api/v1/devices/probe-all`
- `POST /api/v1/nvr/{deviceID}/inventory/refresh`

## 2. Normalized Device Model

The bridge normalizes Dahua-specific data into a stable model used by:

- the admin page
- `/api/v1/devices`
- `/api/v1/streams`
- the native Home Assistant catalog

Read more:

- [device-and-stream-model.md](device-and-stream-model.md)

## 3. Event Collection And Event APIs

The bridge collects and exposes normalized events.

Key uses:

- recent event browsing
- diagnostic review
- Home Assistant-facing state derivation

APIs:

- `GET /api/v1/events`
- `DELETE /api/v1/events`

## 4. Snapshots

The bridge supports:

- NVR channel snapshots
- VTO snapshots
- IPC snapshots
- generic stream-backed snapshots

Current behavior:

- snapshot endpoints now prefer capturing from the configured stream path
- device snapshot providers remain available as fallback where needed
- identical snapshot requests are cached briefly and coalesced in-flight to reduce repeated NVR/VTO/IPC fetches

APIs:

- `GET /api/v1/media/snapshot/{streamID}`
- `GET /api/v1/nvr/{deviceID}/channels/{channel}/snapshot`
- `GET /api/v1/vto/{deviceID}/snapshot`
- `GET /api/v1/ipc/{deviceID}/snapshot`

## 5. Live Media

The bridge can expose live media helpers for any known stream:

- MJPEG
- HLS
- preview pages
- WebRTC helper pages and answer flow

Bridge media policy:

- NVR and camera audio settings are no longer toggled by the bridge for normal stream viewing
- transcode output includes audio only when the source stream actually provides audio
- playback workers now stop FFmpeg at archive end-of-file instead of running past the requested window

APIs:

- `GET /api/v1/media/mjpeg/{streamID}`
- `GET /api/v1/media/hls/{streamID}/{profile}/index.m3u8`
- `GET /api/v1/media/hls/{streamID}/{profile}/{segmentName}`
- `GET /api/v1/media/preview/{streamID}`
- `GET /api/v1/media/webrtc/{streamID}/{profile}`
- `POST /api/v1/media/webrtc/{streamID}/{profile}/offer`
- `GET /api/v1/media/workers`

## 6. Bridge-Owned Recording

The bridge can record MP4 clips from streams into its own storage volume.

This is separate from device-side NVR recording mode control.

Capabilities:

- start clip capture for a stream
- stop an active clip
- list clips
- fetch clip metadata
- download the resulting MP4
- merge bridge clips into NVR recordings search results

Read more:

- [media-and-recording.md](media-and-recording.md)

## 7. NVR Archive And Playback

The bridge supports:

- archive search
- optional archive event-type filtering
- playback session creation
- playback session lookup
- playback seek
- playback HLS/WebRTC helpers
- MP4 export by capturing a native archive playback stream

Operational notes:

- finite archive playback now enforces the requested playback duration across HLS, MJPEG, WebRTC, and clip export
- identical recorder archive searches are cached briefly and coalesced in-flight to reduce repeated recorder RPC/CGI searches
- archive export is the supported bridge path for recorder footage download
- when `file_path` is known, archive export can transcode directly from the recorder `.dav` file instead of going through archive RTSP playback
- SMD and IVS event-backed archive items are supported through the same playback-session and bridge MP4 export path as regular recorder footage
- live validation on May 2, 2026 confirmed channel 1 human/vehicle SMD archive lookups, 11 concurrent live HLS channel starts, near-end 24/7 playback EOF handling, and successful SMD MP4 export

APIs:

- `GET /api/v1/nvr/{deviceID}/recordings`
- `POST /api/v1/nvr/{deviceID}/recordings/export`
- `POST /api/v1/nvr/{deviceID}/playback/sessions`
- `GET /api/v1/nvr/playback/sessions/{sessionID}`
- `POST /api/v1/nvr/playback/sessions/{sessionID}/seek`

## 8. NVR Channel Controls

Per-channel capability discovery can expose:

- PTZ
- aux/light/wiper outputs
- deterrence feature aliases such as siren and warning light
- audio capability metadata
- recording state metadata

Important note:

- device-side manual recording endpoints still exist
- NVR channel mute control is no longer exposed by the bridge
- source-audio metadata is still published, but bridge output audio is decided at transcode time
- they are no longer the preferred recording UX for higher layers
- `action:"auto"` returns a channel to schedule-controlled recording after manual start/stop
- bridge-owned clip recording is the intended recording flow for integration and UI work

APIs:

- `GET /api/v1/nvr/{deviceID}/channels/{channel}/controls`
- `POST /api/v1/nvr/{deviceID}/channels/{channel}/ptz`
- `POST /api/v1/nvr/{deviceID}/channels/{channel}/aux`
- `POST /api/v1/nvr/{deviceID}/channels/{channel}/recording`
- `POST /api/v1/nvr/{deviceID}/channels/{channel}/diagnostics`

Operator diagnostics:

- `/admin/test-bridge` lets you select a channel, switch stream transports, and compare bridge-selected control behavior with explicit NVR CGI, NVR RPC/config, and direct IPC attempts.

## 9. VTO Controls And Intercom Helpers

The bridge exposes:

- lock unlock
- call answer
- call hangup
- audio output volume
- audio input volume
- mute
- VTO auto-record toggle
- intercom session status
- bridge session reset
- browser microphone uplink export controls

APIs:

- `GET /api/v1/vto/{deviceID}/controls`
- `POST /api/v1/vto/{deviceID}/locks/{lockIndex}/unlock`
- `POST /api/v1/vto/{deviceID}/call/answer`
- `POST /api/v1/vto/{deviceID}/call/hangup`
- `POST /api/v1/vto/{deviceID}/audio/output-volume`
- `POST /api/v1/vto/{deviceID}/audio/input-volume`
- `POST /api/v1/vto/{deviceID}/audio/mute`
- `POST /api/v1/vto/{deviceID}/recording`
- `GET /api/v1/vto/{deviceID}/intercom`
- `GET /api/v1/vto/{deviceID}/intercom/status`
- `POST /api/v1/vto/{deviceID}/intercom/reset`
- `POST /api/v1/vto/{deviceID}/intercom/uplink/enable`
- `POST /api/v1/vto/{deviceID}/intercom/uplink/disable`

## 10. Native Home Assistant Catalog

The bridge publishes a Home Assistant-oriented catalog consumed by the Python integration.

API:

- `GET /api/v1/home-assistant/native/catalog`

It includes:

- normalized devices
- stream entries
- control metadata
- capture metadata
- bridge-generated URLs used by the integration

## 11. Admin And Operations Surface

Operational features include:

- admin page
- health endpoints
- readiness endpoint
- metrics endpoint
- runtime settings display

APIs:

- `GET /admin`
- `GET /admin/test-bridge`
- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/status`
- `GET /metrics`

## Next Step

- [media-and-recording.md](media-and-recording.md)
- [api-reference.md](api-reference.md)
