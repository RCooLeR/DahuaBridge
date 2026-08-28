from __future__ import annotations

from urllib.parse import urlsplit, urlunsplit


_API_V1_PATH = "/api/v1"


def normalize_bridge_url(raw: str) -> str:
    value = raw.strip().rstrip("/")
    parsed = urlsplit(value)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("bridge URL must include http:// or https:// and a host")
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path.rstrip("/"), "", ""))


def is_absolute_target(target: str) -> bool:
    parsed = urlsplit(target.strip())
    return bool(parsed.scheme)


def canonical_bridge_api_path(target_path: str) -> str:
    """Remove an advertised proxy prefix from a bridge API resource path."""
    normalized = target_path or "/"
    if not normalized.startswith("/"):
        normalized = "/" + normalized

    search_end = len(normalized)
    while search_end > 0:
        marker_index = normalized.rfind(_API_V1_PATH, 0, search_end)
        if marker_index < 0:
            return normalized
        suffix_index = marker_index + len(_API_V1_PATH)
        if suffix_index == len(normalized) or normalized[suffix_index] == "/":
            return normalized[marker_index:]
        search_end = marker_index
    return normalized


def apply_base_path(base_path: str, target_path: str) -> str:
    normalized_base = base_path.rstrip("/")
    normalized_target = target_path or "/"
    if not normalized_target.startswith("/"):
        normalized_target = "/" + normalized_target
    if not normalized_base:
        return normalized_target
    if normalized_target == normalized_base or normalized_target.startswith(
        normalized_base + "/"
    ):
        return normalized_target
    return normalized_base + normalized_target
