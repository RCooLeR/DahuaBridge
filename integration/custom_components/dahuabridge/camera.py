from __future__ import annotations

from homeassistant.components.camera import Camera, CameraEntityFeature
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .catalog import (
    catalog_records,
    device_id_for_record,
    snapshot_url_for_record,
    stream_for_record,
    stream_source_for_record,
)
from .const import DOMAIN
from .entity import DahuaBridgeEntity


async def async_setup_entry(
    hass: HomeAssistant, entry: ConfigEntry, async_add_entities: AddEntitiesCallback
) -> None:
    coordinator = hass.data[DOMAIN][entry.entry_id]
    seen: set[str] = set()

    @callback
    def async_discover_entities() -> None:
        new_entities: list[Camera] = []

        for record in catalog_records(coordinator.data):
            if not stream_for_record(record):
                continue

            device_id = device_id_for_record(record)
            if not device_id or device_id in seen:
                continue

            seen.add(device_id)
            new_entities.append(DahuaBridgeCamera(coordinator, device_id))

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
            if stream_source_for_record(self.record)
            else CameraEntityFeature(0)
        )

    @property
    def available(self) -> bool:
        return super().available and self.device_online

    @property
    def is_on(self) -> bool:
        return self.record is not None

    @property
    def extra_state_attributes(self) -> dict[str, str]:
        record = self.record
        stream = stream_for_record(record)
        attrs: dict[str, str] = {}

        recommended = str(stream.get("recommended_profile", "")).strip()
        if recommended:
            attrs["recommended_profile"] = recommended

        snapshot_url = snapshot_url_for_record(record)
        if snapshot_url:
            attrs["snapshot_url"] = self.coordinator.api.absolute_url(snapshot_url)

        source = stream_source_for_record(record)
        if source:
            attrs["stream_source"] = self.coordinator.api.absolute_url(source)

        preview_url = str(stream.get("local_preview_url", "")).strip()
        if preview_url:
            attrs["preview_url"] = self.coordinator.api.absolute_url(preview_url)

        return attrs

    async def stream_source(self) -> str | None:
        source = stream_source_for_record(self.record)
        if not source:
            return None
        return self.coordinator.api.absolute_url(source)

    async def async_camera_image(
        self, width: int | None = None, height: int | None = None
    ) -> bytes | None:
        snapshot_url = snapshot_url_for_record(self.record)
        if not snapshot_url:
            return None
        return await self.coordinator.api.async_get_bytes(snapshot_url)
