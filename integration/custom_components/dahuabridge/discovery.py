from __future__ import annotations

from collections.abc import Callable, Iterable
from dataclasses import dataclass
from typing import Any

from homeassistant.core import callback
from homeassistant.helpers.entity_platform import AddEntitiesCallback

from . import DahuaBridgeConfigEntry
from .catalog import catalog_records

EntityFactory = Callable[[], Any]
EntityCollector = Callable[[dict[str, Any]], Iterable["CatalogEntityCandidate"]]


@dataclass(frozen=True, slots=True)
class CatalogEntityCandidate:
    key: str
    create_entity: EntityFactory


def setup_catalog_entity_discovery(
    entry: DahuaBridgeConfigEntry,
    coordinator: Any,
    seen: set[str],
    async_add_entities: AddEntitiesCallback,
    collect_entities: EntityCollector,
) -> None:
    @callback
    def async_discover_entities() -> None:
        new_entities: list[Any] = []

        for record in catalog_records(coordinator.data):
            for candidate in collect_entities(record):
                if candidate.key in seen:
                    continue
                seen.add(candidate.key)
                new_entities.append(candidate.create_entity())

        if new_entities:
            async_add_entities(new_entities)

    async_discover_entities()
    entry.async_on_unload(coordinator.async_add_listener(async_discover_entities))
