# Media And Recording

This file describes the current media and archive model.

## Profiles

The bridge publishes two user-facing profiles:

- `quality`: main stream, normally `subtype=0`
- `stable`: low-bandwidth stream, normally `subtype=1`

Legacy names such as `default` and `substream` are accepted and normalized, but new code should use `quality` and `stable`.

## Live Source

The Home Assistant integration options set the bridge's default **Live source** preference. Each NVR camera can inherit that default or set an override in the bridge admin stream card and the surveillance card camera inspector:

- **Default** (per-camera): inherit the bridge default.
- **NVR** (initial bridge default): prefer the recorder's live channel.
- **Camera**: prefer the camera's RTSP stream using the channel's existing `direct_ipc_credentials` and discovered RTSP port (554 if absent).

The choice is saved in the bridge state store and survives restarts and device probes. It does not require a YAML edit or a duplicate IPC device. The state store must be enabled and writable. The selector changes the upstream device independently of the quality/stable profile and the browser output format.

Direct mode requires the bridge to reach the camera. Home Assistant connects to one stable bridge RTSP relay URL per live profile; the bridge makes all upstream decisions. Ordinary IPC streams use the camera's channel 1, while NVR channel identity, controls, events, and recorded playback remain attached to the recorder. Multi-lens devices can set `direct_ipc_channel` in each channel's credential entry to its verified 1-based camera input (for example, two NVR channels can share an address and login while selecting camera inputs 1 and 2). Omitted or zero values retain input 1. HTTP/ONVIF port 80 is independent of the RTSP video port.

If the preferred stream fails, the bridge validates the alternate route and switches only when all its advertised profiles accept authenticated RTSP DESCRIBE. The same behavior applies in either direction. Missing camera credentials leave Camera saved as the preference while the bridge uses the NVR and reports the reason. The health loop checks every 15 seconds with at most four concurrent workers and waits 60 seconds before retrying a failed route. Terminal media failures also trigger alternate validation. If both routes fail, the bridge keeps its current selection; it returns to the preferred route after recovery.

Source changes reconnect affected live viewers through the same public bridge URL. HLS, DASH, MJPEG, and WebRTC remain independent output choices. Other cameras, historical playback sessions, and running clip recordings continue. The catalog reports the preference, effective source, and fallback reason; Home Assistant does not receive a list of upstream alternatives to choose from.

The private resolver and operator stream catalog retain `recorder_stream_url`
for historical playback. The Home Assistant catalog omits upstream URLs and
credentials; archive viewing uses existing bridge playback sessions.

## Live Outputs

The bridge can expose stream-backed:

- snapshots
- RTSP relay (TCP, no re-encoding, shared upstream per profile)
- MJPEG
- HLS
- DASH
- WebRTC helper pages and answers
- HTML previews

These outputs are generated from the stream catalog. The bridge does not toggle camera or NVR audio settings for normal viewing; browser playback audio is controlled by the player.

The RTSP listener defaults to port `8554`. Each profile has one stable path:
`/api/v1/rtsp/live/{streamID}/{quality|stable}`. HA uses the same bridge host as
its API connection. Token-protected bridges require RTSP authentication using
username `dahuabridge` and the bridge API token. Camera credentials stay private.
By default, shared upstreams stop after the configured idle timeout when no
viewers remain. The integration's live connection warm-up option can extend this.

Live HLS, DASH, MJPEG, and WebRTC workers also consume this shared relay over a
container-local connection. Native HA viewers and browser formats share one
camera/NVR input per profile. Output encoders remain separate when their formats,
scaling, or audio requirements differ. Historical playback and finite clip
recordings keep their own original inputs. In a shared Docker network, HA keeps
its integration URL at `http://dahua-bridge:9020`; RTSP uses the same container
hostname on port 8554 without requiring a NAS port mapping.

Completed archive HLS/DASH output is retained for `hls_keep_after_exit`, but no
longer occupies an active worker slot. Retained outputs have a separate count
limit equal to `max_workers`; the oldest completed output is evicted first.
Shutdown cancels and joins media tasks and removes their temporary output.
Unexpectedly finished live streams are restarted rather than cached as recordings.

