from __future__ import annotations

import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from urllib.parse import unquote, urlsplit

from ha_stubs import install

install()
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from custom_components.dahuabridge.api import DahuaBridgeAPI
from custom_components.dahuabridge.api.client import redact_url_for_log


class LiveRelayURLTests(unittest.TestCase):
    def test_relay_uses_configured_internal_host_and_bridge_token(self):
        api = DahuaBridgeAPI(SimpleNamespace(), "http://dahua-bridge:9020/proxy", "secret:@/token")
        target = api.bridge_resource_url("rtsp://advertised.example:8554/api/v1/rtsp/live/channel_5/stable")
        parsed = urlsplit(target)
        self.assertEqual(parsed.hostname, "dahua-bridge")
        self.assertEqual(parsed.port, 8554)
        self.assertEqual(parsed.path, "/api/v1/rtsp/live/channel_5/stable")
        self.assertEqual(parsed.username, "dahuabridge")
        self.assertEqual(unquote(parsed.password), "secret:@/token")
        self.assertEqual(parsed.query, "")

    def test_relay_rebase_handles_ipv6_and_removes_stale_advertised_token(self):
        api = DahuaBridgeAPI(SimpleNamespace(), "http://[2001:db8::10]:9020")
        target = api.bridge_resource_url("rtsp://old:token@advertised.example:8555/api/v1/rtsp/live/channel_5/quality")
        self.assertEqual(target, "rtsp://[2001:db8::10]:8555/api/v1/rtsp/live/channel_5/quality")

    def test_existing_nvr_archive_url_is_preserved(self):
        api = DahuaBridgeAPI(SimpleNamespace(), "http://dahua-bridge:9020", "bridge-token")
        target = "rtsp://nvr:password@recorder.local:554/cam/playback?channel=5"
        self.assertEqual(api.bridge_resource_url(target), target)

    def test_relay_and_legacy_rtsp_log_urls_hide_credentials(self):
        for target in (
            "rtsp://dahuabridge:secret@bridge:8554/api/v1/rtsp/live/cam/stable",
            "rtsp://admin:secret@camera:554/cam/playback?channel=5&auth_token=another-secret",
        ):
            redacted = redact_url_for_log(target)
            self.assertNotIn("secret", redacted)
            self.assertIn("REDACTED", redacted)


if __name__ == "__main__":
    unittest.main()
