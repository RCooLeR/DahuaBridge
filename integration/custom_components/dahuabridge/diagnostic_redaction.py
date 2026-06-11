from __future__ import annotations

from typing import Any

from .const import CONF_API_TOKEN, CONF_BRIDGE_URL

REDACTED = "**REDACTED**"
CONFIG_REDACT_KEYS = {CONF_API_TOKEN, CONF_BRIDGE_URL}
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


def redact_payload(value: Any) -> Any:
    if isinstance(value, dict):
        redacted: dict[str, Any] = {}
        for key, item in value.items():
            if should_redact_payload_key(key):
                redacted[key] = REDACTED
                continue
            redacted[key] = redact_payload(item)
        return redacted

    if isinstance(value, list):
        return [redact_payload(item) for item in value]

    return value


def should_redact_payload_key(key: Any) -> bool:
    normalized = str(key).strip()
    if normalized in PAYLOAD_REDACT_KEYS:
        return True
    lowered = normalized.lower()
    return lowered == "url" or lowered.endswith("_url") or lowered.endswith("_urls")
