from __future__ import annotations

from functools import partial
from typing import Any

from homeassistant.components.button import ButtonEntity
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from . import DahuaBridgeConfigEntry
from .catalog import button_specs_for_record, device_id_for_record
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

    def collect_buttons(record: dict[str, Any]):
        device_id = device_id_for_record(record)
        if not device_id:
            return []

        return [
            CatalogEntityCandidate(
                key=f"{device_id}:{spec.key}",
                create_entity=partial(
                    DahuaBridgeActionButton,
                    coordinator,
                    device_id,
                    spec.key,
                    spec.name,
                    spec.url,
                    spec.icon,
                ),
            )
            for spec in button_specs_for_record(record, language)
        ]

    setup_catalog_entity_discovery(
        entry,
        coordinator,
        seen,
        async_add_entities,
        collect_buttons,
    )


class DahuaBridgeActionButton(DahuaBridgeEntity, ButtonEntity):
    def __init__(
        self,
        coordinator,
        device_id: str,
        key: str,
        name: str,
        target_url: str,
        icon: str,
    ) -> None:
        super().__init__(coordinator, device_id)
        self._target_url = target_url
        self._attr_unique_id = f"{device_id}_{key}"
        self._attr_name = name
        self._attr_icon = icon

    async def async_press(self) -> None:
        await self.coordinator.api.async_post_action(self._target_url)
        await self.coordinator.async_request_refresh()

    @property
    def available(self) -> bool:
        return super().available and self.device_online
