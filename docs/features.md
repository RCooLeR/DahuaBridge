# Feature Map

This page shows which features live in which layer and where to read about them in detail.

## Bridge Features

The bridge owns device communication, state normalization, APIs, and media.

Main feature areas:

- device probing and inventory
- normalized device and stream catalog
- event collection and event APIs
- admin UI and operational endpoints
- snapshots
- live media helpers: MJPEG, HLS, preview pages, WebRTC helpers
- NVR archive search and playback sessions
- bridge-owned clip recording from streams
- VTO intercom/session helpers
- native Home Assistant catalog generation

Read more:

- [bridge/docs/features.md](../bridge/docs/features.md)
- [bridge/docs/media-and-recording.md](../bridge/docs/media-and-recording.md)
- [bridge/docs/api-reference.md](../bridge/docs/api-reference.md)

## Integration Features

The integration owns Home Assistant-facing behavior.

Main feature areas:

- config flow and options flow
- polling the native bridge catalog
- Home Assistant device grouping
- camera entities
- binary sensors
- sensors
- buttons
- numbers
- switches
- diagnostics export
- bridge-backed camera recording services

Read more:

- [integration/docs/features.md](../integration/docs/features.md)
- [integration/docs/entities-and-controls.md](../integration/docs/entities-and-controls.md)
- [integration/docs/camera-recording.md](../integration/docs/camera-recording.md)

## Cards

The repository includes an optional Lovelace card workspace.

Current card surface:

- `custom:dahuabridge-surveillance-panel`
- `custom:dahuabridge-surveillance-tile`
- TypeScript build output under `ha-cards/dist/`
- optional browser-side bridge URL override for deployments where Home Assistant and the browser reach the bridge differently

See:

- [ha-cards.md](ha-cards.md)
- [../ha-cards/README.md](../ha-cards/README.md)
- [../ha-cards/docs/README.md](../ha-cards/docs/README.md)

## Quick Ownership Rules

- Dahua protocol behavior: bridge
- Home Assistant entities and services: integration
- dashboard composition: cards
