from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Any
from urllib.parse import quote

from homeassistant.components.binary_sensor import BinarySensorDeviceClass
from homeassistant.components.sensor import SensorDeviceClass
from homeassistant.const import EntityCategory
from homeassistant.util import dt as dt_util


@dataclass(frozen=True)
class ButtonSpec:
    key: str
    name: str
    url: str
    icon: str


DIAGNOSTIC_FIELDS = {
    "audio_codec",
    "bridge_forward_errors",
    "bridge_forwarded_packets",
    "bridge_session_count",
    "bridge_uplink_codec",
    "bridge_uplink_packets",
    "build_date",
    "channel",
    "channel_count",
    "channel_index",
    "channel_number",
    "configured_external_uplink_target_count",
    "disk_count",
    "firmware",
    "free_bytes",
    "main_codec",
    "main_resolution",
    "model",
    "onvif_h264_available",
    "onvif_profile_name",
    "onvif_profile_token",
    "recommended_ha_integration",
    "recommended_ha_reason",
    "recommended_profile",
    "serial",
    "sub_codec",
    "sub_resolution",
    "total_bytes",
    "used_bytes",
    "used_percent",
}

BINARY_DEVICE_CLASSES = {
    "bridge_session_active": BinarySensorDeviceClass.RUNNING,
    "bridge_uplink_active": BinarySensorDeviceClass.RUNNING,
    "disk_fault": BinarySensorDeviceClass.PROBLEM,
    "external_uplink_enabled": BinarySensorDeviceClass.RUNNING,
    "human": BinarySensorDeviceClass.MOTION,
    "intrusion": BinarySensorDeviceClass.MOTION,
    "motion": BinarySensorDeviceClass.MOTION,
    "online": BinarySensorDeviceClass.CONNECTIVITY,
    "tamper": BinarySensorDeviceClass.TAMPER,
    "tripwire": BinarySensorDeviceClass.MOTION,
    "vehicle": BinarySensorDeviceClass.MOTION,
}

FIELD_NAMES = {
    "bridge_forward_errors": "Bridge Forward Errors",
    "bridge_forwarded_packets": "Bridge Forwarded Packets",
    "bridge_session_active": "Bridge Session Active",
    "bridge_session_count": "Bridge Session Count",
    "bridge_uplink_active": "Bridge Uplink Active",
    "bridge_uplink_codec": "Bridge Uplink Codec",
    "bridge_uplink_packets": "Bridge Uplink Packets",
    "call_state": "Call State",
    "channel_index": "Channel Index",
    "configured_external_uplink_target_count": "Configured RTP Export Targets",
    "disk_fault": "Disk Fault",
    "external_uplink_enabled": "RTP Export Enabled",
    "last_call_duration_seconds": "Last Call Duration",
    "last_call_ended_at": "Last Call Ended At",
    "last_call_source": "Last Call Source",
    "last_call_started_at": "Last Call Started At",
    "last_ring_at": "Last Ring At",
    "main_codec": "Main Codec",
    "main_resolution": "Main Resolution",
    "onvif_h264_available": "ONVIF H264 Available",
    "onvif_profile_name": "ONVIF Profile Name",
    "onvif_profile_token": "ONVIF Profile Token",
    "recommended_ha_integration": "Recommended HA Integration",
    "recommended_ha_reason": "Recommended HA Reason",
    "recommended_profile": "Recommended Profile",
    "sub_codec": "Sub Codec",
    "sub_resolution": "Sub Resolution",
    "used_percent": "Storage Used Percent",
}


def catalog_records(data: dict[str, Any] | None) -> list[dict[str, Any]]:
    if not data:
        return []
    records = data.get("devices", [])
    return [record for record in records if isinstance(record, dict)]


def record_by_device_id(
    data: dict[str, Any] | None, device_id: str
) -> dict[str, Any] | None:
    for record in catalog_records(data):
        if device_id_for_record(record) == device_id:
            return record
    return None


