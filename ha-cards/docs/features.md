# Card Features

The bundle registers two Lovelace cards:

- `custom:dahuabridge-surveillance-panel`
- `custom:dahuabridge-surveillance-tile`

## Languages

The cards render their built-in UI text in English or Ukrainian. They use the
resolved integration language exposed on DahuaBridge camera attributes, so the
same integration option controls entity labels and card labels.

The cards fall back in this order:

- `bridge_integration_language` or `integration_language` from any DahuaBridge camera entity
- Home Assistant frontend language when it is `en` or `uk`
- English

## Live Streams

The panel shows live camera and VTO streams in the overview grid and selected
device viewport. The tile shows one selected camera or VTO.

Supported live paths are the stream sources exposed by the integration:

- HLS
- DASH
- MJPEG
- native Home Assistant camera view when available
- snapshots as a visual fallback

The selected stream profile and source are browser-side UI choices. They do not
create archive playback sessions.

## Events And Summaries

The panel shows bridge events from the configured lookback window and can poll
for updates. It also shows daily event summary counters for cameras/NVR channels
when the bridge exposes summary endpoints.

Visible event workflows:

- recent event list
- history window selection
- event type filtering
- daily human, vehicle, and IVS counters

## SMD/IVS Archive

The Events tab queries `bridge_archive_smd_ivs_url_template`.

Visible SMD/IVS workflows:

- list events for the selected date
- filter by SMD/IVS type
- play an event through direct RTSP archive playback via the Home Assistant timeframe proxy
- download an existing bridge MP4 asset from `asset_download_url`
- ask the bridge to create an MP4 from `export_url`, then download it

The selected camera viewport also has an archive seek control. Moving the seek
slider builds a Dahua `/cam/playback` RTSP window and plays it through the
Home Assistant camera timeframe proxy.

## Recording Chunks

The Recordings tab queries `bridge_archive_recording_chunks_url_template`.
If that attribute is missing, `bridge_archive_recordings_url_template` is used
only as a compatibility fallback.

Visible chunk workflow:

- list 30-minute recording chunks for the selected date
- download a chunk from row-level `download_url`

Recording chunks are download-only in the card. There is no chunk playback or
coverage timeline.

## Manual MP4 Clips

The MP4 tab lists bridge MP4 clips created with the card recording button.

Visible MP4 workflows:

- start or stop a bridge MP4 recording when the integration exposes recording action URLs
- list MP4 clips for the selected date
- play completed clips in the selected camera viewport
- download completed clips

The card does not delete MP4 clips.

## Device Controls

Depending on the capabilities exposed by the integration, the cards can show:

- PTZ controls
- aux, light, warning light, or siren actions
- browser-local stream mute and volume controls
- VTO call, lock, intercom, and auto-record controls

Controls without a real entity or bridge action URL are hidden.
VTO device mute and VTO speaker/microphone volume controls are not shown.

## Removed From HA Cards

These workflows are intentionally not present:

- playback sessions from `bridge_playback_sessions_url`
- archive coverage from `bridge_archive_coverage_url`
- coverage timeline UI
- playback profile selection for archive sessions
- card-side MP4 delete actions
