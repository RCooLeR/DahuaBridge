from __future__ import annotations

from typing import Any

from homeassistant.components.binary_sensor import BinarySensorDeviceClass
from homeassistant.components.sensor import SensorDeviceClass
from homeassistant.const import EntityCategory
from homeassistant.util import dt as dt_util

from ..localization import localized_field_name
from .accessors import attributes_for_record, info_for_record, stream_fields_for_record

DIAGNOSTIC_FIELDS = {
    "audio_codec",
    "control_audio_authority",
    "control_audio_semantic",
    "bridge_forward_errors",
    "bridge_forwarded_packets",
    "bridge_session_count",
    "bridge_uplink_codec",
    "bridge_uplink_packets",
    "direct_ipc_configured",
    "direct_ipc_configured_ip",
    "direct_ipc_http_port",
    "direct_ipc_https_port",
    "direct_ipc_inventory_username",
    "direct_ipc_ip",
    "direct_ipc_model",
    "direct_ipc_rtsp_port",
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
    "nvr_config_reason",
    "nvr_config_writable",
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
    "stream_available": BinarySensorDeviceClass.RUNNING,
    "tamper": BinarySensorDeviceClass.TAMPER,
    "tripwire": BinarySensorDeviceClass.MOTION,
    "vehicle": BinarySensorDeviceClass.MOTION,
}

TRANSIENT_BINARY_FIELDS = {
    "access",
    "active",
    "call",
    "doorbell",
    "human",
    "intrusion",
    "motion",
    "tamper",
    "tripwire",
    "vehicle",
}

TRANSIENT_EVENT_PREFIXES = {
    "access": ("accesscontrol_",),
    "active": ("alarmlocal_",),
    "call": ("call_",),
    "doorbell": ("doorbell_",),
    "human": ("smartmotionhuman_",),
    "intrusion": ("crossregiondetection_",),
    "motion": ("videomotion_",),
    "tamper": ("tamper_",),
    "tripwire": ("crosslinedetection_",),
    "vehicle": ("smartmotionvehicle_",),
}

def merged_fields_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    merged: dict[str, Any] = {}
    merged.update(attributes_for_record(record))
    merged.update(stream_fields_for_record(record))
    merged.update(info_for_record(record))
    return merged


def bool_field_names(record: dict[str, Any]) -> list[str]:
    result = []
    for key, value in merged_fields_for_record(record).items():
        if isinstance(value, bool):
            if field_requires_online(key) and not should_expose_transient_field(
                record, key, value
            ):
                continue
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


def name_for_field(field: str, language: str = "en") -> str:
    return localized_field_name(field, language)


def binary_device_class_for_field(field: str) -> str | None:
    return BINARY_DEVICE_CLASSES.get(field)


def field_requires_online(field: str) -> bool:
    return field in TRANSIENT_BINARY_FIELDS


def should_expose_transient_field(
    record: dict[str, Any], field: str, value: bool
) -> bool:
    if value:
        return True

    last_event_type = str(value_for_field(record, "last_event_type") or "").strip().lower()
    if not last_event_type:
        return False

    prefixes = TRANSIENT_EVENT_PREFIXES.get(field, ())
    return any(last_event_type.startswith(prefix) for prefix in prefixes)


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
