from __future__ import annotations

from functools import partial
from typing import Any

from homeassistant.components.switch import SwitchEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .catalog import (
    bool_switch_value_for_record,
    device_id_for_record,
    switch_payload_for_value,
    switch_specs_for_record,
)
from .const import DOMAIN
from .discovery import CatalogEntityCandidate, setup_catalog_entity_discovery
from .entity import DahuaBridgeEntity


async def async_setup_entry(
    hass: HomeAssistant, entry: ConfigEntry, async_add_entities: AddEntitiesCallback
) -> None:
    coordinator = hass.data[DOMAIN][entry.entry_id]
    seen: set[str] = set()
    language = getattr(coordinator, "integration_language", "en")

    def collect_switches(record: dict[str, Any]):
        device_id = device_id_for_record(record)
        if not device_id:
            return []

        return [
            CatalogEntityCandidate(
                key=f"{device_id}:{spec.key}",
                create_entity=partial(DahuaBridgeControlSwitch, coordinator, device_id, spec),
            )
            for spec in switch_specs_for_record(record, language)
        ]

    setup_catalog_entity_discovery(
        entry,
        coordinator,
        seen,
        async_add_entities,
        collect_switches,
    )


class DahuaBridgeControlSwitch(DahuaBridgeEntity, SwitchEntity):
    def __init__(self, coordinator, device_id: str, spec) -> None:
        super().__init__(coordinator, device_id)
        self._spec = spec
        self._target_url = spec.url
        self._attr_unique_id = f"{device_id}_{spec.key}"
        self._attr_name = spec.name
        self._attr_icon = spec.icon

    @property
    def is_on(self) -> bool:
        return bool(bool_switch_value_for_record(self.record, self._spec))

    @property
    def available(self) -> bool:
        return super().available and self.device_online

    async def async_turn_on(self, **kwargs) -> None:
        await self._async_set_state(True)

    async def async_turn_off(self, **kwargs) -> None:
        await self._async_set_state(False)

    async def _async_set_state(self, value: bool) -> None:
        await self.coordinator.api.async_post_json(
            self._target_url,
            switch_payload_for_value(self._spec, value),
        )
        await self.coordinator.async_request_refresh()
