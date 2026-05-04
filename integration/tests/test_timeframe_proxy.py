from __future__ import annotations

import sys
import unittest
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge.proxy.timeframe_datetime import (  # noqa: E402
    bridge_playback_datetime,
    datetime_for_compare,
    parse_query_datetime,
)


class FakeConfig:
    time_zone = "Europe/Kiev"


class FakeHass:
    config = FakeConfig()


class TimeframeProxyTests(unittest.TestCase):
    def test_dahua_timeframe_query_is_forwarded_as_nvr_wall_clock(self) -> None:
        parsed = parse_query_datetime(
            FakeHass(),
            "2026_05_03_16_11_05",
            "starttime",
        )

        self.assertIsNotNone(parsed)
        self.assertIsNone(parsed.tzinfo)
        self.assertEqual(
            bridge_playback_datetime(parsed),
            "2026-05-03 16:11:05",
        )
        self.assertIsNotNone(datetime_for_compare(FakeHass(), parsed).tzinfo)

    def test_iso_timeframe_query_keeps_absolute_time(self) -> None:
        parsed = parse_query_datetime(
            FakeHass(),
            "2026-05-03T16:11:05+03:00",
            "starttime",
        )

        self.assertIsNotNone(parsed)
        self.assertEqual(
            bridge_playback_datetime(parsed),
            "2026-05-03T13:11:05+00:00",
        )


if __name__ == "__main__":
    unittest.main()
