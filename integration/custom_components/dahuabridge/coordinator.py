from __future__ import annotations

import logging
from datetime import timedelta
from typing import TYPE_CHECKING, Any

from homeassistant.core import HomeAssistant
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .api import DahuaBridgeAPI, DahuaBridgeAPIError
from .catalog import normalize_profile_name
from .const import (
    CONF_ENABLE_VIDEO_FALLBACKS,
    CONF_LANGUAGE,
    CONF_PREFERRED_VIDEO_PROFILE,
    CONF_PREFERRED_VIDEO_SOURCE,
    DEFAULT_ENABLE_VIDEO_FALLBACKS,
    DEFAULT_LANGUAGE,
    DEFAULT_PREFERRED_VIDEO_PROFILE,
    DEFAULT_PREFERRED_VIDEO_SOURCE,
)
from .localization import resolve_language

if TYPE_CHECKING:
    from . import DahuaBridgeConfigEntry

_LOGGER = logging.getLogger(__name__)


class DahuaBridgeCoordinator(DataUpdateCoordinator[dict[str, Any]]):
    def __init__(
        self,
        hass: HomeAssistant,
        api: DahuaBridgeAPI,
        config_entry: DahuaBridgeConfigEntry,
        scan_interval: int,
    ) -> None:
        super().__init__(
            hass,
            logger=_LOGGER,
            name="DahuaBridge catalog",
            config_entry=config_entry,
            update_interval=timedelta(seconds=scan_interval),
            always_update=False,
        )
        self.api = api
        self.config_entry = config_entry

    @property
    def preferred_video_profile(self) -> str:
        return normalize_profile_name(
            self.config_entry.options.get(
                CONF_PREFERRED_VIDEO_PROFILE, DEFAULT_PREFERRED_VIDEO_PROFILE
            )
        ) or DEFAULT_PREFERRED_VIDEO_PROFILE

    @property
    def preferred_video_source(self) -> str:
        return str(
            self.config_entry.options.get(
                CONF_PREFERRED_VIDEO_SOURCE, DEFAULT_PREFERRED_VIDEO_SOURCE
            )
        ).strip() or DEFAULT_PREFERRED_VIDEO_SOURCE

    @property
    def video_fallbacks_enabled(self) -> bool:
        return bool(
            self.config_entry.options.get(
                CONF_ENABLE_VIDEO_FALLBACKS, DEFAULT_ENABLE_VIDEO_FALLBACKS
            )
        )

    @property
    def integration_language(self) -> str:
        hass_language = getattr(getattr(self.hass, "config", None), "language", None)
        return resolve_language(
            self.config_entry.options.get(CONF_LANGUAGE, DEFAULT_LANGUAGE),
            hass_language,
        )

    @property
    def include_stream_credentials(self) -> bool:
        return self.preferred_video_source.lower() in {"rtsp", "native", "direct_rtsp"}

    async def _async_update_data(self) -> dict[str, Any]:
        try:
            return await self.api.async_get_catalog(
                include_credentials=self.include_stream_credentials
            )
        except DahuaBridgeAPIError as err:
            raise UpdateFailed(str(err)) from err
