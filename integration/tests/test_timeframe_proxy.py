from __future__ import annotations

import asyncio
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
from custom_components.dahuabridge.proxy.timeframe_playback import (  # noqa: E402
    playback_sessions_url,
    select_mjpeg_url,
    select_mjpeg_urls,
)
from custom_components.dahuabridge.api import DahuaBridgeAPI  # noqa: E402
from custom_components.dahuabridge import timeframe_proxy as timeframe_proxy_module  # noqa: E402
from custom_components.dahuabridge.timeframe_proxy import (  # noqa: E402
    DahuaBridgeTimeframeProxyView,
    _camera_proxy_request_authenticated,
)


class FakeConfig:
    time_zone = "Europe/Kiev"


class FakeHass:
    config = FakeConfig()


class FakeAPI:
    base_url = "https://ha.example.com/dahua-bridge"

    def __init__(self) -> None:
        self.bytes_requests: list[str] = []

    def bridge_resource_url(self, target: str) -> str:
        if target.startswith("https://ha.example.com/dahua-bridge/"):
            return target
        if target.startswith("https://ha.example.com/"):
            return target.replace(
                "https://ha.example.com/",
                "https://ha.example.com/dahua-bridge/",
                1,
            )
        if target.startswith("/"):
            return f"{self.base_url}{target}"
        return f"{self.base_url}/{target}"

    def absolute_url(self, target: str) -> str:
        return self.bridge_resource_url(target)

    async def async_get_bytes(self, target: str) -> bytes:
        self.bytes_requests.append(target)
        return b"snapshot"


class FakeCoordinator:
    def __init__(self) -> None:
        self.api = FakeAPI()


class FakeRequest(dict):
    def __init__(self, query: dict[str, str] | None = None, authenticated=False) -> None:
        super().__init__()
        self.query = query or {}
        if authenticated:
            self["ha_authenticated"] = True


class FakeWebResponse:
    def __init__(
        self, body: bytes, status: int = 200, headers: dict[str, str] | None = None
    ) -> None:
        self.body = body
        self.status = status
        self.headers = headers or {}


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

    def test_timeframe_proxy_accepts_signed_home_assistant_request(self) -> None:
        self.assertTrue(
            _camera_proxy_request_authenticated(FakeRequest(authenticated=True), {})
        )

    def test_timeframe_proxy_accepts_camera_access_token(self) -> None:
        self.assertTrue(
            _camera_proxy_request_authenticated(
                FakeRequest({"token": "abc"}), {"access_token": "abc"}
            )
        )

    def test_timeframe_proxy_rejects_unauthenticated_request(self) -> None:
        self.assertFalse(
            _camera_proxy_request_authenticated(
                FakeRequest({"token": "wrong"}), {"access_token": "abc"}
            )
        )

    def test_playback_sessions_url_preserves_proxy_base_path(self) -> None:
        self.assertEqual(
            playback_sessions_url(
                FakeCoordinator(),
                {
                    "bridge_playback_sessions_url": (
                        "https://ha.example.com/api/v1/nvr/west20_nvr/playback/sessions"
                    )
                },
            ),
            "https://ha.example.com/dahua-bridge/api/v1/nvr/west20_nvr/playback/sessions",
        )

    def test_bridge_resource_url_preserves_media_query_order(self) -> None:
        api = DahuaBridgeAPI(object(), "https://ha.example.com/dahua-bridge")

        self.assertEqual(
            api.bridge_resource_url(
                "https://ha.example.com/api/v1/media/mjpeg/nvrpb_test?profile=quality&width=640"
            ),
            "https://ha.example.com/dahua-bridge/api/v1/media/mjpeg/nvrpb_test?profile=quality&width=640",
        )

    def test_select_mjpeg_urls_returns_ordered_profile_fallbacks(self) -> None:
        payload = {
            "recommended_profile": "stable",
            "profiles": {
                "quality": {"mjpeg_url": "/api/v1/media/mjpeg/session?profile=quality"},
                "stable": {"mjpeg_url": "/api/v1/media/mjpeg/session?profile=stable"},
            },
        }

        self.assertEqual(
            select_mjpeg_url(payload, "quality"),
            "/api/v1/media/mjpeg/session?profile=quality",
        )
        self.assertEqual(
            select_mjpeg_urls(payload, "quality"),
            [
                "/api/v1/media/mjpeg/session?profile=quality",
                "/api/v1/media/mjpeg/session?profile=stable",
            ],
        )

    def test_timeframe_snapshot_does_not_forward_requested_width(self) -> None:
        coordinator = FakeCoordinator()
        view = DahuaBridgeTimeframeProxyView()

        original_response = timeframe_proxy_module.web.Response
        timeframe_proxy_module.web.Response = FakeWebResponse
        try:
            response = asyncio.run(
                view._snapshot(
                    FakeRequest({"width": "320"}),
                    {
                        "coordinator": coordinator,
                        "entity_id": "camera.front_gate",
                        "profile_name": "quality",
                        "session_payload": {"stream_id": "nvrpb_front_gate"},
                    },
                )
            )
        finally:
            timeframe_proxy_module.web.Response = original_response

        self.assertEqual(response.status, 200)
        self.assertEqual(response.body, b"snapshot")
        self.assertEqual(
            coordinator.api.bytes_requests,
            ["/api/v1/media/snapshot/nvrpb_front_gate?profile=quality"],
        )


if __name__ == "__main__":
    unittest.main()
