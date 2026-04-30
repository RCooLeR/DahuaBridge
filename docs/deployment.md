# 🧭 Deployment Guide

This is the recommended setup path for the full system.

## ✅ Before You Start

You need:

- a host that can reach your Dahua devices over the network
- Docker or Go for running the bridge
- Home Assistant that can reach the bridge over HTTP
- `ffmpeg` available if you want bridge-hosted media

## 1. Configure And Start The Bridge

Use the bridge guide:

- [bridge/docs/getting-started.md](../bridge/docs/getting-started.md)
- [github-deploy.md](github-deploy.md)

That guide covers:

- direct Go execution
- Docker
- first validation steps

## 2. Verify The Bridge First

Do not install the Home Assistant integration until the bridge works on its own.

At minimum, confirm:

- `/healthz`
- `/readyz`
- `/api/v1/status`
- `/api/v1/devices`
- `/api/v1/home-assistant/native/catalog`
- `/admin`
- `/admin/test-bridge`

These are documented in:

- [bridge/docs/api-reference.md](../bridge/docs/api-reference.md)

## 3. Install The Home Assistant Integration

Use the integration guide:

- [integration/docs/install.md](../integration/docs/install.md)

That guide covers:

- where to copy `custom_components/dahuabridge`
- how to add the integration in Home Assistant
- where poll interval and preferred media options fit in the setup flow

## 4. Validate The Full Stack

After the integration is installed, verify:

- the `DahuaBridge` integration loads
- devices appear in Home Assistant
- streamable things such as `nvr_channel` and `ipc` show up as camera devices
- bridge-backed actions appear where expected
- camera preview and stream access work from Home Assistant

For the expected entity model, see:

- [integration/docs/entities-and-controls.md](../integration/docs/entities-and-controls.md)

## 5. Optional Dashboard Layer

Cards are optional and intentionally not the main setup path.

For now, see:

- [ha-cards.md](ha-cards.md)

## 📚 Where To Go Next

- architecture: [architecture.md](architecture.md)
- feature map: [features.md](features.md)
- bridge docs: [../bridge/docs/README.md](../bridge/docs/README.md)
- integration docs: [../integration/docs/README.md](../integration/docs/README.md)
