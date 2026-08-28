from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any
from urllib.parse import quote

from homeassistant.core import HomeAssistant

from ..const import DOMAIN

if TYPE_CHECKING:
    from ..coordinator import DahuaBridgeCoordinator


def resolve_coordinator(
    hass: HomeAssistant, attrs: Mapping[str, Any]
) -> DahuaBridgeCoordinator | None:
    bridge_base_url = str(attrs.get("bridge_base_url", "")).strip().rstrip("/")
    fallback = None
    for entry in hass.config_entries.async_loaded_entries(DOMAIN):
        value = entry.runtime_data
        api = value.api
        if fallback is None:
            fallback = value
        if bridge_base_url and str(api.base_url).rstrip("/") == bridge_base_url:
            return value
    return fallback


def playback_sessions_url(coordinator: Any, attrs: Mapping[str, Any]) -> str | None:
    playback_url = str(attrs.get("bridge_playback_sessions_url", "")).strip()
    if playback_url:
        return coordinator.api.bridge_resource_url(playback_url)

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
    urls = select_mjpeg_urls(session_payload, requested_profile)
    return urls[0] if urls else None


def select_mjpeg_urls(
    session_payload: Mapping[str, Any],
    requested_profile: str | None,
) -> list[str]:
    profiles = session_payload.get("profiles")
    if not isinstance(profiles, Mapping):
        return []

    candidates: list[str] = []
    if requested_profile:
        candidates.append(requested_profile)
    recommended = session_payload.get("recommended_profile")
    if isinstance(recommended, str):
        candidates.append(recommended)
    candidates.extend(str(key) for key in profiles)

    seen: set[str] = set()
    seen_urls: set[str] = set()
    urls: list[str] = []
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
            normalized_url = mjpeg_url.strip()
            if normalized_url not in seen_urls:
                seen_urls.add(normalized_url)
                urls.append(normalized_url)
    return urls
