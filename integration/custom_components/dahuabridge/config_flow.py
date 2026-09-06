from __future__ import annotations

import logging
from urllib.parse import urlsplit

from homeassistant import config_entries
from homeassistant.core import callback
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from . import DahuaBridgeConfigEntry
from .api import DahuaBridgeAPI, DahuaBridgeAPIError, normalize_bridge_url
from .config_options import (
    LIVE_PRECONNECT_OPTIONS,
    LIVE_SOURCE_OPTIONS,
    VIDEO_PROFILE_OPTIONS,
    VIDEO_SOURCE_OPTIONS,
    build_options_schema,
    build_user_schema,
    normalize_choice,
    normalize_language_choice,
)
from .const import (
    CONF_API_TOKEN,
    CONF_BRIDGE_URL,
    CONF_ENABLE_VIDEO_FALLBACKS,
    CONF_LANGUAGE,
    CONF_LIVE_PRECONNECT,
    CONF_LIVE_SOURCE,
    CONF_PREFERRED_VIDEO_PROFILE,
    CONF_PREFERRED_VIDEO_SOURCE,
    CONF_SCAN_INTERVAL,
    DEFAULT_ENABLE_VIDEO_FALLBACKS,
    DEFAULT_LANGUAGE,
    DEFAULT_LIVE_PRECONNECT,
    DEFAULT_LIVE_SOURCE,
    DEFAULT_PREFERRED_VIDEO_PROFILE,
    DEFAULT_PREFERRED_VIDEO_SOURCE,
    DEFAULT_SCAN_INTERVAL,
    DOMAIN,
)

_LOGGER = logging.getLogger(__name__)


class DahuaBridgeConfigFlow(config_entries.ConfigFlow, domain=DOMAIN):
    VERSION = 1

    @staticmethod
    @callback
    def async_get_options_flow(config_entry: DahuaBridgeConfigEntry):
        return DahuaBridgeOptionsFlow(config_entry)

    async def async_step_user(self, user_input: dict | None = None):
        errors: dict[str, str] = {}

        if user_input is not None:
            try:
                bridge_url = normalize_bridge_url(user_input[CONF_BRIDGE_URL])
            except ValueError:
                errors["base"] = "invalid_url"
            else:
                api_token = str(user_input.get(CONF_API_TOKEN, "")).strip()
                api = DahuaBridgeAPI(async_get_clientsession(self.hass), bridge_url, api_token)
                try:
                    await api.async_get_catalog()
                except DahuaBridgeAPIError as err:
                    _LOGGER.warning(
                        "Bridge connectivity check failed for %s: %s",
                        bridge_url,
                        err,
                    )
                    errors["base"] = "invalid_auth" if err.status in {401, 403} else "cannot_connect"
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
                    language = normalize_language_choice(
                        user_input.get(CONF_LANGUAGE, DEFAULT_LANGUAGE)
                    )
                    enable_video_fallbacks = bool(
                        user_input.get(
                            CONF_ENABLE_VIDEO_FALLBACKS,
                            DEFAULT_ENABLE_VIDEO_FALLBACKS,
                        )
                    )
                    await self.async_set_unique_id(bridge_url)
                    self._abort_if_unique_id_configured()

                    host = urlsplit(bridge_url).hostname or "DahuaBridge"
                    return self.async_create_entry(
                        title=host,
                        data={
                            CONF_BRIDGE_URL: bridge_url,
                            CONF_API_TOKEN: api_token,
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
                            CONF_ENABLE_VIDEO_FALLBACKS: enable_video_fallbacks,
                            CONF_LANGUAGE: language,
                        },
                    )

        schema = build_user_schema(CONF_BRIDGE_URL)
        return self.async_show_form(step_id="user", data_schema=schema, errors=errors)


