from __future__ import annotations

from typing import Any

from homeassistant.components.diagnostics import async_redact_data
from homeassistant.core import HomeAssistant

from . import DahuaBridgeConfigEntry
from .diagnostic_redaction import CONFIG_REDACT_KEYS, REDACTED, redact_payload


async def async_get_config_entry_diagnostics(
    hass: HomeAssistant, entry: DahuaBridgeConfigEntry
) -> dict[str, Any]:
    coordinator = entry.runtime_data
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
