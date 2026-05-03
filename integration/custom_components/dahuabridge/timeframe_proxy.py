from __future__ import annotations

import logging
from collections.abc import Mapping
from datetime import datetime, timedelta, timezone
from typing import Any
from urllib.parse import quote, unquote

from aiohttp import ClientError, web
from homeassistant.components.http import HomeAssistantView
from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.util import dt as dt_util

from .bridge_api import DahuaBridgeAPIError
from .const import DOMAIN

_LOGGER = logging.getLogger(__name__)

_REGISTERED_KEY = "_timeframe_proxy_registered"
_TIMEFRAME_LAYOUT = "%Y_%m_%d_%H_%M_%S"
_DEFAULT_DURATION = timedelta(minutes=30)
_MAX_DURATION = timedelta(hours=6)


def async_register_timeframe_proxy_view(hass: HomeAssistant) -> None:
    domain_data = hass.data.setdefault(DOMAIN, {})
    if domain_data.get(_REGISTERED_KEY):
        return

    hass.http.register_view(DahuaBridgeTimeframeProxyView())
    domain_data[_REGISTERED_KEY] = True


class DahuaBridgeTimeframeProxyView(HomeAssistantView):
    url = "/api/camera_proxy/{entity_id}/timeframe"
    extra_urls = ["/api/camera_proxy/{entity_id}/timeframe/"]
    name = "api:dahuabridge:camera_proxy_timeframe"
    requires_auth = True

    async def get(self, request: web.Request, entity_id: str) -> web.StreamResponse:
        hass: HomeAssistant = request.app["hass"]
        entity_id = unquote(entity_id).strip()
        state = hass.states.get(entity_id)
        if state is None:
            return _json_error(404, f"Camera entity {entity_id!r} was not found")

        attrs = state.attributes
        coordinator = _resolve_coordinator(hass, attrs)
        if coordinator is None:
            return _json_error(503, "DahuaBridge coordinator is unavailable")

        channel = _positive_int(attrs.get("bridge_channel"))
        if channel is None:
            return _json_error(400, "Camera entity does not expose bridge_channel")

        start_time = _parse_query_datetime(
            hass,
            _query_value(request.query, "starttime", "start_time", "start"),
            "starttime",
        )
        if start_time is None:
            return _json_error(400, "starttime must use Y_m_d_H_i_S")

        end_time = _parse_query_datetime(
            hass,
            _query_value(request.query, "endtime", "end_time", "end"),
            "endtime",
        )
        if end_time is None:
            end_time = start_time + _DEFAULT_DURATION
        compare_start_time = _datetime_for_compare(hass, start_time)
        compare_end_time = _datetime_for_compare(hass, end_time)
        if compare_end_time <= compare_start_time:
            return _json_error(400, "endtime must be after starttime")
        if compare_end_time - compare_start_time > _MAX_DURATION:
            return _json_error(400, "timeframe duration is too large")

        playback_url = _playback_sessions_url(coordinator, attrs)
        if playback_url is None:
            return _json_error(400, "Camera entity does not expose playback support")

        payload = {
            "channel": channel,
            "start_time": _bridge_playback_datetime(start_time),
            "end_time": _bridge_playback_datetime(end_time),
            "seek_time": _bridge_playback_datetime(start_time),
        }

        try:
            session_payload = await coordinator.api.async_post_json(playback_url, payload)
        except DahuaBridgeAPIError as err:
            _LOGGER.warning(
                "Failed to create DahuaBridge timeframe playback for %s: %s",
                entity_id,
                err,
            )
            return _json_error(502, "Bridge playback session request failed")

        profile_name = request.query.get("profile", "quality")
        mjpeg_url = _select_mjpeg_url(session_payload, profile_name)
        if mjpeg_url is None:
            return _json_error(502, "Bridge playback session did not expose MJPEG")

        upstream_url = coordinator.api.bridge_resource_url(mjpeg_url)
        return await _proxy_stream(request, hass, upstream_url)


