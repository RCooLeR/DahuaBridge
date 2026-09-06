from __future__ import annotations

from typing import Any
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit


def with_requested_width(target: str, width: int | None) -> str:
    if width is None or width <= 0:
        return target

    parsed = urlsplit(target)
    query = dict(parse_qsl(parsed.query, keep_blank_values=True))
    query["width"] = str(width)
    return urlunsplit(
        (
            parsed.scheme,
            parsed.netloc,
            parsed.path,
            urlencode(query),
            parsed.fragment,
        )
    )


def resolve_bridge_urls(api: Any, value: Any) -> Any:
    if isinstance(value, dict):
        return {
            str(key): resolve_bridge_urls(api, nested_value)
            for key, nested_value in value.items()
        }
    if isinstance(value, list):
        return [resolve_bridge_urls(api, item) for item in value]
    if isinstance(value, str) and looks_like_bridge_path(value):
        return api.bridge_resource_url(value)
    return value


def looks_like_bridge_path(value: str) -> bool:
    text = value.strip()
    if not text:
        return False
    parsed = urlsplit(text)
    if parsed.scheme == "rtsp" and parsed.path.startswith("/api/v1/rtsp/live/"):
        return True
    return text.startswith("/") or text.startswith("http://") or text.startswith(
        "https://"
    )
