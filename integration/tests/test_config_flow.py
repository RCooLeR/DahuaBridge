from __future__ import annotations

import copy
import json
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock, patch

from ha_stubs import install


install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from homeassistant.config_entries import ConfigEntry

from custom_components import dahuabridge as integration
from custom_components.dahuabridge import config_flow, config_options
from custom_components.dahuabridge.api import DahuaBridgeAPI, DahuaBridgeAPIError
from custom_components.dahuabridge.const import (
    CONF_API_TOKEN,
    CONF_BRIDGE_URL,
    CONF_ENABLE_VIDEO_FALLBACKS,
    CONF_LANGUAGE,
    CONF_LIVE_PRECONNECT,
    CONF_LIVE_SOURCE,
    CONF_PREFERRED_VIDEO_PROFILE,
    CONF_PREFERRED_VIDEO_SOURCE,
    CONF_SCAN_INTERVAL,
)
from custom_components.dahuabridge.coordinator import DahuaBridgeCoordinator


class OptionsFlowTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self) -> None:
        self.entry = ConfigEntry(
            data={CONF_BRIDGE_URL: "http://bridge.local:8080", CONF_API_TOKEN: "old-token"},
            options={
                CONF_SCAN_INTERVAL: 300,
                CONF_PREFERRED_VIDEO_PROFILE: "stable",
                CONF_PREFERRED_VIDEO_SOURCE: "rtsp",
                CONF_ENABLE_VIDEO_FALLBACKS: False,
                CONF_LANGUAGE: "uk",
            },
        )
        self.events = []
        self.states = {"http://bridge.local:8080": {"source": "nvr"}}
        self.updates = []
        self.defaults = {}

        def update_entry(entry, **kwargs):
            self.updates.append(kwargs)
            for key, value in kwargs.items():
                setattr(entry, key, value)

        self.hass = SimpleNamespace(
            config=SimpleNamespace(language="uk"),
            config_entries=SimpleNamespace(async_update_entry=update_entry),
        )
        self.flow = config_flow.DahuaBridgeOptionsFlow(self.entry)
        self.flow.hass = self.hass
        self.api_patch = patch.object(config_flow, "DahuaBridgeAPI", self._make_api)
        self.api_patch.start()
        self.addCleanup(self.api_patch.stop)

        def optional(key, *args, **kwargs):
            self.defaults[key] = kwargs.get("default")
            return key

        self.optional_patch = patch.object(config_options.vol, "Optional", optional)
        self.optional_patch.start()
        self.addCleanup(self.optional_patch.stop)

    def _make_api(self, session, url, token=""):
        state = self.states[url]

        async def get_status():
            self.events.append(("status", url, token))
            if state.get("status_error"):
                raise DahuaBridgeAPIError("bridge unavailable")
            return {"status": "ok"}

        async def get_live_source():
            self.events.append(("get", url, token))
            if state.get("get_error"):
                raise DahuaBridgeAPIError("live-source endpoint unavailable")
            return {"source": state["source"]}

        async def set_live_source(source):
            self.events.append(("set", url, token, source))
            if state.get("set_error"):
                raise DahuaBridgeAPIError("live-source update failed")
            state["source"] = source
            return {"source": source}

        async def get_live_preconnect():
            self.events.append(("get_preconnect", url, token))
            if state.get("preconnect_get_error"):
                raise DahuaBridgeAPIError("preconnect endpoint unavailable", status=404)
            return {"mode": state.get("mode", "off"), "profile": state.get("profile", "stable")}

        async def set_live_preconnect(mode, profile):
            self.events.append(("set_preconnect", url, token, mode, profile))
            if state.get("preconnect_set_error"):
                raise DahuaBridgeAPIError("preconnect update failed")
            state.update(mode=mode, profile=profile)
            return {"mode": mode, "profile": profile}

        return SimpleNamespace(
            async_get_status=get_status,
            async_get_catalog=get_status,
            async_get_live_source=get_live_source,
            async_set_live_source=set_live_source,
            async_get_live_preconnect=get_live_preconnect,
            async_set_live_preconnect=set_live_preconnect,
        )

    def _input(self, **overrides):
        return {**self.entry.options, **overrides}

    def _sets(self):
        return [event for event in self.events if event[0] == "set"]

    async def test_new_user_form_does_not_offer_global_preference(self):
        self.assertNotIn(CONF_LIVE_SOURCE, config_options.build_user_schema(CONF_BRIDGE_URL))
        self.assertNotIn(CONF_LIVE_PRECONNECT, config_options.build_user_schema(CONF_BRIDGE_URL))

    async def test_preconnect_form_reads_bridge_mode_without_changing_profile(self):
        self.entry.options[CONF_LIVE_PRECONNECT] = "always"
        self.states["http://bridge.local:8080"].update(mode="recent", profile="quality")
        result = await self.flow.async_step_init()
        self.assertEqual(self.defaults[CONF_LIVE_PRECONNECT], "recent")
        self.assertEqual(self.defaults[CONF_PREFERRED_VIDEO_PROFILE], "stable")
        self.assertEqual(set(result["data_schema"][CONF_LIVE_PRECONNECT]), {"off", "recent", "always"})
        self.assertFalse(any(event[0] == "set_preconnect" for event in self.events))

    async def test_preconnect_save_sends_mode_and_selected_profile(self):
        result = await self.flow.async_step_init(self._input(live_preconnect="always", preferred_video_profile="quality"))
        self.assertEqual(result["type"], "create_entry")
        self.assertEqual(result["data"][CONF_LIVE_PRECONNECT], "always")
        self.assertEqual(result["data"][CONF_PREFERRED_VIDEO_PROFILE], "quality")
        self.assertIn(("set_preconnect", "http://bridge.local:8080", "old-token", "always", "quality"), self.events)

    async def test_preconnect_save_skips_matching_mode_and_profile(self):
        result = await self.flow.async_step_init(self._input(live_preconnect="off"))
        self.assertEqual(result["type"], "create_entry")
        self.assertFalse(any(event[0] == "set_preconnect" for event in self.events))

    async def test_preconnect_profile_change_updates_unchanged_mode(self):
        self.states["http://bridge.local:8080"].update(mode="always", profile="stable")
        result = await self.flow.async_step_init(self._input(live_preconnect="always", preferred_video_profile="quality"))
        self.assertEqual(result["type"], "create_entry")
        self.assertIn(("set_preconnect", "http://bridge.local:8080", "old-token", "always", "quality"), self.events)

    async def test_preconnect_failure_preserves_entry_and_submitted_mode_and_profile(self):
        self.states["http://replacement.local:8080"] = {"source": "nvr", "preconnect_set_error": True}
        before = copy.deepcopy((self.entry.data, self.entry.options))
        result = await self.flow.async_step_init(self._input(
            bridge_url="http://replacement.local:8080", api_token="new-token",
            live_preconnect="recent", preferred_video_profile="quality",
        ))
        self.assertEqual(result["errors"]["base"], "cannot_set_live_preconnect")
        self.assertEqual(self.defaults[CONF_LIVE_PRECONNECT], "recent")
        self.assertEqual(self.defaults[CONF_PREFERRED_VIDEO_PROFILE], "quality")
        self.assertEqual((self.entry.data, self.entry.options), before)
        self.assertEqual(self.updates, [])
        self.assertIn(("set_preconnect", "http://replacement.local:8080", "new-token", "recent", "quality"), self.events)

    async def test_unsupported_preconnect_hidden_and_does_not_block_other_options(self):
        self.states["http://bridge.local:8080"]["preconnect_get_error"] = True
        form = await self.flow.async_step_init()
        self.assertNotIn(CONF_LIVE_PRECONNECT, form["data_schema"])
        self.assertIn(CONF_LIVE_SOURCE, form["data_schema"])
        result = await self.flow.async_step_init(self._input(scan_interval=120))
        self.assertEqual(result["type"], "create_entry")
        self.assertEqual(result["data"][CONF_SCAN_INTERVAL], 120)
        self.assertNotIn(CONF_LIVE_PRECONNECT, result["data"])

    async def test_submitted_preconnect_get_failure_prevents_commit(self):
        self.states["http://bridge.local:8080"]["preconnect_get_error"] = True
        before = copy.deepcopy((self.entry.data, self.entry.options))
        result = await self.flow.async_step_init(self._input(live_preconnect="always"))
        self.assertEqual(result["errors"]["base"], "cannot_set_live_preconnect")
        self.assertEqual((self.entry.data, self.entry.options), before)
        self.assertFalse(any(event[0] == "set_preconnect" for event in self.events))

    async def test_form_uses_backend_default_and_preserves_existing_player_settings(self):
        self.entry.options[CONF_LIVE_SOURCE] = "nvr"
        self.states["http://bridge.local:8080"]["source"] = "camera"

        result = await self.flow.async_step_init()

        self.assertEqual(result["type"], "form")
        self.assertEqual(self.defaults[CONF_LIVE_SOURCE], "camera")
        self.assertEqual(self.defaults[CONF_SCAN_INTERVAL], 300)
        self.assertEqual(self.defaults[CONF_PREFERRED_VIDEO_PROFILE], "stable")
        self.assertEqual(self.defaults[CONF_PREFERRED_VIDEO_SOURCE], "rtsp")
        fields = list(result["data_schema"])
        self.assertLess(fields.index(CONF_PREFERRED_VIDEO_PROFILE), fields.index(CONF_LIVE_SOURCE))
        self.assertLess(fields.index(CONF_LIVE_SOURCE), fields.index(CONF_PREFERRED_VIDEO_SOURCE))
        self.assertEqual(set(result["data_schema"][CONF_LIVE_SOURCE]), {"nvr", "camera"})
        self.assertEqual(self._sets(), [])

    async def test_camera_default_save_preserves_player_options(self):
        result = await self.flow.async_step_init(self._input(live_source="camera"))

        self.assertEqual(result["type"], "create_entry")
        self.assertEqual(result["data"][CONF_SCAN_INTERVAL], 300)
        self.assertEqual(result["data"][CONF_PREFERRED_VIDEO_PROFILE], "stable")
        self.assertEqual(result["data"][CONF_PREFERRED_VIDEO_SOURCE], "rtsp")
        self.assertEqual(result["data"][CONF_LANGUAGE], "uk")
        self.assertFalse(result["data"][CONF_ENABLE_VIDEO_FALLBACKS])
        self.assertEqual(self._sets(), [("set", "http://bridge.local:8080", "old-token", "camera")])

    async def test_save_rechecks_backend_and_skips_unchanged_default(self):
        await self.flow.async_step_init()
        self.states["http://bridge.local:8080"]["source"] = "camera"

        result = await self.flow.async_step_init(self._input(live_source="camera"))

        self.assertEqual(result["type"], "create_entry")
        self.assertEqual(len([event for event in self.events if event[0] == "get"]), 2)
        self.assertEqual(self._sets(), [])

    async def test_changed_bridge_checks_requested_default_before_updating_entry(self):
        self.states["http://replacement.local:8080"] = {"source": "camera"}

        result = await self.flow.async_step_init(self._input(
            bridge_url="http://replacement.local:8080/",
            api_token="new-token",
            live_source="camera",
        ))

        self.assertEqual(result["type"], "create_entry")
        self.assertIn(("get", "http://replacement.local:8080", "new-token"), self.events)
        self.assertEqual(self._sets(), [])
        self.assertEqual(self.entry.data[CONF_BRIDGE_URL], "http://replacement.local:8080")
        self.assertEqual(self.entry.data[CONF_API_TOKEN], "new-token")

    async def test_put_failure_keeps_connection_and_options_unchanged(self):
        self.states["http://replacement.local:8080"] = {"source": "nvr", "set_error": True}
        before = copy.deepcopy((self.entry.data, self.entry.options, self.entry.title))

        result = await self.flow.async_step_init(self._input(
            bridge_url="http://replacement.local:8080",
            api_token="new-token",
            live_source="camera",
            scan_interval=60,
            preferred_video_profile="quality",
        ))

        self.assertEqual(result["type"], "form")
        self.assertEqual(result["errors"]["base"], "cannot_set_live_source")
        self.assertIn(CONF_LIVE_SOURCE, result["data_schema"])
        self.assertEqual(self.defaults[CONF_LIVE_SOURCE], "camera")
        self.assertEqual((self.entry.data, self.entry.options, self.entry.title), before)
        self.assertEqual(self.updates, [])

    async def test_get_failure_with_submitted_source_does_not_commit_changes(self):
        self.states["http://replacement.local:8080"] = {"source": "nvr", "get_error": True}
        before = copy.deepcopy((self.entry.data, self.entry.options))

        result = await self.flow.async_step_init(self._input(
            bridge_url="http://replacement.local:8080",
            api_token="new-token",
            live_source="camera",
        ))

        self.assertEqual(result["type"], "form")
        self.assertIn(result["errors"]["base"], {"cannot_connect", "cannot_set_live_source"})
        self.assertEqual((self.entry.data, self.entry.options), before)
        self.assertEqual(self.updates, [])
        self.assertEqual(self._sets(), [])

    async def test_old_bridge_allows_other_options_when_live_source_is_omitted(self):
        self.states["http://bridge.local:8080"]["get_error"] = True
        form = await self.flow.async_step_init()
        self.assertNotIn(CONF_LIVE_SOURCE, form["data_schema"])

        # Home Assistant applies Voluptuous defaults when submitting a form.
        # An unsupported source field must not be silently reintroduced here.
        submitted = {
            key: value
            for key, value in self.defaults.items()
            if key in form["data_schema"] and value is not None
        }
        submitted.update(self._input(scan_interval=60))

        result = await self.flow.async_step_init(submitted)

        self.assertEqual(result["type"], "create_entry")
        self.assertEqual(result["data"][CONF_SCAN_INTERVAL], 60)
        self.assertEqual(result["data"][CONF_PREFERRED_VIDEO_PROFILE], "stable")
        self.assertEqual(self._sets(), [])


