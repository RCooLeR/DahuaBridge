from __future__ import annotations

from homeassistant.components.button import ButtonEntity
from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant, callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from .catalog import button_specs_for_record, catalog_records, device_id_for_record
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
        new_entities: list[ButtonEntity] = []
        desired_unique_ids: set[str] = set()

        for record in catalog_records(coordinator.data):
            device_id = device_id_for_record(record)
            if not device_id:
                continue

            for spec in button_specs_for_record(record):
                entity_key = f"{device_id}:{spec.key}"
                desired_unique_ids.add(f"{device_id}_{spec.key}")
                if entity_key in seen:
                    continue
                seen.add(entity_key)
                new_entities.append(
                    DahuaBridgeActionButton(
                        coordinator, device_id, spec.key, spec.name, spec.url, spec.icon
                    )
                )

        if coordinator.can_prune_registry:
            prune_stale_entities(
                hass,
                entry,
                "button",
                desired_unique_ids,
                coordinator.stale_entity_miss_counts,
            )
        if new_entities:
            async_add_entities(new_entities)

    async_discover_entities()
    entry.async_on_unload(coordinator.async_add_listener(async_discover_entities))


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
