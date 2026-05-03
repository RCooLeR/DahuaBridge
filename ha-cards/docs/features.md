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
It is also the supported fix when the bridge emits `public_base_url` media URLs that the local browser must rewrite to a different reachable base.

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
- filtering by event type and date window

## 5. Archive And Playback Workflows

The panel supports:

- SMD/IVS browsing from `/api/v1/nvr/{deviceID}/smd-ivs`
- normal recording chunk browsing from `/api/v1/nvr/{deviceID}/recording-chunks`
- SMD/IVS type and date filters
- selected-camera archive browsing
- direct native RTSP playback for SMD/IVS rows
- direct native RTSP seek from a date picker and visible time-of-day slider, up to 90 days back
- SMD/IVS MP4 export through the bridge, followed by download when the export clip completes
- direct original DAV download for recording chunks
- manual bridge MP4 clip browsing with download and delete actions
- daily human, vehicle, and IVS counters on overview tiles and in the selected-camera toolbar

The card does not call archive coverage before seek playback. It recreates the native Home Assistant camera player with a Dahua `/cam/playback` RTSP URL.

Browser viewport playback currently uses:

- HLS as the primary stream path
- MJPEG as the fallback path

This is an intentional stability choice for multi-camera dashboards.

For the selected live camera view, the card can also stay on the native Home Assistant camera element while overriding the selected main/sub stream source.

## 6. Device Actions

Depending on what the bridge exposes for a device, the cards can surface:

- PTZ actions
- aux/light/warning light/siren actions
- browser-local stream audio toggles
- bridge clip recording actions
- VTO call, lock, and intercom actions

The cards do not invent capabilities on their own. They render what the bridge and integration already expose.

Controls without a real Home Assistant entity or bridge action URL are hidden in overview, tile, and selected-device views rather than shown as dead buttons.

## Related Docs

- [configuration.md](configuration.md)
- [../../docs/ha-cards.md](../../docs/ha-cards.md)
