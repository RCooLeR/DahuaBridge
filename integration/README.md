# Home Assistant Integration

This directory contains the bridge-native Home Assistant custom integration.

Install source:

- `custom_components/dahuabridge`

This subtree is intended to be copyable on its own. You do not need the Go bridge source code inside `bridge/` to install the Home Assistant component, only a running DahuaBridge instance that exposes:

- `/api/v1/home-assistant/native/catalog`

Main docs:

- install guide: [../docs/install.md](../docs/install.md)
- system overview: [../docs/how-it-works.md](../docs/how-it-works.md)
- device and naming model: [../docs/device-model.md](../docs/device-model.md)
- migration cleanup guide: [../docs/migration.md](../docs/migration.md)

Short version:

1. Make sure the bridge is running first.
2. Copy `custom_components/dahuabridge` into your Home Assistant config.
3. Restart Home Assistant.
4. Add the `DahuaBridge` integration from the UI.
5. Enter the bridge base URL, for example `http://bridge-host:9205`.
6. Adjust the poll interval later from integration options if needed.

Install target:

```text
<your-home-assistant-config>/custom_components/dahuabridge
```

Current characteristics:

- unified device grouping for bridge-native camera, sensor, binary sensor, and button entities
- runtime-configurable poll interval through the Home Assistant options dialog
- diagnostics download support for support/debugging, with bridge URLs and stream/action URLs redacted
