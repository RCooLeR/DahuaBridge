# Configuration

All configuration is done through the Home Assistant UI.

## Bridge URL

The bridge URL must be reachable by Home Assistant. It must include `http://` or `https://`.

Examples:

```text
http://bridge-host:9205
https://ha.example.com/dahuabridge
```

Use the reverse-proxy URL if Home Assistant reaches the bridge through a proxy path. The integration uses this configured URL when it rewrites bridge-hosted links from the catalog.

For example, if nginx exposes the bridge with:

```nginx
location /dahua-bridge/ {
  rewrite ^/dahua-bridge/(.*) /$1 break;
  proxy_pass http://bridge:9205;
}
```

configure the integration bridge URL as:

```text
https://ha.example.com/dahua-bridge
```

If `/dahua-bridge/api/...` requests return `404`, check that the proxy strips
the `/dahua-bridge` prefix before forwarding to the bridge process. The bridge
routes themselves are mounted at `/api/...`.

## Bridge API Token

If bridge-level HTTP auth is enabled, enter the same API token in the integration
config flow or options flow. The integration sends it as a bearer token for API
polling and appends a redacted query-token form to browser-facing media/snapshot
URLs so Home Assistant camera entities can still load images and streams.

## Options

The options flow exposes:

- bridge URL
- bridge API token
- poll interval
- preferred video profile
- preferred video source
- integration language

Defaults:

| Option | Default | Allowed values |
| --- | --- | --- |
| Bridge URL | Current configured URL | Any reachable `http://` or `https://` bridge base URL |
| Bridge API token | Empty | Token configured on the bridge, if any |
| Poll interval | `15` seconds | `5` to `300` seconds |
| Preferred video profile | `quality` | `auto`, `quality`, `stable` |
| Preferred video source | `rtsp` | `auto`, `hls`, `mjpeg`, `rtsp` |
| Integration language | `auto` | `auto`, `en`, `uk` |

Changing the bridge URL in options validates the new URL with `GET /api/v1/status`
and then reloads the config entry.

## Poll Interval

The coordinator polls the native catalog. Entity state comes from the latest successful catalog refresh.

Changing the interval changes how quickly Home Assistant sees bridge-side state changes. It does not change device-side event handling inside the bridge.

## Preferred Video Profile

This controls which bridge profile is preferred when choosing stream URLs.

| Value | Behavior |
| --- | --- |
| `auto` | Use the bridge `recommended_profile`, then fallback profiles. |
| `quality` | Prefer the main/quality profile, then fallback. |
| `stable` | Prefer the lower-bandwidth stable profile, then fallback. |

Aliases accepted internally:

- `default` and `main` map to `quality`
- `substream` and `sub` map to `stable`

## Preferred Video Source

This controls which URL type the camera entity prefers.

| Value | Source order |
| --- | --- |
| `auto` | HLS, RTSP, MJPEG |
| `hls` | HLS, RTSP, MJPEG |
| `mjpeg` | MJPEG, HLS, RTSP |
| `rtsp` | RTSP, HLS, MJPEG |

HTTP URLs from the bridge are rewritten through the configured bridge URL. Direct `rtsp://` URLs are preserved as-is.

When `rtsp` is selected, the integration asks the bridge catalog to include stream credentials.

## Integration Language

This controls labels owned by the integration, including entity names such as camera, online sensors, bridge-generated field names, and bridge-backed controls.

| Value | Behavior |
| --- | --- |
| `auto` | Use Home Assistant's configured language when it is supported; otherwise use English. |
| `en` | Use English integration labels. |
| `uk` | Use Ukrainian integration labels. |

Home Assistant's own UI language still controls the surrounding Home Assistant interface. This option exists so integration-generated entity/control labels can be predictable even when Home Assistant is used from multiple browsers or accounts.

## Diagnostics

Diagnostics export includes config-entry details, coordinator state, bridge status, and the native catalog. Sensitive URL-shaped fields are redacted.

## Next

- [Entities and controls](entities-and-controls.md)
- [How it works](architecture.md)
