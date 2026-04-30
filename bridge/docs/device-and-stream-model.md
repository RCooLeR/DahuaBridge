# Device And Stream Model

The bridge publishes a normalized model of devices and streams.

This model drives:

- `/api/v1/devices`
- `/api/v1/streams`
- `/api/v1/home-assistant/native/catalog`

## Device Kinds

Current device kinds include:

- `nvr`
- `nvr_channel`
- `nvr_disk`
- `ipc`
- `vto`
- `vto_lock`
- `vto_alarm`

## Root Device IDs

Root IDs come from bridge configuration.

Examples:

- `west20_nvr`
- `front_vto`
- `yard_ipc`

These IDs should be stable because they flow into URLs, catalog records, and Home Assistant unique IDs.

## Important Modeling Rules

### NVR Root

Represents the recorder itself.

### NVR Channel

Represents the actual camera-like child streamable unit.

This is the most important Home Assistant-facing camera model for NVR-connected cameras.

### IPC

Represents a single standalone camera.

### VTO

Represents the door station root plus related call/intercom semantics.

## Stream Catalog Entries

Each stream entry can include:

- identity fields
- device linkage
- stream profiles
- preview/snapshot URLs
- control summaries
- feature summaries
- intercom summary for VTO
- capture summary for bridge-owned snapshot/recording helpers

## Why This Matters

The integration relies on this normalized model so it does not need to understand raw Dahua protocol details.

For the Home Assistant-facing interpretation of this model, see:

- [../../integration/docs/entities-and-controls.md](../../integration/docs/entities-and-controls.md)
