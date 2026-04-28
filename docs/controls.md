# Control Surface

This document is the bridge and Home Assistant control reference.

It describes:

- which device-side controls DahuaBridge exposes
- which HTTP APIs back those controls
- which MQTT/Home Assistant surfaces exist
- which bridge-native Home Assistant entities exist
- which known device quirks affect control behavior

This file is intentionally focused on controls and archive/playback actions, not on generic inventory or event APIs.

## Scope

Current control coverage is centered on:

- `nvr_channel` control surfaces behind a Dahua NVR
- `vto` door-station control surfaces
- NVR archive search and playback session control
- Home Assistant MQTT discovery command entities
- the bridge-native Home Assistant custom integration

## NVR Channel Controls

Per-channel capabilities are discovered dynamically.
The bridge does not assume that every channel supports PTZ, deterrence outputs, recording control, or audio control.

### Capability Read API

```text
GET /api/v1/nvr/{deviceID}/channels/{channel}/controls
```

Returns:

- PTZ support and command set
- aux/light/wiper outputs
- semantic aux features such as `siren` and `warning_light`
- recording support and current mode/state
- channel audio support metadata
- validation notes

Catalog summary is also exposed in:

```text
GET /api/v1/streams
```

under:

- `entry.controls` for detailed capability-specific objects
- `entry.features` for a normalized card-facing feature list

The intent of `entry.features` is to let a frontend render from discovered capabilities instead of assuming every channel has the same controls.

### PTZ API

```text
POST /api/v1/nvr/{deviceID}/channels/{channel}/ptz
```

Request body:

```json
{
  "command": "up",
  "action": "pulse",
  "duration_ms": 400
}
```

Supported command families:

- directional: `up`, `down`, `left`, `right`
- diagonals
- `zoom_in`, `zoom_out`
- `focus_near`, `focus_far`

Supported actions:

- `start`
- `stop`
- `pulse`

### Aux / Light / Wiper / Siren API

```text
POST /api/v1/nvr/{deviceID}/channels/{channel}/aux
```

Request body:

```json
{
  "action": "pulse",
  "output": "warning_light",
  "duration_ms": 3000
}
```

Accepted `output` values:

- raw outputs: `aux`, `light`, `wiper`
- semantic aliases:
  - `siren` -> `aux`
  - `warning_light` -> `light`

### Recording Control API

```text
POST /api/v1/nvr/{deviceID}/channels/{channel}/recording
```

Request body:

```json
{
  "action": "start"
}
```

Accepted values:

- `start`
- `stop`

### Channel Audio Support

The bridge models channel audio control support, but on the validated `DHI-NVR5232-EI` with the configured bridge account there is no usable channel mute/volume CGI surface on channels `1-11`.

The bridge still reports:

- whether mute/volume are supported
- whether remote audio playback/siren support exists
- whether playback volume is permission-denied rather than unsupported

That metadata appears in:

- `GET /api/v1/nvr/{deviceID}/channels/{channel}/controls`
- `GET /api/v1/streams`
- Home Assistant diagnostics

## NVR Archive / Playback Controls

### Archive Search API

```text
GET /api/v1/nvr/{deviceID}/recordings?channel=1&start=2026-04-28T00:00:00Z&end=2026-04-28T01:00:00Z&limit=25
```

### Playback Session Create

```text
POST /api/v1/nvr/{deviceID}/playback/sessions
```

### Playback Session Read

```text
GET /api/v1/nvr/playback/sessions/{sessionID}
```

### Playback Session Seek

```text
POST /api/v1/nvr/playback/sessions/{sessionID}/seek
```

Validated device note:

- on the real `DHI-NVR5232-EI`, long-range archive queries on channels `1`, `5`, and `11` worked
- empty post-clip windows returned device-side `400 Bad Request` instead of `0` items

## VTO Controls

### Capability Read API

```text
GET /api/v1/vto/{deviceID}/controls
```

Returns:

- call-answer / hangup support
- lock support
- audio output/input volume support and current values
- mute support and current state
- auto-record support and current state
- stream-audio flag
- talkback/full-call-acceptance support flags
- validation notes

Catalog summary is also exposed in:

