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

Home Assistant's native camera player uses the profile configured in the
integration. Selecting a different profile in the card uses that profile's
bridge HLS/DASH/MJPEG endpoint and displays the actual playback format. It does
not change the shared camera entity's profile for other viewers.

For an NVR channel, **Settings → Preferred live source** offers **Use integration
default (NVR/Camera)**, **NVR**, and **Camera**. The default follows the global
preference in the Home Assistant integration options; explicit NVR/Camera choices
are saved as per-camera overrides. Changing the global default preserves these
overrides. The setting applies to bridge RTSP and bridge media outputs and uses the authenticated
Home Assistant `dahuabridge.set_live_source` service. The Camera preference can
be saved without direct camera credentials. The bridge chooses the alternate
source if the preferred route fails, and the card displays its effective source
and fallback reason.
The existing camera entity, video profile, and output protocol remain usable.

Home Assistant receives one stable bridge URL per profile. The bridge owns
camera/NVR health checks and failover; the card does not select or poll alternate
upstream URLs. Changing effective-source metadata does not restart an unchanged
player URL. Missing or restored Home Assistant entities are not rendered as
native cameras during integration reloads.

Archive playback uses bridge sessions backed by the recorder. Native recorder
RTSP derivation is retained only for older catalogs; a bridge live RTSP URL is
never converted into a Dahua archive URL. Older bridges without source settings
display NVR with a disabled selector until updated.

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
- play an event through a bridge archive playback session
- download an existing bridge MP4 asset from `asset_download_url`
- ask the bridge to create an MP4 from `export_url`, then download it

The selected camera viewport also has an archive seek control. Moving the seek
slider creates a bridge playback window for the selected NVR channel.
New playback choices supersede pending requests. Returning live, changing camera,
or closing the card cancels pending playback and export polling. Pausing a player
persists through HA updates, incoming media buffers, and volume changes.

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
Clips include source audio when the bridge source stream advertises audio.

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
VTO still previews use the direct VTO snapshot endpoint first, then fall back to
the bundled bridge logo if the browser cannot load the snapshot.

## Removed From HA Cards

These workflows are intentionally not present:

- archive coverage from `bridge_archive_coverage_url`
- coverage timeline UI
- card-side MP4 delete actions
