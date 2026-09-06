# Feature Summary

## Integration Core

- UI config flow and options flow.
- Bridge URL and token validation through the protected native catalog endpoint.
- Polling coordinator for `GET /api/v1/home-assistant/native/catalog`.
- Reverse-proxy-safe bridge URL rewriting.
- Direct `rtsp://` passthrough when RTSP is selected or used as fallback.
- English and Ukrainian translations.
- Integration language option for generated entity/control labels.
- Diagnostics export with URL-shaped sensitive fields redacted.

## Platforms

- `camera`
- `binary_sensor`
- `sensor`
- `button`
- `switch`

## Cameras

- Stream source selection by preferred profile and source.
- Snapshot fetching through bridge capture metadata.
- MJPEG frame fallback for camera images.
- Bridge live clip capture services.
- Bridge capture/recording attributes.
- NVR archive search/playback/export attributes.
- Timeframe MJPEG playback proxy for NVR channel cameras.

## State Entities

- Online binary sensor for every catalog record.
- Boolean state fields as binary sensors.
- Scalar fields as sensors.
- Timestamp/device-class/unit classification based on field names.
- Diagnostic category for metadata-heavy fields.

## Controls

Controls appear only when the catalog advertises a backing URL.

- Probe root devices.
- Refresh NVR inventory.
- VTO answer/hangup/reset/RTP export actions.
- VTO unlock actions.
- VTO auto record.
- Camera output feature toggles for light, warning light, and siren.

## Boundaries

The integration does not:

- talk directly to Dahua devices
- own recorder schedules
- synthesize unsupported controls
- delete Home Assistant registry entries
- download recorder DAV files directly

## Related

- [How it works](architecture.md)
- [Entities and controls](entities-and-controls.md)
- [Camera recording and archive access](camera-recording.md)
