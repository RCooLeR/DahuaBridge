# Install

This guide assumes DahuaBridge is already running. If not, start with the bridge guide:

- [Bridge getting started](../../bridge/docs/getting-started.md)

## Copy The Integration

Copy this directory:

```text
integration/custom_components/dahuabridge
```

to your Home Assistant config directory:

```text
<home-assistant-config>/custom_components/dahuabridge
```

Restart Home Assistant after copying the files.

## Add The Config Entry

1. Open `Settings -> Devices & Services`.
2. Select `Add Integration`.
3. Search for `DahuaBridge`.
4. Enter the bridge URL that Home Assistant can reach.

Example:

```text
http://192.168.1.50:9020
```

For reverse proxy setups, use the full public bridge path:

```text
https://ha.example.com/dahuabridge
```

The setup flow validates the URL and API token by calling the protected catalog:

```text
GET /api/v1/home-assistant/native/catalog
```

After setup, the integration polls the same endpoint:

```text
GET /api/v1/home-assistant/native/catalog
```

The setup flow also exposes the initial poll interval, video preferences, and integration language. These can be changed later from the integration options.

## Validate The Result

After setup:

- the config entry should be loaded
- bridge devices should appear as Home Assistant devices
- streamable records should have camera entities
- controls should appear only when the bridge catalog advertises backing URLs

## Next

- [Configuration](configuration.md)
- [How it works](architecture.md)
