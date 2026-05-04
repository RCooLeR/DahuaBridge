from __future__ import annotations

import logging
from collections.abc import Sequence
from typing import Any

from aiohttp import ClientError, web
from homeassistant.core import HomeAssistant
from homeassistant.helpers.aiohttp_client import async_get_clientsession

_LOGGER = logging.getLogger(__name__)


async def proxy_stream(
    request: web.Request,
    hass: HomeAssistant,
    upstream_url: str,
) -> web.StreamResponse:
    return await proxy_first_available_stream(request, hass, [upstream_url])


async def proxy_first_available_stream(
    request: web.Request,
    hass: HomeAssistant,
    upstream_urls: Sequence[str],
) -> web.StreamResponse:
    session = async_get_clientsession(hass)
    for upstream_url in upstream_urls:
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
                    continue

                return await stream_upstream_response(request, upstream)
        except ClientError as err:
            _LOGGER.warning(
                "DahuaBridge timeframe stream failed: GET %s raised %r",
                upstream_url,
                err,
            )
    return json_error(502, "Bridge timeframe stream request failed")


async def stream_upstream_response(
    request: web.Request,
    upstream: Any,
) -> web.StreamResponse:
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


def json_error(status: int, message: str) -> web.Response:
    return web.json_response({"error": message}, status=status)
