# HA Cards Architecture

The HA cards are a browser UI layer on top of the DahuaBridge integration. They
do not own camera discovery, recording creation, archive indexing, or media file
management.

## Data Flow

1. Home Assistant provides entities and attributes created by the integration.
2. `src/domain/devices.ts` reads entity attributes and bridge feature metadata.
3. `src/domain/model.ts` builds the panel view model and rewrites bridge URLs
   with `browser_bridge_url` when configured.
4. `src/cards/*` renders the panel, tile, live viewport, archive lists, and controls.
5. `src/ha/*` contains browser requests to bridge endpoints.

## Live Media

`src/cards/surveillance-panel-media.ts` chooses a live source for the selected
camera or VTO. `src/cards/surveillance-remote-stream.ts` attaches HLS, DASH, or
MJPEG sources and falls back through the ordered source list when a source fails.

Native Home Assistant camera rendering is only used for live camera/VTO views
when that source is available. Archive playback does not use the native camera
element.

## Archive Media

Archive support is split by the two lists the card still shows:

- SMD/IVS events from `bridge_archive_smd_ivs_url_template`
- 30-minute recording chunks from `bridge_archive_recording_chunks_url_template`

`src/domain/archive.ts` models only those two archive capabilities.
`src/ha/bridge-archive.ts` fetches list responses, maps row-level URLs, exports
bridge MP4 clips when requested, and lists manual MP4 clips.

Archive state is split into small card-side models:

- SMD/IVS list source selection lives in `surveillance-panel-smd-ivs-model.ts`.
- 30-minute chunk list source selection lives in `surveillance-panel-chunks-model.ts`.
- Shared archive list routing helpers live in `surveillance-panel-archive-state-model.ts`.
- Direct RTSP seek date/time math and timeframe proxy path building live in
  `surveillance-panel-archive-seek-model.ts`.
- The seek UI element lives in `surveillance-panel-archive-seek.ts`.
- Manual MP4 list playback/download state lives in `surveillance-panel-mp4-model.ts`.
- Native archive playback state/profile selection lives in
  `surveillance-panel-native-playback-model.ts`.
- Selected camera live stream source selection lives in
  `surveillance-panel-live-stream-model.ts`.

The panel card now coordinates those modules, owns the active Home Assistant
connection, and dispatches bridge requests. SMD/IVS results and recording chunk
results are held in separate state slots so one list cannot replace the other.

## Module Map

- `src/cards/surveillance-panel-card.ts`: panel coordination, selection, bridge request dispatch
- `src/cards/surveillance-panel-archive.ts`: SMD/IVS, chunk, and MP4 list rendering
- `src/cards/surveillance-panel-smd-ivs-model.ts`: SMD/IVS archive list source model
- `src/cards/surveillance-panel-chunks-model.ts`: recording chunk archive list source model
- `src/cards/surveillance-panel-archive-state-model.ts`: active archive list mode and list state helpers
- `src/cards/surveillance-panel-archive-seek-model.ts`: archive seek date/time and proxy URL helpers
- `src/cards/surveillance-panel-archive-seek.ts`: archive seek UI element
- `src/cards/surveillance-panel-mp4-model.ts`: manual MP4 list playback/download model
- `src/cards/surveillance-panel-native-playback-model.ts`: direct RTSP archive playback model
- `src/cards/surveillance-panel-live-stream-model.ts`: selected camera live stream model
- `src/cards/surveillance-panel-media.ts`: live and MP4 viewport composition
- `src/cards/surveillance-panel-viewport-sources.ts`: live source/profile selection
- `src/cards/surveillance-remote-stream.ts`: browser media attach lifecycle
- `src/cards/surveillance-tile-card.ts`: compact single-device card
- `src/domain/archive.ts`: archive capability and row models
- `src/domain/devices.ts`: Home Assistant entity to device capability mapping
- `src/domain/model.ts`: panel/tile view models
- `src/ha/bridge-archive.ts`: archive list, MP4 export, and MP4 list requests

## Removed Runtime Paths

The card runtime no longer has a playback-session path or archive coverage path.
There is no HA card code that reads `bridge_playback_sessions_url` or
`bridge_archive_coverage_url`.

Keep future archive UI changes aligned with the current ownership split:

- the bridge creates and serves MP4 files
- the integration exposes attributes and action URLs
- the cards list, play, and download only the URLs they are given
