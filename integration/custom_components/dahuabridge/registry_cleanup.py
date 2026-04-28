from __future__ import annotations

from collections.abc import Collection

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant


def prune_stale_entities(
    hass: HomeAssistant,
    config_entry: ConfigEntry,
    entity_domain: str,
    desired_unique_ids: Collection[str],
    miss_counts: dict[str, int],
) -> None:
    return None


def prune_stale_devices(
    hass: HomeAssistant,
    config_entry: ConfigEntry,
    desired_device_ids: Collection[str],
    miss_counts: dict[str, int],
) -> None:
    return None
