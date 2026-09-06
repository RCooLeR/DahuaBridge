# Card Install

The HA cards have one Lovelace entry module, optional JavaScript chunks, and image
assets built from the `ha-cards/` workspace.

## Build

From `ha-cards/`:

```bash
npm install
npm run build
```

Build output:

```text
dist/dahuabridge-surveillance-panel.js
dist/chunks/*.js
dist/logo-white.png
```

The entry module registers both:

- `custom:dahuabridge-surveillance-panel`
- `custom:dahuabridge-surveillance-tile`

## Manual Home Assistant Install

Copy the entire contents of `dist/` to a Home Assistant `www` directory, preserving
the `chunks/` subdirectory. For example:

```text
/config/www/dahuabridge/dahuabridge-surveillance-panel.js
/config/www/dahuabridge/chunks/...
/config/www/dahuabridge/logo-white.png
```

Add this Lovelace resource:

```text
/local/dahuabridge/dahuabridge-surveillance-panel.js
```

Then add either custom card type to a dashboard.

Only the entry module is added as a Lovelace resource. The browser loads the
other files relative to it. When upgrading, copy chunks and assets before the
entry module; retain older hashed chunks until existing dashboard tabs reload.

## Requirements

The supported order is:

1. Run the DahuaBridge Go bridge.
2. Install the DahuaBridge Home Assistant integration.
3. Let the integration create devices, entities, attributes, and bridge action URLs.
4. Build and add the Lovelace card bundle.

The cards are not a replacement for the bridge or integration.
