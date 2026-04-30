# Getting Started

This is the shortest practical path to a working bridge.

## Requirements

You need:

- network access to your Dahua devices
- Go or Docker
- `ffmpeg` available if media is enabled

## Configuration File

1. Copy `config.example.yaml` to `config.yaml`.
2. Fill in your device entries.
3. Set `home_assistant.public_base_url` to the URL that browsers and Home Assistant can actually reach.

See:

- [configuration.md](configuration.md)

## Run Directly

```bash
go run ./cmd/dahuabridge --config config.yaml
```

## Run With Docker

Build:

```bash
docker build -t dahuabridge .
```

Run with:

- `config.yaml` mounted at `/config/config.yaml`
- writable storage mounted at `/data`

Start from:

- `compose.example.yaml`

The current repo keeps the supported container path simple: mount the config file and `/data` volume instead of baking config into the image.

## First Validation

Check these endpoints:

- `/healthz`
- `/readyz`
- `/api/v1/status`
- `/api/v1/devices`
- `/api/v1/home-assistant/native/catalog`
- `/admin`

If the bridge is not correct here, do not move on to the integration yet.

Useful interpretation:

- `/healthz` proves the process is alive
- `/readyz` proves the bridge has a populated probe model
- `/api/v1/devices` proves device probing is working
- `/api/v1/home-assistant/native/catalog` proves the Home Assistant-facing model is ready

## Next Step

- [configuration.md](configuration.md)
- [../../integration/docs/install.md](../../integration/docs/install.md)
