from __future__ import annotations

from typing import Any

from homeassistant.helpers import device_registry as dr
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from .catalog import (
    available_for_record,
    device_for_record,
    parent_id_for_record,
    record_by_device_id,
)
from .const import DOMAIN
from .coordinator import DahuaBridgeCoordinator


class DahuaBridgeEntity(CoordinatorEntity[DahuaBridgeCoordinator]):
    _attr_has_entity_name = True

    def __init__(self, coordinator: DahuaBridgeCoordinator, device_id: str) -> None:
        super().__init__(coordinator)
        self._device_id = device_id

    @property
    def record(self) -> dict[str, Any] | None:
        return record_by_device_id(self.coordinator.data, self._device_id)

    @property
    def available(self) -> bool:
        return super().available and self.record is not None

    @property
    def device_online(self) -> bool:
        record = self.record
        return record is not None and available_for_record(record)

    @property
    def device_info(self) -> DeviceInfo:
        record = self.record or {}
        kwargs = self._device_info_values(record, self._device_id)
        parent_id = parent_id_for_record(record)
        parent_record = record_by_device_id(self.coordinator.data, parent_id) if parent_id else None
        if parent_id and parent_id != self._device_id and parent_record is not None:
            lookup = getattr(dr, "async_get_device_id_by_identifier", None)
            if lookup is None:
                # Home Assistant versions predating the scoped registry API.
                kwargs["via_device"] = (DOMAIN, parent_id)
            else:
                hass = self.coordinator.hass
                entry_id = self.coordinator.config_entry.entry_id
                try:
                    parent_device_id = lookup(hass, (DOMAIN, parent_id), config_entry_id=entry_id)
                except ValueError:
                    # Platforms add entities concurrently. Register the recorder
                    # before linking a camera if its own entities are not ready.
                    parent_device_id = dr.async_get(hass).async_get_or_create(
                        config_entry_id=entry_id,
                        **self._device_info_values(parent_record, parent_id),
                    ).id
                kwargs["via_device_id"] = parent_device_id
        return DeviceInfo(**kwargs)

    def _device_info_values(self, record: dict[str, Any], device_id: str) -> dict[str, Any]:
        device = device_for_record(record)
        return {
            "identifiers": {(DOMAIN, device_id)},
            "name": str(device.get("name", device_id)),
            "manufacturer": str(device.get("manufacturer", "")).strip() or None,
            "model": str(device.get("model", "")).strip() or None,
            "serial_number": str(device.get("serial", "")).strip() or None,
            "sw_version": str(device.get("firmware", "")).strip() or None,
            "configuration_url": self.coordinator.api.base_url,
        }
