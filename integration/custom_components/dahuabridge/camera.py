from __future__ import annotations

import logging
from functools import partial
from inspect import isawaitable
from typing import Any

from homeassistant.components.camera import Camera, CameraEntityFeature
from homeassistant.core import HomeAssistant, callback
from homeassistant.exceptions import HomeAssistantError
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from . import DahuaBridgeConfigEntry
from .api import DahuaBridgeAPIError
from .api.client import redact_url_for_log
from .camera_support import (
    async_placeholder_logo_bytes,
    camera_extra_state_attributes,
    with_requested_width,
)
from .catalog import (
    capture_for_record,
    device_id_for_record,
    mjpeg_url_for_record_with_preferences,
    snapshot_url_for_record,
    stream_for_record,
    stream_source_for_record_with_preferences,
)
from .discovery import CatalogEntityCandidate, setup_catalog_entity_discovery
from .entity import DahuaBridgeEntity
from .localization import localized_label

_LOGGER = logging.getLogger(__name__)


async def async_setup_entry(
    hass: HomeAssistant,
    entry: DahuaBridgeConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    coordinator = entry.runtime_data
    seen: set[str] = set()

    def collect_camera_entities(record: dict[str, Any]):
        if not stream_for_record(record):
            return []

        device_id = device_id_for_record(record)
        if not device_id:
            return []

        return [
            CatalogEntityCandidate(
                key=device_id,
                create_entity=partial(DahuaBridgeCamera, coordinator, device_id),
            )
        ]

    setup_catalog_entity_discovery(
        entry,
        coordinator,
        seen,
        async_add_entities,
        collect_camera_entities,
    )


class DahuaBridgeCamera(DahuaBridgeEntity, Camera):
    def __init__(self, coordinator, device_id: str) -> None:
        DahuaBridgeEntity.__init__(self, coordinator, device_id)
        Camera.__init__(self)
        self._attr_unique_id = f"{device_id}_camera"
        self._attr_name = localized_label(
            "camera", getattr(coordinator, "integration_language", "en")
        )
        self._native_playback_source: str | None = None
        self._last_live_source_key = self._live_source_key()

    def _live_source_key(self) -> str | None:
        return stream_source_for_record_with_preferences(
            self.record,
            self.coordinator.preferred_video_profile,
            self.coordinator.preferred_video_source,
            self.coordinator.video_fallbacks_enabled,
        )

    @callback
    def _handle_coordinator_update(self) -> None:
        live_source_key = self._live_source_key()
        changed = live_source_key != self._last_live_source_key
        self._last_live_source_key = live_source_key
        if changed and not self._native_playback_source:
            stream = getattr(self, "stream", None)
            if stream is not None:
                # Clear before publishing a changed profile/format URL so HA
                # cannot reuse the old stream. Camera/NVR failover itself keeps
                # the bridge relay URL stable and does not enter this branch.
                self.stream = None
                self.hass.async_create_task(self._stop_cached_stream(stream))
        self._write_state_if_added()

    @property
    def supported_features(self) -> CameraEntityFeature:
        return (
            CameraEntityFeature.STREAM
            if self._stream_source()
            else CameraEntityFeature(0)
        )

    @property
    def is_on(self) -> bool:
        return self.record is not None

    @property
    def is_recording(self) -> bool:
        capture = capture_for_record(self.record)
        return bool(capture.get("active", False))

    @property
    def extra_state_attributes(self) -> dict[str, Any]:
        return camera_extra_state_attributes(
            self.coordinator.api,
            self.record,
            self.coordinator.preferred_video_profile,
            self.coordinator.preferred_video_source,
            self.coordinator.video_fallbacks_enabled,
            self.coordinator.integration_language,
        )

    async def stream_source(self) -> str | None:
        source = self._stream_source()
        if not source:
            return None
        if self._native_playback_source:
            return source
        resolved = self.coordinator.api.bridge_resource_url(source)
        _LOGGER.debug(
            "Resolved stream source for %s to %s", self.entity_id or self._device_id, redact_url_for_log(resolved)
        )
        return resolved

    async def async_camera_image(
        self, width: int | None = None, height: int | None = None
    ) -> bytes | None:
        snapshot_url = self._snapshot_url()
        if snapshot_url:
            resolved = self.coordinator.api.bridge_resource_url(snapshot_url)
            _LOGGER.debug(
                "Fetching camera snapshot for %s from %s",
                self.entity_id or self._device_id,
                resolved,
            )
            try:
                return await self.coordinator.api.async_get_bytes(resolved)
            except DahuaBridgeAPIError as err:
                _LOGGER.warning(
                    "Snapshot fetch failed for %s via %s: %s",
                    self.entity_id or self._device_id,
                    resolved,
                    err,
                )

        mjpeg_url = self._mjpeg_url()
        if mjpeg_url:
            resolved = self.coordinator.api.bridge_resource_url(
                with_requested_width(mjpeg_url, width)
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
        if self._native_playback_source:
            return self._native_playback_source
        return stream_source_for_record_with_preferences(
            self.record,
            self.coordinator.preferred_video_profile,
            self.coordinator.preferred_video_source,
            self.coordinator.video_fallbacks_enabled,
        )

    def _mjpeg_url(self) -> str | None:
        return mjpeg_url_for_record_with_preferences(
            self.record, self.coordinator.preferred_video_profile
        )

    def _snapshot_url(self) -> str | None:
        capture = capture_for_record(self.record)
        value = str(capture.get("snapshot_url", "")).strip()
        if value:
            return value
        return snapshot_url_for_record(self.record)

    async def async_start_recording(
        self, profile: str | None = None, duration_seconds: int | None = None
    ) -> None:
        capture = capture_for_record(self.record)
        start_url = str(capture.get("start_recording_url", "")).strip()
        if not start_url:
            raise HomeAssistantError("Bridge recording is not available for this camera")

        payload: dict[str, Any] = {}
        resolved_profile = (profile or "").strip()
        if resolved_profile:
            payload["profile"] = resolved_profile
        if duration_seconds is not None:
            payload["duration_seconds"] = duration_seconds

        await self.coordinator.api.async_post_json(start_url, payload)
        await self.coordinator.async_request_refresh()

    async def async_stop_recording(self) -> None:
        capture = capture_for_record(self.record)
        stop_url = str(capture.get("stop_recording_url", "")).strip()
        if not stop_url:
            raise HomeAssistantError("No active bridge recording for this camera")

        await self.coordinator.api.async_post_action(stop_url)
        await self.coordinator.async_request_refresh()

    async def async_set_native_playback_source(self, stream_source: str) -> None:
        source = str(stream_source or "").strip()
        if not source.lower().startswith("rtsp://"):
            raise HomeAssistantError("Native playback source must be an RTSP URL")

        changed = self._native_playback_source != source
        self._native_playback_source = source
        if changed:
            await self._reset_cached_ha_stream()
        self._write_state_if_added()

    async def async_set_live_source(self, source: str) -> None:
        if source not in {"default", "nvr", "camera"}:
            raise HomeAssistantError("Live source must be default, nvr or camera")
        live_source = stream_for_record(self.record).get("live_source", {})
        if not isinstance(live_source, dict) or not live_source.get("url"):
            raise HomeAssistantError("Live source selection is not available for this camera")
        previous_live_source_key = self._live_source_key()
        target = self.coordinator.api.bridge_resource_url(str(live_source["url"]))
        await self.coordinator.api.async_put_json(target, {"source": source})
        await self.coordinator.async_request_refresh()
        # The catalog refresh restores credentials omitted from the PUT response.
        # Archive playback has its own source and must continue uninterrupted.
        if not self._native_playback_source and self._live_source_key() != previous_live_source_key:
            await self._reset_cached_ha_stream()
        self._write_state_if_added()

    async def async_clear_native_playback_source(self) -> None:
        if not self._native_playback_source:
            return
        self._native_playback_source = None
        await self._reset_cached_ha_stream()
        self._write_state_if_added()

    async def _placeholder_logo_bytes(self) -> bytes | None:
        return await async_placeholder_logo_bytes(self.hass)

    async def _reset_cached_ha_stream(self) -> None:
        stream = getattr(self, "stream", None)
        if stream is None:
            return
        self.stream = None
        await self._stop_cached_stream(stream)

    async def _stop_cached_stream(self, stream: Any) -> None:
        stop = getattr(stream, "stop", None)
        if not callable(stop):
            return
        result = stop()
        if isawaitable(result):
            await result

    def _write_state_if_added(self) -> None:
        write_state = getattr(self, "async_write_ha_state", None)
        if callable(write_state):
            write_state()
