# Archive SMD/IVS Extraction

This page documents the current archive extraction flow.

## Goal

The bridge keeps SMD/IVS detections separate from normal NVR recording chunks:

- SMD/IVS rows are used for event Play and MP4 export.
- Recording chunks are used for direct original DAV download and optional MP4 export.

API reads come from SQLite. The UI does not call the NVR when listing SMD/IVS or recording chunks.

## Tables

### `smd_ivs_events`

Stores one row per SMD/IVS detection:

- `event_id`
- `device_id`
- `channel`
- `event_type`
- `start_time`
- `end_time`
- `rtsp_main_url`
- `rtsp_sub_url`
- `source_file_path`
- `mp4_clip_id`
- `mp4_file_path`
- `mp4_status`
- `mp4_error`

### `nvr_recording_chunks`

Stores normal recorder DAV chunks:

- `chunk_id`
- `device_id`
- `channel`
- `start_time`
- `end_time`
- `file_path`
- NVR file metadata

### `bridge_mp4_clips`

Stores bridge-owned MP4 export metadata:

- `clip_id`
- `device_id`
- `channel`
- `stream_id`
- `start_time`
- `end_time`
- `file_path`
- `status`
- `error_text`

The current schema is recreated without migration when the schema version changes.

## NVR APIs Used

Normal chunk search uses Dahua recorder file search:

- primary: RPC2 `mediaFileFind.factory.create/findFile/findNextFile/close/destroy`
- fallback: CGI `mediaFileFind.cgi`

SMD/IVS search uses event-specific Dahua flows:

- SMD finder for human, vehicle, and animal detections
- event/log search paths for IVS tripwire and intrusion windows where available

Video playback/export uses Dahua RTSP playback:

```text
/cam/playback?channel=<channel>&subtype=<subtype>&starttime=YYYY_MM_DD_HH_mm_ss&endtime=YYYY_MM_DD_HH_mm_ss
```

The query order is significant and must remain `channel`, `subtype`, `starttime`, optional `endtime`.

Original DAV download for recording chunks uses the NVR file path through the bridge download endpoint.

## Background Jobs

The archive service has two logical sync flows:

- chunk sync fills `nvr_recording_chunks`
- SMD/IVS sync fills `smd_ivs_events` and can queue missing MP4 backups

SMD/IVS MP4 is for export/download backup. Card playback uses direct native RTSP playback.

## Bridge APIs

- `GET /api/v1/nvr/{deviceID}/smd-ivs`
- `GET /api/v1/nvr/{deviceID}/recording-chunks`
- `POST /api/v1/nvr/{deviceID}/recordings/export`
- `GET /api/v1/nvr/{deviceID}/recordings/download`
- `GET /api/v1/media/recordings`

See [api-reference.md](api-reference.md) for request and response details.
