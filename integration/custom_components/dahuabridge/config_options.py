from __future__ import annotations

from typing import Any

import voluptuous as vol

from .const import (
    CONF_BRIDGE_URL,
    CONF_ENABLE_VIDEO_FALLBACKS,
    CONF_LANGUAGE,
    CONF_PREFERRED_VIDEO_PROFILE,
    CONF_PREFERRED_VIDEO_SOURCE,
    CONF_SCAN_INTERVAL,
    DEFAULT_ENABLE_VIDEO_FALLBACKS,
    DEFAULT_LANGUAGE,
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
    "rtsp": "Direct RTSP",
    "hls": "Bridge HLS (H.264/AAC)",
    "dash": "Bridge DASH (H.264/AAC)",
    "mjpeg": "Bridge MJPEG",
}


def build_user_schema(bridge_url_key: str) -> vol.Schema:
    return vol.Schema(
        {
            vol.Required(bridge_url_key): str,
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


def build_options_schema(config_entry: Any) -> vol.Schema:
    current_bridge_url = str(config_entry.data.get(CONF_BRIDGE_URL, "")).strip()
    current_interval = int(
        config_entry.options.get(
            CONF_SCAN_INTERVAL,
            config_entry.data.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL),
        )
    )
    current_profile = normalize_choice(
        config_entry.options.get(
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
    return vol.Schema(
        {
            vol.Optional(CONF_BRIDGE_URL, default=current_bridge_url): str,
            vol.Optional(
                CONF_SCAN_INTERVAL, default=current_interval
            ): vol.All(vol.Coerce(int), vol.Range(min=5, max=300)),
            vol.Optional(
                CONF_PREFERRED_VIDEO_PROFILE, default=current_profile
            ): vol.In(VIDEO_PROFILE_OPTIONS),
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
            "ha",
            "homeassistant",
            "home_assistant",
        }:
            return "rtsp"

    for key, label in mapping.items():
        if lowered == str(label).strip().lower():
            return key

    return default
