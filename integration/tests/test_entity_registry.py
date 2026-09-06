from __future__ import annotations

import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock, patch

from ha_stubs import install

install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge import entity


class DeviceParentTests(unittest.TestCase):
    def setUp(self):
        self.parent = {"device": {"id": "nvr", "name": "Recorder"}}
        self.cameras = [
            {"device": {"id": f"camera_{index}", "parent_id": "nvr"}}
            for index in range(1, 12)
        ]
        self.coordinator = SimpleNamespace(
            data={"devices": [self.parent, *self.cameras]},
            api=SimpleNamespace(base_url="http://bridge.local"),
            hass=object(),
            config_entry=SimpleNamespace(entry_id="entry-id"),
        )

    def test_all_eleven_camera_parents_use_scoped_registry_id(self):
        lookup = Mock(return_value="ha-recorder-id")
        with patch.object(entity.dr, "async_get_device_id_by_identifier", lookup, create=True):
            for index in range(1, 12):
                info = entity.DahuaBridgeEntity(self.coordinator, f"camera_{index}").device_info
                self.assertEqual(info["via_device_id"], "ha-recorder-id")
                self.assertNotIn("via_device", info)
        self.assertEqual(lookup.call_count, 11)
        lookup.assert_called_with(self.coordinator.hass, ("dahuabridge", "nvr"), config_entry_id="entry-id")

    def test_parent_is_registered_when_camera_platform_runs_first(self):
        registry = SimpleNamespace(async_get_or_create=Mock(return_value=SimpleNamespace(id="new-parent")))
        with patch.object(entity.dr, "async_get_device_id_by_identifier", Mock(side_effect=ValueError), create=True), patch.object(
            entity.dr, "async_get", Mock(return_value=registry), create=True
        ):
            info = entity.DahuaBridgeEntity(self.coordinator, "camera_2").device_info
        self.assertEqual(info["via_device_id"], "new-parent")
        args = registry.async_get_or_create.call_args.kwargs
        self.assertEqual(args["config_entry_id"], "entry-id")
        self.assertEqual(args["identifiers"], {("dahuabridge", "nvr")})
        self.assertEqual(args["name"], "Recorder")
        self.assertNotIn("via_device", args)

    def test_older_ha_retains_legacy_parent_link(self):
        with patch.object(entity.dr, "async_get_device_id_by_identifier", None, create=True):
            info = entity.DahuaBridgeEntity(self.coordinator, "camera_2").device_info
        self.assertEqual(info["via_device"], ("dahuabridge", "nvr"))
        self.assertNotIn("via_device_id", info)

    def test_missing_or_self_parent_does_not_block_entity_creation(self):
        lookup = Mock(side_effect=AssertionError("must not resolve invalid parent"))
        with patch.object(entity.dr, "async_get_device_id_by_identifier", lookup, create=True):
            for parent_id in ("missing", "camera_1"):
                self.cameras[0]["device"]["parent_id"] = parent_id
                info = entity.DahuaBridgeEntity(self.coordinator, "camera_1").device_info
                self.assertNotIn("via_device", info)
                self.assertNotIn("via_device_id", info)


if __name__ == "__main__":
    unittest.main()
