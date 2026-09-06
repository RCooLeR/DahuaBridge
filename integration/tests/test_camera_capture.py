from __future__ import annotations

import sys
import unittest
import asyncio
from types import SimpleNamespace
from pathlib import Path

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from homeassistant.exceptions import HomeAssistantError

from custom_components.dahuabridge.api import DahuaBridgeAPI
from custom_components.dahuabridge.api import DahuaBridgeAPIError
from custom_components.dahuabridge.api.client import redact_url_for_log
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
        self.put_json_requests: list[tuple[str, dict]] = []
        self.post_action_requests: list[str] = []
        self.fail_snapshot = False
        self.fail_put = False

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

    async def async_put_json(self, target: str, payload: dict) -> dict:
        if self.fail_put:
            raise DahuaBridgeAPIError("Source update failed")
        self.put_json_requests.append((target, payload))
        return {"live_source": {"source": payload["source"]}}


class FakeCoordinator:
    def __init__(self, record: dict) -> None:
        self.api = FakeAPI()
        self.data = {"devices": [record]}
        self.preferred_video_profile = "stable"
        self.preferred_video_source = "hls"
        self.video_fallbacks_enabled = True
        self.integration_language = "en"
        self.refresh_count = 0
        self.refresh_update = None
        self.last_update_success = True

    async def async_request_refresh(self) -> None:
        self.refresh_count += 1
        if callable(self.refresh_update):
            self.refresh_update()


class FakeBridgeResponse:
    def __init__(self, body: str = '{"ready": true}', status: int = 200) -> None:
        self._body = body
        self.status = status

    async def __aenter__(self) -> "FakeBridgeResponse":
        return self

    async def __aexit__(self, exc_type, exc, tb) -> None:
        return None

    async def text(self) -> str:
        return self._body

    async def read(self) -> bytes:
        return self._body.encode()


class HeaderCaptureSession:
    def __init__(self) -> None:
        self.requests: list[dict] = []

    def request(self, method: str, url: str, **kwargs) -> FakeBridgeResponse:
        self.requests.append({"method": method, "url": url, **kwargs})
        return FakeBridgeResponse()

    def get(self, url: str, **kwargs) -> FakeBridgeResponse:
        self.requests.append({"method": "GET", "url": url, **kwargs})
        return FakeBridgeResponse("snapshot-bytes")


