from __future__ import annotations

import sys
import unittest
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge.catalog import (
    capture_for_record,
    source_order_for_preference,
    stream_source_for_record_with_preferences,
)


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

    def test_source_order_prefers_direct_rtsp_with_dash_bridge_fallbacks(self) -> None:
        self.assertEqual(
            source_order_for_preference("rtsp"),
            ("stream_url", "local_hls_url", "local_dash_url", "local_mjpeg_url"),
        )
        self.assertEqual(
            source_order_for_preference("hls"),
            ("local_hls_url", "local_dash_url", "local_mjpeg_url", "stream_url"),
        )
        self.assertEqual(
            source_order_for_preference("dash"),
            ("local_dash_url", "local_hls_url", "local_mjpeg_url", "stream_url"),
        )
        self.assertEqual(source_order_for_preference("rtsp", False), ("stream_url",))
        self.assertEqual(source_order_for_preference("hls", False), ("local_hls_url",))

    def test_stream_source_uses_dash_between_hls_and_mjpeg(self) -> None:
        record = {
            "stream": {
                "recommended_profile": "quality",
                "profiles": {
                    "quality": {
                        "stream_url": "rtsp://camera.local:554/cam/realmonitor",
                        "local_dash_url": "/api/v1/media/dash/cam1/quality/manifest.mpd",
                        "local_mjpeg_url": "/api/v1/media/mjpeg/cam1/quality",
                    }
                },
            }
        }

        self.assertEqual(
            stream_source_for_record_with_preferences(record, "quality", "hls"),
            "/api/v1/media/dash/cam1/quality/manifest.mpd",
        )
        self.assertIsNone(
            stream_source_for_record_with_preferences(record, "quality", "hls", False)
        )


if __name__ == "__main__":
    unittest.main()