class LiveSourceLifecycleTests(unittest.IsolatedAsyncioTestCase):
    async def test_catalog_refresh_does_not_reapply_global_default(self):
        api = SimpleNamespace(
            async_get_catalog=AsyncMock(return_value={"streams": []}),
            async_set_live_source=AsyncMock(),
        )
        entry = ConfigEntry(options={CONF_LIVE_SOURCE: "camera", CONF_PREFERRED_VIDEO_SOURCE: "rtsp"})
        coordinator = DahuaBridgeCoordinator(SimpleNamespace(), api, entry, 300)

        await coordinator._async_update_data()
        await coordinator._async_update_data()

        self.assertEqual(api.async_get_catalog.await_count, 2)
        api.async_set_live_source.assert_not_awaited()

    async def test_entry_setup_does_not_reapply_global_default(self):
        api = SimpleNamespace(async_set_live_source=AsyncMock())
        coordinator = SimpleNamespace(async_config_entry_first_refresh=AsyncMock())
        entry = ConfigEntry(
            data={CONF_BRIDGE_URL: "http://bridge.local:8080"},
            options={CONF_LIVE_SOURCE: "camera"},
        )
        entry.async_on_unload = Mock()
        entry.add_update_listener = Mock(return_value=lambda: None)
        hass = SimpleNamespace(config_entries=SimpleNamespace(async_forward_entry_setups=AsyncMock()))

        with patch.object(integration, "DahuaBridgeAPI", return_value=api), patch.object(
            integration, "DahuaBridgeCoordinator", return_value=coordinator
        ):
            self.assertTrue(await integration.async_setup_entry(hass, entry))

        api.async_set_live_source.assert_not_awaited()
        coordinator.async_config_entry_first_refresh.assert_awaited_once()


