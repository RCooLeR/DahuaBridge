# Card Configuration

The cards expect the DahuaBridge Home Assistant integration to be installed first.
The integration supplies the entities, stream metadata, archive URLs, and bridge
action URLs that the cards render.

If the browser reaches the bridge through a different base URL than Home
Assistant, set `browser_bridge_url`. The card rewrites bridge media and action
URLs for the browser only.

When the bridge is mounted under a reverse-proxy path, include that path:

```yaml
browser_bridge_url: https://ha.example.com/dahua-bridge
```

The proxy must forward `/dahua-bridge/api/...` to the bridge as `/api/...`.
If event summary or snapshot URLs return `404` with the prefix in the browser,
the card configuration is usually correct and the proxy path rewrite is the
piece to check.

## Language

The cards do not have their own language option. They read the resolved
DahuaBridge integration language from camera attributes:

- `bridge_integration_language`
- `integration_language` as a compatibility fallback

Supported card languages are:

- `en`
- `uk`

If no DahuaBridge camera attribute exposes a supported language, the cards use
Home Assistant's frontend language when it is English or Ukrainian. Otherwise
they fall back to English.

Change the language from the DahuaBridge integration options:

- `auto`: follow Home Assistant language when supported
- `en`: force English
- `uk`: force Ukrainian

## Full Panel

Card type:

```yaml
type: custom:dahuabridge-surveillance-panel
```

Supported fields:

- `title`: optional panel title
- `subtitle`: optional panel subtitle
- `browser_bridge_url`: optional browser-reachable bridge base URL
- `event_lookback_hours`: initial bridge event window, 1-168 hours
- `bridge_event_poll_seconds`: event poll interval, 5-300 seconds
- `max_events`: visible recent event limit, 1-50
- `vto`: optional preferred VTO settings

Example:

```yaml
type: custom:dahuabridge-surveillance-panel
title: DahuaBridge Surveillance
subtitle: Home perimeter
browser_bridge_url: https://dahua.example.com
event_lookback_hours: 12
bridge_event_poll_seconds: 15
max_events: 14
vto:
  device_id: front_vto
```

## Tile

Card type:

```yaml
type: custom:dahuabridge-surveillance-tile
```

Required fields:

- `device_id`: DahuaBridge device id for one camera or VTO

Optional fields:

- `title`: display label override
- `browser_bridge_url`: browser-reachable bridge base URL
- `vto`: optional VTO settings

Example:

```yaml
type: custom:dahuabridge-surveillance-tile
device_id: west20_nvr_channel_09
title: Yard
browser_bridge_url: https://dahua.example.com
```

## VTO Settings

Both cards accept the same optional `vto` object:

- `device_id`: preferred VTO device id
- `label`: display label override
- `lock_button_entity`: Home Assistant lock button entity
- `auto_record_entity`: auto-record switch entity

Only controls backed by real Home Assistant entities or bridge URLs are shown.
The cards do not expose VTO device mute or VTO speaker/microphone volume
controls; camera audio is controlled only in the browser player.
VTO preview images prefer the direct VTO snapshot endpoint and fall back to the
bundled card logo if the snapshot image fails to load.

## Archive Metadata Used By The Cards

For archive tabs, the cards read these channel attributes:

- `bridge_archive_smd_ivs_url_template`
- `bridge_archive_recording_chunks_url_template`
- `bridge_archive_recordings_url_template` as a compatibility fallback for chunks
- `bridge_channel`
- `bridge_root_device_id`

SMD/IVS playback and download use row-level MP4 fields returned by the bridge,
such as `asset_download_url` and `export_url` for MP4 downloads. SMD/IVS Play
uses direct RTSP archive playback through the Home Assistant timeframe proxy.
Recording chunk download uses row-level `download_url`.

The cards do not read `bridge_playback_sessions_url` or
`bridge_archive_coverage_url`.
