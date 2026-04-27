# Device Model And Naming

This file explains what devices and entities the project creates, how they are grouped, and how they are named.

## Main Idea

The project tries to represent one real streamable thing as one Home Assistant device.

Examples:

- one standalone IPC camera
- one NVR channel
- one VTO door station

That device should then contain:

- the camera entity
- the related binary sensors
- the related state sensors
- the related action buttons

## Device Kinds

Current bridge device kinds are:

- `nvr`
- `nvr_channel`
- `nvr_disk`
- `ipc`
- `vto`
- `vto_lock`
- `vto_alarm`

## Root Device IDs

Root device IDs come directly from your config.

Examples:

- `west20_nvr`
- `yard_ipc`
- `front_vto`

Choose stable IDs. Recommended style:

- lowercase
- ASCII
- words separated with `_`
- no spaces
- do not rename them after Home Assistant is already using them

## Child Device ID Patterns

### NVR Channels

Pattern:

```text
<nvr_id>_channel_01
<nvr_id>_channel_02
...
```

Examples:

- `west20_nvr_channel_01`
- `west20_nvr_channel_08`

This is the most important child device type.
Each NVR channel is intended to behave like one actual camera device inside Home Assistant.

### NVR Disks

Pattern:

```text
<nvr_id>_disk_00
<nvr_id>_disk_01
...
```

Note:

- disks currently use the Dahua inventory index as-is
- that means the first disk may be `00`

### VTO Locks

Pattern:

```text
<vto_id>_lock_00
<vto_id>_lock_01
...
```

Note:

- lock IDs use the raw Dahua index as-is

### VTO Alarm Inputs

Pattern:

```text
<vto_id>_alarm_00
<vto_id>_alarm_01
...
```

Note:

- alarm IDs use the raw Dahua index as-is
- if the device reports alarm index `3`, the child ID becomes `_alarm_03`

## What Gets Created Per Device Type

### `nvr`

The NVR root device represents the recorder itself.

Typical root-level entities:

- `Online`
- storage and capacity sensors
- root diagnostics
- `Probe Now` button
- `Refresh Inventory` button

The NVR root is not the main camera device.
The channel children are.

### `nvr_channel`

This is the main per-camera device model for NVR-connected cameras.

Typical entities:

- `Camera`
- `Online`
- `Motion`
- `Human`
- `Vehicle`
- `Tripwire`
- `Intrusion`
- stream/codec/resolution diagnostic sensors

Typical event categories:

- video motion
- person detection
- vehicle detection
- tripwire
- intrusion

### `nvr_disk`

This is a child device for recorder storage.

Typical entities:

- `Online`
- disk state and storage sensors

### `ipc`

A standalone IPC is already a single direct device.

Typical entities:

- `Camera`
- `Online`
- `Motion`
- `Human`
- `Vehicle`
- `Tripwire`
- `Intrusion`

### `vto`

The VTO root device represents the door station itself.

Typical entities:

- `Camera`
- `Online`
- `Doorbell`
- `Call Active`
- `Call State`
- `Last Call Started At`
- `Last Call Ended At`
- `Last Call Duration`
- `Last Call Source`
- `Tamper`
- `Access Active`
- `Probe Now`

Possible buttons on the VTO root:

- `Answer Call`
- `Hang Up Call`
- `Reset Bridge Session`
- `Enable RTP Export`
- `Disable RTP Export`

### `vto_lock`

This is a child device for a VTO lock/output.

Typical entities:

- lock-related state fields
- unlock action through the root/native integration path

### `vto_alarm`

This is a child device for a VTO alarm input.

Typical entities:

- input enabled/state details
- related alarm event state

## Sensor And Event Categories

### NVR And IPC Detection Categories

These state categories are supported:

- `motion`
- `human`
- `vehicle`
- `tripwire`
- `intrusion`

Typical Home Assistant binary sensor names:

- `Motion`
- `Human`
- `Vehicle`
- `Tripwire`
- `Intrusion`

### VTO Categories

These state categories are supported on the VTO side:

- `doorbell`
- `call`
- `call_state`
- `tamper`
- `access`

Call-session related sensor fields include:

- `last_call_started_at`
- `last_call_ended_at`
- `last_call_duration_seconds`
- `last_call_source`

## Event Naming

Raw Dahua event codes are normalized internally.

Examples:

- `VideoMotion` -> motion
- `SmartMotionHuman` -> human
- `SmartMotionVehicle` -> vehicle
- `CrossLineDetection` -> tripwire
- `CrossRegionDetection` -> intrusion
- `DoorBell` -> doorbell
- `AccessCtl` or `AccessControl` -> access
- `Tamper` -> tamper
- `Call` -> call

For MQTT/Home Assistant triggers, common trigger payload names look like:

- `videomotion_start`
- `smartmotionhuman_start`
- `smartmotionvehicle_start`
- `crosslinedetection_start`
- `crossregiondetection_start`
- `doorbell_start`
- `call_start`
- `call_stop`
- `accesscontrol_start`
- `tamper_start`

Typical Home Assistant trigger type names look like:

- `motion_detected`
- `human_detected`
- `vehicle_detected`
- `tripwire_detected`
- `intrusion_detected`
- `doorbell_pressed`
- `call_started`
- `call_ended`
- `access_granted`
- `tamper_detected`

## Native Integration Entity Naming Pattern

The native Home Assistant integration generally follows these patterns:

- camera: `camera.<device_id>_camera`
- online binary sensor: `binary_sensor.<device_id>_online`
- state binary sensor: `binary_sensor.<device_id>_<field>`
- state sensor: `sensor.<device_id>_<field>`
- action button: `button.<device_id>_<action>`

Examples:

- `camera.west20_nvr_channel_01_camera`
- `binary_sensor.west20_nvr_channel_01_motion`
- `sensor.front_vto_call_state`
- `button.west20_nvr_refresh_inventory`

Final Home Assistant entity IDs can still be adjusted by Home Assistant rules or user renames, but these are the default patterns.

## Recommended Mental Model

Use this model when thinking about the system:

- NVR root = recorder
- NVR channel = actual camera device
- IPC root = actual camera device
- VTO root = actual door station device
- VTO lock/alarm = child accessory devices

If your goal is:

- one device
- one live stream
- all related sensors on that same device

then `nvr_channel` and `ipc` are the key device types to focus on.