def device_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    if not record:
        return {}
    device = record.get("device", {})
    return device if isinstance(device, dict) else {}


def state_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    if not record:
        return {}
    state = record.get("state", {})
    return state if isinstance(state, dict) else {}


def info_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    info = state_for_record(record).get("info", {})
    return info if isinstance(info, dict) else {}


def attributes_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    attributes = device_for_record(record).get("attributes", {})
    if not isinstance(attributes, dict):
        return {}

    normalized: dict[str, Any] = {}
    for key, value in attributes.items():
        normalized[str(key)] = coerce_catalog_value(value)
    return normalized


def stream_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    if not record:
        return {}
    stream = record.get("stream", {})
    return stream if isinstance(stream, dict) else {}


def merged_fields_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    merged: dict[str, Any] = {}
    merged.update(attributes_for_record(record))
    merged.update(stream_fields_for_record(record))
    merged.update(info_for_record(record))
    return merged


def device_id_for_record(record: dict[str, Any]) -> str:
    return str(device_for_record(record).get("id", "")).strip()


def parent_id_for_record(record: dict[str, Any]) -> str:
    return str(device_for_record(record).get("parent_id", "")).strip()


def available_for_record(record: dict[str, Any] | None) -> bool:
    return bool(state_for_record(record).get("available", False))


def stream_source_for_record(record: dict[str, Any] | None) -> str | None:
    stream = stream_for_record(record)
    profiles = stream.get("profiles", {})
    if not isinstance(profiles, dict):
        return None

    preferred = str(stream.get("recommended_profile", "")).strip()
    order = [preferred, "stable", "default", "quality", "substream"]
    seen: set[str] = set()
    for name in order:
        if not name or name in seen:
            continue
        seen.add(name)
        profile = profiles.get(name, {})
        if not isinstance(profile, dict):
            continue
        for key in ("local_hls_url", "stream_url", "local_mjpeg_url"):
            value = str(profile.get(key, "")).strip()
            if value:
                return value
    return None


def snapshot_url_for_record(record: dict[str, Any] | None) -> str | None:
    stream = stream_for_record(record)
    value = str(stream.get("snapshot_url", "")).strip()
    return value or None


def bool_field_names(record: dict[str, Any]) -> list[str]:
    result = []
    for key, value in merged_fields_for_record(record).items():
        if isinstance(value, bool):
            result.append(key)
    return sorted(result)


def scalar_field_names(record: dict[str, Any]) -> list[str]:
    result = []
    for key, value in merged_fields_for_record(record).items():
        if isinstance(value, bool):
            continue
        if value is None:
            continue
        if isinstance(value, (list, dict)):
            continue
        if isinstance(value, str) and not value.strip():
            continue
        result.append(key)
    return sorted(result)


def value_for_field(record: dict[str, Any], field: str) -> Any:
    return merged_fields_for_record(record).get(field)


def name_for_field(field: str) -> str:
    return FIELD_NAMES.get(field, field.replace("_", " ").title())


def binary_device_class_for_field(field: str) -> str | None:
    return BINARY_DEVICE_CLASSES.get(field)


def entity_category_for_field(field: str) -> EntityCategory | None:
    if field == "online" or field in DIAGNOSTIC_FIELDS:
        return EntityCategory.DIAGNOSTIC
    return None


def sensor_device_class_for_field(field: str) -> SensorDeviceClass | None:
    if field.endswith("_at"):
        return SensorDeviceClass.TIMESTAMP
    return None


def native_value_for_field(record: dict[str, Any], field: str) -> Any:
    value = value_for_field(record, field)
    if value is None:
        return None
    if field.endswith("_at") and isinstance(value, str):
        parsed = dt_util.parse_datetime(value)
        if parsed is not None:
            return parsed
    return value


