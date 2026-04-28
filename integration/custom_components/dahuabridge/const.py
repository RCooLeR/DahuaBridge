from __future__ import annotations

from homeassistant.const import Platform

DOMAIN = "dahuabridge"

CONF_BRIDGE_URL = "bridge_url"
CONF_SCAN_INTERVAL = "scan_interval"
CONF_PREFERRED_VIDEO_PROFILE = "preferred_video_profile"
CONF_PREFERRED_VIDEO_SOURCE = "preferred_video_source"

DEFAULT_SCAN_INTERVAL = 15
DEFAULT_PREFERRED_VIDEO_PROFILE = "quality"
DEFAULT_PREFERRED_VIDEO_SOURCE = "hls"

CATALOG_PATH = "/api/v1/home-assistant/native/catalog"
STATUS_PATH = "/api/v1/status"

PLATFORMS: list[Platform] = [
    Platform.CAMERA,
    Platform.BINARY_SENSOR,
    Platform.SENSOR,
    Platform.BUTTON,
]
