from __future__ import annotations

import sys
import unittest
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge.catalog import capture_for_record


class CaptureCatalogTests(unittest.TestCase):
    def test_capture_for_record_returns_capture_mapping(self) -> None:
        record = {
            "stream": {
                "capture": {
                    "snapshot_url": "/api/v1/media/snapshot/front_gate",
                    "start_recording_url": "/api/v1/media/streams/front_gate/recordings",
                    "active": True,
                }
            }
        }

        capture = capture_for_record(record)

        self.assertEqual(
            capture["snapshot_url"], "/api/v1/media/snapshot/front_gate"
        )
        self.assertEqual(
            capture["start_recording_url"],
            "/api/v1/media/streams/front_gate/recordings",
        )
        self.assertTrue(capture["active"])

    def test_capture_for_record_returns_empty_mapping_when_missing(self) -> None:
        self.assertEqual(capture_for_record({"stream": {}}), {})
        self.assertEqual(capture_for_record(None), {})


if __name__ == "__main__":
    unittest.main()
