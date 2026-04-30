# Card Install

The cards are distributed as a Lovelace JavaScript bundle built from the `ha-cards/` workspace.

## 1. Build The Bundle

From `ha-cards/`:

```bash
npm install
npm run build
```

Build output:

- `dist/dahuabridge-surveillance-panel.js`

That single bundle registers both custom cards.

## 2. Copy The Bundle Into Home Assistant

Typical manual install path:

```text
/config/www/dahuabridge/dahuabridge-surveillance-panel.js
```

Typical Lovelace resource URL:

```text
/local/dahuabridge/dahuabridge-surveillance-panel.js
```

## 3. Add A Card

Available types:

- `custom:dahuabridge-surveillance-panel`
- `custom:dahuabridge-surveillance-tile`

## Requirements

The supported setup is:

1. run the Go bridge
2. install the Home Assistant integration
3. let the integration create the underlying devices and entities
4. add cards on top of those entities

The cards are not a replacement for the integration.
