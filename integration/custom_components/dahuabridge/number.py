from __future__ import annotations

from homeassistant.components.number import NumberEntity, NumberMode
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .catalog import (
    catalog_records,
    device_id_for_record,
    int_intercom_value_for_record,
    number_specs_for_record,
)
from .const import DOMAIN
from .entity import DahuaBridgeEntity
from .registry_cleanup import prune_stale_entities


async def async_setup_entry(
    hass: HomeAssistant, entry: ConfigEntry, async_add_entities: AddEntitiesCallback
) -> None:
    coordinator = hass.data[DOMAIN][entry.entry_id]
    seen: set[str] = set()

    @callback
    def async_discover_entities() -> None:
        new_entities: list[NumberEntity] = []
        desired_unique_ids: set[str] = set()

        for record in catalog_records(coordinator.data):
            device_id = device_id_for_record(record)
            if not device_id:
                continue

            for spec in number_specs_for_record(record):
                entity_key = f"{device_id}:{spec.key}"
                desired_unique_ids.add(f"{device_id}_{spec.key}")
                if entity_key in seen:
                    continue
                seen.add(entity_key)
                new_entities.append(
                    DahuaBridgeControlNumber(coordinator, device_id, spec)
                )

        if coordinator.can_prune_registry:
            prune_stale_entities(
                hass,
                entry,
                "number",
                desired_unique_ids,
                coordinator.stale_entity_miss_counts,
            )
        if new_entities:
            async_add_entities(new_entities)

    async_discover_entities()
    entry.async_on_unload(coordinator.async_add_listener(async_discover_entities))


class DahuaBridgeControlNumber(DahuaBridgeEntity, NumberEntity):
    def __init__(self, coordinator, device_id: str, spec) -> None:
        super().__init__(coordinator, device_id)
        self._spec = spec
        self._target_url = spec.url
        self._attr_unique_id = f"{device_id}_{spec.key}"
        self._attr_name = spec.name
        self._attr_icon = spec.icon
        self._attr_native_min_value = spec.min_value
        self._attr_native_max_value = spec.max_value
        self._attr_native_step = spec.step
        self._attr_mode = NumberMode.SLIDER

    @property
    def native_value(self) -> float | None:
        value = int_intercom_value_for_record(self.record, self._spec.value_key)
        if value is None:
            return None
        return float(value)

    @property
    def available(self) -> bool:
        return super().available and self.device_online

    async def async_set_native_value(self, value: float) -> None:
        level = int(round(value))
        await self.coordinator.api.async_post_json(
            self._target_url,
            {
                "slot": self._spec.slot,
                "level": level,
            },
        )
        await self.coordinator.async_request_refresh()
