from __future__ import annotations

import logging
from datetime import timedelta
from urllib.parse import unquote

from aiohttp import web
from homeassistant.components.http import HomeAssistantView
from homeassistant.core import HomeAssistant

from .api import DahuaBridgeAPIError
from .const import DOMAIN
from .proxy import (
    bridge_playback_datetime,
    datetime_for_compare,
    json_error,
    parse_query_datetime,
    playback_sessions_url,
    positive_int,
    proxy_stream,
    query_value,
    resolve_coordinator,
    select_mjpeg_url,
)

_LOGGER = logging.getLogger(__name__)

_REGISTERED_KEY = "_timeframe_proxy_registered"
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
            return json_error(404, f"Camera entity {entity_id!r} was not found")

        attrs = state.attributes
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
        compare_start_time = datetime_for_compare(hass, start_time)
        compare_end_time = datetime_for_compare(hass, end_time)
        if compare_end_time <= compare_start_time:
            return json_error(400, "endtime must be after starttime")
        if compare_end_time - compare_start_time > _MAX_DURATION:
            return json_error(400, "timeframe duration is too large")

        playback_url = playback_sessions_url(coordinator, attrs)
        if playback_url is None:
            return json_error(400, "Camera entity does not expose playback support")

        payload = {
            "channel": channel,
            "start_time": bridge_playback_datetime(start_time),
            "end_time": bridge_playback_datetime(end_time),
            "seek_time": bridge_playback_datetime(start_time),
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
        mjpeg_url = select_mjpeg_url(session_payload, profile_name)
        if mjpeg_url is None:
            return json_error(502, "Bridge playback session did not expose MJPEG")

        upstream_url = coordinator.api.bridge_resource_url(mjpeg_url)
        return await proxy_stream(request, hass, upstream_url)
