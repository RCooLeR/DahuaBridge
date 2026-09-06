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

Native Home Assistant rendering uses the integration's configured live profile.
Other profiles use their corresponding bridge HTTP streams. The bridge owns
upstream NVR/camera selection and failover; the cards choose output formats only.

## Archive Media

Archive support is split by the two lists the card still shows:

- SMD/IVS events from `bridge_archive_smd_ivs_url_template`
- 30-minute recording chunks from `bridge_archive_recording_chunks_url_template`

`src/domain/archive.ts` models archive recordings, clips, and playback sessions.
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
- `playback-lifecycle.ts` owns cancellation and generation checks for pending
  playback operations. Downloads have independent operations until the selection
  changes or the card disconnects. Legacy native HA source services are serialized
  per entity so a canceled set cannot overtake the subsequent clear/new seek.
- `src/ha/bridge-resource.ts` resolves API response links against the authenticated
  request. Query authentication follows same-bridge HTTP API links through lists,
  exports, polling, and clip playback; unrelated origins keep their original URL.

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

## Archive Runtime

Relay catalogs use `bridge_playback_sessions_url` for archive viewing. Older
catalogs retain native RTSP archive compatibility. The card does not use
`bridge_archive_coverage_url`.

Keep future archive UI changes aligned with the current ownership split:

- the bridge creates and serves MP4 files
- the integration exposes attributes and action URLs
- the cards list, play, and download only the URLs they are given

## Loading and Validation

The entry module registers both cards. HLS, DASH, and the editor are dynamic
imports. Deployment copies the entire `dist/` tree. The build checks the static
dependency graph for eager player imports and enforces a 175 kB gzip JavaScript
budget. Unit and component lifecycle tests cover async cancellation and pause
intent; a separate Chromium smoke test verifies production module loading.