```text
GET /api/v1/streams
```

under:

- `entry.intercom` for detailed VTO call/audio/session state
- `entry.features` for a normalized card-facing feature list

Typical normalized feature keys include:

- NVR channel:
  - `archive_search`
  - `archive_playback`
  - `ptz`
  - `siren`
  - `warning_light`
  - `wiper`
  - `recording`
- VTO:
  - `call_answer`
  - `call_hangup`
  - `unlock`
  - `output_volume`
  - `input_volume`
  - `mute`
  - `auto_record`
  - `intercom_reset`

### Call Control APIs

```text
POST /api/v1/vto/{deviceID}/call/answer
POST /api/v1/vto/{deviceID}/call/hangup
```

### Lock / Door Control API

```text
POST /api/v1/vto/{deviceID}/locks/{lockIndex}/unlock
```

### Audio Output Volume API

```text
POST /api/v1/vto/{deviceID}/audio/output-volume
```

Request body:

```json
{
  "slot": 0,
  "level": 80
}
```

### Audio Input Volume API

```text
POST /api/v1/vto/{deviceID}/audio/input-volume
```

Request body:

```json
{
  "slot": 0,
  "level": 65
}
```

### Mute API

```text
POST /api/v1/vto/{deviceID}/audio/mute
```

Request body:

```json
{
  "muted": true
}
```

### Recording API

```text
POST /api/v1/vto/{deviceID}/recording
```

Request body:

```json
{
  "auto_record_enabled": true
}
```

On the validated `DHI-VTO2311R-WP`, this controls automatic call recording, not a generic ad-hoc start/stop recorder surface.

### Bridge Intercom APIs

```text
GET  /api/v1/vto/{deviceID}/intercom
GET  /api/v1/vto/{deviceID}/intercom/status
POST /api/v1/vto/{deviceID}/intercom/reset
POST /api/v1/vto/{deviceID}/intercom/uplink/enable
POST /api/v1/vto/{deviceID}/intercom/uplink/disable
```

These are bridge media/session controls, not proof of a fully separate device-native Dahua talkback RPC family.

## Home Assistant MQTT Discovery Controls

### NVR Channel MQTT Entities

The bridge publishes MQTT discovery command entities for supported NVR channel actions such as:

- `siren`
- `warning_light`
- `wiper`
- `recording_start`
- `recording_stop`

Typical command payload:

- `PRESS`

### VTO MQTT Entities

The bridge now publishes MQTT discovery control entities for:

- `button`
  - `answer`
  - `hangup`
  - `intercom_reset`
  - `uplink_enable`
  - `uplink_disable`
- `switch`
  - `mute`
  - `auto_record`
- `number`
  - `output_volume_control`
  - `input_volume_control`

MQTT command payloads:

- `button`: `PRESS`
- `switch`: `ON` / `OFF`
- `number`: numeric text payload in the `0..100` range

Examples:

```text
dahuabridge/devices/front_vto/command/mute          payload=ON
dahuabridge/devices/front_vto/command/auto_record   payload=OFF
dahuabridge/devices/front_vto/command/output_volume payload=75
dahuabridge/devices/front_vto/command/input_volume  payload=65
```

## Bridge-Native Home Assistant Integration

The custom integration in `integration/custom_components/dahuabridge` consumes:

```text
GET /api/v1/home-assistant/native/catalog
```

and exposes bridge-native entities grouped by device.

Current native-integration VTO control coverage:

- `number`
  - output volume
  - input volume
- `switch`
  - mute
  - auto record
- `button`
  - answer call
  - hang up call
  - reset bridge session
  - enable/disable RTP export
  - unlock

The native integration uses bridge HTTP APIs directly, not MQTT.

## Error Model

Action APIs use a stable error payload model.

Representative `error_code` values include:

- `invalid_request`
- `unsupported_operation`
- `device_not_found`
- `playback_session_not_found`
- `transport_failure`
- `device_failure`

## Related Docs

- [README](README.md)
- [device-model.md](device-model.md)
- [how-it-works.md](how-it-works.md)
- [migration.md](migration.md)
- [../bridge/README.md](../bridge/README.md)
- [../integration/README.md](../integration/README.md)
