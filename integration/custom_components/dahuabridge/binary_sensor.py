from __future__ import annotations

from functools import partial
from typing import Any

from homeassistant.components.binary_sensor import BinarySensorEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.const import EntityCategory
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .catalog import (
    available_for_record,
    binary_device_class_for_field,
    bool_field_names,
    device_id_for_record,
    entity_category_for_field,
    field_requires_online,
    name_for_field,
    value_for_field,
)
from .const import DOMAIN
from .discovery import CatalogEntityCandidate, setup_catalog_entity_discovery
from .entity import DahuaBridgeEntity
from .localization import localized_label


async def async_setup_entry(
    hass: HomeAssistant, entry: ConfigEntry, async_add_entities: AddEntitiesCallback
) -> None:
    coordinator = hass.data[DOMAIN][entry.entry_id]
    seen: set[str] = set()
    language = getattr(coordinator, "integration_language", "en")

    def collect_binary_sensors(record: dict[str, Any]):
        device_id = device_id_for_record(record)
        if not device_id:
            return []

        candidates = [
            CatalogEntityCandidate(
                key=f"{device_id}:online",
                create_entity=partial(
                    DahuaBridgeOnlineBinarySensor, coordinator, device_id, language
                ),
            )
        ]
        for field in bool_field_names(record):
            candidates.append(
                CatalogEntityCandidate(
                    key=f"{device_id}:{field}",
                    create_entity=partial(
                        DahuaBridgeStateBinarySensor,
                        coordinator,
                        device_id,
                        field,
                        language,
                    ),
                )
            )
        return candidates

    setup_catalog_entity_discovery(
        entry,
        coordinator,
        seen,
        async_add_entities,
        collect_binary_sensors,
    )


class DahuaBridgeOnlineBinarySensor(DahuaBridgeEntity, BinarySensorEntity):
    def __init__(self, coordinator, device_id: str, language: str = "en") -> None:
        super().__init__(coordinator, device_id)
        self._attr_unique_id = f"{device_id}_online"
        self._attr_name = localized_label("online", language)
        self._attr_device_class = binary_device_class_for_field("online")
        self._attr_entity_category = EntityCategory.DIAGNOSTIC

    @property
    def is_on(self) -> bool:
        return available_for_record(self.record)


class DahuaBridgeStateBinarySensor(DahuaBridgeEntity, BinarySensorEntity):
    def __init__(
        self, coordinator, device_id: str, field: str, language: str = "en"
    ) -> None:
        super().__init__(coordinator, device_id)
        self._field = field
        self._attr_unique_id = f"{device_id}_{field}"
        self._attr_name = name_for_field(field, language)
        self._attr_device_class = binary_device_class_for_field(field)
        self._attr_entity_category = entity_category_for_field(field)

    @property
    def is_on(self) -> bool:
        return bool(value_for_field(self.record or {}, self._field))

    @property
    def available(self) -> bool:
        return super().available and (
            self.device_online or not field_requires_online(self._field)
        )
