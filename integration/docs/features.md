# Integration Features

This page lists the integration feature set with drill-down by area.

## 1. Native Bridge Catalog Consumption

The integration consumes:

- `GET /api/v1/home-assistant/native/catalog`

It does not talk directly to Dahua devices.

## 2. Config Flow And Options Flow

The integration provides:

- initial setup through Home Assistant UI
- runtime options updates for poll/media preferences

## 3. Device Grouping

The integration groups Home Assistant entities around the normalized device model from the bridge.

Examples:

- NVR root as recorder device
- NVR channel as camera-like device
- IPC as camera device
- VTO as door station device

## 4. Platforms

The integration provides these Home Assistant platforms:

- camera
- binary sensor
- sensor
- button
- number
- switch

## 5. Camera Behavior

The integration camera entity provides:

- stream support
- bridge snapshot fetching
- stream source resolution from bridge-generated URLs
- bridge capture metadata in attributes
- bridge recording state exposure
- bridge-backed start/stop recording services

Read more:

- [camera-recording.md](camera-recording.md)

## 6. Binary Sensors

The integration exposes:

- online/connectivity state
- event-derived boolean state such as motion, human, vehicle, tripwire, intrusion, tamper, doorbell, and call-related state where applicable

## 7. Sensors

The integration exposes:

- scalar diagnostic fields
- timestamps
- device and stream metadata

## 8. Buttons

The integration can expose bridge-backed actions such as:

- probe now
- refresh inventory
- answer call
- hang up call
- reset bridge session
- enable RTP export
- disable RTP export
- unlock outputs

## 9. Numbers

The integration can expose bridge-backed numeric controls such as:

- VTO output volume
- VTO input volume

## 10. Switches

The integration can expose bridge-backed toggles such as:

- VTO mute
- VTO auto record

## 11. Diagnostics

The integration supports diagnostics downloads for troubleshooting.

## Related Docs

- [entities-and-controls.md](entities-and-controls.md)
- [camera-recording.md](camera-recording.md)
