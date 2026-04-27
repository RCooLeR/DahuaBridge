from __future__ import annotations

from homeassistant.components.sensor import SensorEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .catalog import (
    catalog_records,
    device_id_for_record,
    entity_category_for_field,
    name_for_field,
    native_value_for_field,
    scalar_field_names,
    sensor_device_class_for_field,
    unit_for_field,
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
        new_entities: list[SensorEntity] = []

        for record in catalog_records(coordinator.data):
            device_id = device_id_for_record(record)
            if not device_id:
                continue

            for field in scalar_field_names(record):
                entity_key = f"{device_id}:{field}"
                if entity_key in seen:
                    continue
                seen.add(entity_key)
                new_entities.append(DahuaBridgeStateSensor(coordinator, device_id, field))

        if new_entities:
            async_add_entities(new_entities)

    async_discover_entities()
    entry.async_on_unload(coordinator.async_add_listener(async_discover_entities))


class DahuaBridgeStateSensor(DahuaBridgeEntity, SensorEntity):
    def __init__(self, coordinator, device_id: str, field: str) -> None:
        super().__init__(coordinator, device_id)
        self._field = field
        self._attr_unique_id = f"{device_id}_{field}"
        self._attr_name = name_for_field(field)
        self._attr_device_class = sensor_device_class_for_field(field)
        self._attr_entity_category = entity_category_for_field(field)
        self._attr_native_unit_of_measurement = unit_for_field(field)

    @property
    def native_value(self):
        return native_value_for_field(self.record or {}, self._field)

    @property
    def available(self) -> bool:
        return super().available and self.device_online
