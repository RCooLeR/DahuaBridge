from __future__ import annotations

import logging
from datetime import timedelta
from typing import Any

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .bridge_api import DahuaBridgeAPI, DahuaBridgeAPIError
from .const import (
    CONF_PREFERRED_VIDEO_PROFILE,
    CONF_PREFERRED_VIDEO_SOURCE,
    DEFAULT_PREFERRED_VIDEO_PROFILE,
    DEFAULT_PREFERRED_VIDEO_SOURCE,
)

_LOGGER = logging.getLogger(__name__)


class DahuaBridgeCoordinator(DataUpdateCoordinator[dict[str, Any]]):
    def __init__(
        self,
        hass: HomeAssistant,
        api: DahuaBridgeAPI,
        config_entry: ConfigEntry,
        scan_interval: int,
    ) -> None:
        super().__init__(
            hass,
            logger=_LOGGER,
            name="DahuaBridge catalog",
            update_interval=timedelta(seconds=scan_interval),
        )
        self.api = api
        self.config_entry = config_entry
        self.stale_entity_miss_counts: dict[str, int] = {}
        self.stale_device_miss_counts: dict[str, int] = {}

    @property
    def preferred_video_profile(self) -> str:
        return str(
            self.config_entry.options.get(
                CONF_PREFERRED_VIDEO_PROFILE, DEFAULT_PREFERRED_VIDEO_PROFILE
            )
        ).strip() or DEFAULT_PREFERRED_VIDEO_PROFILE

    @property
    def preferred_video_source(self) -> str:
        return str(
            self.config_entry.options.get(
                CONF_PREFERRED_VIDEO_SOURCE, DEFAULT_PREFERRED_VIDEO_SOURCE
            )
        ).strip() or DEFAULT_PREFERRED_VIDEO_SOURCE

    @property
    def can_prune_registry(self) -> bool:
        devices = (self.data or {}).get("devices", [])
        return self.last_update_success and isinstance(devices, list) and len(devices) > 0

    @property
    def include_stream_credentials(self) -> bool:
        return self.preferred_video_source == "rtsp"

    async def _async_update_data(self) -> dict[str, Any]:
        try:
            return await self.api.async_get_catalog(
                include_credentials=self.include_stream_credentials
            )
        except DahuaBridgeAPIError as err:
            raise UpdateFailed(str(err)) from err
