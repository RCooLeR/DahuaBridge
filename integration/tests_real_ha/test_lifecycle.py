from __future__ import annotations

from copy import deepcopy
from unittest.mock import AsyncMock, patch

import pytest
from homeassistant import config_entries
from homeassistant.const import STATE_UNAVAILABLE
from homeassistant.helpers import device_registry as dr, entity_registry as er
from pytest_homeassistant_custom_component.common import MockConfigEntry

from custom_components.dahuabridge.api import DahuaBridgeAPIError
from custom_components.dahuabridge.const import DOMAIN

CATALOG = {
    "devices": [
        {
            "device": {"id": "recorder", "kind": "nvr", "name": "Recorder"},
            "state": {"available": True, "info": {"channel_count": 2}},
        },
        {
            "device": {"id": "cam1", "kind": "nvr_channel", "name": "Camera", "parent_id": "recorder"},
            "state": {"available": True, "info": {"motion": False, "main_codec": "H264"}},
            "stream": {
                "channel": 1,
                "recommended_profile": "stable",
                "profiles": {"stable": {"stream_url": "rtsp://advertised:8554/api/v1/rtsp/live/cam1/stable"}},
                "live_source": {"source": "nvr", "url": "/api/v1/streams/cam1/live-source"},
                "features": [{"key": "light", "label": "Light", "supported": True, "parameter_key": "output", "parameter_value": "light", "actions": ["start", "stop"], "active": False, "url": "/api/v1/nvr/recorder/channels/1/aux"}],
            },
        },
    ]
}


@pytest.fixture
def api():
    with (
        patch("custom_components.dahuabridge.api.DahuaBridgeAPI.async_get_catalog", new_callable=AsyncMock) as catalog,
        patch("custom_components.dahuabridge.api.DahuaBridgeAPI.async_get_live_source", new_callable=AsyncMock) as get_source,
        patch("custom_components.dahuabridge.api.DahuaBridgeAPI.async_set_live_source", new_callable=AsyncMock) as set_source,
        patch("custom_components.dahuabridge.api.DahuaBridgeAPI.async_get_live_preconnect", new_callable=AsyncMock) as get_preconnect,
        patch("custom_components.dahuabridge.api.DahuaBridgeAPI.async_set_live_preconnect", new_callable=AsyncMock) as set_preconnect,
    ):
        catalog.return_value = deepcopy(CATALOG)
        get_source.return_value = {"source": "nvr"}
        set_source.return_value = {"source": "camera"}
        get_preconnect.return_value = {"mode": "off", "profile": "stable"}
        set_preconnect.return_value = {"mode": "always", "profile": "quality"}
        yield catalog, get_source, set_source, get_preconnect, set_preconnect


def new_entry(hass):
    entry = MockConfigEntry(
        domain=DOMAIN,
        unique_id="http://bridge.internal:9020",
        data={"bridge_url": "http://bridge.internal:9020", "api_token": "test-token"},
        options={"scan_interval": 300, "preferred_video_profile": "stable", "preferred_video_source": "rtsp", "enable_video_fallbacks": False},
    )
    entry.add_to_hass(hass)
    return entry


