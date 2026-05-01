from __future__ import annotations

import logging
from urllib.parse import urlsplit

import voluptuous as vol

from homeassistant import config_entries
from homeassistant.core import callback
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .bridge_api import DahuaBridgeAPI, DahuaBridgeAPIError, normalize_bridge_url
from .const import (
    CONF_BRIDGE_URL,
    CONF_PREFERRED_VIDEO_PROFILE,
    CONF_PREFERRED_VIDEO_SOURCE,
    CONF_SCAN_INTERVAL,
    DEFAULT_PREFERRED_VIDEO_PROFILE,
    DEFAULT_PREFERRED_VIDEO_SOURCE,
    DEFAULT_SCAN_INTERVAL,
    DOMAIN,
)

_LOGGER = logging.getLogger(__name__)

VIDEO_PROFILE_OPTIONS = {
    "auto": "Auto (Bridge Recommended)",
    "quality": "Quality (Main Stream)",
    "default": "Default (Main Stream)",
    "stable": "Stable (Substream)",
    "substream": "Substream (Native)",
}
VIDEO_PROFILE_VALUES = tuple(VIDEO_PROFILE_OPTIONS.keys())

VIDEO_SOURCE_OPTIONS = {
    "auto": "Auto (Bridge Recommended)",
    "rtsp": "Direct RTSP",
    "hls": "Bridge HLS (H.264/AAC)",
    "mjpeg": "Bridge MJPEG",
}
VIDEO_SOURCE_VALUES = tuple(VIDEO_SOURCE_OPTIONS.keys())


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
                except DahuaBridgeAPIError as err:
                    _LOGGER.warning(
                        "Bridge connectivity check failed for %s: %s",
                        bridge_url,
                        err,
                    )
                    errors["base"] = "cannot_connect"
                else:
                    _LOGGER.debug(
                        "Bridge connectivity check succeeded for %s", bridge_url
                    )
                    preferred_profile = normalize_choice(
                        user_input.get(
                            CONF_PREFERRED_VIDEO_PROFILE,
                            DEFAULT_PREFERRED_VIDEO_PROFILE,
                        ),
                        VIDEO_PROFILE_OPTIONS,
                        DEFAULT_PREFERRED_VIDEO_PROFILE,
                    )
                    preferred_source = normalize_choice(
                        user_input.get(
                            CONF_PREFERRED_VIDEO_SOURCE,
                            DEFAULT_PREFERRED_VIDEO_SOURCE,
                        ),
                        VIDEO_SOURCE_OPTIONS,
                        DEFAULT_PREFERRED_VIDEO_SOURCE,
                    )
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
                        options={
                            CONF_SCAN_INTERVAL: int(
                                user_input.get(
                                    CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL
                                )
                            ),
                            CONF_PREFERRED_VIDEO_PROFILE: preferred_profile,
                            CONF_PREFERRED_VIDEO_SOURCE: preferred_source,
                        },
                    )

        schema = build_user_schema()
        return self.async_show_form(step_id="user", data_schema=schema, errors=errors)


class DahuaBridgeOptionsFlow(config_entries.OptionsFlow):
    def __init__(self, config_entry: config_entries.ConfigEntry) -> None:
        self._config_entry = config_entry

    async def async_step_init(self, user_input: dict | None = None):
        if user_input is not None:
            preferred_profile = normalize_choice(
                user_input.get(
                    CONF_PREFERRED_VIDEO_PROFILE, DEFAULT_PREFERRED_VIDEO_PROFILE
                ),
                VIDEO_PROFILE_OPTIONS,
                DEFAULT_PREFERRED_VIDEO_PROFILE,
            )
            preferred_source = normalize_choice(
                user_input.get(
                    CONF_PREFERRED_VIDEO_SOURCE, DEFAULT_PREFERRED_VIDEO_SOURCE
                ),
                VIDEO_SOURCE_OPTIONS,
                DEFAULT_PREFERRED_VIDEO_SOURCE,
            )
            return self.async_create_entry(
                title="",
                data={
                    CONF_SCAN_INTERVAL: int(
                        user_input.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)
                    ),
                    CONF_PREFERRED_VIDEO_PROFILE: preferred_profile,
                    CONF_PREFERRED_VIDEO_SOURCE: preferred_source,
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
        current_profile = normalize_choice(
            self._config_entry.options.get(
                CONF_PREFERRED_VIDEO_PROFILE, DEFAULT_PREFERRED_VIDEO_PROFILE
            ),
            VIDEO_PROFILE_OPTIONS,
            DEFAULT_PREFERRED_VIDEO_PROFILE,
        )
        current_source = normalize_choice(
            self._config_entry.options.get(
                CONF_PREFERRED_VIDEO_SOURCE, DEFAULT_PREFERRED_VIDEO_SOURCE
            ),
            VIDEO_SOURCE_OPTIONS,
            DEFAULT_PREFERRED_VIDEO_SOURCE,
        )
        schema = vol.Schema(
            {
                vol.Optional(
                    CONF_SCAN_INTERVAL, default=current_interval
                ): vol.All(vol.Coerce(int), vol.Range(min=5, max=300)),
                vol.Optional(
                    CONF_PREFERRED_VIDEO_PROFILE, default=current_profile
                ): vol.In(VIDEO_PROFILE_OPTIONS),
                vol.Optional(
                    CONF_PREFERRED_VIDEO_SOURCE, default=current_source
                ): vol.In(VIDEO_SOURCE_OPTIONS),
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
            vol.Optional(
                CONF_PREFERRED_VIDEO_PROFILE,
                default=DEFAULT_PREFERRED_VIDEO_PROFILE,
            ): vol.In(VIDEO_PROFILE_OPTIONS),
            vol.Optional(
                CONF_PREFERRED_VIDEO_SOURCE,
                default=DEFAULT_PREFERRED_VIDEO_SOURCE,
            ): vol.In(VIDEO_SOURCE_OPTIONS),
        }
    )


def normalize_choice(raw: object, mapping: dict[str, str], default: str) -> str:
    value = str(raw or "").strip()
    if value in mapping:
        return value

    lowered = value.lower()
    for key, label in mapping.items():
        if lowered == str(label).strip().lower():
            return key

    return default