class DahuaBridgeOptionsFlow(config_entries.OptionsFlow):
    """Edit HA playback options and explicitly save bridge-owned live settings."""

    def __init__(self, config_entry: DahuaBridgeConfigEntry) -> None:
        self._config_entry = config_entry
        self._live_source: str | None = None
        self._show_live_source = True
        self._live_preconnect: str | None = None
        self._show_live_preconnect = True
        self._preferred_video_profile: str | None = None

    def _schema(self):
        return build_options_schema(
            self._config_entry,
            live_source=self._live_source,
            language=getattr(getattr(self.hass, "config", None), "language", None),
            include_live_source=self._show_live_source,
            live_preconnect=self._live_preconnect,
            include_live_preconnect=self._show_live_preconnect,
            preferred_video_profile=self._preferred_video_profile,
        )

    async def async_step_init(self, user_input: dict | None = None):
        errors: dict[str, str] = {}

        if user_input is not None:
            bridge_url = str(self._config_entry.data.get(CONF_BRIDGE_URL, "")).strip()
            self._preferred_video_profile = normalize_choice(
                user_input.get(
                    CONF_PREFERRED_VIDEO_PROFILE,
                    self._config_entry.options.get(CONF_PREFERRED_VIDEO_PROFILE, DEFAULT_PREFERRED_VIDEO_PROFILE),
                ),
                VIDEO_PROFILE_OPTIONS,
                DEFAULT_PREFERRED_VIDEO_PROFILE,
            )
            if CONF_LIVE_PRECONNECT in user_input:
                # The same profile drives HA playback and bridge preconnection.
                # Keep the selection for redisplay if a bridge save fails.
                self._show_live_preconnect = True
                self._live_preconnect = normalize_choice(
                    user_input[CONF_LIVE_PRECONNECT], LIVE_PRECONNECT_OPTIONS, DEFAULT_LIVE_PRECONNECT
                )
            try:
                requested_bridge_url = normalize_bridge_url(
                    user_input.get(CONF_BRIDGE_URL, bridge_url)
                )
                requested_api_token = str(
                    user_input.get(
                        CONF_API_TOKEN,
                        self._config_entry.data.get(CONF_API_TOKEN, ""),
                    )
                ).strip()
            except ValueError:
                errors["base"] = "invalid_url"
            else:
                bridge_url_changed = requested_bridge_url != bridge_url
                api_token_changed = requested_api_token != str(
                    self._config_entry.data.get(CONF_API_TOKEN, "")
                ).strip()
                api = DahuaBridgeAPI(
                    async_get_clientsession(self.hass),
                    requested_bridge_url,
                    requested_api_token,
                )
                if bridge_url_changed or api_token_changed:
                    try:
                        await api.async_get_catalog()
                    except DahuaBridgeAPIError as err:
                        _LOGGER.warning(
                            "Bridge connectivity check failed for %s: %s",
                            requested_bridge_url,
                            err,
                        )
                        errors["base"] = "invalid_auth" if err.status in {401, 403} else "cannot_connect"

                if not errors and CONF_LIVE_SOURCE in user_input:
                    self._show_live_source = True
                    self._live_source = normalize_choice(
                        user_input[CONF_LIVE_SOURCE], LIVE_SOURCE_OPTIONS, DEFAULT_LIVE_SOURCE
                    )
                    try:
                        current = await api.async_get_live_source()
                        if current["source"] != self._live_source:
                            await api.async_set_live_source(self._live_source)
                    except DahuaBridgeAPIError:
                        errors["base"] = "cannot_set_live_source"

                if not errors and CONF_LIVE_PRECONNECT in user_input:
                    try:
                        current = await api.async_get_live_preconnect()
                        if current["mode"] != self._live_preconnect or current["profile"] != self._preferred_video_profile:
                            await api.async_set_live_preconnect(self._live_preconnect, self._preferred_video_profile)
                    except DahuaBridgeAPIError:
                        errors["base"] = "cannot_set_live_preconnect"

                # Each bridge endpoint persists independently. Commit HA settings
                # only after every requested save succeeds; a later failure does
                # not roll back an earlier successful bridge preference update.
                if not errors and (bridge_url_changed or api_token_changed):
                    self.hass.config_entries.async_update_entry(
                        self._config_entry,
                        data={
                            **dict(self._config_entry.data),
                            CONF_BRIDGE_URL: requested_bridge_url,
                            CONF_API_TOKEN: requested_api_token,
                        },
                        title=urlsplit(requested_bridge_url).hostname or "DahuaBridge",
                    )

            if errors:
                schema = self._schema()
                return self.async_show_form(
                    step_id="init", data_schema=schema, errors=errors
                )

            preferred_source = normalize_choice(
                user_input.get(
                    CONF_PREFERRED_VIDEO_SOURCE,
                    self._config_entry.options.get(CONF_PREFERRED_VIDEO_SOURCE, DEFAULT_PREFERRED_VIDEO_SOURCE),
                ),
                VIDEO_SOURCE_OPTIONS,
                DEFAULT_PREFERRED_VIDEO_SOURCE,
            )
            language = normalize_language_choice(
                user_input.get(CONF_LANGUAGE, self._config_entry.options.get(CONF_LANGUAGE, DEFAULT_LANGUAGE))
            )
            enable_video_fallbacks = bool(
                user_input.get(
                    CONF_ENABLE_VIDEO_FALLBACKS,
                    self._config_entry.options.get(CONF_ENABLE_VIDEO_FALLBACKS, DEFAULT_ENABLE_VIDEO_FALLBACKS),
                )
            )
            return self.async_create_entry(
                title="",
                data={
                    **dict(self._config_entry.options),
                    CONF_SCAN_INTERVAL: int(
                        user_input.get(
                            CONF_SCAN_INTERVAL,
                            self._config_entry.options.get(CONF_SCAN_INTERVAL, self._config_entry.data.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)),
                        )
                    ),
                    **({CONF_LIVE_SOURCE: self._live_source} if CONF_LIVE_SOURCE in user_input else {}),
                    **({CONF_LIVE_PRECONNECT: self._live_preconnect} if CONF_LIVE_PRECONNECT in user_input else {}),
                    CONF_PREFERRED_VIDEO_PROFILE: self._preferred_video_profile,
                    CONF_PREFERRED_VIDEO_SOURCE: preferred_source,
                    CONF_ENABLE_VIDEO_FALLBACKS: enable_video_fallbacks,
                    CONF_LANGUAGE: language,
                },
            )

        api = DahuaBridgeAPI(
            async_get_clientsession(self.hass),
            self._config_entry.data[CONF_BRIDGE_URL],
            self._config_entry.data.get(CONF_API_TOKEN, ""),
        )
        # Read current bridge preferences independently: older bridges may expose
        # only one endpoint. Omitted fields must not acquire schema defaults that
        # block unrelated saves. These saved HA values are never replayed on poll.
        try:
            self._live_source = (await api.async_get_live_source())["source"]
            self._show_live_source = True
        except DahuaBridgeAPIError:
            self._show_live_source = False
        try:
            self._live_preconnect = (await api.async_get_live_preconnect())["mode"]
            self._show_live_preconnect = True
        except DahuaBridgeAPIError:
            self._show_live_preconnect = False
        schema = self._schema()
        return self.async_show_form(step_id="init", data_schema=schema)