async def _proxy_stream(
    request: web.Request,
    hass: HomeAssistant,
    upstream_url: str,
) -> web.StreamResponse:
    session = async_get_clientsession(hass)
    try:
        async with session.get(upstream_url) as upstream:
            if upstream.status >= 400:
                body = await upstream.text()
                _LOGGER.warning(
                    "DahuaBridge timeframe stream failed: GET %s returned %s: %s",
                    upstream_url,
                    upstream.status,
                    body,
                )
                return _json_error(502, "Bridge timeframe stream request failed")

            response = web.StreamResponse(
                status=200,
                headers={
                    "Cache-Control": "no-store",
                    "Content-Type": upstream.headers.get(
                        "Content-Type",
                        "multipart/x-mixed-replace",
                    ),
                    "X-Accel-Buffering": "no",
                },
            )
            await response.prepare(request)
            async for chunk in upstream.content.iter_chunked(64 * 1024):
                if chunk:
                    await response.write(chunk)
            await response.write_eof()
            return response
    except ClientError as err:
        _LOGGER.warning(
            "DahuaBridge timeframe stream failed: GET %s raised %r",
            upstream_url,
            err,
        )
        return _json_error(502, "Bridge timeframe stream request failed")


def _resolve_coordinator(hass: HomeAssistant, attrs: Mapping[str, Any]) -> Any | None:
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


def _playback_sessions_url(coordinator: Any, attrs: Mapping[str, Any]) -> str | None:
    playback_url = str(attrs.get("bridge_playback_sessions_url", "")).strip()
    if playback_url:
        return playback_url

    root_device_id = str(attrs.get("bridge_root_device_id", "")).strip()
    if not root_device_id:
        return None
    return coordinator.api.absolute_url(
        f"/api/v1/nvr/{quote(root_device_id, safe='')}/playback/sessions"
    )


def _select_mjpeg_url(
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


def _parse_query_datetime(
    hass: HomeAssistant,
    raw: str | None,
    field: str,
) -> datetime | None:
    if raw is None:
        return None

    value = raw.strip().lstrip("=")
    if not value:
        return None

    try:
        return datetime.strptime(value, _TIMEFRAME_LAYOUT)
    except ValueError:
        parsed = dt_util.parse_datetime(value)
        if parsed is None:
            _LOGGER.debug("Invalid DahuaBridge timeframe %s value %r", field, raw)
            return None

    if parsed.tzinfo is None:
        parsed = _with_timezone(parsed, _local_time_zone(hass))
    return parsed


def _datetime_for_compare(hass: HomeAssistant, value: datetime) -> datetime:
    if value.tzinfo is not None:
        return value
    return _with_timezone(value, _local_time_zone(hass))


def _bridge_playback_datetime(value: datetime) -> str:
    if value.tzinfo is None:
        return value.strftime("%Y-%m-%d %H:%M:%S")
    return value.astimezone(timezone.utc).isoformat()


def _local_time_zone(hass: HomeAssistant) -> Any:
    time_zone = getattr(getattr(hass, "config", None), "time_zone", None)
    if time_zone:
        zone = dt_util.get_time_zone(time_zone)
        if zone is not None:
            return zone
    return dt_util.DEFAULT_TIME_ZONE


def _with_timezone(value: datetime, zone: Any) -> datetime:
    localize = getattr(zone, "localize", None)
    if callable(localize):
        return localize(value)
    return value.replace(tzinfo=zone)


def _query_value(query: Mapping[str, str], *names: str) -> str | None:
    for name in names:
        value = query.get(name)
        if value:
            return value
    return None


def _positive_int(value: Any) -> int | None:
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        return None
    return parsed if parsed > 0 else None


def _json_error(status: int, message: str) -> web.Response:
    return web.json_response({"error": message}, status=status)
