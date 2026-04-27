from __future__ import annotations

from typing import Any

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
        device = device_for_record(record)
        kwargs: dict[str, Any] = {
            "identifiers": {(DOMAIN, self._device_id)},
            "name": str(device.get("name", self._device_id)),
            "manufacturer": str(device.get("manufacturer", "")).strip() or None,
            "model": str(device.get("model", "")).strip() or None,
            "serial_number": str(device.get("serial", "")).strip() or None,
            "sw_version": str(device.get("firmware", "")).strip() or None,
            "configuration_url": self.coordinator.api.base_url,
        }

        parent_id = parent_id_for_record(record)
        if parent_id and record_by_device_id(self.coordinator.data, parent_id) is not None:
            kwargs["via_device"] = (DOMAIN, parent_id)

        return DeviceInfo(**kwargs)
