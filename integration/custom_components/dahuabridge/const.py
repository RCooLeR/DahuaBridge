from __future__ import annotations

from homeassistant.const import Platform

DOMAIN = "dahuabridge"

CONF_BRIDGE_URL = "bridge_url"
CONF_API_TOKEN = "api_token"
CONF_SCAN_INTERVAL = "scan_interval"
CONF_PREFERRED_VIDEO_PROFILE = "preferred_video_profile"
CONF_PREFERRED_VIDEO_SOURCE = "preferred_video_source"
CONF_ENABLE_VIDEO_FALLBACKS = "enable_video_fallbacks"
CONF_LANGUAGE = "language"
CONF_LIVE_SOURCE = "live_source"
CONF_LIVE_PRECONNECT = "live_preconnect"

DEFAULT_SCAN_INTERVAL = 15
DEFAULT_PREFERRED_VIDEO_PROFILE = "quality"
DEFAULT_PREFERRED_VIDEO_SOURCE = "rtsp"
DEFAULT_ENABLE_VIDEO_FALLBACKS = True
DEFAULT_LANGUAGE = "auto"
DEFAULT_LIVE_SOURCE = "nvr"
DEFAULT_LIVE_PRECONNECT = "off"

CATALOG_PATH = "/api/v1/home-assistant/native/catalog"
STATUS_PATH = "/api/v1/status"
LIVE_SOURCE_PATH = "/api/v1/settings/live-source"
LIVE_PRECONNECT_PATH = "/api/v1/settings/live-preconnect"

PLATFORMS: list[Platform] = [
    Platform.CAMERA,
    Platform.BINARY_SENSOR,
    Platform.SENSOR,
    Platform.BUTTON,
    Platform.SWITCH,
]
