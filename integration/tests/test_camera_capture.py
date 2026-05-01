from __future__ import annotations

import sys
import unittest
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from homeassistant.exceptions import HomeAssistantError

from custom_components.dahuabridge.bridge_api import DahuaBridgeAPI
from custom_components.dahuabridge.bridge_api import DahuaBridgeAPIError
from custom_components.dahuabridge.camera import DahuaBridgeCamera


def make_record(capture: dict | None = None) -> dict:
    return {
        "device": {
            "id": "cam1",
            "parent_id": "west20_nvr",
            "name": "Front Gate",
            "kind": "nvr_channel",
        },
        "state": {"available": True},
        "stream": {
            "channel": 5,
            "recommended_profile": "stable",
            "profiles": {
                "stable": {
                    "local_mjpeg_url": "/api/v1/media/mjpeg/cam1?profile=stable",
                    "local_hls_url": "/api/v1/media/hls/cam1/stable/index.m3u8",
                }
            },
            "capture": capture or {},
        },
    }


class FakeAPI:
    def __init__(self) -> None:
        self.base_url = "http://bridge.local:8080"
        self.bytes_requests: list[str] = []
        self.mjpeg_requests: list[str] = []
        self.post_json_requests: list[tuple[str, dict]] = []
        self.post_action_requests: list[str] = []
        self.fail_snapshot = False

    def bridge_resource_url(self, target: str) -> str:
        if target.startswith("http://") or target.startswith("https://"):
            return target
        if not target.startswith("/"):
            target = "/" + target
        return self.base_url + target

    def absolute_url(self, target: str) -> str:
        return self.bridge_resource_url(target)

    async def async_get_bytes(self, target: str) -> bytes:
        self.bytes_requests.append(target)
        if self.fail_snapshot:
            raise DahuaBridgeAPIError("snapshot failed")
        return b"snapshot"

    async def async_get_mjpeg_frame(self, target: str) -> bytes:
        self.mjpeg_requests.append(target)
        return b"mjpeg"

    async def async_post_json(self, target: str, payload: dict) -> dict:
        self.post_json_requests.append((target, payload))
        return {"status": "ok"}

    async def async_post_action(self, target: str) -> dict:
        self.post_action_requests.append(target)
        return {"status": "ok"}


class FakeCoordinator:
    def __init__(self, record: dict) -> None:
        self.api = FakeAPI()
        self.data = {"devices": [record]}
        self.preferred_video_profile = "stable"
        self.preferred_video_source = "hls"
        self.refresh_count = 0
        self.last_update_success = True

    async def async_request_refresh(self) -> None:
        self.refresh_count += 1


class CameraCaptureTests(unittest.IsolatedAsyncioTestCase):
    def test_bridge_api_preserves_rtsp_targets(self) -> None:
        api = DahuaBridgeAPI(object(), "http://bridge.local:8080")
        self.assertEqual(
            api.bridge_resource_url("rtsp://camera.local:554/cam/realmonitor?channel=1"),
            "rtsp://camera.local:554/cam/realmonitor?channel=1",
        )

    def test_bridge_api_preserves_base_path_when_rewriting_bridge_urls(self) -> None:
        api = DahuaBridgeAPI(object(), "https://public.example/bridge")
        self.assertEqual(
            api.bridge_resource_url("http://127.0.0.1:19215/api/v1/events"),
            "https://public.example/bridge/api/v1/events",
        )

    def test_is_recording_reflects_bridge_capture_state(self) -> None:
        camera = DahuaBridgeCamera(
            FakeCoordinator(make_record({"active": True})),
            "cam1",
        )
        self.assertTrue(camera.is_recording)

    def test_camera_available_follows_coordinator_success(self) -> None:
        coordinator = FakeCoordinator(make_record())
        camera = DahuaBridgeCamera(coordinator, "cam1")
        self.assertTrue(camera.available)
        coordinator.last_update_success = False
        self.assertFalse(camera.available)

    async def test_async_camera_image_prefers_snapshot_endpoint(self) -> None:
        camera = DahuaBridgeCamera(
            FakeCoordinator(
                make_record({"snapshot_url": "/api/v1/media/snapshot/cam1"})
            ),
            "cam1",
        )

        image = await camera.async_camera_image(width=640)

        self.assertEqual(image, b"snapshot")
        self.assertEqual(
            camera.coordinator.api.bytes_requests,
            ["http://bridge.local:8080/api/v1/media/snapshot/cam1?width=640"],
        )
        self.assertEqual(camera.coordinator.api.mjpeg_requests, [])

    async def test_async_camera_image_falls_back_to_mjpeg(self) -> None:
        coordinator = FakeCoordinator(
            make_record({"snapshot_url": "/api/v1/media/snapshot/cam1"})
        )
        coordinator.api.fail_snapshot = True
        camera = DahuaBridgeCamera(coordinator, "cam1")

        image = await camera.async_camera_image(width=320)

        self.assertEqual(image, b"mjpeg")
        self.assertEqual(
            coordinator.api.mjpeg_requests,
            ["http://bridge.local:8080/api/v1/media/mjpeg/cam1?profile=stable&width=320"],
        )

    def test_camera_attributes_include_archive_endpoints_for_nvr_channels(self) -> None:
        camera = DahuaBridgeCamera(FakeCoordinator(make_record()), "cam1")

        attrs = camera.extra_state_attributes

        self.assertEqual(
            attrs["bridge_archive_export_url"],
            "http://bridge.local:8080/api/v1/nvr/west20_nvr/recordings/export",
        )
        self.assertEqual(
            attrs["bridge_playback_sessions_url"],
            "http://bridge.local:8080/api/v1/nvr/west20_nvr/playback/sessions",
        )
        self.assertEqual(
            attrs["bridge_archive_recordings_url_template"],
            "http://bridge.local:8080/api/v1/nvr/west20_nvr/recordings?channel=5&start={start}&end={end}&limit={limit}&event={event}",
        )

    async def test_async_start_recording_calls_bridge_capture_service(self) -> None:
        camera = DahuaBridgeCamera(
            FakeCoordinator(
                make_record(
                    {
                        "start_recording_url": "/api/v1/media/streams/cam1/recordings",
                    }
                )
            ),
            "cam1",
        )

        await camera.async_start_recording(profile="quality", duration_seconds=12)

        self.assertEqual(
            camera.coordinator.api.post_json_requests,
            [
                (
                    "/api/v1/media/streams/cam1/recordings",
                    {"profile": "quality", "duration_seconds": 12},
                )
            ],
        )
        self.assertEqual(camera.coordinator.refresh_count, 1)

    async def test_async_stop_recording_calls_bridge_capture_service(self) -> None:
        camera = DahuaBridgeCamera(
            FakeCoordinator(
                make_record(
                    {
                        "stop_recording_url": "/api/v1/media/recordings/clip_1/stop",
                    }
                )
            ),
            "cam1",
        )

        await camera.async_stop_recording()

        self.assertEqual(
            camera.coordinator.api.post_action_requests,
            ["/api/v1/media/recordings/clip_1/stop"],
        )
        self.assertEqual(camera.coordinator.refresh_count, 1)

    async def test_async_start_recording_rejects_missing_capture_url(self) -> None:
        camera = DahuaBridgeCamera(FakeCoordinator(make_record()), "cam1")

        with self.assertRaises(HomeAssistantError):
            await camera.async_start_recording()


if __name__ == "__main__":
    unittest.main()
