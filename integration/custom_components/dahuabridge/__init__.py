from __future__ import annotations

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .bridge_api import DahuaBridgeAPI
from .catalog import catalog_records, device_id_for_record
from .const import (
    CONF_BRIDGE_URL,
    CONF_SCAN_INTERVAL,
    DEFAULT_SCAN_INTERVAL,
    DOMAIN,
    PLATFORMS,
)
from .coordinator import DahuaBridgeCoordinator
from .registry_cleanup import prune_stale_devices


async def async_setup_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    api = DahuaBridgeAPI(
        async_get_clientsession(hass),
        entry.data[CONF_BRIDGE_URL],
    )
    coordinator = DahuaBridgeCoordinator(
        hass,
        api,
        entry,
        int(
            entry.options.get(
                CONF_SCAN_INTERVAL,
                entry.data.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL),
            )
        ),
    )
    await coordinator.async_config_entry_first_refresh()

    @callback
    def async_prune_registry_devices() -> None:
        desired_device_ids = {
            device_id_for_record(record)
            for record in catalog_records(coordinator.data)
            if device_id_for_record(record)
        }
        if coordinator.can_prune_registry:
            prune_stale_devices(
                hass,
                entry,
                desired_device_ids,
                coordinator.stale_device_miss_counts,
            )

    async_prune_registry_devices()

    hass.data.setdefault(DOMAIN, {})[entry.entry_id] = coordinator
    entry.async_on_unload(entry.add_update_listener(async_reload_entry))
    entry.async_on_unload(coordinator.async_add_listener(async_prune_registry_devices))
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    unload_ok = await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
    if unload_ok:
        hass.data.get(DOMAIN, {}).pop(entry.entry_id, None)
    return unload_ok


async def async_reload_entry(hass: HomeAssistant, entry: ConfigEntry) -> None:
    await hass.config_entries.async_reload(entry.entry_id)
