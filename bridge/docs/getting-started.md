# Getting Started

This is the shortest practical path to a bridge that Home Assistant and the
dashboard cards can actually use.

## Requirements

You need:

- network access from the bridge host to the Dahua NVR, IPC, or VTO devices
- Docker, or Go plus a local `ffmpeg`
- writable storage for bridge state, archive SQLite data, temporary HLS files, and MP4 clips
- a browser/HA-reachable URL for the bridge, usually through a reverse proxy

## Configure

1. Copy `config.example.yaml` to `config.yaml`.
2. Add NVR, IPC, and VTO entries under `devices`.
3. Set `home_assistant.public_base_url` to the exact URL browsers and Home
   Assistant will open, including any reverse-proxy prefix.
4. If the bridge is exposed outside a trusted LAN, set `http.auth_token` or
   `DAHUABRIDGE_HTTP_AUTH_TOKEN` and enter the same token in the Home Assistant
   integration.
5. If traffic reaches the bridge through Home Assistant, nginx, or another
   trusted proxy, set `http.trusted_proxies` so rate limiting uses the real
   client IP from forwarded headers.
6. Tune media only after the basic catalog works. For lower HLS latency, start
   with `media.input_preset: low_latency`, `hls_segment_time: 1s`, and a small
   `hls_list_size`.
7. For noisy SMD/IVS cameras, use `archive.export_ivs`,
   `archive.export_smd_person`, `archive.export_smd_transport`, and
   `archive.export_smd_animal` to limit automatic MP4 creation while still
   indexing the events.

See [configuration.md](configuration.md) for every active key.

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
- `/dev/dri` mounted when using Intel QSV hardware acceleration

Start from `compose.example.yaml`. The supported container path is to mount the
config file and writable volumes; do not bake site-specific config into the
image.

After changing the source or config on a Docker host, rebuild or restart the
container:

```bash
docker compose up -d --build dahua-bridge
```

## First Validation

Check these endpoints through the same URL Home Assistant will use:

- `/healthz`
- `/readyz`
- `/api/v1/status`
- `/api/v1/devices`
- `/api/v1/streams`
- `/api/v1/home-assistant/native/catalog`
- `/admin`

Interpretation:

- `/healthz` proves the process is alive.
- `/readyz` proves at least one configured device has been probed.
- `/api/v1/devices` proves Dahua probing is working.
- `/api/v1/streams` proves stream metadata, profiles, and audio codecs are visible.
- `/api/v1/home-assistant/native/catalog` proves the integration-facing model is ready.

If the bridge is mounted under a proxy prefix and `/prefix/api/...` returns
`404`, check the proxy rewrite first. The bridge routes are mounted at `/api/...`
inside the process.

## Next Step

- [configuration.md](configuration.md)
- [../../integration/docs/install.md](../../integration/docs/install.md)
