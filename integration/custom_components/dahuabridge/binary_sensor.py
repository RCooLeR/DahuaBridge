from __future__ import annotations

from homeassistant.components.binary_sensor import BinarySensorEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.const import EntityCategory
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .catalog import (
    available_for_record,
    binary_device_class_for_field,
    bool_field_names,
    catalog_records,
    device_id_for_record,
    entity_category_for_field,
    name_for_field,
    value_for_field,
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
        new_entities: list[BinarySensorEntity] = []

        for record in catalog_records(coordinator.data):
            device_id = device_id_for_record(record)
            if not device_id:
                continue

            online_key = f"{device_id}:online"
            if online_key not in seen:
                seen.add(online_key)
                new_entities.append(DahuaBridgeOnlineBinarySensor(coordinator, device_id))

            for field in bool_field_names(record):
                entity_key = f"{device_id}:{field}"
                if entity_key in seen:
                    continue
                seen.add(entity_key)
                new_entities.append(
                    DahuaBridgeStateBinarySensor(coordinator, device_id, field)
                )

        if new_entities:
            async_add_entities(new_entities)

    async_discover_entities()
    entry.async_on_unload(coordinator.async_add_listener(async_discover_entities))


class DahuaBridgeOnlineBinarySensor(DahuaBridgeEntity, BinarySensorEntity):
    def __init__(self, coordinator, device_id: str) -> None:
        super().__init__(coordinator, device_id)
        self._attr_unique_id = f"{device_id}_online"
        self._attr_name = "Online"
        self._attr_device_class = binary_device_class_for_field("online")
        self._attr_entity_category = EntityCategory.DIAGNOSTIC

    @property
    def is_on(self) -> bool:
        return available_for_record(self.record)


class DahuaBridgeStateBinarySensor(DahuaBridgeEntity, BinarySensorEntity):
    def __init__(self, coordinator, device_id: str, field: str) -> None:
        super().__init__(coordinator, device_id)
        self._field = field
        self._attr_unique_id = f"{device_id}_{field}"
        self._attr_name = name_for_field(field)
        self._attr_device_class = binary_device_class_for_field(field)
        self._attr_entity_category = entity_category_for_field(field)

    @property
    def is_on(self) -> bool:
        return bool(value_for_field(self.record or {}, self._field))

    @property
    def available(self) -> bool:
        return super().available and self.device_online
