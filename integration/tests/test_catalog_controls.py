from __future__ import annotations

import sys
import unittest
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge.catalog import (  # noqa: E402
    bool_switch_value_for_record,
    switch_payload_for_value,
    switch_specs_for_record,
)


class CatalogControlTests(unittest.TestCase):
    def test_switch_specs_include_camera_deterrence_toggles(self) -> None:
        record = {
            "device": {
                "id": "cam1",
                "kind": "nvr_channel",
            },
            "stream": {
                "features": [
                    {
                        "key": "light",
                        "label": "White Light",
                        "supported": True,
                        "parameter_key": "output",
                        "parameter_value": "light",
                        "actions": ["start", "stop"],
                        "active": True,
                        "url": "/api/v1/nvr/west20/channels/5/aux",
                    },
                    {
                        "key": "warning_light",
                        "label": "Warning Light",
                        "supported": True,
                        "parameter_key": "output",
                        "parameter_value": "warning_light",
                        "actions": ["start", "stop"],
                        "active": False,
                        "url": "/api/v1/nvr/west20/channels/5/aux",
                    },
                    {
                        "key": "siren",
                        "label": "Siren",
                        "supported": True,
                        "parameter_key": "output",
                        "parameter_value": "siren",
                        "actions": ["start", "stop"],
                        "active": False,
                        "url": "/api/v1/nvr/west20/channels/5/aux",
                    },
                ]
            },
        }

        specs = switch_specs_for_record(record)

        self.assertEqual([spec.key for spec in specs], ["light", "warning_light", "siren"])
        self.assertEqual(
            switch_payload_for_value(specs[0], True),
            {"output": "light", "action": "start"},
        )
        self.assertEqual(
            switch_payload_for_value(specs[0], False),
            {"output": "light", "action": "stop"},
        )
        self.assertTrue(bool_switch_value_for_record(record, specs[0]))
        self.assertFalse(bool_switch_value_for_record(record, specs[1]))

    def test_vto_switch_specs_keep_boolean_payloads(self) -> None:
        record = {
            "device": {
                "id": "front_vto",
                "kind": "vto",
            },
            "stream": {
                "intercom": {
                    "mute_url": "/api/v1/vto/front_vto/audio/mute",
                    "supports_vto_mute_control": True,
                    "muted": True,
                }
            },
        }

        specs = switch_specs_for_record(record)

        self.assertEqual(len(specs), 1)
        self.assertEqual(specs[0].key, "muted")
        self.assertEqual(switch_payload_for_value(specs[0], False), {"muted": False})
        self.assertTrue(bool_switch_value_for_record(record, specs[0]))


if __name__ == "__main__":
    unittest.main()
