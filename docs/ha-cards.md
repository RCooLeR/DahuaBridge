# HA Cards

The `ha-cards/` workspace is an optional Lovelace UI layer on top of the bridge plus Home Assistant integration path.

Current repo status:

- two custom card entry points exist:
  - `custom:dahuabridge-surveillance-panel`
  - `custom:dahuabridge-surveillance-tile`
- the workspace is built with TypeScript
- output is written to `ha-cards/dist/`
- the cards expect the DahuaBridge Home Assistant integration to have already created the underlying entities

## What The Card Layer Owns

- dashboard composition
- browser-facing bridge URL overrides where needed
- room and device presentation
- higher-level surveillance workflows inside Lovelace

## What The Card Layer Does Not Own

- Dahua device communication
- normalized device and stream modeling
- Home Assistant device/entity creation

Those remain owned by:

- the Go bridge in `bridge/`
- the Home Assistant integration in `integration/custom_components/dahuabridge`

## Current Reading Path

- [../ha-cards/docs/README.md](../ha-cards/docs/README.md)
- [../ha-cards/docs/install.md](../ha-cards/docs/install.md)
- [../ha-cards/docs/configuration.md](../ha-cards/docs/configuration.md)
- [../ha-cards/docs/features.md](../ha-cards/docs/features.md)

## Where To Read More

- [../ha-cards/README.md](../ha-cards/README.md)
- [architecture.md](architecture.md)
- [deployment.md](deployment.md)
