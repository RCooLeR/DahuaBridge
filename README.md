# DahuaBridge

`DahuaBridge` is a Go service that sits between Dahua devices and Home Assistant.
It probes devices, reads event streams, publishes MQTT discovery/state, exposes an admin HTTP API, and serves bridge-hosted media paths for browsers and dashboards.

<p align="center" style="text-align: center;">
  <img src="./bridge/assets/overview.png" alt="DahuaBridge diagram" width="70%">
</p>

This repository is split into three main areas:

- `bridge/`: standalone Go bridge module, Dockerfile, docs, and example config
- `integration/`: Home Assistant custom integration code
- `ha-cards/`: reserved for future custom Home Assistant cards

## What To Use

- Use `bridge/` to build and run the actual bridge service.
- Use `integration/custom_components/dahuabridge` to install the Home Assistant custom integration.
- Ignore `ha-cards/` for now. It is just a reserved folder for future UI cards.

## Fastest Path

1. Open [docs/install.md](/D:/Work/Projects/Go/src/RCooLeR/DahuaBridge/docs/install.md:1).
2. Follow that guide from top to bottom.
3. Start the bridge.
4. Open `http://YOUR_BRIDGE_HOST:9205/admin` and make sure it loads.
5. Install the Home Assistant integration.
6. Add the `DahuaBridge` integration in Home Assistant.
7. Enter the bridge URL, for example `http://192.168.1.50:9205`.

## Quick Commands

```bash
cd bridge
docker build -t dahuabridge .
```

```text
Home Assistant custom component source:
integration/custom_components/dahuabridge
```

## Documentation

- Index: [docs/README.md](/D:/Work/Projects/Go/src/RCooLeR/DahuaBridge/docs/README.md:1)
- Install: [docs/install.md](/D:/Work/Projects/Go/src/RCooLeR/DahuaBridge/docs/install.md:1)
- How it works: [docs/how-it-works.md](/D:/Work/Projects/Go/src/RCooLeR/DahuaBridge/docs/how-it-works.md:1)
- Device model and naming: [docs/device-model.md](/D:/Work/Projects/Go/src/RCooLeR/DahuaBridge/docs/device-model.md:1)
- Migration to clean native HA devices: [docs/migration.md](/D:/Work/Projects/Go/src/RCooLeR/DahuaBridge/docs/migration.md:1)
- Bridge technical details: [bridge/README.md](/D:/Work/Projects/Go/src/RCooLeR/DahuaBridge/bridge/README.md:1)
- Integration details: [integration/README.md](/D:/Work/Projects/Go/src/RCooLeR/DahuaBridge/integration/README.md:1)