class CameraCaptureTests(unittest.IsolatedAsyncioTestCase):
    async def test_live_source_put_is_authenticated(self) -> None:
        session = HeaderCaptureSession()
        api = DahuaBridgeAPI(session, "https://bridge.local", "secret-token")
        await api.async_put_json("/api/v1/streams/cam1/live-source", {"source": "camera"})
        self.assertEqual(session.requests[0]["method"], "PUT")
        self.assertEqual(session.requests[0]["json"], {"source": "camera"})
        self.assertEqual(session.requests[0]["headers"]["Authorization"], "Bearer secret-token")

    async def test_live_source_changes_refresh_catalog_without_resetting_stable_bridge_stream(self) -> None:
        record = make_record()
        record["stream"]["live_source"] = {
            "source": "nvr", "camera_available": True,
            "url": "/api/v1/streams/cam1/live-source",
        }
        camera = DahuaBridgeCamera(FakeCoordinator(record), "cam1")
        old_stream = FakeCameraStream()
        camera.stream = old_stream
        camera.coordinator.refresh_update = lambda: record["stream"]["live_source"].update({"source": "camera"})

        await camera.async_set_live_source("camera")

        self.assertEqual(camera.coordinator.api.put_json_requests, [
            ("http://bridge.local:8080/api/v1/streams/cam1/live-source", {"source": "camera"}),
        ])
        self.assertEqual(camera.coordinator.refresh_count, 1)
        self.assertEqual(old_stream.stop_count, 0)
        self.assertIs(camera.stream, old_stream)

    async def test_clearing_override_uses_default_even_when_camera_unavailable_without_restarting_same_input(self) -> None:
        record = make_record()
        record["stream"]["live_source"] = {
            "source": "nvr", "default_source": "camera", "override_source": "nvr",
            "camera_available": False, "url": "/api/v1/streams/cam1/live-source",
        }
        camera = DahuaBridgeCamera(FakeCoordinator(record), "cam1")
        stream = FakeCameraStream()
        camera.stream = stream
        camera.coordinator.refresh_update = lambda: record["stream"]["live_source"].update({"override_source": ""})

        await camera.async_set_live_source("default")

        self.assertEqual(camera.coordinator.api.put_json_requests, [
            ("http://bridge.local:8080/api/v1/streams/cam1/live-source", {"source": "default"}),
        ])
        self.assertEqual(camera.extra_state_attributes["bridge_live_source"]["override_source"], "")
        self.assertIs(camera.stream, stream)
        self.assertEqual(stream.stop_count, 0)

    def test_default_metadata_update_preserves_unchanged_effective_live_stream(self) -> None:
        record = make_record()
        record["stream"]["live_source"] = {
            "source": "nvr", "default_source": "nvr", "override_source": "nvr",
            "camera_available": True,
        }
        camera = DahuaBridgeCamera(FakeCoordinator(record), "cam1")
        stream = FakeCameraStream()
        camera.stream = stream
        record["stream"]["live_source"]["default_source"] = "camera"
        camera._handle_coordinator_update()
        self.assertIs(camera.stream, stream)
        self.assertEqual(stream.stop_count, 0)

    async def test_camera_preference_is_saved_when_unavailable_but_old_bridge_rejects_setting(self) -> None:
        record = make_record()
        camera = DahuaBridgeCamera(FakeCoordinator(record), "cam1")
        with self.assertRaisesRegex(HomeAssistantError, "not available"):
            await camera.async_set_live_source("camera")
        record["stream"]["live_source"] = {
            "source": "nvr", "camera_available": False,
            "camera_unavailable_reason": "Direct camera credentials are missing",
            "url": "/api/v1/streams/cam1/live-source",
        }
        await camera.async_set_live_source("camera")
        self.assertEqual(camera.coordinator.api.put_json_requests[0][1], {"source": "camera"})
        await camera.async_set_live_source("nvr")
        self.assertEqual(camera.coordinator.refresh_count, 2)

    async def test_failed_source_change_keeps_current_native_stream(self) -> None:
        record = make_record()
        record["stream"]["live_source"] = {
            "source": "nvr", "camera_available": True,
            "url": "/api/v1/streams/cam1/live-source",
        }
        camera = DahuaBridgeCamera(FakeCoordinator(record), "cam1")
        camera.coordinator.api.fail_put = True
        stream = FakeCameraStream()
        camera.stream = stream
        with self.assertRaises(DahuaBridgeAPIError):
            await camera.async_set_live_source("camera")
        self.assertIs(camera.stream, stream)
        self.assertEqual(stream.stop_count, 0)
        self.assertEqual(camera.coordinator.refresh_count, 0)
        self.assertEqual(camera.extra_state_attributes["bridge_live_source"]["source"], "nvr")

    async def test_unavailable_camera_preference_uses_effective_nvr_fallback(self) -> None:
        record = make_record()
        record["state"]["info"] = {"stream_available": True}
        record["stream"]["live_source"] = {"source": "nvr", "preferred_source": "camera", "camera_available": False, "fallback_reason": "Camera unavailable"}
        recorder_url = "rtsp://nvr-user:nvr-pass@nvr.local/cam/realmonitor?channel=5"
        record["stream"]["profiles"] = {
            "quality": {
                "stream_url": recorder_url,
                "recorder_stream_url": recorder_url,
                "local_hls_url": "/api/v1/media/hls/cam1/quality/index.m3u8",
            },
        }
        coordinator = FakeCoordinator(record)
        coordinator.preferred_video_source = "rtsp"
        coordinator.video_fallbacks_enabled = True
        camera = DahuaBridgeCamera(coordinator, "cam1")
        self.assertEqual(camera._stream_source(), recorder_url)
        self.assertTrue(camera.extra_state_attributes["stream_available"])

    def test_effective_camera_fallback_is_available_when_nvr_probe_is_down(self) -> None:
        record = make_record()
        record["state"]["info"] = {"stream_available": False}
        record["stream"]["live_source"] = {"source": "camera", "preferred_source": "nvr", "camera_available": True, "fallback_reason": "NVR unavailable"}
        camera = DahuaBridgeCamera(FakeCoordinator(record), "cam1")
        self.assertTrue(camera.extra_state_attributes["stream_available"])

    async def test_catalog_source_change_resets_stream_but_preserves_archive_playback(self) -> None:
        record = make_record()
        record["stream"]["live_source"] = {"source": "nvr"}
        record["stream"]["profiles"]["stable"]["stream_url"] = "rtsp://nvr.local/cam/realmonitor?channel=5"
        coordinator = FakeCoordinator(record)
        coordinator.preferred_video_source = "rtsp"
        camera = DahuaBridgeCamera(coordinator, "cam1")
        tasks = []
        camera.hass = SimpleNamespace(async_create_task=lambda coroutine: tasks.append(asyncio.create_task(coroutine)))
        old_stream = FakeCameraStream()
        camera.stream = old_stream
        record["stream"]["live_source"]["source"] = "camera"
        record["stream"]["profiles"]["stable"]["stream_url"] = "rtsp://camera.local/cam/realmonitor?channel=1"

        camera._handle_coordinator_update()
        self.assertIsNone(camera.stream)
        await asyncio.gather(*tasks)
        self.assertEqual(old_stream.stop_count, 1)

        await camera.async_set_native_playback_source("rtsp://nvr.local/cam/playback?channel=5")
        playback_stream = FakeCameraStream()
        camera.stream = playback_stream
        record["stream"]["live_source"]["source"] = "nvr"
        record["stream"]["profiles"]["stable"]["stream_url"] = "rtsp://nvr.local/cam/realmonitor?channel=5"
        camera._handle_coordinator_update()
        self.assertIs(camera.stream, playback_stream)
        self.assertEqual(playback_stream.stop_count, 0)

    def test_live_source_and_recorder_metadata_are_exposed_without_rewriting_rtsp(self) -> None:
        record = make_record()
        recorder_url = "rtsp://nvr-user:nvr-pass@nvr.local/cam/realmonitor?channel=5"
        record["stream"]["profiles"]["stable"]["recorder_stream_url"] = recorder_url
        record["stream"]["live_source"] = {
            "source": "camera", "camera_available": True,
            "url": "/api/v1/streams/cam1/live-source",
        }
        camera = DahuaBridgeCamera(FakeCoordinator(record), "cam1")
        attrs = camera.extra_state_attributes
        self.assertEqual(attrs["bridge_live_source"]["source"], "camera")
        self.assertEqual(attrs["bridge_live_source"]["url"], "http://bridge.local:8080/api/v1/streams/cam1/live-source")
        self.assertEqual(attrs["bridge_profiles"]["stable"]["recorder_stream_url"], recorder_url)
        del record["stream"]["live_source"]
        self.assertEqual(camera.extra_state_attributes["bridge_live_source"], {"source": "nvr", "camera_available": False})

    def test_bridge_relay_camera_does_not_request_upstream_archive_credentials(self) -> None:
        record = make_record()
        record["stream"]["profiles"]["stable"]["stream_url"] = "rtsp://bridge.local:8554/api/v1/rtsp/live/cam1/stable"
        coordinator = FakeCoordinator(record)
        coordinator.preferred_video_source = "rtsp"
        camera = DahuaBridgeCamera(coordinator, "cam1")
        self.assertNotIn("include_credentials=true", camera.extra_state_attributes["bridge_archive_smd_ivs_url_template"])

    def test_all_profile_relay_urls_use_configured_host_and_bridge_auth(self) -> None:
        record = make_record()
        record["stream"]["profiles"] = {
            key: {"stream_url": f"rtsp://advertised.local:8554/api/v1/rtsp/live/cam1/{key}"}
            for key in ("quality", "stable")
        }
        coordinator = FakeCoordinator(record)
        coordinator.preferred_video_source = "rtsp"
        coordinator.api = DahuaBridgeAPI(object(), "http://internal.local:9020/prefix", "bridge-token")
        attrs = DahuaBridgeCamera(coordinator, "cam1").extra_state_attributes
        for key in ("quality", "stable"):
            self.assertEqual(attrs["bridge_profiles"][key]["stream_url"], f"rtsp://dahuabridge:bridge-token@internal.local:8554/api/v1/rtsp/live/cam1/{key}")
        self.assertEqual(attrs["stream_source"], attrs["bridge_profiles"]["stable"]["stream_url"])

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

    def test_bridge_api_removes_advertised_prefix_for_direct_connection(self) -> None:
        api = DahuaBridgeAPI(object(), "http://bridge.local:9020")
        self.assertEqual(
            api.bridge_resource_url(
                "https://public.example/dahua-bridge/api/v1/media/mjpeg/cam1?profile=stable"
            ),
            "http://bridge.local:9020/api/v1/media/mjpeg/cam1?profile=stable",
        )

    def test_bridge_api_replaces_advertised_prefix_with_configured_prefix(self) -> None:
        api = DahuaBridgeAPI(object(), "https://internal.example/bridge")
        self.assertEqual(
            api.bridge_resource_url(
                "https://public.example/dahua-bridge/api/v1/media/mjpeg/cam1?profile=stable"
            ),
            "https://internal.example/bridge/api/v1/media/mjpeg/cam1?profile=stable",
        )

    def test_bridge_api_rebases_root_relative_advertised_prefix(self) -> None:
        api = DahuaBridgeAPI(object(), "http://bridge.local:9020")
        self.assertEqual(
            api.bridge_resource_url(
                "/dahua-bridge/api/v1/media/mjpeg/cam1?profile=stable"
            ),
            "http://bridge.local:9020/api/v1/media/mjpeg/cam1?profile=stable",
        )

    def test_bridge_api_appends_query_token_to_browser_urls(self) -> None:
        api = DahuaBridgeAPI(object(), "https://public.example/bridge", "secret-token")

        self.assertEqual(
            api.bridge_resource_url(
                "http://127.0.0.1:19215/api/v1/media/mjpeg/cam1?profile=stable"
            ),
            "https://public.example/bridge/api/v1/media/mjpeg/cam1?profile=stable&auth_token=secret-token",
        )

    def test_bridge_api_does_not_duplicate_existing_query_token(self) -> None:
        api = DahuaBridgeAPI(object(), "https://public.example/bridge", "secret-token")

        self.assertEqual(
            api.absolute_url("/api/v1/media/hls/cam1/index.m3u8?auth_token=existing"),
            "https://public.example/bridge/api/v1/media/hls/cam1/index.m3u8?auth_token=existing",
        )

    def test_redact_url_for_log_hides_tokens(self) -> None:
        self.assertEqual(
            redact_url_for_log(
                "https://public.example/bridge/api/v1/media/mjpeg/cam1?profile=stable&auth_token=secret-token"
            ),
            "https://public.example/bridge/api/v1/media/mjpeg/cam1?profile=stable&auth_token=%2A%2AREDACTED%2A%2A",
        )

    async def test_bridge_api_sends_bearer_token_for_api_requests(self) -> None:
        session = HeaderCaptureSession()
        api = DahuaBridgeAPI(session, "https://public.example/bridge", "secret-token")

        await api.async_get_status()

        self.assertEqual(len(session.requests), 1)
        self.assertEqual(
            session.requests[0]["url"],
            "https://public.example/bridge/api/v1/status",
        )
        self.assertEqual(
            session.requests[0]["headers"],
            {"Authorization": "Bearer secret-token"},
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
            ["http://bridge.local:8080/api/v1/media/snapshot/cam1"],
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
            coordinator.api.bytes_requests,
            ["http://bridge.local:8080/api/v1/media/snapshot/cam1"],
        )
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
            "http://bridge.local:8080/api/v1/nvr/west20_nvr/recording-chunks?channel=5&start={start}&end={end}&limit={limit}",
        )
        self.assertEqual(
            attrs["bridge_archive_smd_ivs_url_template"],
            "http://bridge.local:8080/api/v1/nvr/west20_nvr/smd-ivs?channel=5&start={start}&end={end}&limit={limit}&event={event}",
        )
        self.assertEqual(
            attrs["bridge_archive_recording_chunks_url_template"],
            "http://bridge.local:8080/api/v1/nvr/west20_nvr/recording-chunks?channel=5&start={start}&end={end}&limit={limit}",
        )
        self.assertEqual(attrs["bridge_integration_language"], "en")
        self.assertTrue(attrs["video_fallbacks_enabled"])

    def test_camera_attributes_request_smd_ivs_credentials_for_direct_rtsp(self) -> None:
        coordinator = FakeCoordinator(make_record())
        coordinator.preferred_video_source = "rtsp"
        camera = DahuaBridgeCamera(coordinator, "cam1")

        attrs = camera.extra_state_attributes

        self.assertEqual(
            attrs["bridge_archive_smd_ivs_url_template"],
            "http://bridge.local:8080/api/v1/nvr/west20_nvr/smd-ivs?channel=5&start={start}&end={end}&limit={limit}&event={event}&include_credentials=true",
        )

    def test_camera_attributes_disable_source_fallbacks(self) -> None:
        coordinator = FakeCoordinator(make_record())
        coordinator.preferred_video_source = "hls"
        coordinator.video_fallbacks_enabled = False
        camera = DahuaBridgeCamera(coordinator, "cam1")

        attrs = camera.extra_state_attributes

        self.assertFalse(attrs["video_fallbacks_enabled"])
        self.assertEqual(
            attrs["stream_source"],
            "http://bridge.local:8080/api/v1/media/hls/cam1/stable/index.m3u8",
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

    async def test_native_playback_source_overrides_camera_stream_source(self) -> None:
        camera = DahuaBridgeCamera(FakeCoordinator(make_record()), "cam1")
        playback_url = (
            "rtsp://user:pass@192.0.2.10:554/cam/playback"
            "?channel=5&subtype=1&starttime=2026_05_01_10_15_30"
        )

        await camera.async_set_native_playback_source(playback_url)

        self.assertEqual(await camera.stream_source(), playback_url)

        await camera.async_clear_native_playback_source()

        self.assertEqual(
            await camera.stream_source(),
            "http://bridge.local:8080/api/v1/media/hls/cam1/stable/index.m3u8",
        )

    async def test_native_playback_source_resets_cached_ha_stream(self) -> None:
        camera = DahuaBridgeCamera(FakeCoordinator(make_record()), "cam1")
        playback_url = (
            "rtsp://user:pass@192.0.2.10:554/cam/playback"
            "?channel=5&subtype=1&starttime=2026_05_01_10_15_30"
        )
        live_stream = FakeCameraStream()
        camera.stream = live_stream

        await camera.async_set_native_playback_source(playback_url)

        self.assertIsNone(camera.stream)
        self.assertEqual(live_stream.stop_count, 1)

        playback_stream = FakeCameraStream()
        camera.stream = playback_stream

        await camera.async_clear_native_playback_source()

        self.assertIsNone(camera.stream)
        self.assertEqual(playback_stream.stop_count, 1)

    async def test_native_playback_source_rejects_non_rtsp_urls(self) -> None:
        camera = DahuaBridgeCamera(FakeCoordinator(make_record()), "cam1")

        with self.assertRaises(HomeAssistantError):
            await camera.async_set_native_playback_source(
                "http://bridge.local:8080/api/v1/media/hls/cam1/stable/index.m3u8"
            )


class FakeCameraStream:
    def __init__(self) -> None:
        self.stop_count = 0

    async def stop(self) -> None:
        self.stop_count += 1


if __name__ == "__main__":
    unittest.main()
