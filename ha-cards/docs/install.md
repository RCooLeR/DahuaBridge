# Card Install

The HA cards are distributed as one Lovelace JavaScript bundle built from the
`ha-cards/` workspace.

## Build

From `ha-cards/`:

```bash
npm install
npm run build
```

Build output:

```text
dist/dahuabridge-surveillance-panel.js
```

That single bundle registers both:

- `custom:dahuabridge-surveillance-panel`
- `custom:dahuabridge-surveillance-tile`

## Manual Home Assistant Install

Copy the bundle to a Home Assistant `www` path, for example:

```text
/config/www/dahuabridge/dahuabridge-surveillance-panel.js
```

Add this Lovelace resource:

```text
/local/dahuabridge/dahuabridge-surveillance-panel.js
```

Then add either custom card type to a dashboard.

## Requirements

The supported order is:

1. Run the DahuaBridge Go bridge.
2. Install the DahuaBridge Home Assistant integration.
3. Let the integration create devices, entities, attributes, and bridge action URLs.
4. Build and add the Lovelace card bundle.

The cards are not a replacement for the bridge or integration.
