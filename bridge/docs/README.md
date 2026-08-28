# Bridge Documentation

The Go bridge talks to Dahua devices, builds the stream/catalog model, indexes
NVR archive data, runs media workers, and exposes the HTTP API consumed by Home
Assistant and the cards.

For setup or operations, read in this order:

1. [Getting Started](getting-started.md): first run, Docker mounts, and smoke checks.
2. [Configuration](configuration.md): every active YAML section and the security/media/archive knobs.
3. [Media And Recording](media-and-recording.md): stream profiles, HLS/WebRTC/MJPEG workers, MP4 clips, and archive exports.
4. [API Reference](api-reference.md): routes, auth behavior, and archive query contracts.

Reference pages:

- [Features](features.md)
- [Device And Stream Model](device-and-stream-model.md)
- [Archive Event Extraction](archive-event-extraction.md)

## Operational Model

Configuration is loaded at bridge startup. After changing `config.yaml`, rebuild
or restart the bridge container/process before expecting media presets, archive
filters, auth, or proxy settings to take effect.

Use `home_assistant.public_base_url` for the exact URL browsers and Home
Assistant open. When a reverse proxy mounts the bridge under a prefix such as
`/dahua-bridge`, include that prefix in `public_base_url` and make sure the
proxy strips it before forwarding to the bridge.

Bridge-level API auth is optional. If enabled with `http.auth_token` or
`http.auth_token_env`, configure the same token in the Home Assistant integration
so API calls use bearer auth and browser-facing media URLs receive a query token.

For lower bridge-hosted HLS latency, tune `media.input_preset`,
`media.hls_segment_time`, and `media.hls_list_size`. More aggressive settings
reduce delay but increase worker churn and short-lived segment writes.

Archive data is intentionally split by responsibility:

- `smd_ivs_events`: indexed SMD/IVS detections plus MP4 export state
- `nvr_recording_chunks`: normal NVR DAV archive chunks
- `bridge_mp4_clips`: bridge-owned MP4 clips and archive exports

New clients should call `/api/v1/nvr/{deviceID}/smd-ivs` for SMD/IVS rows and
`/api/v1/nvr/{deviceID}/recording-chunks` for normal chunks.
`/api/v1/nvr/{deviceID}/recordings` remains a compatibility endpoint.
