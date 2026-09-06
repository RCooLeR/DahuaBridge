from __future__ import annotations

from typing import Any
from urllib.parse import quote, urlsplit

from ..catalog import (
    capture_for_record,
    controls_for_record,
    device_for_record,
    device_id_for_record,
    features_for_record,
    intercom_for_record,
    parent_id_for_record,
    snapshot_url_for_record,
    stream_available_for_record,
    stream_for_record,
    stream_source_for_record_with_preferences,
)
from .urls import resolve_bridge_urls


def camera_extra_state_attributes(
    api: Any,
    record: dict[str, Any] | None,
    preferred_video_profile: str,
    preferred_video_source: str,
    video_fallbacks_enabled: bool,
    integration_language: str,
) -> dict[str, Any]:
    stream = stream_for_record(record)
    device = device_for_record(record)
    device_id = device_id_for_record(record or {})
    parent_id = parent_id_for_record(record or {})
    attrs: dict[str, Any] = {}
    capture = capture_for_record(record)

    recommended = str(stream.get("recommended_profile", "")).strip()
    if recommended:
        attrs["recommended_profile"] = recommended

    snapshot_url = snapshot_url_for_record(record)
    if snapshot_url:
        attrs["snapshot_url"] = api.bridge_resource_url(snapshot_url)

    if capture:
        attrs["bridge_capture"] = resolve_bridge_urls(api, capture)
        attrs["bridge_recording_active"] = bool(capture.get("active", False))
        _copy_capture_url_attr(api, capture, attrs, "start_recording_url")
        _copy_capture_url_attr(api, capture, attrs, "stop_recording_url")
        _copy_capture_url_attr(api, capture, attrs, "recordings_url")

    source = stream_source_for_record_with_preferences(
        record,
        preferred_video_profile,
        preferred_video_source,
        video_fallbacks_enabled,
    )
    if source:
        attrs["stream_source"] = api.bridge_resource_url(source)

    attrs["bridge_base_url"] = api.base_url
    attrs["bridge_device_id"] = device_id
    attrs["bridge_root_device_id"] = parent_id or device_id
    attrs["bridge_device_kind"] = str(device.get("kind", "")).strip()
    attrs["bridge_device_name"] = str(device.get("name", "")).strip()
    attrs["bridge_integration_language"] = integration_language

    channel = stream.get("channel")
    if isinstance(channel, int):
        attrs["bridge_channel"] = channel
        if parent_id:
            attrs.update(
                _nvr_archive_attrs(
                    api,
                    parent_id,
                    channel,
                    _include_direct_archive_credentials(preferred_video_source)
                    and "/api/v1/rtsp/" not in urlsplit(source or "").path,
                )
            )

    attrs["stream_available"] = stream_available_for_record(record)
    attrs["preferred_video_profile"] = preferred_video_profile
    attrs["preferred_video_source"] = preferred_video_source
    attrs["video_fallbacks_enabled"] = video_fallbacks_enabled

    live_source = stream.get("live_source")
    if isinstance(live_source, dict):
        attrs["bridge_live_source"] = resolve_bridge_urls(api, live_source)
    elif str(device.get("kind", "")).strip() == "nvr_channel":
        attrs["bridge_live_source"] = {"source": "nvr", "camera_available": False}

    _copy_stream_url_attr(api, stream, attrs, "local_preview_url", "preview_url")
    _copy_stream_url_attr(
        api, stream, attrs, "local_intercom_url", "bridge_local_intercom_url"
    )
    _copy_stream_url_attr(api, stream, attrs, "onvif_stream_url", "bridge_onvif_stream_url")
    _copy_stream_url_attr(
        api, stream, attrs, "onvif_snapshot_url", "bridge_onvif_snapshot_url"
    )

    profiles = stream.get("profiles")
    if isinstance(profiles, dict) and profiles:
        attrs["bridge_profiles"] = resolve_bridge_urls(api, profiles)

    controls = controls_for_record(record)
    if controls:
        attrs["bridge_controls"] = resolve_bridge_urls(api, controls)

    features = features_for_record(record)
    if features:
        attrs["bridge_features"] = resolve_bridge_urls(api, features)

    intercom = intercom_for_record(record)
    if intercom:
        attrs["bridge_intercom"] = resolve_bridge_urls(api, intercom)

    return attrs


def _copy_capture_url_attr(
    api: Any,
    capture: dict[str, Any],
    attrs: dict[str, Any],
    key: str,
) -> None:
    value = str(capture.get(key, "")).strip()
    if not value:
        return
    attrs[f"bridge_{key}"] = api.bridge_resource_url(value)


def _copy_stream_url_attr(
    api: Any,
    stream: dict[str, Any],
    attrs: dict[str, Any],
    key: str,
    attr_key: str,
) -> None:
    value = str(stream.get(key, "")).strip()
    if value:
        attrs[attr_key] = api.bridge_resource_url(value)


def _nvr_archive_attrs(
    api: Any,
    parent_id: str,
    channel: int,
    include_credentials: bool,
) -> dict[str, str]:
    encoded_parent_id = quote(parent_id, safe="")
    chunks_template = (
        api.absolute_url(
            f"/api/v1/nvr/{encoded_parent_id}/recording-chunks"
            f"?channel={channel}&start={{start}}&end={{end}}&limit={{limit}}"
        )
    )
    smd_ivs_credentials = "&include_credentials=true" if include_credentials else ""
    smd_ivs_template = (
        api.absolute_url(
            f"/api/v1/nvr/{encoded_parent_id}/smd-ivs"
            f"?channel={channel}&start={{start}}&end={{end}}&limit={{limit}}&event={{event}}{smd_ivs_credentials}"
        )
    )
    return {
        "bridge_archive_smd_ivs_url_template": smd_ivs_template,
        "bridge_archive_recording_chunks_url_template": chunks_template,
        "bridge_archive_recordings_url_template": chunks_template,
        "bridge_archive_export_url": api.absolute_url(
            f"/api/v1/nvr/{encoded_parent_id}/recordings/export"
        ),
        "bridge_playback_sessions_url": api.absolute_url(
            f"/api/v1/nvr/{encoded_parent_id}/playback/sessions"
        ),
        "bridge_archive_coverage_url": (
            api.absolute_url(f"/api/v1/nvr/{encoded_parent_id}/recordings/coverage?channel={channel}")
        ),
    }


def _include_direct_archive_credentials(preferred_video_source: str) -> bool:
    return str(preferred_video_source or "").strip().lower() in {
        "rtsp",
        "native",
        "direct",
        "direct_rtsp",
        "ha",
        "homeassistant",
        "home_assistant",
    }
