from __future__ import annotations

import sys
import unittest
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge.diagnostics import REDACTED, redact_payload


class DiagnosticsTests(unittest.TestCase):
    def test_redact_payload_redacts_url_shaped_keys(self) -> None:
        payload = {
            "export_url": "http://bridge.local/export",
            "download_url": "http://bridge.local/download",
            "bridge_archive_export_url": "http://bridge.local/archive/export",
            "stream_url": "rtsp://camera.local/live",
            "nested": {
                "url": "http://bridge.local/action",
                "keep": "value",
            },
        }

        redacted = redact_payload(payload)

        self.assertEqual(redacted["export_url"], REDACTED)
        self.assertEqual(redacted["download_url"], REDACTED)
        self.assertEqual(redacted["bridge_archive_export_url"], REDACTED)
        self.assertEqual(redacted["stream_url"], REDACTED)
        self.assertEqual(redacted["nested"]["url"], REDACTED)
        self.assertEqual(redacted["nested"]["keep"], "value")


if __name__ == "__main__":
    unittest.main()