class LiveSourceAPITests(unittest.IsolatedAsyncioTestCase):
    async def test_preconnect_api_uses_protected_settings_path_and_validates_confirmation(self):
        api = DahuaBridgeAPI(SimpleNamespace(), "https://bridge.local/proxy/", "test-token")
        with patch.object(api, "_async_request_json", new_callable=AsyncMock) as request:
            request.return_value = {"mode": "always", "profile": "stable"}
            self.assertEqual(await api.async_get_live_preconnect(), request.return_value)
            request.assert_awaited_once_with("GET", "/api/v1/settings/live-preconnect")
            request.reset_mock()
            self.assertEqual(await api.async_set_live_preconnect("always", "stable"), request.return_value)
            request.assert_awaited_once_with("PUT", "/api/v1/settings/live-preconnect", {"mode": "always", "profile": "stable"})
            request.reset_mock()
            with self.assertRaises(DahuaBridgeAPIError):
                await api.async_set_live_preconnect("invalid", "stable")
            request.assert_not_awaited()
            request.return_value = {"mode": "off", "profile": "stable"}
            with self.assertRaises(DahuaBridgeAPIError):
                await api.async_set_live_preconnect("always", "stable")
            request.return_value = {"mode": "always"}
            with self.assertRaises(DahuaBridgeAPIError):
                await api.async_get_live_preconnect()

    async def test_global_source_get_and_put_use_settings_endpoint(self):
        requests = []

        class Response:
            status = 200

            async def __aenter__(self):
                return self

            async def __aexit__(self, *args):
                return None

            async def text(self):
                return json.dumps({"source": "camera"})

        def request(method, url, **kwargs):
            requests.append((method, url, kwargs))
            return Response()

        api = DahuaBridgeAPI(
            SimpleNamespace(request=request), "https://bridge.local/proxy/", "test-token"
        )

        self.assertEqual(await api.async_get_live_source(), {"source": "camera"})
        self.assertEqual(await api.async_set_live_source("camera"), {"source": "camera"})

        self.assertEqual(requests, [
            (
                "GET",
                "https://bridge.local/proxy/api/v1/settings/live-source",
                {"json": None, "headers": {"Authorization": "Bearer test-token"}},
            ),
            (
                "PUT",
                "https://bridge.local/proxy/api/v1/settings/live-source",
                {"json": {"source": "camera"}, "headers": {"Authorization": "Bearer test-token"}},
            ),
        ])


if __name__ == "__main__":
    unittest.main()
