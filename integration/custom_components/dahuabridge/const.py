from __future__ import annotations

from homeassistant.const import Platform

DOMAIN = "dahuabridge"

CONF_BRIDGE_URL = "bridge_url"
CONF_SCAN_INTERVAL = "scan_interval"

DEFAULT_SCAN_INTERVAL = 15

CATALOG_PATH = "/api/v1/home-assistant/native/catalog"
STATUS_PATH = "/api/v1/status"

PLATFORMS: list[Platform] = [
    Platform.CAMERA,
    Platform.BINARY_SENSOR,
    Platform.SENSOR,
    Platform.BUTTON,
]
