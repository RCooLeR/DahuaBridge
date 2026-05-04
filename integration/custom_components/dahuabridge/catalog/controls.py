from __future__ import annotations

from typing import Any
from urllib.parse import quote

from .accessors import (
    bool_intercom_value_for_record,
    device_for_record,
    device_id_for_record,
    feature_by_key,
    features_for_record,
    intercom_for_record,
    parent_id_for_record,
)
from .fields import name_for_field
from ..localization import localized_label
from .models import ButtonSpec, NumberSpec, SwitchSpec


def button_specs_for_record(
    record: dict[str, Any], language: str = "en"
) -> list[ButtonSpec]:
    device = device_for_record(record)
    device_id = device_id_for_record(record)
    device_kind = str(device.get("kind", "")).strip()
    parent_id = parent_id_for_record(record)

    specs: list[ButtonSpec] = []

    if device_id and not parent_id:
        specs.append(
            ButtonSpec(
                "probe_now",
                localized_label("probe_now", language),
                f"/api/v1/devices/{quote(device_id, safe='')}/probe",
                "mdi:radar",
            )
        )
        if device_kind == "nvr":
            specs.append(
                ButtonSpec(
                    "refresh_inventory",
                    localized_label("refresh_inventory", language),
                    f"/api/v1/nvr/{quote(device_id, safe='')}/inventory/refresh",
                    "mdi:database-refresh",
                )
            )

    intercom = intercom_for_record(record)
    if not intercom:
        return specs

    answer_url = str(intercom.get("answer_url", "")).strip()
    if answer_url:
        specs.append(
            ButtonSpec(
                "answer_call",
                localized_label("answer_call", language),
                answer_url,
                "mdi:phone",
            )
        )

    hangup_url = str(intercom.get("hangup_url", "")).strip()
    if hangup_url:
        specs.append(
            ButtonSpec(
                "hangup_call",
                localized_label("hangup_call", language),
                hangup_url,
                "mdi:phone-hangup",
            )
        )

    reset_url = str(intercom.get("bridge_session_reset_url", "")).strip()
    if reset_url:
        specs.append(
            ButtonSpec(
                "reset_bridge_session",
                localized_label("reset_bridge_session", language),
                reset_url,
                "mdi:restart",
            )
        )

    enable_url = str(intercom.get("external_uplink_enable_url", "")).strip()
    if enable_url:
        specs.append(
            ButtonSpec(
                "enable_rtp_export",
                localized_label("enable_rtp_export", language),
                enable_url,
                "mdi:upload-network",
            )
        )

    disable_url = str(intercom.get("external_uplink_disable_url", "")).strip()
    if disable_url:
        specs.append(
            ButtonSpec(
                "disable_rtp_export",
                localized_label("disable_rtp_export", language),
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
                    localized_label("unlock", language, index=index),
                    lock_url,
                    "mdi:lock-open-variant",
                )
            )

    return specs


def number_specs_for_record(
    record: dict[str, Any], language: str = "en"
) -> list[NumberSpec]:
    intercom = intercom_for_record(record)
    if not intercom:
        return []

    specs: list[NumberSpec] = []

    output_url = str(intercom.get("output_volume_url", "")).strip()
    if output_url and bool(intercom.get("supports_vto_output_volume_control")):
        specs.append(
            NumberSpec(
                key="output_volume",
                name=localized_label("output_volume", language),
                url=output_url,
                icon="mdi:volume-high",
                value_key="output_volume_level",
                slot=0,
                min_value=0,
                max_value=100,
                step=1,
            )
        )

    input_url = str(intercom.get("input_volume_url", "")).strip()
    if input_url and bool(intercom.get("supports_vto_input_volume_control")):
        specs.append(
            NumberSpec(
                key="input_volume",
                name=localized_label("input_volume", language),
                url=input_url,
                icon="mdi:microphone",
                value_key="input_volume_level",
                slot=0,
                min_value=0,
                max_value=100,
                step=1,
            )
        )

    return specs


def switch_specs_for_record(
    record: dict[str, Any], language: str = "en"
) -> list[SwitchSpec]:
    specs: list[SwitchSpec] = []

    intercom = intercom_for_record(record)
    if intercom:
        mute_url = str(intercom.get("mute_url", "")).strip()
        if mute_url and bool(intercom.get("supports_vto_mute_control")):
            specs.append(
                SwitchSpec(
                    key="muted",
                    name=localized_label("muted", language),
                    url=mute_url,
                    icon="mdi:volume-mute",
                    value_key="muted",
                    payload_key="muted",
                )
            )

        recording_url = str(intercom.get("recording_url", "")).strip()
        if recording_url and bool(intercom.get("supports_vto_recording_control")):
            specs.append(
                SwitchSpec(
                    key="auto_record_enabled",
                    name=localized_label("auto_record_enabled", language),
                    url=recording_url,
                    icon="mdi:record-rec",
                    value_key="auto_record_enabled",
                    payload_key="auto_record_enabled",
                )
            )

    for feature in features_for_record(record):
        if not _is_toggle_output_feature(feature):
            continue

        key = str(feature.get("key", "")).strip()
        url = str(feature.get("url", "")).strip()
        parameter_key = str(feature.get("parameter_key", "")).strip()
        parameter_value = str(feature.get("parameter_value", "")).strip()
        if not key or not url or not parameter_key or not parameter_value:
            continue

        specs.append(
            SwitchSpec(
                key=key,
                name=str(feature.get("label", "")).strip()
                or name_for_field(key, language),
                url=url,
                icon=_feature_icon_for_key(key),
                value_key=key,
                value_source="feature",
                payload_on={parameter_key: parameter_value, "action": "start"},
                payload_off={parameter_key: parameter_value, "action": "stop"},
            )
        )

    return specs


def bool_switch_value_for_record(
    record: dict[str, Any] | None, spec: SwitchSpec
) -> bool | None:
    if spec.value_source == "feature":
        feature = feature_by_key(record, spec.value_key)
        if feature is None:
            return None
        value = feature.get("active")
        return value if isinstance(value, bool) else None
    return bool_intercom_value_for_record(record, spec.value_key)


def switch_payload_for_value(spec: SwitchSpec, value: bool) -> dict[str, Any]:
    if value:
        if spec.payload_on is not None:
            return dict(spec.payload_on)
    elif spec.payload_off is not None:
        return dict(spec.payload_off)
    if spec.payload_key:
        return {spec.payload_key: value}
    return {}


def _is_toggle_output_feature(feature: dict[str, Any]) -> bool:
    if not bool(feature.get("supported")):
        return False
    if str(feature.get("parameter_key", "")).strip() != "output":
        return False
    key = str(feature.get("key", "")).strip()
    if key not in {"light", "warning_light", "siren"}:
        return False
    actions = feature.get("actions", [])
    if not isinstance(actions, list):
        return False
    normalized_actions = {str(action).strip() for action in actions}
    return "start" in normalized_actions and "stop" in normalized_actions


def _feature_icon_for_key(key: str) -> str:
    return {
        "light": "mdi:lightbulb-on-outline",
        "warning_light": "mdi:alarm-light-outline",
        "siren": "mdi:bullhorn",
    }.get(key, "mdi:toggle-switch")
