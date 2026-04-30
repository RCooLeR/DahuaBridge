# Card Configuration

This page documents the supported card configuration fields.

## Shared Assumptions

The cards expect:

- the DahuaBridge Home Assistant integration to already be installed
- bridge-generated device metadata to exist in Home Assistant entities
- the browser to be able to reach any bridge URLs rendered into the card

If Home Assistant reaches the bridge on one URL but the browser must use another, set `browser_bridge_url`.

## `custom:dahuabridge-surveillance-panel`

This is the full dashboard surface.

Supported fields:

- `title`
- `subtitle`
- `browser_bridge_url`
- `event_lookback_hours`
- `bridge_event_poll_seconds`
- `max_events`
- `vto.device_id`

Example:

```yaml
type: custom:dahuabridge-surveillance-panel
title: DahuaBridge Surveillance
subtitle: Full-panel command center
browser_bridge_url: https://dahua.example.com
event_lookback_hours: 12
bridge_event_poll_seconds: 15
max_events: 14
vto:
  device_id: front_vto
```

Field notes:

- `browser_bridge_url` overrides the bridge base URL for browser-side requests only
- `event_lookback_hours` controls the initial event query window
- `bridge_event_poll_seconds` controls how often the card refreshes bridge events
- `max_events` limits the visible event timeline window in the card
- `vto.device_id` pins a preferred VTO when more than one is available

## `custom:dahuabridge-surveillance-tile`

This is the compact single-device surface.

Required fields:

- `device_id`

Optional fields:

- `title`
- `browser_bridge_url`
- `vto.device_id`

Example:

```yaml
type: custom:dahuabridge-surveillance-tile
device_id: west20_nvr_channel_09
title: Yard
browser_bridge_url: https://dahua.example.com
```

Use the tile card when you want a compact camera or VTO view on an existing dashboard instead of the full panel experience.
