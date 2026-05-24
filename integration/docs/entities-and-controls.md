# Entities And Controls

Entities are generated from the native catalog. The integration does not hard-code a list of Dahua models.

## Platforms

The integration registers:

- `camera`
- `binary_sensor`
- `sensor`
- `button`
- `switch`

Each platform receives the same catalog and creates only the entities it can back with current catalog data.

## Device Model

The bridge normalizes Dahua devices into records. The integration follows those records.

| Bridge kind | Home Assistant behavior |
| --- | --- |
| `nvr` | Recorder device, no camera entity, root state sensors, `Probe Now`, `Refresh Inventory`. |
| `nvr_channel` | Camera-like child device, camera entity, state sensors, archive attributes. |
| `nvr_disk` | Disk child device, online/state entities only. |
| `ipc` | Camera device, camera entity, state sensors, `Probe Now`. |
| `vto` | Door station, camera entity, call/intercom state, intercom buttons, and switches. |
| `vto_lock` | Lock child record, online/state entities only; unlock buttons live on the VTO root. |
| `vto_alarm` | Alarm child record, online/state entities when advertised. |

## Cameras

A camera entity is created for any catalog record that has a stream section.

Unique ID:

```text
<device_id>_camera
```

The camera can expose:

- stream support when a usable stream URL is resolved
- snapshot fetching
- bridge capture metadata
- bridge recording state
- stream profiles
- controls and feature metadata
- VTO intercom metadata
- NVR archive/playback/export URLs for NVR channels

Common attributes:

- `recommended_profile`
- `snapshot_url`
- `stream_source`
- `bridge_capture`
- `bridge_recording_active`
- `bridge_profiles`
- `bridge_controls`
- `bridge_features`
- `bridge_intercom`
- `preferred_video_profile`
- `preferred_video_source`

NVR channel archive attributes:

- `bridge_archive_smd_ivs_url_template`
- `bridge_archive_recording_chunks_url_template`
- `bridge_archive_recordings_url_template`
- `bridge_archive_export_url`
- `bridge_playback_sessions_url`
- `bridge_archive_coverage_url`

`bridge_archive_recordings_url_template` remains as a compatibility alias for recording chunks. New code should prefer the explicit SMD/IVS and recording chunk attributes.

## Binary Sensors

Every catalog record gets an `Online` binary sensor.

Additional boolean fields become binary sensors when they are present in the merged catalog fields:

- device attributes
- selected stream fields
- state info

Typical fields:

- `motion`
- `human`
- `vehicle`
- `tripwire`
- `intrusion`
- `tamper`
- `doorbell`
- `call`
- `stream_available`

Transient event fields are exposed only when the bridge state or latest event indicates the field is meaningful.

## Sensors

Scalar fields become sensors.

Examples:

- call state
- last call timestamps
- codec and resolution metadata
- storage counters
- bridge session counters

Field suffix rules:

| Suffix | Home Assistant handling |
| --- | --- |
| `_at` | Timestamp device class |
| `_bytes` | `B` unit |
| `_percent` | `%` unit |
| `_seconds` | `s` unit |
| `_packets` | `packets` unit |

Metadata-heavy fields are marked as diagnostic entities.

## Buttons

Buttons are created only when the catalog advertises a backing URL.

Current buttons:

- `Probe Now`
- `Refresh Inventory`
- `Answer Call`
- `Hang Up Call`
- `Reset Bridge Session`
- `Enable RTP Export`
- `Disable RTP Export`
- `Unlock 1`, `Unlock 2`, and so on

Button presses call the bridge and then request a catalog refresh.

## Switches

Switches are created from two sources:

- VTO intercom controls such as auto record
- supported output features such as light, warning light, and siren

Feature switches send bridge payloads such as:

```json
{"output": "light", "action": "start"}
```

and:

```json
{"output": "light", "action": "stop"}
```

Device-side audio mute and volume controls are not exposed as Home Assistant entities. Browser playback audio stays in the player, and bridge MP4/export audio is decided during capture/transcode.

## Entity Availability

An entity is available when:

- the last coordinator refresh succeeded
- its backing catalog record still exists

Some control entities also require the device to be online.

The integration does not delete stale Home Assistant registry entries. If a bridge device is permanently removed, remove stale entities manually in Home Assistant.

## Unique ID Patterns

| Entity type | Unique ID pattern |
| --- | --- |
| Camera | `<device_id>_camera` |
| Online binary sensor | `<device_id>_online` |
| State binary sensor | `<device_id>_<field>` |
| State sensor | `<device_id>_<field>` |
| Button | `<device_id>_<action_key>` |
| Number | `<device_id>_<control_key>` |
| Switch | `<device_id>_<control_key>` |

Home Assistant may still choose a different visible `entity_id`.

## Related

- [Camera recording and archive access](camera-recording.md)
- [How it works](architecture.md)
