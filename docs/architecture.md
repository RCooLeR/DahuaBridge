# Architecture

This repository is split into three layers with different responsibilities.

## 1. Bridge

The Go bridge in `bridge/` is the Dahua-facing backend.

It is responsible for:

- connecting to Dahua devices
- probing identity, inventory, channels, locks, alarms, and stream metadata
- normalizing live device state and events
- exposing HTTP APIs
- generating a Home Assistant native catalog
- serving bridge-hosted media such as MJPEG, HLS, WebRTC helpers, snapshots, and bridge-owned clip recordings

The bridge owns Dahua-specific behavior.

Start here:

- [bridge/docs/README.md](../bridge/docs/README.md)

## 2. Home Assistant Integration

The Python integration in `integration/custom_components/dahuabridge` is the Home Assistant-facing adapter.

It is responsible for:

- connecting to the bridge over HTTP
- polling the native bridge catalog
- creating Home Assistant devices and entities
- exposing bridge-backed actions as Home Assistant entities and services
- shaping diagnostics for Home Assistant support workflows

The integration does not talk directly to Dahua devices.

Start here:

- [integration/docs/README.md](../integration/docs/README.md)

## 3. HA Cards

The `ha-cards/` workspace is an optional UI layer.

Its role is to:

- consume the entities already created in Home Assistant
- present higher-level workflows and dashboards
- avoid duplicating device discovery or protocol logic

Current repo status:

- two custom card entry points exist:
  - `custom:dahuabridge-surveillance-panel`
  - `custom:dahuabridge-surveillance-tile`
- the workspace builds Lovelace resources from TypeScript
- the cards depend on entities created by the Home Assistant integration

Read more:

- [ha-cards.md](ha-cards.md)
- [../ha-cards/README.md](../ha-cards/README.md)
- [../ha-cards/docs/README.md](../ha-cards/docs/README.md)

## Supported Flow

The supported deployment model is:

1. Run the bridge.
2. Point the Home Assistant integration at the bridge.
3. Let the integration create devices and entities from the bridge catalog.
4. Optionally add cards on top of the Home Assistant entities.

That means:

- Dahua protocol logic belongs in the bridge.
- Home Assistant entity logic belongs in the integration.
- dashboard behavior belongs in cards.

## Data Flow

The runtime data flow is:

1. the bridge loads configuration
2. the bridge probes configured devices
3. the bridge builds a normalized device and stream model
4. the bridge keeps state current through polling and event handling
5. the bridge exposes HTTP APIs and media helpers
6. the integration polls the native catalog endpoint
7. Home Assistant devices and entities are created or updated
8. dashboards and cards render those entities

## Why The Split Matters

This split keeps responsibilities clear:

- the bridge can evolve Dahua behavior without teaching Home Assistant about Dahua internals
- the integration can evolve entity behavior without duplicating protocol work
- cards can iterate on UX without taking ownership of device semantics

For the practical setup path, continue with [deployment.md](deployment.md).
