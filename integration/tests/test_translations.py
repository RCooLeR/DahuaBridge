from __future__ import annotations

import json
from pathlib import Path
import unittest


class TranslationValidityTests(unittest.TestCase):
    def test_all_translations_are_utf8_json_without_surrogate_text(self):
        translations = Path(__file__).resolve().parents[1] / "custom_components" / "dahuabridge" / "translations"
        files = sorted(translations.glob("*.json"))
        self.assertTrue(files, "No translation files were found")
        for path in files:
            with self.subTest(language=path.stem):
                # Mixed encodings can prevent the entire integration from loading.
                payload = json.loads(path.read_text(encoding="utf-8", errors="strict"))
                # JSON permits escaped lone surrogates; HA's JSON loader does not.
                json.dumps(payload, ensure_ascii=False).encode("utf-8", errors="strict")


if __name__ == "__main__":
    unittest.main()
