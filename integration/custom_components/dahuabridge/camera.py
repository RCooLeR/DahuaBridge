from __future__ import annotations

import logging
from pathlib import Path
from typing import Any
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

from homeassistant.components.camera import Camera, CameraEntityFeature
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .bridge_api import DahuaBridgeAPIError
from .catalog import (
    catalog_records,
    controls_for_record,
    device_for_record,
    device_id_for_record,
    features_for_record,
    intercom_for_record,
    mjpeg_url_for_record_with_preferences,
    parent_id_for_record,
    snapshot_url_for_record,
    stream_for_record,
    stream_available_for_record,
    stream_source_for_record_with_preferences,
)
from .const import DOMAIN
from .entity import DahuaBridgeEntity
from .registry_cleanup import prune_stale_entities

_LOGGER = logging.getLogger(__name__)
_LOGO_BYTES: bytes | None = None
_LOGO_PATH = Path(__file__).resolve().parent / "brand" / "logo.png"


async def async_setup_entry(
    hass: HomeAssistant, entry: ConfigEntry, async_add_entities: AddEntitiesCallback
) -> None:
    coordinator = hass.data[DOMAIN][entry.entry_id]
    seen: set[str] = set()

    @callback
    def async_discover_entities() -> None:
        new_entities: list[Camera] = []
        desired_unique_ids: set[str] = set()

        for record in catalog_records(coordinator.data):
            if not stream_for_record(record):
                continue

            device_id = device_id_for_record(record)
            if not device_id or device_id in seen:
                continue

            desired_unique_ids.add(f"{device_id}_camera")
            seen.add(device_id)
            new_entities.append(DahuaBridgeCamera(coordinator, device_id))

        if coordinator.can_prune_registry:
            prune_stale_entities(
                hass,
                entry,
                "camera",
                desired_unique_ids,
                coordinator.stale_entity_miss_counts,
            )
        if new_entities:
            async_add_entities(new_entities)

    async_discover_entities()
    entry.async_on_unload(coordinator.async_add_listener(async_discover_entities))


