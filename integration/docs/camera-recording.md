# Camera Recording Behavior

This page explains the recording-related behavior of the Home Assistant camera entity in this integration.

## Snapshot Behavior

For still images, the camera entity now prefers the bridge snapshot endpoint generated from capture metadata.

That means the integration can request a frame from the bridge stream path first instead of depending only on device snapshot CGI behavior.

If needed, the camera entity can still fall back to MJPEG frame extraction behavior.

## Bridge Recording Services

The camera platform registers bridge-backed entity services for DahuaBridge camera entities:

- `start_recording`
- `stop_recording`

These services map to the bridge clip capture APIs, not to device-side NVR manual recording mode changes.

## What These Services Do

`start_recording`:

- starts a bridge-owned MP4 clip capture for the camera stream
- can accept:
  - `profile`
  - `duration_seconds`

`stop_recording`:

- stops the active bridge-owned clip for that camera stream

In practice these services target the camera entity that the integration created from the bridge catalog. The integration sends the request to the bridge URL advertised in that camera's `capture` metadata.

The camera entity also exposes bridge recording state in attributes such as:

- `bridge_capture`
- `bridge_recording_active`
- `bridge_start_recording_url`
- `bridge_stop_recording_url`
- `bridge_recordings_url`

## What They Do Not Do

They do not:

- toggle long-running NVR recorder configuration
- replace the NVR's own circular recording policy
- convert Home Assistant into the owner of the bridge clip file

## Important Home Assistant Boundary

Home Assistant's built-in `camera.record` flow records from the camera stream on the Home Assistant side.

That is a different behavior from:

- telling the bridge to record an MP4 into the bridge volume

Because of that, the bridge-backed services documented here are the integration-supported path for bridge-owned recording.

## Where To Find The Result

Bridge-generated recording metadata and URLs come from the bridge catalog and from the bridge recording APIs.

After starting a recording, the bridge returns clip metadata with the clip ID, current status, and download URL. The integration refreshes its catalog view after the service call so the camera entity can expose the updated recording state.

For the bridge side, see:

- [../../bridge/docs/media-and-recording.md](../../bridge/docs/media-and-recording.md)
- [../../bridge/docs/api-reference.md](../../bridge/docs/api-reference.md)
