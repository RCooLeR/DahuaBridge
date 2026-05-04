# Bridge Documentation

The Go bridge owns device polling, stream catalog generation, archive indexing, media workers, and the HTTP API consumed by Home Assistant and the cards.

Read these first:

1. [Getting Started](getting-started.md)
2. [Configuration](configuration.md)
3. [Media And Recording](media-and-recording.md)
4. [API Reference](api-reference.md)

Reference material:

- [Features](features.md)
- [Device And Stream Model](device-and-stream-model.md)
- [Dahua API Notes](dahua-api.md)
- [Archive Event Extraction](archive-event-extraction.md)

Archive data is intentionally split by responsibility:

- `smd_ivs_events`: indexed SMD/IVS detections plus MP4 export state
- `nvr_recording_chunks`: normal NVR DAV archive chunks
- `bridge_mp4_clips`: bridge-owned MP4 clips and archive exports

New clients should call `/api/v1/nvr/{deviceID}/smd-ivs` for SMD/IVS rows and `/api/v1/nvr/{deviceID}/recording-chunks` for normal chunks. `/api/v1/nvr/{deviceID}/recordings` remains a compatibility endpoint.
