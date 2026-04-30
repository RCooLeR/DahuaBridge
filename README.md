# DahuaBridge

DahuaBridge is a Dahua-focused bridge for Home Assistant.

It has three parts:

- `bridge/`: the Go service that talks to Dahua devices, owns the runtime model, exposes HTTP APIs, and serves bridge-hosted media
- `integration/`: the Home Assistant custom integration that consumes the bridge catalog and creates Home Assistant entities
- `ha-cards/`: optional custom cards for higher-level dashboard UX

Most users need:

1. the Go bridge from `bridge/`
2. the Home Assistant custom integration from `integration/custom_components/dahuabridge`

## Documentation

- System overview: [docs/README.md](docs/README.md)
- GitHub and deployment checklist: [docs/github-deploy.md](docs/github-deploy.md)
- Bridge docs: [bridge/docs/README.md](bridge/docs/README.md)
- Home Assistant integration docs: [integration/docs/README.md](integration/docs/README.md)
- HA cards docs: [ha-cards/docs/README.md](ha-cards/docs/README.md)

## Recommended Path

1. Read the system docs in [docs/deployment.md](docs/deployment.md).
2. Check the GitHub/deploy checklist in [docs/github-deploy.md](docs/github-deploy.md).
3. Set up the bridge with [bridge/docs/getting-started.md](bridge/docs/getting-started.md).
4. Install the Home Assistant integration with [integration/docs/install.md](integration/docs/install.md).

## Repository Layout

- [bridge/README.md](bridge/README.md)
- [integration/README.md](integration/README.md)
- [ha-cards/README.md](ha-cards/README.md)

The primary deployment path is still bridge plus Home Assistant integration, but the repository also contains an optional Lovelace card workspace with its own build and install path.
