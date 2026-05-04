# DahuaBridge Home Assistant Integration

This directory contains the Home Assistant custom integration for DahuaBridge.

The integration does not talk to Dahua devices directly. Home Assistant talks to the bridge, reads the bridge-native catalog, and creates devices, cameras, sensors, and controls from that catalog.

## Docs

- [How it works](docs/architecture.md)
- [Install](docs/install.md)
- [Configuration](docs/configuration.md)
- [Entities and controls](docs/entities-and-controls.md)
- [Camera recording and archive access](docs/camera-recording.md)
- [Feature summary](docs/features.md)

## Code Map

- `custom_components/dahuabridge/__init__.py`: config entry setup/unload.
- `custom_components/dahuabridge/coordinator.py`: catalog polling.
- `custom_components/dahuabridge/api/`: bridge HTTP client and URL handling.
- `custom_components/dahuabridge/catalog/`: catalog parsing, stream selection, entity field metadata, and control specs.
- `custom_components/dahuabridge/localization.py`: English/Ukrainian labels and language resolution.
- `custom_components/dahuabridge/camera.py`: Home Assistant camera entity.
- `custom_components/dahuabridge/camera_support/`: camera attributes, bridge URL rewriting, placeholder image handling.
- `custom_components/dahuabridge/proxy/`: timeframe playback proxy helpers.
- `custom_components/dahuabridge/discovery.py`: shared dynamic entity discovery for platforms.

## Related Docs

- [Project docs](../docs/README.md)
- [Bridge docs](../bridge/docs/README.md)
