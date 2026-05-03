# HA Cards Architecture

This page documents the current frontend structure and the browser media policy.

## Player Policy

Viewport playback in the cards is intentionally limited to:

- HLS for primary browser playback
- MJPEG as the fallback path

WebRTC is not used for camera, VTO, or archive viewport playback in the cards.

Reason:

- the bridge/browser path has been more stable with HLS
- WebRTC introduced more reconnect and decode churn than it solved for dashboard use
- the main panel must support dense multi-camera layouts, where transport stability matters more than protocol variety

`hls.js` is bundled into the card build. When the runtime checks `Hls.isSupported()`, it is checking browser Media Source Extensions support, not whether the library was loaded.

If the browser supports native HLS, the card can attach the playlist directly to the video element. Otherwise it uses `hls.js`. If neither path is available, the card falls back to MJPEG or snapshot behavior.

Archive search is resolved from either:

- archive feature metadata exposed by the bridge integration
- integration archive URL attributes such as `bridge_archive_recordings_url_template`, `bridge_playback_sessions_url`, and `bridge_archive_coverage_url`

Template-style archive search URLs are normalized in the card runtime before query parameters are appended.

For the selected live camera view, `surveillance-panel-media.ts` can keep the Home Assistant native camera element while overriding `stream_source`, snapshot URLs, and `subtype`-specific query params so main/sub stream selection actually follows the selected profile.

Archive seek is rendered by `surveillance-panel-archive-seek.ts`, which wraps `nouislider` and consumes bridge archive coverage chunks instead of relying on a plain browser range input.

## Module Layout

The main frontend modules are split by responsibility:

- `src/cards/surveillance-panel-card.ts`
  - panel state orchestration
  - selection transitions
  - archive and playback actions
- `src/cards/surveillance-panel-overview.ts`
  - overview grid rendering
- `src/cards/surveillance-panel-sidebar.ts`
  - sidebar rendering and discovery lists
- `src/cards/surveillance-panel-inspector*.ts`
  - device-specific inspector rendering
- `src/cards/surveillance-panel-media.ts`
  - viewport composition and stream styling hooks
- `src/cards/surveillance-panel-viewport-sources.ts`
  - stream/profile/source selection logic
- `src/cards/surveillance-remote-stream.ts`
  - browser video attach lifecycle and HLS/MJPEG fallback
- `src/cards/surveillance-tile-card.ts`
  - compact single-device card

## Review Boundaries

When refactoring, prefer these seams:

- split render-only modules out of `surveillance-panel-card.ts`
- keep bridge request logic in action/runtime helpers instead of embedding fetch flows in render modules
- keep source-selection policy in `surveillance-panel-viewport-sources.ts`
- keep transport attach/recovery logic in `surveillance-remote-stream.ts`

Avoid:

- reintroducing a second DOM-level playback lifecycle outside `dahuabridge-remote-stream`
- mixing player transport policy with inspector/sidebar rendering
- exposing frontend source choices the browser path does not support reliably
