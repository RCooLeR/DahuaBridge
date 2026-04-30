# Card Features

This page lists the current Lovelace card feature set.

## 1. Dashboard Surfaces

The workspace currently provides:

- `custom:dahuabridge-surveillance-panel`
- `custom:dahuabridge-surveillance-tile`

The panel is the full command surface. The tile is the compact single-device surface.

## 2. Bridge-Aware URL Handling

The cards can use:

- bridge-generated URLs from the Home Assistant catalog
- a browser-side `browser_bridge_url` override when Home Assistant and the browser reach the bridge differently

This is important for reverse-proxy and split-network deployments.

## 3. Topology And Presentation

The panel can discover and present:

- NVR roots
- NVR channels
- VTO devices
- room grouping from Home Assistant areas
- device metadata and capability state

## 4. Event Workflows

The panel supports:

- bridge event polling
- recent event timeline display
- event window selection
- filtering by room, device kind, severity, and event code

## 5. Archive And Playback Workflows

The panel supports:

- NVR archive search
- selected-camera archive browsing
- inline playback session creation
- playback seek and playback source switching

## 6. Device Actions

Depending on what the bridge exposes for a device, the cards can surface:

- PTZ actions
- aux/light/warning light/siren actions
- audio mute or stream-audio toggles
- bridge clip recording actions
- VTO call, lock, and intercom actions

The cards do not invent capabilities on their own. They render what the bridge and integration already expose.

## Related Docs

- [configuration.md](configuration.md)
- [../../docs/ha-cards.md](../../docs/ha-cards.md)