class DahuaBridgeCamera(DahuaBridgeEntity, Camera):
    def __init__(self, coordinator, device_id: str) -> None:
        DahuaBridgeEntity.__init__(self, coordinator, device_id)
        Camera.__init__(self)
        self._attr_unique_id = f"{device_id}_camera"
        self._attr_name = "Camera"

    @property
    def supported_features(self) -> CameraEntityFeature:
        return (
            CameraEntityFeature.STREAM
            if self._stream_source()
            else CameraEntityFeature(0)
        )

    @property
    def available(self) -> bool:
        return self.record is not None

    @property
    def is_on(self) -> bool:
        return self.record is not None

    @property
    def extra_state_attributes(self) -> dict[str, Any]:
        record = self.record
        stream = stream_for_record(record)
        device = device_for_record(record)
        device_id = device_id_for_record(record)
        parent_id = parent_id_for_record(record)
        attrs: dict[str, Any] = {}

        recommended = str(stream.get("recommended_profile", "")).strip()
        if recommended:
            attrs["recommended_profile"] = recommended

        snapshot_url = snapshot_url_for_record(record)
        if snapshot_url:
            attrs["snapshot_url"] = self.coordinator.api.bridge_resource_url(snapshot_url)

        source = self._stream_source()
        if source:
            attrs["stream_source"] = self.coordinator.api.bridge_resource_url(source)
        attrs["bridge_base_url"] = self.coordinator.api.base_url
        attrs["bridge_events_url"] = self.coordinator.api.absolute_url("/api/v1/events")
        attrs["bridge_device_id"] = device_id
        attrs["bridge_root_device_id"] = parent_id or device_id
        attrs["bridge_device_kind"] = str(device.get("kind", "")).strip()
        attrs["bridge_device_name"] = str(device.get("name", "")).strip()
        channel = stream.get("channel")
        if isinstance(channel, int):
            attrs["bridge_channel"] = channel
        attrs["stream_available"] = self._stream_available()
        attrs["preferred_video_profile"] = self.coordinator.preferred_video_profile
        attrs["preferred_video_source"] = self.coordinator.preferred_video_source

        preview_url = str(stream.get("local_preview_url", "")).strip()
        if preview_url:
            attrs["preview_url"] = self.coordinator.api.bridge_resource_url(preview_url)

        controls = controls_for_record(record)
        if controls:
            attrs["bridge_controls"] = _resolve_bridge_urls(
                self.coordinator.api, controls
            )

        features = features_for_record(record)
        if features:
            attrs["bridge_features"] = _resolve_bridge_urls(
                self.coordinator.api, features
            )

        intercom = intercom_for_record(record)
        if intercom:
            attrs["bridge_intercom"] = _resolve_bridge_urls(
                self.coordinator.api, intercom
            )

        return attrs

    async def stream_source(self) -> str | None:
        source = self._stream_source()
        if not source:
            return None
        resolved = self.coordinator.api.bridge_resource_url(source)
        _LOGGER.debug(
            "Resolved stream source for %s to %s", self.entity_id or self._device_id, resolved
        )
        return resolved

    async def async_camera_image(
        self, width: int | None = None, height: int | None = None
    ) -> bytes | None:
        mjpeg_url = self._mjpeg_url()
        if not mjpeg_url:
            return await self._placeholder_logo_bytes()

        resolved = self.coordinator.api.bridge_resource_url(
            _with_requested_width(mjpeg_url, width)
        )
        _LOGGER.debug(
            "Fetching camera MJPEG frame for %s from %s",
            self.entity_id or self._device_id,
            resolved,
        )
        try:
            return await self.coordinator.api.async_get_mjpeg_frame(resolved)
        except DahuaBridgeAPIError as err:
            _LOGGER.warning(
                "MJPEG frame fetch failed for %s via %s: %s",
                self.entity_id or self._device_id,
                resolved,
                err,
            )
            return await self._placeholder_logo_bytes()

    def _stream_source(self) -> str | None:
        return stream_source_for_record_with_preferences(
            self.record,
            self.coordinator.preferred_video_profile,
            self.coordinator.preferred_video_source,
        )

    def _mjpeg_url(self) -> str | None:
        return mjpeg_url_for_record_with_preferences(
            self.record, self.coordinator.preferred_video_profile
        )

    def _stream_available(self) -> bool:
        return stream_available_for_record(self.record)

    async def _placeholder_logo_bytes(self) -> bytes | None:
        global _LOGO_BYTES
        if _LOGO_BYTES is not None:
            return _LOGO_BYTES or None

        _LOGO_BYTES = await self.hass.async_add_executor_job(read_logo_bytes)
        return _LOGO_BYTES or None


def read_logo_bytes() -> bytes:
    global _LOGO_BYTES
    try:
        return _LOGO_PATH.read_bytes()
    except OSError as err:
        _LOGGER.warning(
            "Failed to load DahuaBridge logo placeholder from %s: %s",
            _LOGO_PATH,
            err,
        )
        return b""


def _with_requested_width(target: str, width: int | None) -> str:
    if width is None or width <= 0:
        return target

    parsed = urlsplit(target)
    query = dict(parse_qsl(parsed.query, keep_blank_values=True))
    query["width"] = str(width)
    return urlunsplit(
        (
            parsed.scheme,
            parsed.netloc,
            parsed.path,
            urlencode(query),
            parsed.fragment,
        )
    )


def _resolve_bridge_urls(api, value: Any) -> Any:
    if isinstance(value, dict):
        return {
            str(key): _resolve_bridge_urls(api, nested_value)
            for key, nested_value in value.items()
        }
    if isinstance(value, list):
        return [_resolve_bridge_urls(api, item) for item in value]
    if isinstance(value, str) and _looks_like_bridge_path(value):
        return api.bridge_resource_url(value)
    return value


def _looks_like_bridge_path(value: str) -> bool:
    text = value.strip()
    if not text:
        return False
    return text.startswith("/") or text.startswith("http://") or text.startswith(
        "https://"
    )
