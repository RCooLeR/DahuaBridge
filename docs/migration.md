# Migration To Clean Native Home Assistant Devices

Use this if you want the cleanest Home Assistant device and entity list.

## Goal

Make the bridge-native Home Assistant integration the main path, and reduce duplicate devices created by:

- MQTT discovery
- generic camera packages
- ONVIF config entries

## Recommended Target State

For normal use:

1. Use the custom integration from `integration/custom_components/dahuabridge`.
2. Set `home_assistant.entity_mode: native` in the bridge config.
3. Remove old retained MQTT discovery entries once.
4. Remove old ONVIF and generic-camera paths if they are duplicating the same devices.

## Step 1: Switch The Bridge To Native Entity Mode

In `bridge/config.yaml`:

```yaml
home_assistant:
  entity_mode: native
```

Meaning:

- `hybrid`: old behavior, bridge still publishes Home Assistant MQTT discovery
- `native`: bridge keeps MQTT topics, but stops publishing Home Assistant MQTT discovery configs

That means `native` helps keep automations and raw topics available while preventing new duplicate Home Assistant MQTT devices from being recreated.

## Step 2: Restart The Bridge

Restart the bridge after changing the config.

## Step 3: Remove Old Retained MQTT Discovery Entries

Run this once:

```text
POST /api/v1/home-assistant/mqtt/discovery/remove
```

You can do it from:

- the admin page button `Remove Legacy MQTT Discovery`
- or the API directly

This removes retained Home Assistant MQTT discovery configs that were previously published by the bridge.

## Step 4: Remove Old Duplicate Home Assistant Paths

If you previously used them, remove:

1. old generic camera package entries
2. ONVIF config entries for devices now represented by the native integration

Why:

- the native integration is the clean grouping path
- ONVIF and generic camera paths usually create parallel camera devices or entities

## Step 5: Review The Bridge Migration Helper

The bridge now exposes:

- `GET /api/v1/home-assistant/migration/plan`
- `GET /api/v1/home-assistant/migration/guide.md`

Use these to see:

- which devices are expected to use the native integration
- which legacy paths are likely to duplicate them

## Important Note About MQTT

`entity_mode: native` does not disable MQTT itself.

It only suppresses Home Assistant MQTT discovery publishing.

That is intentional.

It allows you to:

- keep raw MQTT topics if you still use them elsewhere
- stop cluttering Home Assistant with duplicate MQTT-created devices
