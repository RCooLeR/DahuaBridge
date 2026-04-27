# How It Works

## Short Version

The system has three layers:

1. `bridge/`
2. `integration/`
3. `ha-cards/`

## The Bridge

The Go bridge is the backend.

It does the real work:

- connects to Dahua devices
- probes device identity, inventory, and stream metadata
- listens to supported Dahua event streams
- normalizes device state and events into one internal model
- publishes MQTT discovery, MQTT state, and MQTT triggers
- exposes HTTP APIs for devices, events, snapshots, streams, and admin actions
- serves bridge-hosted media paths such as `MJPEG`, `HLS`, and playback `WebRTC`
- exposes a native Home Assistant catalog endpoint for unified device/entity creation

In simple terms:

- the bridge talks to Dahua
- the bridge keeps the current truth
- the bridge exposes that truth to Home Assistant and browsers

## The Home Assistant Custom Integration

The custom integration in `integration/custom_components/dahuabridge` is the Home Assistant-side adapter.

It does not talk directly to Dahua devices.

It talks to the bridge and creates proper Home Assistant devices and entities from bridge data.

Its main jobs are:

- connect to the bridge over HTTP
- poll the bridge native catalog
- create Home Assistant devices
- create camera, sensor, binary sensor, and button entities
- group related entities under one real Home Assistant device
- expose bridge actions such as `Probe Now` and `Refresh Inventory`
- provide diagnostics for support/debugging

This is the important architectural point:

- the bridge understands Dahua
- the integration understands Home Assistant

## The Future HA Cards

`ha-cards/` is reserved for future custom Home Assistant cards.

The intended role of cards is:

- consume the devices and entities already created in Home Assistant
- build a cleaner and more useful UI on top of them
- present streams, state, motion, call status, and actions in one place

Cards should not be responsible for discovering devices or decoding Dahua behavior.
That work belongs in the bridge and the integration.

## Typical Data Flow

1. The bridge starts.
2. The bridge probes configured Dahua devices.
3. The bridge stores normalized state in memory.
4. The bridge listens for event updates and updates state.
5. The bridge publishes MQTT and exposes HTTP endpoints.
6. The Home Assistant custom integration polls the native bridge catalog.
7. Home Assistant creates or updates devices and entities.
8. Future cards can render those Home Assistant devices and entities into a nicer dashboard.

## Why The Native Integration Exists

Home Assistant grouping is better when one streamable thing is represented as one actual device.

Example:

- one NVR channel
- one Home Assistant device
- one camera entity
- related motion, person, vehicle, and other entities on that same device

That is the main reason for the bridge-native integration.

Without it, the system tends to split across:

- MQTT entities
- ONVIF-created devices
- generic camera entities

That split works, but it is messier in Home Assistant.

## Optional Legacy Paths

The project still has other paths that can be useful:

- MQTT discovery/state
- generated Home Assistant helper packages
- ONVIF helper/provisioning

Those are still valid, but the long-term clean UI path is:

- bridge-native catalog
- custom Home Assistant integration
- future custom cards

For cleanup of duplicate legacy Home Assistant devices, use:

- `home_assistant.entity_mode: native`
- `POST /api/v1/home-assistant/mqtt/discovery/remove`
