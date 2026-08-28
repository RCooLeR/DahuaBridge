from __future__ import annotations

import logging
from collections.abc import Mapping
from datetime import timedelta
from typing import Any
from urllib.parse import quote, unquote, urlencode

from aiohttp import web
from homeassistant.components.http import KEY_AUTHENTICATED, HomeAssistantView
from homeassistant.core import HomeAssistant

from .api import DahuaBridgeAPIError
from .proxy import (
    bridge_playback_datetime,
    datetime_for_compare,
    json_error,
    parse_query_datetime,
    playback_sessions_url,
    positive_int,
    proxy_first_available_stream,
    query_value,
    resolve_coordinator,
    select_mjpeg_urls,
)

_LOGGER = logging.getLogger(__name__)

_DEFAULT_DURATION = timedelta(minutes=30)
_MAX_DURATION = timedelta(hours=6)


def async_register_timeframe_proxy_view(hass: HomeAssistant) -> None:
    hass.http.register_view(DahuaBridgeTimeframeProxyView())


class DahuaBridgeTimeframeProxyView(HomeAssistantView):
    url = "/api/camera_proxy/{entity_id}/timeframe"
    extra_urls = [
        "/api/camera_proxy/{entity_id}/timeframe/",
        "/api/camera_proxy/{entity_id}/timeframe/snapshot",
        "/api/camera_proxy/{entity_id}/timeframe/snapshot/",
    ]
    name = "api:dahuabridge:camera_proxy_timeframe"
    requires_auth = False

    async def get(self, request: web.Request, entity_id: str) -> web.StreamResponse:
        playback_context = await self._create_playback_context(request, entity_id)
        if isinstance(playback_context, web.StreamResponse):
            return playback_context

        if request.path.rstrip("/").endswith("/snapshot"):
            return await self._snapshot(request, playback_context)
        return await self._stream(request, playback_context)

    async def _create_playback_context(
        self, request: web.Request, entity_id: str
    ) -> dict[str, Any] | web.StreamResponse:
        hass: HomeAssistant = request.app["hass"]
        entity_id = unquote(entity_id).strip()
        state = hass.states.get(entity_id)
        if state is None:
            return json_error(404, f"Camera entity {entity_id!r} was not found")

        attrs = state.attributes
        if not _camera_proxy_request_authenticated(request, attrs):
            return json_error(403, "Camera proxy authentication failed")

        coordinator = resolve_coordinator(hass, attrs)
        if coordinator is None:
            return json_error(503, "DahuaBridge coordinator is unavailable")

        channel = positive_int(attrs.get("bridge_channel"))
        if channel is None:
            return json_error(400, "Camera entity does not expose bridge_channel")

        start_time = parse_query_datetime(
            hass,
            query_value(request.query, "starttime", "start_time", "start"),
            "starttime",
        )
        if start_time is None:
            return json_error(400, "starttime must use Y_m_d_H_i_S")

        end_time = parse_query_datetime(
            hass,
            query_value(request.query, "endtime", "end_time", "end"),
            "endtime",
        )
        if end_time is None:
            end_time = start_time + _DEFAULT_DURATION
        seek_time = parse_query_datetime(
            hass,
            query_value(request.query, "seektime", "seek_time", "seek"),
            "seektime",
        )
        if seek_time is None:
            seek_time = start_time
        compare_start_time = datetime_for_compare(hass, start_time)
        compare_end_time = datetime_for_compare(hass, end_time)
        compare_seek_time = datetime_for_compare(hass, seek_time)
        if compare_end_time <= compare_start_time:
            return json_error(400, "endtime must be after starttime")
        if compare_seek_time < compare_start_time or compare_seek_time > compare_end_time:
            return json_error(400, "seektime must be within the timeframe")
        if compare_end_time - compare_start_time > _MAX_DURATION:
            return json_error(400, "timeframe duration is too large")

        playback_url = playback_sessions_url(coordinator, attrs)
        if playback_url is None:
            return json_error(400, "Camera entity does not expose playback support")

        payload = {
            "channel": channel,
            "start_time": bridge_playback_datetime(start_time),
            "end_time": bridge_playback_datetime(end_time),
            "seek_time": bridge_playback_datetime(seek_time),
        }

        try:
            session_payload = await coordinator.api.async_post_json(playback_url, payload)
        except DahuaBridgeAPIError as err:
            _LOGGER.warning(
                "Failed to create DahuaBridge timeframe playback for %s: %s",
                entity_id,
                err,
            )
            return json_error(502, "Bridge playback session request failed")

        profile_name = request.query.get("profile", "quality")
        return {
            "hass": hass,
            "coordinator": coordinator,
            "entity_id": entity_id,
            "profile_name": profile_name,
            "session_payload": session_payload,
        }

    async def _stream(
        self, request: web.Request, playback_context: Mapping[str, Any]
    ) -> web.StreamResponse:
        coordinator = playback_context["coordinator"]
        profile_name = str(playback_context["profile_name"])
        session_payload = playback_context["session_payload"]
        mjpeg_urls = select_mjpeg_urls(session_payload, profile_name)
        if not mjpeg_urls:
            return json_error(502, "Bridge playback session did not expose MJPEG")

        upstream_urls = [
            coordinator.api.bridge_resource_url(mjpeg_url)
            for mjpeg_url in mjpeg_urls
        ]
        return await proxy_first_available_stream(
            request,
            playback_context["hass"],
            upstream_urls,
        )

    async def _snapshot(
        self, request: web.Request, playback_context: Mapping[str, Any]
    ) -> web.StreamResponse:
        coordinator = playback_context["coordinator"]
        session_payload = playback_context["session_payload"]
        stream_id = str(
            session_payload.get("stream_id") or session_payload.get("id") or ""
        ).strip()
        if not stream_id:
            return json_error(502, "Bridge playback session did not expose stream_id")

        query = {"profile": str(playback_context["profile_name"])}
        snapshot_path = (
            f"/api/v1/media/snapshot/{quote(stream_id, safe='')}?{urlencode(query)}"
        )
        try:
            body = await coordinator.api.async_get_bytes(snapshot_path)
        except DahuaBridgeAPIError as err:
            _LOGGER.warning(
                "Failed to capture DahuaBridge timeframe snapshot for %s: %s",
                playback_context["entity_id"],
                err,
            )
            return json_error(502, "Bridge timeframe snapshot request failed")

        return web.Response(
            body=body,
            status=200,
            headers={
                "Cache-Control": "no-store",
                "Content-Type": "image/jpeg",
            },
        )


def _camera_proxy_request_authenticated(
    request: web.Request, attrs: Mapping[str, Any]
) -> bool:
    if request.get(KEY_AUTHENTICATED, False):
        return True

    access_token = str(attrs.get("access_token", "")).strip()
    return bool(access_token) and request.query.get("token") == access_token
