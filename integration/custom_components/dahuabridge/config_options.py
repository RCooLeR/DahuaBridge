from __future__ import annotations

from typing import Any

import voluptuous as vol

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
)
from .localization import LANGUAGE_OPTIONS, normalize_language_choice

VIDEO_PROFILE_OPTIONS = {
    "auto": "Auto (Bridge Recommended)",
    "quality": "Quality (Main Stream)",
    "stable": "Stable (Substream)",
}

VIDEO_SOURCE_OPTIONS = {
    "auto": "Auto (Bridge Recommended)",
    "rtsp": "Bridge RTSP (no re-encoding)",
    "hls": "Bridge HLS (H.264/AAC)",
    "dash": "Bridge DASH (H.264/AAC)",
    "mjpeg": "Bridge MJPEG",
}

LIVE_SOURCE_OPTIONS = {
    "nvr": "NVR (fall back to camera)",
    "camera": "Camera (fall back to NVR)",
}

LIVE_PRECONNECT_OPTIONS = {
    "off": "On demand (default)",
    "recent": "Keep recently viewed streams warm (5 minutes)",
    "always": "Preconnect selected video profile (continuous)",
}


def build_user_schema(bridge_url_key: str) -> vol.Schema:
    return vol.Schema(
        {
            vol.Required(bridge_url_key): str,
            vol.Optional(CONF_API_TOKEN, default=""): str,
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
            vol.Optional(
                CONF_ENABLE_VIDEO_FALLBACKS,
                default=DEFAULT_ENABLE_VIDEO_FALLBACKS,
            ): bool,
            vol.Optional(CONF_LANGUAGE, default=DEFAULT_LANGUAGE): vol.In(
                LANGUAGE_OPTIONS
            ),
        }
    )


def build_options_schema(
    config_entry: Any,
    live_source: str | None = None,
    language: str | None = None,
    include_live_source: bool = True,
    live_preconnect: str | None = None,
    include_live_preconnect: bool = True,
    preferred_video_profile: str | None = None,
) -> vol.Schema:
    current_bridge_url = str(config_entry.data.get(CONF_BRIDGE_URL, "")).strip()
    current_api_token = str(config_entry.data.get(CONF_API_TOKEN, "")).strip()
    current_interval = int(
        config_entry.options.get(
            CONF_SCAN_INTERVAL,
            config_entry.data.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL),
        )
    )
    current_profile = normalize_choice(
        preferred_video_profile if preferred_video_profile is not None else config_entry.options.get(
            CONF_PREFERRED_VIDEO_PROFILE, DEFAULT_PREFERRED_VIDEO_PROFILE
        ),
        VIDEO_PROFILE_OPTIONS,
        DEFAULT_PREFERRED_VIDEO_PROFILE,
    )
    current_source = normalize_choice(
        config_entry.options.get(
            CONF_PREFERRED_VIDEO_SOURCE, DEFAULT_PREFERRED_VIDEO_SOURCE
        ),
        VIDEO_SOURCE_OPTIONS,
        DEFAULT_PREFERRED_VIDEO_SOURCE,
    )
    current_language = normalize_language_choice(
        config_entry.options.get(
            CONF_LANGUAGE,
            config_entry.data.get(CONF_LANGUAGE, DEFAULT_LANGUAGE),
        )
    )
    current_fallbacks_enabled = bool(
        config_entry.options.get(
            CONF_ENABLE_VIDEO_FALLBACKS,
            config_entry.data.get(
                CONF_ENABLE_VIDEO_FALLBACKS, DEFAULT_ENABLE_VIDEO_FALLBACKS
            ),
        )
    )
    current_live_source = normalize_choice(
        live_source if live_source is not None else config_entry.options.get(CONF_LIVE_SOURCE),
        LIVE_SOURCE_OPTIONS,
        DEFAULT_LIVE_SOURCE,
    )
    live_source_options = LIVE_SOURCE_OPTIONS
    live_preconnect_options = LIVE_PRECONNECT_OPTIONS
    current_live_preconnect = normalize_choice(
        live_preconnect if live_preconnect is not None else config_entry.options.get(CONF_LIVE_PRECONNECT),
        LIVE_PRECONNECT_OPTIONS,
        DEFAULT_LIVE_PRECONNECT,
    )
    if (current_language if current_language != "auto" else language) == "uk":
        live_source_options = {
            "nvr": "NVR (резервне джерело — камера)",
            "camera": "Камера (резервне джерело — NVR)",
        }
        live_preconnect_options = {
            "off": "На вимогу (за замовчуванням)",
            "recent": "Зберігати з’єднання після перегляду (5 хвилин)",
            "always": "Попередньо підключати вибраний відеопрофіль (постійно)",
        }
    live_source_fields = (
        {
            vol.Optional(CONF_LIVE_SOURCE, default=current_live_source): vol.In(
                live_source_options
            )
        }
        if include_live_source
        else {}
    )
    live_preconnect_fields = (
        {
            vol.Optional(CONF_LIVE_PRECONNECT, default=current_live_preconnect): vol.In(
                live_preconnect_options
            )
        }
        if include_live_preconnect
        else {}
    )
    return vol.Schema(
        {
            vol.Optional(CONF_BRIDGE_URL, default=current_bridge_url): str,
            vol.Optional(CONF_API_TOKEN, default=current_api_token): str,
            vol.Optional(
                CONF_SCAN_INTERVAL, default=current_interval
            ): vol.All(vol.Coerce(int), vol.Range(min=5, max=300)),
            vol.Optional(
                CONF_PREFERRED_VIDEO_PROFILE, default=current_profile
            ): vol.In(VIDEO_PROFILE_OPTIONS),
            **live_source_fields,
            **live_preconnect_fields,
            vol.Optional(
                CONF_PREFERRED_VIDEO_SOURCE, default=current_source
            ): vol.In(VIDEO_SOURCE_OPTIONS),
            vol.Optional(
                CONF_ENABLE_VIDEO_FALLBACKS, default=current_fallbacks_enabled
            ): bool,
            vol.Optional(CONF_LANGUAGE, default=current_language): vol.In(
                LANGUAGE_OPTIONS
            ),
        }
    )


def normalize_choice(raw: object, mapping: dict[str, str], default: str) -> str:
    value = str(raw or "").strip()
    if value in mapping:
        return value

    lowered = value.lower()
    if mapping is VIDEO_PROFILE_OPTIONS:
        if lowered in {"default", "main"}:
            return "quality"
        if lowered in {"substream", "sub"}:
            return "stable"
    if mapping is VIDEO_SOURCE_OPTIONS:
        if lowered in {
            "native",
            "direct",
            "direct_rtsp",
            "direct rtsp",
            "ha",
            "homeassistant",
            "home_assistant",
        }:
            return "rtsp"

    for key, label in mapping.items():
        if lowered == str(label).strip().lower():
            return key

    return default
