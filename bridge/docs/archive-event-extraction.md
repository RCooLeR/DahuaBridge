# Archive Event Extraction

This document describes the bridge archive flow for SMD/IVS events end to end.

## Scope

In bridge terminology:

- `recordings` means recorder-backed archive rows from the NVR
- `events` means SMD/IVS archive event rows, also coming from the NVR archive search
- `/api/v1/events` is a separate live/recent event buffer and is not the source of truth for archive event video

The archive event video path is built from `/api/v1/nvr/{deviceID}/recordings?...event_only=true`.

## Source of Truth

For SMD/IVS video, the bridge searches the recorder archive directly:

- `GET /api/v1/nvr/{deviceID}/recordings?...event_only=true&event=<code>`

Each returned event row may contain:

- event start/end time
- channel
- event type / flags
- `file_path` pointing at the backing recorder DAV file

That `file_path` is the canonical media locator.

## What the Bridge Downloads

For an archive event clip, the bridge does this:

1. Search archive event rows from the NVR.
2. Read the event row `file_path`.
3. Download the full recorder DAV file with:
   - `RPC_Loadfile/<file_path>`
4. Try to fetch the recorder iframe with:
   - `RPC3_Loadfile`
   - `StorageAssistant.getIFrameData`
5. Trim the event window from the downloaded DAV according to the event start/end time.

The bridge does not trust `/api/v1/events` for archive video extraction.

## Time Mapping

Event rows usually point into a larger recorder file, commonly a 30-minute DAV.

The bridge parses the recorder file window from the DAV file name, for example:

- `20.00.00-20.30.00[R][0@0][0].dav`

That gives the recorder file start/end time.

Then:

- event trim start = `event_start - file_start`
- event trim duration = `event_end - event_start`

This is the authoritative trim window used for event MP4 extraction.

## IFrame Handling

Iframe data is optional.

If RPC admin credentials are configured and the recorder returns iframe data:

- the bridge downloads the iframe DAV
- prefixes it to the trimmed event clip path

If iframe download fails, decode fails, or FFmpeg fails with the prefixed path:

- the bridge retries automatically without the iframe
- the event clip still extracts from the full DAV trim path

This is intentional because some recorders can return iframe payloads from the wrong stream/camera or with incompatible dimensions.

## Transcoding Isolation

Every archive event export/prefetch job is isolated by archive record identity:

- device
- channel
- event start/end
- `file_path`
- source/type/video stream

The bridge generates a unique export stream ID from that identity.

This prevents one event extraction from blocking another event or the parent 30-minute file.

Temporary DAV files are handed off to the media job and are only cleaned up by the job lifecycle, not by the request path after FFmpeg has started.

## Playback vs Download

There are two separate archive behaviors:

### Event rows

- Playback uses prefetched/exported bridge MP4 clips.
- Download can use:
  - bridge MP4 asset download
  - raw recorder DAV download when available

### Recording rows

- Playback uses NVR playback sessions over archive RTSP with seek.
- Download uses the original recorder DAV file.

Recording rows are not supposed to reuse short event MP4 assets.

## Background Prefetch

When `archive.enabled` is on, the archive service:

1. indexes archive file rows
2. indexes event rows for SMD/IVS codes
3. prefetches missing SMD/IVS event MP4 clips for the configured retention window

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
- includes event start/end time, type, flags, `file_path`

### `archive_event_files`

- links event rows to recorder file rows

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
- which recorder DAV file produced it

## Failure Behavior

The bridge intentionally degrades in this order:

1. full DAV + iframe + trim
2. full DAV + trim without iframe
3. expose failure state in SQLite/API if extraction still fails

It should never require `/api/v1/events` to recover archive event video.

## Expected Result

With current behavior, the correct archive event flow is:

1. archive search returns SMD/IVS event row
2. bridge resolves backing DAV from `file_path`
3. bridge downloads the correct full DAV
4. bridge optionally downloads iframe data
5. bridge trims only the event time window
6. bridge stores the resulting MP4 and job metadata in SQLite
7. later API/card requests reuse that stored asset instead of rebuilding it
