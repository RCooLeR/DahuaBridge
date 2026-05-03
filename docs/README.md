# DahuaBridge Documentation

This section explains how the repository works as one system.

Use these pages in order:

1. [Architecture](architecture.md)
2. [Deployment Guide](deployment.md)
3. [Feature Map](features.md)
4. [HA Cards](ha-cards.md)

Then move into the component-specific docs:

- [Bridge docs](../bridge/docs/README.md)
- [Bridge getting started](../bridge/docs/getting-started.md)
- [Bridge configuration](../bridge/docs/configuration.md)
- [Integration docs](../integration/docs/README.md)
- [HA cards docs](../ha-cards/docs/README.md)
- [HA cards install](../ha-cards/docs/install.md)

## What This Section Covers

- how the bridge, integration, and optional cards fit together
- the supported deployment model
- which features belong to which layer
- how to think about ownership and responsibilities across the repo

## What This Section Does Not Duplicate

- low-level bridge API reference: see [bridge/docs/api-reference.md](../bridge/docs/api-reference.md)
- bridge media details: see [bridge/docs/media-and-recording.md](../bridge/docs/media-and-recording.md)
- integration entities and services: see [integration/docs/entities-and-controls.md](../integration/docs/entities-and-controls.md)

## Current Archive Model

The current bridge separates recorder history into:

- SMD/IVS rows: `GET /api/v1/nvr/{deviceID}/smd-ivs`
- normal DAV chunks: `GET /api/v1/nvr/{deviceID}/recording-chunks`
- bridge MP4 clips: `GET /api/v1/media/recordings`

The old mixed archive list is kept only as a compatibility endpoint.
