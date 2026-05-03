# Archive Event Extraction

This document describes the bridge archive flow for SMD/IVS events end to end.

## Scope

In bridge terminology:

- `recordings` means recorder-backed archive rows from the NVR
- event-backed archive rows such as SMD and IVS are also returned by `GET /api/v1/nvr/{deviceID}/recordings`, typically with `event_only=true`

## Source of Truth

For SMD/IVS video, the bridge searches the recorder archive directly and stores the normalized event rows in SQLite:

- `GET /api/v1/nvr/{deviceID}/recordings?...event_only=true&event=<code>`

Each returned event row may contain:

- event start/end time
- channel
- event type / flags
- optional `file_path`
- bridge-generated `rtsp_main_url`
- bridge-generated `rtsp_sub_url`

The current playback and export path for event rows is the archive RTSP window, not direct DAV download by `file_path`.

## What the Bridge Plays And Exports

For an archive event clip, the bridge does this:

1. Search archive event rows from the NVR.
2. Normalize and store those rows in SQLite.
3. Generate archive RTSP playback URLs for the row:
   - main stream: `/cam/playback?channel=...&subtype=0&starttime=...&endtime=...`
   - sub stream: `/cam/playback?channel=...&subtype=1&starttime=...&endtime=...`
4. When a clip is exported or prefetched, create an archive playback session for that exact event window.
5. Record that playback RTSP stream into a bridge-owned MP4 clip.

The bridge does not rely on any live/recent event buffer for archive video extraction.

## RTSP Constraints

Tested Dahua recorders are sensitive to archive RTSP query parameter order.

The bridge emits:

1. `channel`
2. `subtype`
3. `starttime`
4. optional `endtime`

Using the wrong order can return `404 Not Found` from the recorder.

The current bridge playback path is:

```text
rtsp://<nvr>:554/cam/playback?channel=<1-based-channel>&subtype=<0-or-1>&starttime=YYYY_MM_DD_HH_mm_ss&endtime=YYYY_MM_DD_HH_mm_ss
```

## Transcoding Isolation

Every archive event export/prefetch job is isolated by archive record identity:

- device
- channel
- event start/end
- source/type/video stream

The bridge generates a unique export stream ID from that identity.

This prevents one event extraction from blocking another event window.

## Playback vs Download

There are two separate archive behaviors:

### Event rows

- Playback uses archive playback sessions and bridge-hosted media helpers.
- Export uses archive playback RTSP and produces a bridge-owned MP4 clip.
- Download, when needed, should use the resulting bridge MP4 asset.
- Event rows do not expose raw recorder `download_url`.

### Recording rows

- Playback uses NVR playback sessions over archive RTSP with seek.
- Export can transcode directly from recorder DAV when `file_path` is known.
- Download uses the original recorder DAV file.

## Background Prefetch

When `archive.enabled` is on, the archive service:

1. indexes archive file rows
2. indexes event rows for SMD/IVS codes
3. stores main/sub archive RTSP URLs on those event rows
4. rechecks recent SMD/IVS windows every 5 minutes
5. prefetches missing SMD/IVS event MP4 clips for the configured retention window

Default retention/prefetch window:

- last 7 days

The prefetcher:

- skips rows that already have usable asset state in SQLite
- respects `archive.max_parallel_jobs`
- starts more work on later sync cycles until the backlog is consumed

This means the bridge side eventually builds a full local list of recent SMD/IVS assets instead of redownloading and retranscoding the same event repeatedly.

## SQLite Tracking

The bridge stores archive and transcode state in SQLite:

### `archive_files`

- recorder file rows
- includes `file_path`, channel, start/end time

### `archive_events`

- event rows from archive search
- includes event start/end time, type, flags
- includes persisted `rtsp_main_url` and `rtsp_sub_url`
- may also retain `file_path` when the recorder provided it

### `transcode_jobs`

- one logical job per archive record
- includes:
  - `record_kind`
  - `record_id`
  - `source_file_path`
  - `output_path`
  - `status`
  - timestamps

### `transcoded_assets`

- asset row per produced bridge MP4
- includes:
  - `record_kind`
  - `record_id`
  - local asset path
  - status
  - size
  - ready timestamp

These tables are the bridge-side source of truth for:

- whether an event has already been extracted
- where the local MP4 lives
- which archive event window produced it

## Failure Behavior

The bridge intentionally degrades in this order:

1. archive playback RTSP export
2. persisted failure state in SQLite/API if export still fails

It should never require any live/recent event buffer to recover archive event video.

## Expected Result

With current behavior, the correct archive event flow is:

1. archive search returns SMD/IVS event row
2. bridge stores the row and its archive RTSP URLs in SQLite
3. bridge uses archive playback RTSP for playback/export
4. bridge stores the resulting MP4 and job metadata in SQLite
5. later API/card requests reuse that stored asset instead of rebuilding it

