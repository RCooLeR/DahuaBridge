from __future__ import annotations

import logging
from pathlib import Path

from homeassistant.core import HomeAssistant

_LOGGER = logging.getLogger(__name__)
_LOGO_BYTES: bytes | None = None
_LOGO_PATH = Path(__file__).resolve().parents[1] / "brand" / "logo.png"


async def async_placeholder_logo_bytes(hass: HomeAssistant) -> bytes | None:
    global _LOGO_BYTES
    if _LOGO_BYTES is not None:
        return _LOGO_BYTES or None

    _LOGO_BYTES = await hass.async_add_executor_job(read_logo_bytes)
    return _LOGO_BYTES or None


def read_logo_bytes() -> bytes:
    try:
        return _LOGO_PATH.read_bytes()
    except OSError as err:
        _LOGGER.warning(
            "Failed to load DahuaBridge logo placeholder from %s: %s",
            _LOGO_PATH,
            err,
        )
        return b""
