from __future__ import annotations

from urllib.parse import urlsplit, urlunsplit


def normalize_bridge_url(raw: str) -> str:
    value = raw.strip().rstrip("/")
    parsed = urlsplit(value)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("bridge URL must include http:// or https:// and a host")
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path.rstrip("/"), "", ""))


def is_absolute_target(target: str) -> bool:
    parsed = urlsplit(target.strip())
    return bool(parsed.scheme)


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
