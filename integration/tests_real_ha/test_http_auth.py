from __future__ import annotations

import aiohttp
from aiohttp import web
import pytest
from homeassistant import config_entries

from custom_components.dahuabridge.api import DahuaBridgeAPI, DahuaBridgeAPIError
from custom_components.dahuabridge.camera_support.attributes import _nvr_archive_attrs
from custom_components.dahuabridge.const import DOMAIN

TOKEN = "synthetic:@/token&value"
pytestmark = pytest.mark.usefixtures("socket_enabled")


@pytest.fixture
async def bridge(aiohttp_server):
    seen = []
    preconnect = {"mode": "off", "profile": "auto"}

    async def handler(request):
        seen.append((request.method, request.path, dict(request.query)))
        if request.path == "/api/v1/status":
            return web.json_response({"ready": True})
        if request.headers.get("Authorization") != f"Bearer {TOKEN}" and request.query.get("auth_token") != TOKEN:
            return web.json_response({"error": "unauthorized"}, status=401)
        if request.path.endswith("/catalog"):
            return web.json_response({"devices": []})
        if request.path.endswith("/settings/live-preconnect"):
            if request.method == "PUT":
                preconnect.update(await request.json())
            return web.json_response(preconnect)
        return web.json_response({"ok": True, "channel": request.query.get("channel")})

    app = web.Application()
    app.router.add_route("*", "/{tail:.*}", handler)
    server = await aiohttp_server(app)
    return str(server.make_url("/")).rstrip("/"), seen


async def test_authenticated_archive_urls_preserve_token_and_channel(bridge):
    base, seen = bridge
    async with aiohttp.ClientSession() as session:
        api = DahuaBridgeAPI(session, base, TOKEN)
        attrs = _nvr_archive_attrs(api, "recorder", 5, False)
        for key in ("bridge_archive_recording_chunks_url_template", "bridge_archive_smd_ivs_url_template", "bridge_archive_coverage_url"):
            async with session.get(attrs[key]) as response:
                assert response.status == 200
                assert (await response.json())["channel"] == "5"
    assert all(query["auth_token"] == TOKEN for _, _, query in seen)


async def test_advertised_actions_use_internal_bridge_and_auth_header(bridge):
    base, seen = bridge
    async with aiohttp.ClientSession() as session:
        api = DahuaBridgeAPI(session, base, TOKEN)
        result = await api.async_post_action("https://public.example/prefix/api/v1/vto/door/locks/1/unlock")
        assert result["ok"]
        result = await api.async_post_json("https://public.example/prefix/api/v1/nvr/recorder/channels/1/aux", {"output": "light", "action": "start"})
        assert result["ok"]
        with pytest.raises(DahuaBridgeAPIError):
            await api.async_post_action("https://unrelated.example/not-a-bridge-action")
    assert [path for _, path, _ in seen] == ["/api/v1/vto/door/locks/1/unlock", "/api/v1/nvr/recorder/channels/1/aux"]


async def test_initial_flow_validates_protected_catalog_with_real_http(hass, bridge):
    base, seen = bridge
    result = await hass.config_entries.flow.async_init(DOMAIN, context={"source": config_entries.SOURCE_USER}, data={"bridge_url": base, "api_token": "wrong"})
    assert result["type"] == "form"
    assert result["errors"]["base"] == "invalid_auth"
    assert not hass.config_entries.async_entries(DOMAIN)
    result = await hass.config_entries.flow.async_configure(result["flow_id"], {"bridge_url": base, "api_token": TOKEN})
    assert result["type"] == "create_entry"
    await hass.async_block_till_done()
    entry = result["result"]
    assert await hass.config_entries.async_unload(entry.entry_id)
    assert all(path.endswith("/catalog") for _, path, _ in seen)


async def test_preconnect_settings_use_authenticated_get_and_put(bridge):
    base, seen = bridge
    async with aiohttp.ClientSession() as session:
        api = DahuaBridgeAPI(session, base, TOKEN)
        assert await api.async_get_live_preconnect() == {"mode": "off", "profile": "auto"}
        assert await api.async_set_live_preconnect("recent", "stable") == {"mode": "recent", "profile": "stable"}
        assert await api.async_get_live_preconnect() == {"mode": "recent", "profile": "stable"}
        with pytest.raises(DahuaBridgeAPIError) as unauthorized:
            await DahuaBridgeAPI(session, base, "wrong").async_get_live_preconnect()
        assert unauthorized.value.status == 401
    assert all(path == "/api/v1/settings/live-preconnect" for _, path, _ in seen)
