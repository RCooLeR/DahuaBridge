from __future__ import annotations

import json
import logging
import time
from typing import Any
from urllib.parse import urlsplit, urlunsplit

from aiohttp import ClientError, ClientSession

from .const import CATALOG_PATH, STATUS_PATH

_LOGGER = logging.getLogger(__name__)


class DahuaBridgeAPIError(Exception):
    """Raised when the bridge API request fails."""


def normalize_bridge_url(raw: str) -> str:
    value = raw.strip().rstrip("/")
    parsed = urlsplit(value)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("bridge URL must include http:// or https:// and a host")
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path.rstrip("/"), "", ""))


class DahuaBridgeAPI:
    def __init__(self, session: ClientSession, base_url: str) -> None:
        self._session = session
        self._base_url = normalize_bridge_url(base_url)

    @property
    def base_url(self) -> str:
        return self._base_url

    def absolute_url(self, target: str) -> str:
        return self._absolute_url(target)

    def bridge_resource_url(self, target: str) -> str:
        parsed_target = urlsplit(target)
        if parsed_target.scheme not in {"http", "https"}:
            return self._absolute_url(target)

        parsed_base = urlsplit(self._base_url)
        return urlunsplit(
            (
                parsed_base.scheme,
                parsed_base.netloc,
                parsed_target.path,
                parsed_target.query,
                parsed_target.fragment,
            )
        )

    async def async_get_status(self) -> dict[str, Any]:
        return await self._async_request_json("GET", STATUS_PATH)

    async def async_get_catalog(self, include_credentials: bool = False) -> dict[str, Any]:
        target = CATALOG_PATH
        if include_credentials:
            target = f"{CATALOG_PATH}?include_credentials=true"
        return await self._async_request_json("GET", target)

    async def async_get_bytes(self, target: str) -> bytes:
        url = self._absolute_url(target)
        started = time.monotonic()
        try:
            _LOGGER.debug("Requesting bridge bytes from %s", url)
            async with self._session.get(url) as response:
                if response.status >= 400:
                    body = await response.text()
                    _LOGGER.warning(
                        "Bridge request failed: GET %s returned %s with body %s",
                        url,
                        response.status,
                        body,
                    )
                    raise DahuaBridgeAPIError(
                        f"GET {url} returned {response.status}: {body}"
                    )
                payload = await response.read()
                _LOGGER.debug(
                    "Bridge bytes response: GET %s returned %s in %.3fs (%d bytes)",
                    url,
                    response.status,
                    time.monotonic() - started,
                    len(payload),
                )
                return payload
        except ClientError as err:
            _LOGGER.warning("Bridge request failed: GET %s raised %r", url, err)
            raise DahuaBridgeAPIError(f"GET {url} failed: {err}") from err

    async def async_get_mjpeg_frame(self, target: str) -> bytes:
        url = self._absolute_url(target)
        started = time.monotonic()
        jpeg_start = b"\xff\xd8"
        jpeg_end = b"\xff\xd9"
        max_buffer = 4 * 1024 * 1024
        buffer = bytearray()

        try:
            _LOGGER.debug("Requesting bridge MJPEG frame from %s", url)
            async with self._session.get(url) as response:
                if response.status >= 400:
                    body = await response.text()
                    _LOGGER.warning(
                        "Bridge MJPEG request failed: GET %s returned %s with body %s",
                        url,
                        response.status,
                        body,
                    )
                    raise DahuaBridgeAPIError(
                        f"GET {url} returned {response.status}: {body}"
                    )

                async for chunk in response.content.iter_chunked(4096):
                    if not chunk:
                        continue
                    buffer.extend(chunk)
                    start = buffer.find(jpeg_start)
                    if start >= 0:
                        end = buffer.find(jpeg_end, start + 2)
                        if end >= 0:
                            frame = bytes(buffer[start : end + 2])
                            _LOGGER.debug(
                                "Bridge MJPEG frame extracted from %s with status %s in %.3fs (%d bytes)",
                                url,
                                response.status,
                                time.monotonic() - started,
                                len(frame),
                            )
                            return frame
                    if len(buffer) > max_buffer:
                        del buffer[: len(buffer) - max_buffer]
        except ClientError as err:
            _LOGGER.warning("Bridge MJPEG request failed: GET %s raised %r", url, err)
            raise DahuaBridgeAPIError(f"GET {url} failed: {err}") from err

        raise DahuaBridgeAPIError(f"GET {url} did not yield a JPEG frame")

    async def async_post_action(self, target: str) -> dict[str, Any]:
        return await self._async_request_json("POST", target)

    async def async_post_json(
        self, target: str, payload: dict[str, Any]
    ) -> dict[str, Any]:
        return await self._async_request_json("POST", target, payload)

    async def _async_request_json(
        self, method: str, target: str, payload: dict[str, Any] | None = None
    ) -> dict[str, Any]:
        url = self._absolute_url(target)
        started = time.monotonic()
        status = 0
        try:
            _LOGGER.debug("Requesting bridge JSON via %s %s", method, url)
            async with self._session.request(method, url, json=payload) as response:
                status = response.status
                body = await response.text()
                duration = time.monotonic() - started
                if response.status >= 400:
                    _LOGGER.warning(
                        "Bridge request failed: %s %s returned %s in %.3fs with body %s",
                        method,
                        url,
                        response.status,
                        duration,
                        body,
                    )
                    raise DahuaBridgeAPIError(
                        f"{method} {url} returned {response.status}: {body}"
                    )
        except ClientError as err:
            _LOGGER.warning(
                "Bridge request failed: %s %s raised %r", method, url, err
            )
            raise DahuaBridgeAPIError(f"{method} {url} failed: {err}") from err

        if not body.strip():
            _LOGGER.debug(
                "Bridge request returned empty JSON body: %s %s returned %s in %.3fs",
                method,
                url,
                status,
                time.monotonic() - started,
            )
            return {}

        try:
            payload = json.loads(body)
        except json.JSONDecodeError as err:
            _LOGGER.warning(
                "Bridge request returned invalid JSON: %s %s body=%s error=%r",
                method,
                url,
                body,
                err,
            )
            raise DahuaBridgeAPIError(
                f"{method} {url} returned invalid JSON: {err}"
            ) from err

        if not isinstance(payload, dict):
            _LOGGER.warning(
                "Bridge request returned unexpected payload type: %s %s type=%s",
                method,
                url,
                type(payload).__name__,
            )
            raise DahuaBridgeAPIError(
                f"{method} {url} returned unexpected payload type"
            )
        _LOGGER.debug(
            "Bridge JSON response: %s %s returned %s in %.3fs",
            method,
            url,
            status,
            time.monotonic() - started,
        )
        return payload

    def _absolute_url(self, target: str) -> str:
        if target.startswith("http://") or target.startswith("https://"):
            return target
        if not target.startswith("/"):
            target = "/" + target
        return self._base_url + target
