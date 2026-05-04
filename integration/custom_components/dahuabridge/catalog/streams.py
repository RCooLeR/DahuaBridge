from __future__ import annotations

from typing import Any

from .accessors import stream_for_record
from .fields import merged_fields_for_record


def stream_source_for_record(record: dict[str, Any] | None) -> str | None:
    return stream_source_for_record_with_preferences(record, "auto", "auto")


def stream_source_for_record_with_preferences(
    record: dict[str, Any] | None,
    preferred_profile: str = "auto",
    preferred_source: str = "auto",
) -> str | None:
    stream = stream_for_record(record)
    profiles = stream.get("profiles", {})
    if not isinstance(profiles, dict):
        return None

    order = profile_order_for_record(record, preferred_profile)
    source_order = source_order_for_preference(preferred_source)
    seen: set[str] = set()
    for name in order:
        if not name or name in seen:
            continue
        seen.add(name)
        profile = profiles.get(name, {})
        if not isinstance(profile, dict):
            continue
        for key in source_order:
            value = str(profile.get(key, "")).strip()
            if value:
                return value
    return None


def mjpeg_url_for_record_with_preferences(
    record: dict[str, Any] | None, preferred_profile: str = "auto"
) -> str | None:
    stream = stream_for_record(record)
    profiles = stream.get("profiles", {})
    if not isinstance(profiles, dict):
        return None

    order = profile_order_for_record(record, preferred_profile)
    seen: set[str] = set()
    for name in order:
        if not name or name in seen:
            continue
        seen.add(name)
        profile = profiles.get(name, {})
        if not isinstance(profile, dict):
            continue
        value = str(profile.get("local_mjpeg_url", "")).strip()
        if value:
            return value
    return None


def snapshot_url_for_record(record: dict[str, Any] | None) -> str | None:
    stream = stream_for_record(record)
    value = str(stream.get("snapshot_url", "")).strip()
    return value or None


def stream_available_for_record(record: dict[str, Any] | None) -> bool:
    value = merged_fields_for_record(record).get("stream_available")
    if isinstance(value, bool):
        return value
    return stream_source_for_record(record) is not None


def profile_order_for_record(
    record: dict[str, Any] | None, preferred_profile: str = "auto"
) -> list[str]:
    stream = stream_for_record(record)
    recommended = normalize_profile_name(stream.get("recommended_profile", ""))
    preference = normalize_profile_name(preferred_profile) or "auto"
    if preference == "stable":
        return unique_profile_names("stable", recommended, "quality")
    if preference == "quality":
        return unique_profile_names("quality", recommended, "stable")
    return unique_profile_names(recommended, "quality", "stable")


def normalize_profile_name(raw: Any) -> str:
    value = str(raw or "").strip().lower()
    if value in {"", "auto", "quality", "stable"}:
        return value
    if value in {"default", "main"}:
        return "quality"
    if value in {"substream", "sub"}:
        return "stable"
    return value


def unique_profile_names(*names: str) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for raw_name in names:
        name = normalize_profile_name(raw_name)
        if not name or name == "auto" or name in seen:
            continue
        seen.add(name)
        result.append(name)
    return result


def source_order_for_preference(preferred_source: str = "auto") -> tuple[str, ...]:
    preference = str(preferred_source).strip().lower() or "auto"
    if preference == "rtsp":
        return ("stream_url", "local_hls_url", "local_mjpeg_url")
    if preference == "hls":
        return ("local_hls_url", "stream_url", "local_mjpeg_url")
    if preference == "mjpeg":
        return ("local_mjpeg_url", "local_hls_url", "stream_url")
    return ("local_hls_url", "stream_url", "local_mjpeg_url")