async def test_real_setup_registers_all_platforms_parent_and_services_then_unloads(hass, api):
    entry = new_entry(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    assert entry.state is config_entries.ConfigEntryState.LOADED
    registry = er.async_get(hass)
    entities = er.async_entries_for_config_entry(registry, entry.entry_id)
    assert {entity.domain for entity in entities} == {"camera", "sensor", "binary_sensor", "button", "switch"}
    devices = dr.async_entries_for_config_entry(dr.async_get(hass), entry.entry_id)
    parent = next(device for device in devices if (DOMAIN, "recorder") in device.identifiers)
    camera = next(device for device in devices if (DOMAIN, "cam1") in device.identifiers)
    assert camera.via_device_id == parent.id
    assert hass.services.has_service(DOMAIN, "set_live_source")
    assert hass.services.has_service(DOMAIN, "start_recording")
    assert api[2].await_count == 0  # Startup must not reset persisted overrides.
    api[4].assert_not_awaited()
    assert await hass.config_entries.async_unload(entry.entry_id)
    await hass.async_block_till_done()
    assert entry.state is config_entries.ConfigEntryState.NOT_LOADED
    # HA can retain registry placeholders after unloading an integration.
    assert all((state := hass.states.get(entity.entity_id)) is None or state.state == STATE_UNAVAILABLE for entity in entities)


async def test_real_options_schema_and_save_preserve_player_settings(hass, api):
    entry = new_entry(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    result = await hass.config_entries.options.async_init(entry.entry_id)
    assert result["type"] == "form"
    submitted = result["data_schema"]({"live_source": "camera"})
    assert submitted["scan_interval"] == 300
    assert submitted["preferred_video_profile"] == "stable"
    assert submitted["preferred_video_source"] == "rtsp"
    result = await hass.config_entries.options.async_configure(result["flow_id"], submitted)
    assert result["type"] == "create_entry"
    await hass.async_block_till_done()
    api[2].assert_awaited_once_with("camera")
    assert entry.options["scan_interval"] == 300
    assert entry.options["preferred_video_source"] == "rtsp"
    assert await hass.config_entries.async_unload(entry.entry_id)


async def test_real_options_failed_save_keeps_connection_and_selection(hass, api):
    entry = new_entry(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    before = (dict(entry.data), dict(entry.options))
    api[2].side_effect = DahuaBridgeAPIError("preference write failed")
    result = await hass.config_entries.options.async_init(entry.entry_id)
    result = await hass.config_entries.options.async_configure(result["flow_id"], {
        "bridge_url": "http://replacement.internal:9020", "api_token": "replacement-token", "live_source": "camera",
    })
    assert result["type"] == "form"
    assert result["errors"]["base"] == "cannot_set_live_source"
    assert result["data_schema"]({})["live_source"] == "camera"
    assert (dict(entry.data), dict(entry.options)) == before
    assert await hass.config_entries.async_unload(entry.entry_id)


async def test_real_initial_flow_rejects_failed_protected_catalog(hass, api):
    api[0].side_effect = DahuaBridgeAPIError("unauthorized")
    result = await hass.config_entries.flow.async_init(DOMAIN, context={"source": config_entries.SOURCE_USER}, data={"bridge_url": "http://bridge.internal:9020", "api_token": "wrong"})
    assert result["type"] == "form"
    assert result["errors"]["base"] == "cannot_connect"
    assert not hass.config_entries.async_entries(DOMAIN)
    api[0].assert_awaited_once()


async def test_real_old_bridge_schema_can_save_other_options(hass, api):
    entry = new_entry(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    api[1].side_effect = DahuaBridgeAPIError("live-source endpoint unavailable")
    result = await hass.config_entries.options.async_init(entry.entry_id)
    submitted = result["data_schema"]({"scan_interval": 120})
    assert "live_source" not in submitted
    result = await hass.config_entries.options.async_configure(result["flow_id"], submitted)
    assert result["type"] == "create_entry"
    await hass.async_block_till_done()
    assert entry.options["scan_interval"] == 120
    api[2].assert_not_awaited()
    assert await hass.config_entries.async_unload(entry.entry_id)


async def test_real_preconnect_options_save_mode_and_selected_profile(hass, api):
    entry = new_entry(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    result = await hass.config_entries.options.async_init(entry.entry_id)
    assert result["data_schema"]({})["live_preconnect"] == "off"
    submitted = result["data_schema"]({"live_preconnect": "always", "preferred_video_profile": "quality"})
    result = await hass.config_entries.options.async_configure(result["flow_id"], submitted)
    assert result["type"] == "create_entry"
    await hass.async_block_till_done()
    api[4].assert_awaited_once_with("always", "quality")
    assert entry.options["live_preconnect"] == "always"
    assert entry.options["preferred_video_profile"] == "quality"
    assert entry.options["scan_interval"] == 300
    assert entry.options["preferred_video_source"] == "rtsp"
    assert await hass.config_entries.async_unload(entry.entry_id)


async def test_real_preconnect_failed_save_preserves_options_and_selection(hass, api):
    entry = new_entry(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    before = (dict(entry.data), dict(entry.options))
    api[4].side_effect = DahuaBridgeAPIError("preconnect write failed")
    result = await hass.config_entries.options.async_init(entry.entry_id)
    result = await hass.config_entries.options.async_configure(result["flow_id"], {
        "bridge_url": "http://replacement.internal:9020", "api_token": "replacement-token",
        "live_preconnect": "recent", "preferred_video_profile": "quality",
    })
    assert result["type"] == "form"
    assert result["errors"]["base"] == "cannot_set_live_preconnect"
    defaults = result["data_schema"]({})
    assert defaults["live_preconnect"] == "recent"
    assert defaults["preferred_video_profile"] == "quality"
    assert (dict(entry.data), dict(entry.options)) == before
    assert await hass.config_entries.async_unload(entry.entry_id)


async def test_real_unsupported_preconnect_is_absent_from_submitted_defaults(hass, api):
    entry = new_entry(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    api[3].side_effect = DahuaBridgeAPIError("not found", status=404)
    result = await hass.config_entries.options.async_init(entry.entry_id)
    submitted = result["data_schema"]({"scan_interval": 120})
    assert "live_preconnect" not in submitted
    result = await hass.config_entries.options.async_configure(result["flow_id"], submitted)
    assert result["type"] == "create_entry"
    await hass.async_block_till_done()
    api[4].assert_not_awaited()
    assert entry.options["scan_interval"] == 120
    assert await hass.config_entries.async_unload(entry.entry_id)
