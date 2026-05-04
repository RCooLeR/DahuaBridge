from __future__ import annotations

import sys
import unittest
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge.discovery import (  # noqa: E402
    CatalogEntityCandidate,
    setup_catalog_entity_discovery,
)


class FakeEntry:
    def __init__(self) -> None:
        self.unloads = []

    def async_on_unload(self, unload) -> None:
        self.unloads.append(unload)


class FakeCoordinator:
    def __init__(self) -> None:
        self.data = {
            "devices": [
                {"device": {"id": "cam1"}},
                {"device": {"id": "cam2"}},
            ]
        }
        self.listener = None

    def async_add_listener(self, listener):
        self.listener = listener
        return "unload-callback"


class CatalogDiscoveryTests(unittest.TestCase):
    def test_seen_entities_are_not_added_again_on_refresh(self) -> None:
        coordinator = FakeCoordinator()
        entry = FakeEntry()
        added_entities: list[str] = []

        def collect(record):
            device_id = record["device"]["id"]
            return [
                CatalogEntityCandidate(
                    key=device_id,
                    create_entity=lambda device_id=device_id: f"entity:{device_id}",
                )
            ]

        setup_catalog_entity_discovery(
            entry,
            coordinator,
            set(),
            added_entities.extend,
            collect,
        )

        self.assertEqual(added_entities, ["entity:cam1", "entity:cam2"])

        added_entities.clear()
        coordinator.listener()

        self.assertEqual(added_entities, [])


if __name__ == "__main__":
    unittest.main()
