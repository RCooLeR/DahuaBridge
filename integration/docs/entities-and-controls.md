# Entities And Controls

This page explains what the integration creates and which bridge-backed controls appear in Home Assistant.

## Device Model

The integration follows the bridge-normalized model.

Important mental model:

- NVR root = recorder
- NVR channel = actual camera-like device
- IPC = actual camera device
- VTO = actual door station device

Bridge-side model details:

- [../../bridge/docs/device-and-stream-model.md](../../bridge/docs/device-and-stream-model.md)

## Platform Mapping

The integration registers these Home Assistant platforms:

- `camera`
- `binary_sensor`
- `sensor`
- `button`
- `number`
- `switch`

Platform creation is driven by the native catalog, not by hard-coded Dahua model names.

Exact device-kind behavior:

- `nvr`
  - no camera entity
  - gets `Online`
  - gets scalar and boolean state entities from the root recorder record
  - gets `Probe Now`
  - gets `Refresh Inventory`
- `nvr_channel`
  - gets `Camera`
  - gets `Online`
  - gets event/state binary sensors and scalar sensors
  - does not get root-only action buttons
- `nvr_disk`
  - no camera entity
  - gets `Online`
  - gets disk-related binary sensors and scalar sensors surfaced by the bridge
- `ipc`
  - gets `Camera`
  - gets `Online`
  - gets event/state binary sensors and scalar sensors
  - gets `Probe Now`
- `vto`
  - gets `Camera`
  - gets `Online`
  - gets call, doorbell, tamper, access, and intercom-related state entities
  - gets `Probe Now`
  - gets VTO/intercom action buttons
  - gets VTO volume numbers
  - gets VTO mute and auto-record switches
- `vto_lock`
  - no camera entity
  - gets `Online`
  - gets lock-related state entities exposed by the bridge
  - unlock actions are exposed on the VTO root device, not on the lock child record
- `vto_alarm`
  - no camera entity
  - gets `Online`
  - gets alarm-related binary sensors and scalar sensors if the bridge surfaces them

## Camera

Camera entities are created only for records that contain a `stream` object in the native catalog.

That currently means:

- NVR channels
- IPC cameras
- VTO devices

The camera entity can expose:

- stream support
- bridge snapshot path
- bridge capture metadata
- bridge recording state
- bridge profiles
- bridge controls
- bridge features
- bridge intercom metadata for VTO devices

Important notes:

- the entity unique ID is based on `<device_id>_camera`
- the integration enables stream support only when it can resolve a usable stream source from the preferred profile and preferred source settings
- entity availability follows successful catalog refresh, not only the last cached record
- bridge-backed `start_recording` and `stop_recording` services are attached to this camera entity

Useful attributes commonly exposed on the camera:

- `recommended_profile`
- `snapshot_url`
- `stream_source`
- `bridge_capture`
- `bridge_profiles`
- `bridge_controls`
- `bridge_features`
- `bridge_intercom`
- `preferred_video_profile`
- `preferred_video_source`

For NVR channel cameras, archive workflow attributes can also be exposed:

- `bridge_archive_recordings_url_template`
- `bridge_archive_export_url`
- `bridge_playback_sessions_url`

Those attributes point to the supported bridge archive APIs for:

- searching recorder footage
- exporting matching archive windows to bridge MP4 clips
- creating playback sessions for HLS, MJPEG, or WebRTC access

## Binary Sensors

Every catalog record gets:

- `Online`

Additional boolean fields are turned into binary sensors from the merged catalog record, which includes:

- device attributes
- stream fields
- state info

Typical boolean sensors include:

- online
- motion
- human
- vehicle
- tripwire
- intrusion
- tamper
- doorbell
- call-active style states

Behavior details:

- event-derived fields such as motion, human, vehicle, tripwire, intrusion, doorbell, tamper, and call-like states follow bridge state rather than direct device polling from Home Assistant
- transient event-style binary sensors are treated more conservatively when a device is offline so the integration does not manufacture a misleading live event state

## Sensors

Scalar fields from the merged catalog record become normal Home Assistant sensors.

Typical sensors include:

- call state
- last call timestamps
- codec and resolution metadata
- storage counters
- other normalized scalar fields from the bridge catalog

Behavior details:

- fields ending in `_at` are exposed as timestamp sensors
- fields ending in `_bytes`, `_percent`, `_seconds`, and `_packets` get matching units
- many metadata sensors are diagnostic-category entities rather than primary user-facing state

## Buttons

Button entities are created only when the native catalog advertises a backing bridge URL.

Exact button coverage today:

- `Probe Now`
  - created on root devices such as `nvr`, `ipc`, and `vto`
- `Refresh Inventory`
  - created only on NVR root devices
- `Answer Call`
- `Hang Up Call`
- `Reset Bridge Session`
- `Enable RTP Export`
- `Disable RTP Export`
  - created on VTO roots when those intercom URLs exist
- `Unlock 1`, `Unlock 2`, and so on
  - created on the VTO root for each advertised lock URL

All button presses call the bridge over HTTP and then refresh the catalog.

## Numbers

Number entities are currently VTO-only and are created only when the catalog advertises the corresponding URL plus capability flag.

Current number controls:

- output volume
- input volume

Behavior details:

- range is `0` to `100`
- mode is a Home Assistant slider
- writing a value sends a JSON body with `slot` and `level`

## Switches

Switch entities are currently VTO-only and are created only when the catalog advertises the corresponding URL plus capability flag.

Current switch controls:

- mute
- auto record

NVR channel audio mute is not exposed here. Bridge output audio is decided at transcode time.

## Naming And Unique IDs

The integration generates stable unique IDs from the bridge device ID plus a field or control key.

Typical patterns:

- camera: `<device_id>_camera`
- online binary sensor: `<device_id>_online`
- state binary sensor: `<device_id>_<field>`
- state sensor: `<device_id>_<field>`
- button: `<device_id>_<action_key>`
- number: `<device_id>_<control_key>`
- switch: `<device_id>_<control_key>`

Home Assistant can still derive different final entity IDs after its own naming and de-duplication rules, so treat these as unique-ID patterns rather than guaranteed visible entity IDs.

## Recording UX Boundary

The integration no longer treats NVR manual recording control as the primary recording action for cameras.

Instead:

- bridge-owned capture metadata is surfaced on camera entities
- bridge-backed start/stop recording services are attached to the camera entity
- event-backed archive items such as SMD and IVS footage use the bridge playback and export APIs, not direct recorder file download

This is important because device-side NVR circular recording configuration should not be treated as the UI path for ad-hoc clip capture.

## Diagnostics

The integration also provides diagnostics export for config entries.

## Related Docs

- [features.md](features.md)
- [camera-recording.md](camera-recording.md)
