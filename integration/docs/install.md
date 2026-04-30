# 🚀 Install

This guide assumes the bridge is already running and healthy.

If it is not, start here first:

- [../../bridge/docs/getting-started.md](../../bridge/docs/getting-started.md)

## 📦 Install Files

Copy:

```text
integration/custom_components/dahuabridge
```

to:

```text
<home-assistant-config>/custom_components/dahuabridge
```

## 🧩 Add The Integration

1. restart Home Assistant
2. open `Settings -> Devices & Services`
3. click `Add Integration`
4. search for `DahuaBridge`
5. enter the bridge base URL

Example:

```text
http://192.168.1.50:9205
```

The setup flow also lets you set:

- poll interval
- preferred video profile
- preferred video source

If you do not change them, the integration starts with its built-in defaults and you can adjust them later from the options flow.

## 🔗 What The Integration Needs From The Bridge

The key dependency is the native catalog endpoint:

- `GET /api/v1/home-assistant/native/catalog`

The bridge URL you enter must be reachable by Home Assistant itself.
The setup flow also performs a connectivity check against `GET /api/v1/status` before the config entry is created.

## ✅ First Validation

After installation, verify:

- the config entry loads without errors
- devices appear in Home Assistant
- camera entities appear for streamable devices
- bridge-backed control entities appear where supported

## 📚 Next Step

- [configuration.md](configuration.md)
- [entities-and-controls.md](entities-and-controls.md)