def unit_for_field(field: str) -> str | None:
    if field.endswith("_bytes"):
        return "B"
    if field.endswith("_percent"):
        return "%"
    if field.endswith("_seconds"):
        return "s"
    if field.endswith("_packets"):
        return "packets"
    return None


def button_specs_for_record(record: dict[str, Any]) -> list[ButtonSpec]:
    device = device_for_record(record)
    device_id = device_id_for_record(record)
    device_kind = str(device.get("kind", "")).strip()
    parent_id = parent_id_for_record(record)

    specs: list[ButtonSpec] = []

    if device_id and not parent_id:
        specs.append(
            ButtonSpec(
                "probe_now",
                "Probe Now",
                f"/api/v1/devices/{quote(device_id, safe='')}/probe",
                "mdi:radar",
            )
        )
        if device_kind == "nvr":
            specs.append(
                ButtonSpec(
                    "refresh_inventory",
                    "Refresh Inventory",
                    f"/api/v1/nvr/{quote(device_id, safe='')}/inventory/refresh",
                    "mdi:database-refresh",
                )
            )

    stream = stream_for_record(record)
    intercom = stream.get("intercom", {})
    if not isinstance(intercom, dict):
        return specs

    answer_url = str(intercom.get("answer_url", "")).strip()
    if answer_url:
        specs.append(ButtonSpec("answer_call", "Answer Call", answer_url, "mdi:phone"))

    hangup_url = str(intercom.get("hangup_url", "")).strip()
    if hangup_url:
        specs.append(
            ButtonSpec("hangup_call", "Hang Up Call", hangup_url, "mdi:phone-hangup")
        )

    reset_url = str(intercom.get("bridge_session_reset_url", "")).strip()
    if reset_url:
        specs.append(
            ButtonSpec(
                "reset_bridge_session",
                "Reset Bridge Session",
                reset_url,
                "mdi:restart",
            )
        )

    enable_url = str(intercom.get("external_uplink_enable_url", "")).strip()
    if enable_url:
        specs.append(
            ButtonSpec(
                "enable_rtp_export",
                "Enable RTP Export",
                enable_url,
                "mdi:upload-network",
            )
        )

    disable_url = str(intercom.get("external_uplink_disable_url", "")).strip()
    if disable_url:
        specs.append(
            ButtonSpec(
                "disable_rtp_export",
                "Disable RTP Export",
                disable_url,
                "mdi:upload-off",
            )
        )

    lock_urls = intercom.get("lock_urls", [])
    if isinstance(lock_urls, list):
        for index, lock_url in enumerate(lock_urls, start=1):
            if not isinstance(lock_url, str) or not lock_url.strip():
                continue
            specs.append(
                ButtonSpec(
                    f"unlock_{index}",
                    f"Unlock {index}",
                    lock_url,
                    "mdi:lock-open-variant",
                )
            )

    return specs


def update_timestamp(data: dict[str, Any] | None) -> datetime | None:
    if not data:
        return None
    raw = data.get("generated_at")
    if not isinstance(raw, str):
        return None
    return dt_util.parse_datetime(raw)


def stream_fields_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    stream = stream_for_record(record)
    if not stream:
        return {}

    fields: dict[str, Any] = {}
    for key in (
        "recommended_profile",
        "recommended_ha_integration",
        "recommended_ha_reason",
        "onvif_h264_available",
        "onvif_profile_name",
        "onvif_profile_token",
        "main_codec",
        "main_resolution",
        "sub_codec",
        "sub_resolution",
        "audio_codec",
        "channel",
        "lock_count",
    ):
        if key not in stream:
            continue
        fields[key] = stream[key]
    return fields


def coerce_catalog_value(value: Any) -> Any:
    if not isinstance(value, str):
        return value

    text = value.strip()
    if not text:
        return ""

    lowered = text.lower()
    if lowered == "true":
        return True
    if lowered == "false":
        return False

    if text.isdigit():
        try:
            return int(text)
        except ValueError:
            return text

    return text
