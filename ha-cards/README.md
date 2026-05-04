# DahuaBridge HA Cards

This workspace builds the optional Home Assistant Lovelace cards for DahuaBridge.
The cards do not discover Dahua devices directly. They read entities, stream URLs,
attributes, and bridge action URLs that the DahuaBridge Home Assistant integration
already created.

## Cards

- `custom:dahuabridge-surveillance-panel`: full dashboard for live streams, recent events, archive lists, MP4 clips, VTO controls, and device actions.
- `custom:dahuabridge-surveillance-tile`: compact single-device live card for one camera or VTO.

Both cards are bundled into:

```text
dist/dahuabridge-surveillance-panel.js
```

## Current Scope

The HA card code keeps only these media and archive workflows:

- live camera and VTO streams
- SMD/IVS event list, type/date filters, summary counters, direct RTSP playback, and MP4 download/export through bridge-created clips
- 30-minute recording chunk list with download only
- MP4 clips created by the card recording button with play and download
- selected-camera archive seek through the Home Assistant timeframe proxy
- bridge recording start/stop controls when the integration exposes them

Archive support is driven by these integration attributes and bridge response fields:

- `bridge_archive_smd_ivs_url_template`: SMD/IVS event list endpoint
- `bridge_archive_recording_chunks_url_template`: 30-minute recording chunk list endpoint
- `bridge_archive_recordings_url_template`: legacy fallback for recording chunks
- `bridge_channel`: NVR channel number to query
- `bridge_root_device_id`: NVR root device id for channel context
- row-level `export_url`, `asset_playback_url`, and `asset_download_url`: bridge-created MP4 playback/download
- row-level `download_url`: direct recording chunk download

The card does not use `bridge_playback_sessions_url` or
`bridge_archive_coverage_url`. Playback sessions, coverage timelines, and
card-side MP4 delete controls are not used by the HA cards.

## Documentation

- [Install](docs/install.md)
- [Configuration](docs/configuration.md)
- [Features](docs/features.md)
- [Architecture](docs/architecture.md)
- [Removed report](removed-report.md)

## Related Workspaces

- root docs: [../docs/README.md](../docs/README.md)
- bridge docs: [../bridge/docs/README.md](../bridge/docs/README.md)
- integration docs: [../integration/docs/README.md](../integration/docs/README.md)