The media-worker API and admin page include RTSP workers with effective source,
reader count, received bytes, measured bitrate, and reopen count. FFmpeg workers
report shared-input usage and, on Linux, process memory and CPU usage between
diagnostic samples (100% is one CPU core). CPU is unavailable until two samples
exist. Prometheus exposes `dahuabridge_rtsp_*` metrics alongside existing device
request counters; reopen history is bounded and includes idle reopen as well as
failover. Retained output is marked `state: retained`.

## Live Connection Warm-Up

Home Assistant integration options expose a bridge-owned warm-up policy:

- **On demand** (`off`, initial default): open an input when a viewer requests it,
  then use the configured media idle timeout (normally 30 seconds).
- **Keep recently viewed streams warm** (`recent`): keep each viewed input
  connected for five minutes after the last viewer leaves. Unused cameras remain
  on demand, so their first opening still needs connection setup.
- **Preconnect selected video profile** (`always`): connect the selected profile
  for each live camera in advance and keep it running without viewers. The
  existing preferred video profile is saved with this policy; Auto follows the
  bridge recommendation. Changing profiles in a camera inspector can still open
  an input that was not preconnected.

The policy lives in the bridge state store and survives restarts. No YAML edit,
extra HA camera entity, upstream URL, or published port is needed. The relay uses
the same camera/NVR selection and fallback as normal viewing. Continuous mode
therefore receives traffic from the NVR when it is the selected effective source.

Warm-up receives live RTP continuously but does not start an encoder or a
recording. One upstream is shared with real viewers. Warm-only connections count
toward the existing stream limit; viewer requests have priority over idle warm
inputs. Background starts and retries are bounded. Turning the policy off stops
unused preconnections and returns viewed inputs to normal idle cleanup; active
viewers stay connected. The worker API reports `warm` and `preconnect_mode` for
diagnosis.

