from __future__ import annotations

import logging
from datetime import timedelta
from typing import Any

from homeassistant.core import HomeAssistant
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .bridge_api import DahuaBridgeAPI, DahuaBridgeAPIError

_LOGGER = logging.getLogger(__name__)


class DahuaBridgeCoordinator(DataUpdateCoordinator[dict[str, Any]]):
    def __init__(
        self, hass: HomeAssistant, api: DahuaBridgeAPI, scan_interval: int
    ) -> None:
        super().__init__(
            hass,
            logger=_LOGGER,
            name="DahuaBridge catalog",
            update_interval=timedelta(seconds=scan_interval),
        )
        self.api = api

    async def _async_update_data(self) -> dict[str, Any]:
        try:
            return await self.api.async_get_catalog()
        except DahuaBridgeAPIError as err:
            raise UpdateFailed(str(err)) from err
