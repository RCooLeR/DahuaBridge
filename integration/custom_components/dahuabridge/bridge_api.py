from __future__ import annotations

import json
from typing import Any
from urllib.parse import urlsplit, urlunsplit

from aiohttp import ClientError, ClientSession

from .const import CATALOG_PATH, STATUS_PATH


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

    async def async_get_status(self) -> dict[str, Any]:
        return await self._async_request_json("GET", STATUS_PATH)

    async def async_get_catalog(self) -> dict[str, Any]:
        return await self._async_request_json("GET", CATALOG_PATH)

    async def async_get_bytes(self, target: str) -> bytes:
        url = self._absolute_url(target)
        try:
            async with self._session.get(url) as response:
                if response.status >= 400:
                    raise DahuaBridgeAPIError(
                        f"GET {url} returned {response.status}: {await response.text()}"
                    )
                return await response.read()
        except ClientError as err:
            raise DahuaBridgeAPIError(f"GET {url} failed: {err}") from err

    async def async_post_action(self, target: str) -> dict[str, Any]:
        return await self._async_request_json("POST", target)

    async def _async_request_json(
        self, method: str, target: str
    ) -> dict[str, Any]:
        url = self._absolute_url(target)
        try:
            async with self._session.request(method, url) as response:
                body = await response.text()
                if response.status >= 400:
                    raise DahuaBridgeAPIError(
                        f"{method} {url} returned {response.status}: {body}"
                    )
        except ClientError as err:
            raise DahuaBridgeAPIError(f"{method} {url} failed: {err}") from err

        if not body.strip():
            return {}

        try:
            payload = json.loads(body)
        except json.JSONDecodeError as err:
            raise DahuaBridgeAPIError(
                f"{method} {url} returned invalid JSON: {err}"
            ) from err

        if not isinstance(payload, dict):
            raise DahuaBridgeAPIError(
                f"{method} {url} returned unexpected payload type"
            )
        return payload

    def _absolute_url(self, target: str) -> str:
        if target.startswith("http://") or target.startswith("https://"):
            return target
        if not target.startswith("/"):
            target = "/" + target
        return self._base_url + target
