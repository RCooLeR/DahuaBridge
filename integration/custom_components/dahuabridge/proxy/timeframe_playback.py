from __future__ import annotations

from collections.abc import Mapping
from typing import Any
from urllib.parse import quote

from homeassistant.core import HomeAssistant

from ..const import DOMAIN


def resolve_coordinator(
    hass: HomeAssistant, attrs: Mapping[str, Any]
) -> Any | None:
    domain_data = hass.data.get(DOMAIN, {})
    bridge_base_url = str(attrs.get("bridge_base_url", "")).strip().rstrip("/")
    fallback = None
    for value in domain_data.values():
        api = getattr(value, "api", None)
        if api is None:
            continue
        if fallback is None:
            fallback = value
        if bridge_base_url and str(api.base_url).rstrip("/") == bridge_base_url:
            return value
    return fallback


def playback_sessions_url(coordinator: Any, attrs: Mapping[str, Any]) -> str | None:
    playback_url = str(attrs.get("bridge_playback_sessions_url", "")).strip()
    if playback_url:
        return playback_url

    root_device_id = str(attrs.get("bridge_root_device_id", "")).strip()
    if not root_device_id:
        return None
    return coordinator.api.absolute_url(
        f"/api/v1/nvr/{quote(root_device_id, safe='')}/playback/sessions"
    )


def select_mjpeg_url(
    session_payload: Mapping[str, Any],
    requested_profile: str | None,
) -> str | None:
    profiles = session_payload.get("profiles")
    if not isinstance(profiles, Mapping):
        return None

    candidates: list[str] = []
    if requested_profile:
        candidates.append(requested_profile)
    recommended = session_payload.get("recommended_profile")
    if isinstance(recommended, str):
        candidates.append(recommended)
    candidates.extend(str(key) for key in profiles)

    seen: set[str] = set()
    for candidate in candidates:
        key = candidate.strip()
        if not key or key in seen:
            continue
        seen.add(key)
        profile = profiles.get(key)
        if not isinstance(profile, Mapping):
            continue
        mjpeg_url = profile.get("mjpeg_url")
        if isinstance(mjpeg_url, str) and mjpeg_url.strip():
            return mjpeg_url.strip()
    return None
