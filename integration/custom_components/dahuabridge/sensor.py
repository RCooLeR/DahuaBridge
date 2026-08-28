from __future__ import annotations

from functools import partial
from typing import Any

from homeassistant.components.sensor import SensorEntity
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from . import DahuaBridgeConfigEntry
from .catalog import (
    device_id_for_record,
    entity_category_for_field,
    name_for_field,
    native_value_for_field,
    scalar_field_names,
    sensor_device_class_for_field,
    unit_for_field,
)
from .discovery import CatalogEntityCandidate, setup_catalog_entity_discovery
from .entity import DahuaBridgeEntity


async def async_setup_entry(
    hass: HomeAssistant,
    entry: DahuaBridgeConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    coordinator = entry.runtime_data
    seen: set[str] = set()
    language = getattr(coordinator, "integration_language", "en")

    def collect_sensors(record: dict[str, Any]):
        device_id = device_id_for_record(record)
        if not device_id:
            return []

        return [
            CatalogEntityCandidate(
                key=f"{device_id}:{field}",
                create_entity=partial(
                    DahuaBridgeStateSensor, coordinator, device_id, field, language
                ),
            )
            for field in scalar_field_names(record)
        ]

    setup_catalog_entity_discovery(
        entry,
        coordinator,
        seen,
        async_add_entities,
        collect_sensors,
    )


class DahuaBridgeStateSensor(DahuaBridgeEntity, SensorEntity):
    def __init__(
        self, coordinator, device_id: str, field: str, language: str = "en"
    ) -> None:
        super().__init__(coordinator, device_id)
        self._field = field
        self._attr_unique_id = f"{device_id}_{field}"
        self._attr_name = name_for_field(field, language)
        self._attr_device_class = sensor_device_class_for_field(field)
        self._attr_entity_category = entity_category_for_field(field)
        self._attr_native_unit_of_measurement = unit_for_field(field)

    @property
    def native_value(self):
        return native_value_for_field(self.record or {}, self._field)
