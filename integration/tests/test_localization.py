from __future__ import annotations

import sys
import unittest
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge.catalog import (  # noqa: E402
    button_specs_for_record,
    name_for_field,
    switch_specs_for_record,
)
from custom_components.dahuabridge.localization import (  # noqa: E402
    localized_label,
    normalize_language_choice,
    resolve_language,
)


class LocalizationTests(unittest.TestCase):
    def test_language_choice_normalization(self) -> None:
        self.assertEqual(normalize_language_choice("uk"), "uk")
        self.assertEqual(normalize_language_choice("ua"), "uk")
        self.assertEqual(normalize_language_choice("Українська"), "uk")
        self.assertEqual(normalize_language_choice("english"), "en")
        self.assertEqual(normalize_language_choice("missing"), "auto")

    def test_auto_language_uses_home_assistant_language(self) -> None:
        self.assertEqual(resolve_language("auto", "uk-UA"), "uk")
        self.assertEqual(resolve_language("auto", "en-US"), "en")
        self.assertEqual(resolve_language("uk", "en-US"), "uk")

    def test_ukrainian_catalog_labels(self) -> None:
        record = {
            "device": {"id": "front_vto", "kind": "vto"},
            "stream": {
                "intercom": {
                    "answer_url": "/api/v1/vto/front/answer",
                    "recording_url": "/api/v1/vto/front/recording",
                    "supports_vto_recording_control": True,
                    "auto_record_enabled": True,
                    "lock_urls": ["/api/v1/vto/front/locks/1/unlock"],
                }
            },
        }

        buttons = button_specs_for_record(record, "uk")
        switches = switch_specs_for_record(record, "uk")

        self.assertIn("Відповісти на виклик", [spec.name for spec in buttons])
        self.assertIn("Відкрити 1", [spec.name for spec in buttons])
        self.assertEqual(switches[0].name, "Автозапис")
        self.assertEqual(name_for_field("stream_available", "uk"), "Потік доступний")
        self.assertEqual(localized_label("camera", "uk"), "Камера")


if __name__ == "__main__":
    unittest.main()
