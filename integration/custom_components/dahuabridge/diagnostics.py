from __future__ import annotations

from typing import Any

from homeassistant.components.diagnostics import async_redact_data
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant

from .const import CONF_BRIDGE_URL, DOMAIN

REDACTED = "**REDACTED**"
CONFIG_REDACT_KEYS = {CONF_BRIDGE_URL}
PAYLOAD_REDACT_KEYS = {
    "answer_url",
    "base_url",
    "bridge_session_reset_url",
    "external_uplink_disable_url",
    "external_uplink_enable_url",
    "hangup_url",
    "local_hls_url",
    "local_intercom_url",
    "local_mjpeg_url",
    "local_preview_url",
    "local_webrtc_url",
    "lock_urls",
    "onvif_snapshot_url",
    "onvif_stream_url",
    "serial",
    "snapshot_url",
    "stream_url",
}


async def async_get_config_entry_diagnostics(
    hass: HomeAssistant, entry: ConfigEntry
) -> dict[str, Any]:
    coordinator = hass.data[DOMAIN][entry.entry_id]
    status: dict[str, Any] | None = None
    status_error: str | None = None

    try:
        status = await coordinator.api.async_get_status()
    except Exception as err:  # pragma: no cover - defensive diagnostics path
        status_error = str(err)

    return {
        "entry": {
            "title": entry.title,
            "data": async_redact_data(dict(entry.data), CONFIG_REDACT_KEYS),
            "options": dict(entry.options),
        },
        "coordinator": {
            "last_update_success": coordinator.last_update_success,
            "last_update_success_time": getattr(
                coordinator, "last_update_success_time", None
            ),
            "catalog_generated_at": (coordinator.data or {}).get("generated_at"),
            "device_count": len((coordinator.data or {}).get("devices", [])),
        },
        "bridge_status": redact_payload(status),
        "bridge_status_error": status_error,
        "native_catalog": redact_payload(coordinator.data),
    }


def redact_payload(value: Any) -> Any:
    if isinstance(value, dict):
        redacted: dict[str, Any] = {}
        for key, item in value.items():
            if key in PAYLOAD_REDACT_KEYS:
                redacted[key] = REDACTED
                continue
            redacted[key] = redact_payload(item)
        return redacted

    if isinstance(value, list):
        return [redact_payload(item) for item in value]

    return value