Preconnecting removes camera connection/authentication/RTSP setup delay. A new
decoder may still wait for the next keyframe; HA buffering and cold bridge
HLS/DASH encoders remain separate startup costs. Home Assistant's per-camera
[Preload stream](https://www.home-assistant.io/integrations/camera/#streaming-video)
also keeps HA's playback processing running, reducing its frontend startup delay
at the cost of continuous HA resource use. It is independent of this bridge policy.

## Bridge MP4 Clips

Bridge-owned MP4 clips are separate from the NVR archive. They are stored under `media.clip_path` and are controlled through:

- `POST /api/v1/media/streams/{streamID}/recordings`
- `GET /api/v1/media/recordings`
- `GET /api/v1/media/recordings/{clipID}`
- `GET /api/v1/media/recordings/{clipID}/play`
- `GET /api/v1/media/recordings/{clipID}/download`
- `POST /api/v1/media/recordings/{clipID}/stop`
- `DELETE /api/v1/media/recordings/{clipID}`

The archive database has a separate `bridge_mp4_clips` table for MP4 assets produced from SMD/IVS exports.

MP4 clips and archive exports include audio when the selected source stream advertises an audio track. If a source has no audio metadata, the bridge records video only rather than mutating device audio settings.

Unfiltered H.264 archive exports can remux video into MP4 without reencoding.
Accurate seeks, iframe prefixes, wall-clock timestamp conversion, unknown codecs,
and H.265 retain the encoder path. Failed remux/output validation retries with
encoding. A user stop cancels all attempts and finalizes a shorter clip; shutdown
allows FFmpeg to quit before its five-second forced-exit deadline.

## Archive Tables

The archive service intentionally keeps SMD/IVS and normal recorder chunks separate.

### `smd_ivs_events`

Stores SMD/IVS detections:

- device ID
- channel
- event type
- start and end time
- generated main/sub RTSP playback URLs for export and diagnostics
- source DAV path when the NVR reports one
- MP4 backup clip ID, file path, status, and error

The card uses these rows for the SMD/IVS list. Play uses a bridge playback session
against the NVR. Export creates or reuses an MP4 clip.
HTTP list responses redact the raw RTSP playback URLs by default. Use `include_credentials=true` only for operator diagnostics.

### `nvr_recording_chunks`

Stores normal NVR recording chunks:

- device ID
- channel
- start and end time
- original DAV `file_path`
- NVR file metadata and flags

The card uses these rows for the recordings list. Chunks expose raw DAV download when `file_path` is available.

### `bridge_mp4_clips`

Stores archive MP4 export metadata owned by the bridge. SMD/IVS rows point at these clips when export backup exists.

The bridge drops and recreates the archive schema for this model; no migration is kept for the old mixed tables.

## Background Archive Jobs

There are two archive indexing flows:

- recording chunk sync fills `nvr_recording_chunks`
- SMD/IVS sync fills `smd_ivs_events` and queues MP4 backup export when enabled

Recorder searches consume cursor pages to completion, including short pages and
records sharing timestamps. Public list limits do not truncate the index. Each
scan window has a two-minute deadline and a 100,000-row bound; an incomplete or
stalled scan returns an error and its checkpoint remains unchanged. An SMD reply
claiming results without a valid cursor is an error, rather than an empty scan.

SMD/IVS polling runs every five minutes. SQLite persists a separate recent cursor
and historical cursor for each device, channel, and event type in
`archive_event_scan_progress`. Recent scans overlap by 15 minutes to pick up late
events. Each pass also scans at most two older hourly windows per event type and
channel until `prefetch_days` is covered. Restart resumes those cursors; a long
outage prioritizes the newest hour and fills the missed range through backfill.
Events reported more than 15 minutes late after backfill has passed their window
are outside the routine overlap. Recording-chunk indexing retains its configured
cron schedule. The checkpoint table is added without replacing existing version-2
event, recording, or clip tables.

Archive shutdown cancels and joins scheduled and manual syncs before closing
SQLite. Recording-search responses use a cache capped at 128 distinct queries,
with five-second expiry.

Automatic event MP4 export waits for `archive.export_delay` after the event ends
before creating the clip. Set `archive.export_event_mp4: false` for DB-only SMD/IVS
history: event rows, counts, summaries, and search remain available, but automatic
background MP4 backup creation is skipped. Use `archive.export_ivs`,
`archive.export_smd_person`, `archive.export_smd_transport`, and
`archive.export_smd_animal` to limit automatic MP4 creation for noisy event types
while still keeping those events in the index.
For example, `export_smd_transport: [1, 3, 7]` keeps Dahua transport/vehicle event
videos only on channels 1, 3, and 7. Empty channel lists mean all channels.

API reads use SQLite only. SMD/IVS list requests do not call the NVR, and recording chunk list requests do not call the NVR. Event filters are applied in SQLite before `LIMIT`, then checked again in Go against the normalized Dahua event names.

## Native Historical Playback

Current catalogs keep recorder credentials inside the bridge. The cards create
bridge playback sessions for archive seek and events. The bridge builds the
following NVR URLs internally. Legacy catalogs with direct recorder URLs remain
supported by older native playback paths. No coverage lookup is required for seek.

Event playback uses the selected SMD/IVS row:

```text
rtsp://user:pass@host:554/cam/playback?channel=1&subtype=0&starttime=2026_05_03_14_11_22&endtime=2026_05_03_14_11_48
```

Seek playback uses the selected date and second-of-day:

```text
rtsp://user:pass@host:554/cam/playback?channel=9&subtype=0&starttime=2026_05_01_09_59_30
```

The required Dahua query order is:

1. `channel`
2. `subtype`
3. `starttime`
4. `endtime` when an end time is known

## Export Paths

SMD/IVS export:

1. read row from `smd_ivs_events`
2. build/play the NVR archive RTSP URL
3. record the finite window with FFmpeg
4. store MP4 metadata in `bridge_mp4_clips`
5. update the SMD/IVS row with MP4 backup fields

Recording chunk export:

1. read row from `nvr_recording_chunks`
2. use the original DAV path when available
3. transcode to MP4 through the bridge media layer

Raw DAV download is for recording chunks only.

Exports reuse a running job or a completed, nonempty output file. Failed exports,
missing files, and orphaned recording metadata are retried. Background asset
reconciliation checks active jobs first and rotates through completed files in
bounded batches. Reading or reconciling an MP4 does not extend event retention.

SQLite also keeps `archive_clip_retention` ownership for every archive export
attempt, including failed attempts replaced by a retry. Its retention clock uses
the original source window, so retries do not extend it. After retention expires
and no retained event references an export, cleanup processes at most 128 clips
per pass. Pending deletions survive restarts and filesystem errors; active jobs
are skipped and failed attempts rotate behind other pending work. Metadata is
removed only after file deletion succeeds. Missing clip JSON does not strand a
partial MP4: cleanup verifies its retained stream ID and exact filename before
removing it. Manual live recordings are outside automatic archive ownership,
even when their time window happens to match an event.
