# DahuaBridge HA Cards

This workspace builds the optional Home Assistant Lovelace cards for DahuaBridge.
The cards do not discover Dahua devices directly. They read entities, stream URLs,
attributes, and bridge action URLs that the DahuaBridge Home Assistant integration
already created.

## Cards

- `custom:dahuabridge-surveillance-panel`: full dashboard for live streams, recent events, archive lists, MP4 clips, VTO controls, and device actions.
- `custom:dahuabridge-surveillance-tile`: compact single-device live card for one camera or VTO.

Both cards are registered by this entry module:

```text
dist/dahuabridge-surveillance-panel.js
```

Deploy the entire `dist/` directory, including `chunks/` and image assets. HLS,
DASH, and the card editor load their modules when first used.

## Current Scope

The HA card code keeps only these media and archive workflows:

- live camera and VTO streams
- SMD/IVS event list, type/date filters, summary counters, bridge archive playback, and MP4 download/export through bridge-created clips
- 30-minute recording chunk list with download only
- MP4 clips created by the card recording button with play and download
- selected-camera archive seek through bridge playback sessions
- bridge recording start/stop controls when the integration exposes them

Archive support is driven by these integration attributes and bridge response fields:

- `bridge_archive_smd_ivs_url_template`: SMD/IVS event list endpoint
- `bridge_archive_recording_chunks_url_template`: 30-minute recording chunk list endpoint
- `bridge_archive_recordings_url_template`: legacy fallback for recording chunks
- `bridge_channel`: NVR channel number to query
- `bridge_root_device_id`: NVR root device id for channel context
- row-level `export_url`, `asset_playback_url`, and `asset_download_url`: bridge-created MP4 playback/download
- row-level `download_url`: direct recording chunk download

The card uses `bridge_playback_sessions_url` for archive playback when the bridge
exposes stable live relay URLs. Older catalogs retain native RTSP playback
compatibility. `bridge_archive_coverage_url`, coverage timelines, and card-side
MP4 delete controls are not used by the HA cards.

## Documentation

- [Install](docs/install.md)
- [Configuration](docs/configuration.md)
- [Features](docs/features.md)
- [Architecture](docs/architecture.md)
- [Removed report](removed-report.md)

## Development

The workspace requires Node.js `^22.12.0 || ^24.0.0 || >=26.0.0` and pins npm
11.19.1 for reproducible lockfile updates.

```shell
npm ci
npm run check
```

`check` runs the TypeScript typecheck, Vitest suite, and production build.
The build enforces a 175 kB gzip budget for the initial JavaScript dependency
graph and rejects eager HLS/DASH imports. Lifecycle tests exercise delayed
microphone permission, cancellation, out-of-order playback, and user pause.

After building, `npm run test:browser -- /path/to/chrome` runs a local headless
Chromium smoke test of card registration and lazy loading. On Windows the script
also detects standard Chrome/Edge installation paths.

## Related Workspaces

- root docs: [../docs/README.md](../docs/README.md)
- bridge docs: [../bridge/docs/README.md](../bridge/docs/README.md)
- integration docs: [../integration/docs/README.md](../integration/docs/README.md)
