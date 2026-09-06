from __future__ import annotations

import json
import logging
import time
from typing import Any
from urllib.parse import parse_qsl, quote, urlencode, urlsplit, urlunsplit

from aiohttp import ClientError, ClientSession

from ..const import CATALOG_PATH, LIVE_PRECONNECT_PATH, LIVE_SOURCE_PATH, STATUS_PATH
from .errors import DahuaBridgeAPIError
from .urls import (
    apply_base_path,
    canonical_bridge_api_path,
    is_absolute_target,
    normalize_bridge_url,
)

_LOGGER = logging.getLogger(__name__)


class DahuaBridgeAPI:
    def __init__(self, session: ClientSession, base_url: str, api_token: str = "") -> None:
        self._session = session
        self._base_url = normalize_bridge_url(base_url)
        self._api_token = str(api_token or "").strip()

    @property
    def base_url(self) -> str:
        return self._base_url

    def absolute_url(self, target: str) -> str:
        """Build a browser resource URL, preserving an advertised absolute host."""
        return self._with_query_token(self._absolute_url(target))

    def bridge_resource_url(self, target: str) -> str:
        """Address a bridge-owned resource through HA's configured bridge host."""
        parsed_target = urlsplit(target)
        if parsed_target.scheme == "rtsp" and parsed_target.path.startswith("/api/v1/rtsp/live/"):
            # Relay addresses use the same host HA uses for bridge API calls.
            # The RTSP listener has its own port and no HTTP reverse-proxy prefix.
            host = urlsplit(self._base_url).hostname or parsed_target.hostname or "localhost"
            if ":" in host:
                host = f"[{host}]"
            authority = f"{host}:{parsed_target.port or 8554}"
            if self._api_token:
                authority = f"dahuabridge:{quote(self._api_token, safe='')}@{authority}"
            return urlunsplit(("rtsp", authority, parsed_target.path, "", ""))
        if parsed_target.scheme and parsed_target.scheme not in {"http", "https"}:
            return self._with_query_token(self._absolute_url(target))

        parsed_base = urlsplit(self._base_url)
        return self._with_query_token(
            urlunsplit(
                (
                    parsed_base.scheme,
                    parsed_base.netloc,
                    apply_base_path(
                        parsed_base.path,
                        canonical_bridge_api_path(parsed_target.path),
                    ),
                    parsed_target.query,
                    parsed_target.fragment,
                )
            )
        )

    async def async_get_status(self) -> dict[str, Any]:
        return await self._async_request_json("GET", STATUS_PATH)

    async def async_get_live_source(self) -> dict[str, Any]:
        result = await self._async_request_json("GET", LIVE_SOURCE_PATH)
        if result.get("source") not in {"nvr", "camera"}:
            raise DahuaBridgeAPIError("Bridge returned an invalid live-source preference")
        return result

    async def async_set_live_source(self, source: str) -> dict[str, Any]:
        if source not in {"nvr", "camera"}:
            raise DahuaBridgeAPIError("Live-source preference must be nvr or camera")
        result = await self._async_request_json("PUT", LIVE_SOURCE_PATH, {"source": source})
        if result.get("source") != source:
            raise DahuaBridgeAPIError("Bridge did not confirm the live-source preference")
        return result

    async def async_get_live_preconnect(self) -> dict[str, Any]:
        result = await self._async_request_json("GET", LIVE_PRECONNECT_PATH)
        if result.get("mode") not in {"off", "recent", "always"} or result.get("profile") not in {"auto", "quality", "stable"}:
            raise DahuaBridgeAPIError("Bridge returned invalid live preconnect settings")
        return result

    async def async_set_live_preconnect(self, mode: str, profile: str) -> dict[str, Any]:
        """Configure bridge transport warm-up, independently of HA camera preload."""
        if mode not in {"off", "recent", "always"} or profile not in {"auto", "quality", "stable"}:
            raise DahuaBridgeAPIError("Invalid live preconnect mode or video profile")
        settings = {"mode": mode, "profile": profile}
        result = await self._async_request_json("PUT", LIVE_PRECONNECT_PATH, settings)
        if any(result.get(key) != value for key, value in settings.items()):
            raise DahuaBridgeAPIError("Bridge did not confirm the live preconnect settings")
        return result

    async def async_get_catalog(self, include_credentials: bool = False) -> dict[str, Any]:
        target = CATALOG_PATH
        if include_credentials:
            target = f"{CATALOG_PATH}?include_credentials=true"
        result = await self._async_request_json("GET", target)
        if not isinstance(result.get("devices"), list):
            raise DahuaBridgeAPIError("Bridge returned an invalid camera catalog")
        return result

    async def async_get_bytes(self, target: str) -> bytes:
        url = self._bridge_request_url(target)
        started = time.monotonic()
        try:
            _LOGGER.debug("Requesting bridge bytes from %s", redact_url_for_log(url))
            async with self._session.get(url, headers=self._auth_headers()) as response:
                if response.status >= 400:
                    body = await response.text()
                    _LOGGER.warning(
                        "Bridge request failed: GET %s returned %s with body %s",
                        redact_url_for_log(url),
                        response.status,
                        body,
                    )
                    raise DahuaBridgeAPIError(
                        f"GET {redact_url_for_log(url)} returned {response.status}: {body}"
                    )
                payload = await response.read()
                _LOGGER.debug(
                    "Bridge bytes response: GET %s returned %s in %.3fs (%d bytes)",
                    redact_url_for_log(url),
                    response.status,
                    time.monotonic() - started,
                    len(payload),
                )
                return payload
        except ClientError as err:
            _LOGGER.warning("Bridge request failed: GET %s raised %r", redact_url_for_log(url), err)
            raise DahuaBridgeAPIError(f"GET {redact_url_for_log(url)} failed: {err}") from err

    async def async_get_mjpeg_frame(self, target: str) -> bytes:
        url = self._bridge_request_url(target)
        started = time.monotonic()
        jpeg_start = b"\xff\xd8"
        jpeg_end = b"\xff\xd9"
        max_buffer = 4 * 1024 * 1024
        buffer = bytearray()

        try:
            _LOGGER.debug("Requesting bridge MJPEG frame from %s", redact_url_for_log(url))
            async with self._session.get(url, headers=self._auth_headers()) as response:
                if response.status >= 400:
                    body = await response.text()
                    _LOGGER.warning(
                        "Bridge MJPEG request failed: GET %s returned %s with body %s",
                        redact_url_for_log(url),
                        response.status,
                        body,
                    )
                    raise DahuaBridgeAPIError(
                        f"GET {redact_url_for_log(url)} returned {response.status}: {body}"
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
                                redact_url_for_log(url),
                                response.status,
                                time.monotonic() - started,
                                len(frame),
                            )
                            return frame
                    if len(buffer) > max_buffer:
                        del buffer[: len(buffer) - max_buffer]
        except ClientError as err:
            _LOGGER.warning("Bridge MJPEG request failed: GET %s raised %r", redact_url_for_log(url), err)
            raise DahuaBridgeAPIError(f"GET {redact_url_for_log(url)} failed: {err}") from err

        raise DahuaBridgeAPIError(
            f"GET {redact_url_for_log(url)} did not yield a JPEG frame"
        )

    async def async_post_action(self, target: str) -> dict[str, Any]:
        return await self._async_request_json("POST", target)

    async def async_post_json(
        self, target: str, payload: dict[str, Any]
    ) -> dict[str, Any]:
        return await self._async_request_json("POST", target, payload)

    async def async_put_json(
        self, target: str, payload: dict[str, Any]
    ) -> dict[str, Any]:
        return await self._async_request_json("PUT", target, payload)

    async def _async_request_json(
        self, method: str, target: str, payload: dict[str, Any] | None = None
    ) -> dict[str, Any]:
        url = self._bridge_request_url(target)
        started = time.monotonic()
        status = 0
        try:
            _LOGGER.debug("Requesting bridge JSON via %s %s", method, redact_url_for_log(url))
            async with self._session.request(
                method, url, json=payload, headers=self._auth_headers()
            ) as response:
                status = response.status
                body = await response.text()
                duration = time.monotonic() - started
                if response.status >= 400:
                    _LOGGER.warning(
                        "Bridge request failed: %s %s returned %s in %.3fs with body %s",
                        method,
                        redact_url_for_log(url),
                        response.status,
                        duration,
                        body,
                    )
                    raise DahuaBridgeAPIError(
                        f"{method} {redact_url_for_log(url)} returned {response.status}: {body}",
                        status=response.status,
                    )
        except ClientError as err:
            _LOGGER.warning(
                "Bridge request failed: %s %s raised %r", method, redact_url_for_log(url), err
            )
            raise DahuaBridgeAPIError(f"{method} {redact_url_for_log(url)} failed: {err}") from err

        if not body.strip():
            _LOGGER.debug(
                "Bridge request returned empty JSON body: %s %s returned %s in %.3fs",
                method,
                redact_url_for_log(url),
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
                redact_url_for_log(url),
                body,
                err,
            )
            raise DahuaBridgeAPIError(
                f"{method} {redact_url_for_log(url)} returned invalid JSON: {err}"
            ) from err

        if not isinstance(payload, dict):
            _LOGGER.warning(
                "Bridge request returned unexpected payload type: %s %s type=%s",
                method,
                redact_url_for_log(url),
                type(payload).__name__,
            )
            raise DahuaBridgeAPIError(
                f"{method} {redact_url_for_log(url)} returned unexpected payload type"
            )
        _LOGGER.debug(
            "Bridge JSON response: %s %s returned %s in %.3fs",
            method,
            redact_url_for_log(url),
            status,
            time.monotonic() - started,
        )
        return payload

    def _absolute_url(self, target: str) -> str:
        if is_absolute_target(target):
            return target
        if not target.startswith("/"):
            target = "/" + target
        return self._base_url + target

    def _bridge_request_url(self, target: str) -> str:
        """Rebase advertised API actions before attaching HA's Bearer token."""
        parsed = urlsplit(target)
        path = canonical_bridge_api_path(parsed.path)
        if parsed.scheme not in {"", "http", "https"} or not (
            path == "/api/v1" or path.startswith("/api/v1/")
        ):
            raise DahuaBridgeAPIError("Request target is not a bridge API resource")
        base = urlsplit(self._base_url)
        return urlunsplit((base.scheme, base.netloc, apply_base_path(base.path, path), parsed.query, ""))

    def _auth_headers(self) -> dict[str, str] | None:
        if not self._api_token:
            return None
        return {"Authorization": f"Bearer {self._api_token}"}

    def _with_query_token(self, target: str) -> str:
        if not self._api_token:
            return target
        parsed = urlsplit(target)
        if parsed.scheme not in {"http", "https"}:
            return target
        query_items = parse_qsl(parsed.query, keep_blank_values=True)
        existing_keys = {key.lower() for key, _ in query_items}
        if "auth_token" not in existing_keys and "token" not in existing_keys:
            query_items.append(("auth_token", self._api_token))
        return urlunsplit(
            (
                parsed.scheme,
                parsed.netloc,
                parsed.path,
                urlencode(query_items),
                parsed.fragment,
            )
        )


def redact_url_for_log(value: str) -> str:
    parsed = urlsplit(value)
    authority = parsed.netloc
    if parsed.username is not None:
        authority = "**REDACTED**@" + authority.rsplit("@", 1)[-1]
    redacted_items: list[tuple[str, str]] = []
    for key, item in parse_qsl(parsed.query, keep_blank_values=True):
        lowered = key.lower()
        if "token" in lowered or "secret" in lowered or "password" in lowered:
            redacted_items.append((key, "**REDACTED**"))
        else:
            redacted_items.append((key, item))
    return urlunsplit(
        (
            parsed.scheme,
            authority,
            parsed.path,
            urlencode(redacted_items),
            parsed.fragment,
        )
    )
