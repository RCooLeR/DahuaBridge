from __future__ import annotations

import voluptuous as vol
from homeassistant.components.camera import DOMAIN as CAMERA_DOMAIN
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers import config_validation as cv, service
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.helpers.typing import ConfigType

from .api import DahuaBridgeAPI
from .const import (
    CONF_API_TOKEN,
    CONF_BRIDGE_URL,
    CONF_SCAN_INTERVAL,
    DEFAULT_SCAN_INTERVAL,
    DOMAIN,
    PLATFORMS,
)
from .coordinator import DahuaBridgeCoordinator
from .timeframe_proxy import async_register_timeframe_proxy_view

CONFIG_SCHEMA = cv.config_entry_only_config_schema(DOMAIN)

type DahuaBridgeConfigEntry = ConfigEntry[DahuaBridgeCoordinator]


async def async_setup(hass: HomeAssistant, config: ConfigType) -> bool:
    """Register integration-wide services and HTTP views."""
    service.async_register_platform_entity_service(
        hass,
        DOMAIN,
        "start_recording",
        entity_domain=CAMERA_DOMAIN,
        schema={
            vol.Optional("profile"): cv.string,
            vol.Optional("duration_seconds"): cv.positive_int,
        },
        func="async_start_recording",
    )
    service.async_register_platform_entity_service(
        hass,
        DOMAIN,
        "stop_recording",
        entity_domain=CAMERA_DOMAIN,
        schema={},
        func="async_stop_recording",
    )
    service.async_register_platform_entity_service(
        hass,
        DOMAIN,
        "set_native_playback_source",
        entity_domain=CAMERA_DOMAIN,
        schema={vol.Required("stream_source"): cv.string},
        func="async_set_native_playback_source",
    )
    service.async_register_platform_entity_service(
        hass,
        DOMAIN,
        "clear_native_playback_source",
        entity_domain=CAMERA_DOMAIN,
        schema={},
        func="async_clear_native_playback_source",
    )
    service.async_register_platform_entity_service(
        hass,
        DOMAIN,
        "set_live_source",
        entity_domain=CAMERA_DOMAIN,
        schema={vol.Required("source"): vol.In({"default", "nvr", "camera"})},
        func="async_set_live_source",
    )
    async_register_timeframe_proxy_view(hass)
    return True


async def async_setup_entry(hass: HomeAssistant, entry: DahuaBridgeConfigEntry) -> bool:
    api = DahuaBridgeAPI(
        async_get_clientsession(hass),
        entry.data[CONF_BRIDGE_URL],
        str(entry.data.get(CONF_API_TOKEN, "")).strip(),
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

    entry.runtime_data = coordinator
    entry.async_on_unload(entry.add_update_listener(async_reload_entry))
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(
    hass: HomeAssistant, entry: DahuaBridgeConfigEntry
) -> bool:
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)


async def async_reload_entry(
    hass: HomeAssistant, entry: DahuaBridgeConfigEntry
) -> None:
    await hass.config_entries.async_reload(entry.entry_id)
