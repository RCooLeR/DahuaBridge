from __future__ import annotations

from urllib.parse import urlsplit

import voluptuous as vol

from homeassistant import config_entries
from homeassistant.core import callback
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .bridge_api import DahuaBridgeAPI, DahuaBridgeAPIError, normalize_bridge_url
from .const import CONF_BRIDGE_URL, CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL, DOMAIN


class DahuaBridgeConfigFlow(config_entries.ConfigFlow, domain=DOMAIN):
    VERSION = 1

    @staticmethod
    @callback
    def async_get_options_flow(config_entry: config_entries.ConfigEntry):
        return DahuaBridgeOptionsFlow(config_entry)

    async def async_step_user(self, user_input: dict | None = None):
        errors: dict[str, str] = {}

        if user_input is not None:
            try:
                bridge_url = normalize_bridge_url(user_input[CONF_BRIDGE_URL])
            except ValueError:
                errors["base"] = "invalid_url"
            else:
                api = DahuaBridgeAPI(async_get_clientsession(self.hass), bridge_url)
                try:
                    await api.async_get_status()
                except DahuaBridgeAPIError:
                    errors["base"] = "cannot_connect"
                else:
                    await self.async_set_unique_id(bridge_url)
                    self._abort_if_unique_id_configured()

                    host = urlsplit(bridge_url).hostname or "DahuaBridge"
                    return self.async_create_entry(
                        title=host,
                        data={
                            CONF_BRIDGE_URL: bridge_url,
                            CONF_SCAN_INTERVAL: int(
                                user_input.get(
                                    CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL
                                )
                            ),
                        },
                    )

        schema = build_user_schema()
        return self.async_show_form(step_id="user", data_schema=schema, errors=errors)


class DahuaBridgeOptionsFlow(config_entries.OptionsFlow):
    def __init__(self, config_entry: config_entries.ConfigEntry) -> None:
        self._config_entry = config_entry

    async def async_step_init(self, user_input: dict | None = None):
        if user_input is not None:
            return self.async_create_entry(
                title="",
                data={
                    CONF_SCAN_INTERVAL: int(
                        user_input.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)
                    )
                },
            )

        current_interval = int(
            self._config_entry.options.get(
                CONF_SCAN_INTERVAL,
                self._config_entry.data.get(
                    CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL
                ),
            )
        )
        schema = vol.Schema(
            {
                vol.Optional(
                    CONF_SCAN_INTERVAL, default=current_interval
                ): vol.All(vol.Coerce(int), vol.Range(min=5, max=300)),
            }
        )
        return self.async_show_form(step_id="init", data_schema=schema)


def build_user_schema() -> vol.Schema:
    return vol.Schema(
        {
            vol.Required(CONF_BRIDGE_URL): str,
            vol.Optional(
                CONF_SCAN_INTERVAL, default=DEFAULT_SCAN_INTERVAL
            ): vol.All(vol.Coerce(int), vol.Range(min=5, max=300)),
        }
    )
