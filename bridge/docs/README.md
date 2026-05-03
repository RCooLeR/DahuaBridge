# Bridge Documentation

This section documents the Go bridge.

Read in this order:

1. [Getting Started](getting-started.md)
2. [Configuration](configuration.md)
3. [Features](features.md)
4. [Media And Recording](media-and-recording.md)
5. [API Reference](api-reference.md)
6. [Dahua API Notes](dahua-api.md)
7. [Device And Stream Model](device-and-stream-model.md)

The current archive model is split:

- `smd_ivs_events`: SMD/IVS detections and MP4 backup state
- `nvr_recording_chunks`: normal NVR DAV chunks
- `bridge_mp4_clips`: bridge-owned MP4 archive/export clips

New clients should call `/smd-ivs` and `/recording-chunks` instead of relying on the compatibility `/recordings` endpoint.
