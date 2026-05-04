from __future__ import annotations

from typing import Any


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


def intercom_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    intercom = stream_for_record(record).get("intercom", {})
    return intercom if isinstance(intercom, dict) else {}


def controls_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    controls = stream_for_record(record).get("controls", {})
    return controls if isinstance(controls, dict) else {}


def capture_for_record(record: dict[str, Any] | None) -> dict[str, Any]:
    capture = stream_for_record(record).get("capture", {})
    return capture if isinstance(capture, dict) else {}


def features_for_record(record: dict[str, Any] | None) -> list[dict[str, Any]]:
    features = stream_for_record(record).get("features", [])
    if not isinstance(features, list):
        return []
    return [feature for feature in features if isinstance(feature, dict)]


def feature_by_key(record: dict[str, Any] | None, key: str) -> dict[str, Any] | None:
    target = str(key).strip()
    if not target:
        return None
    for feature in features_for_record(record):
        if str(feature.get("key", "")).strip() == target:
            return feature
    return None


def device_id_for_record(record: dict[str, Any]) -> str:
    return str(device_for_record(record).get("id", "")).strip()


def parent_id_for_record(record: dict[str, Any]) -> str:
    return str(device_for_record(record).get("parent_id", "")).strip()


def available_for_record(record: dict[str, Any] | None) -> bool:
    return bool(state_for_record(record).get("available", False))


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


def int_intercom_value_for_record(
    record: dict[str, Any] | None, field: str
) -> int | None:
    value = intercom_for_record(record).get(field)
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return int(value)
    if isinstance(value, str) and value.strip().isdigit():
        try:
            return int(value.strip())
        except ValueError:
            return None
    return None


def bool_intercom_value_for_record(
    record: dict[str, Any] | None, field: str
) -> bool | None:
    value = intercom_for_record(record).get(field)
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        lowered = value.strip().lower()
        if lowered == "true":
            return True
        if lowered == "false":
            return False
    return None
