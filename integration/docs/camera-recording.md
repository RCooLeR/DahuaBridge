# Camera Recording And Archive Access

The integration exposes two different recording-related workflows:

- live clip capture owned by the bridge
- NVR archive playback/export owned by the bridge

Neither workflow changes long-running NVR circular recording configuration.

## Snapshot Flow

When Home Assistant requests a still image:

1. The camera checks capture metadata for `snapshot_url`.
2. If present, it requests that bridge URL.
3. If the snapshot request fails or no snapshot URL exists, it falls back to extracting one JPEG frame from the preferred MJPEG URL.
4. If neither path works, it returns the bundled DahuaBridge placeholder image.

## Live Clip Capture

The camera platform registers two entity services:

- `start_recording`
- `stop_recording`

These services call URLs advertised in the camera record's `stream.capture` section.

`start_recording` accepts:

| Field | Meaning |
| --- | --- |
| `profile` | Optional bridge stream profile. |
| `duration_seconds` | Optional automatic stop duration. |

`start_recording` sends JSON to `start_recording_url`. `stop_recording` posts to `stop_recording_url`. After either call, the integration requests a catalog refresh.

Camera attributes related to live capture:

- `bridge_capture`
- `bridge_recording_active`
- `bridge_start_recording_url`
- `bridge_stop_recording_url`
- `bridge_recordings_url`

## Archive Playback And Export

For NVR channel cameras, the integration exposes archive URLs as attributes:

- `bridge_archive_smd_ivs_url_template`
- `bridge_archive_recording_chunks_url_template`
- `bridge_archive_recordings_url_template`
- `bridge_archive_export_url`
- `bridge_playback_sessions_url`
- `bridge_archive_coverage_url`

Use these for:

- normal recorder timeline search
- SMD/IVS event search
- playback session creation
- MJPEG/HLS/WebRTC playback through the bridge
- MP4 export through the bridge
- coverage-aware archive seeking

For event-backed archive rows such as SMD and IVS, the bridge uses archive playback/export paths rather than direct recorder DAV download.

## Timeframe Proxy

The integration registers:

```text
/api/camera_proxy/{entity_id}/timeframe
/api/camera_proxy/{entity_id}/timeframe/
```

This endpoint accepts Dahua-style timeframe query parameters such as:

```text
starttime=2026_05_03_16_11_05
endtime=2026_05_03_16_41_05
```

Flow:

1. Resolve the Home Assistant camera entity.
2. Resolve its DahuaBridge coordinator from camera attributes.
3. Build a playback session request using `bridge_channel` and the requested time window.
4. Select an MJPEG playback profile from the bridge response.
5. Stream the bridge MJPEG response back to Home Assistant.

Naive Dahua-style timestamps are sent to the bridge as NVR wall-clock time. ISO timestamps with a timezone are converted to UTC ISO strings for playback requests.

The integration does not build Dahua RTSP URLs itself. The selected bridge MJPEG endpoint resolves the playback session inside the bridge media layer, and that bridge path requests a credentialed RTSP input from the runtime. The bridge RTSP builder keeps Dahua's required query order:

```text
rtsp://user:pass@host:554/cam/playback?channel=5&subtype=0&starttime=2026_05_04_04_30_00&endtime=2026_05_04_05_00_00
```

The endpoint accepts the same authentication styles as the camera proxy path:

- a Home Assistant signed path using `authSig`
- the camera entity access token using `token`

Signed `authSig` URLs are short-lived and include the exact path plus non-safe query parameters in the signature. If you add, remove, or reorder query parameters after signing, generate a new signed URL.

## Boundaries

The integration does not:

- toggle NVR recording schedules
- replace the NVR circular recording policy
- make Home Assistant own bridge MP4 files
- download recorder DAV files directly
- mutate NVR audio settings for export

The bridge owns capture, transcode, export, and playback behavior.

## Bridge Docs

- [Bridge media and recording](../../bridge/docs/media-and-recording.md)
- [Bridge API reference](../../bridge/docs/api-reference.md)
