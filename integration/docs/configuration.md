# Configuration

All configuration is done through the Home Assistant UI.

## Bridge URL

The bridge URL must be reachable by Home Assistant. It must include `http://` or `https://`.

Examples:

```text
http://bridge-host:9020
https://ha.example.com/dahuabridge
```

Use the reverse-proxy URL if Home Assistant reaches the bridge through a proxy path. The integration uses this configured URL when it rewrites bridge-hosted links from the catalog.

For example, if nginx exposes the bridge with:

```nginx
location /dahua-bridge/ {
  rewrite ^/dahua-bridge/(.*) /$1 break;
  proxy_pass http://bridge:9020;
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
polling and appends the token as a query parameter to browser-facing media/snapshot
URLs so Home Assistant camera entities can still load images and streams.

## Options

The options flow exposes:

- bridge URL
- bridge API token
- poll interval
- preferred video profile
- preferred live source (default for NVR channels)
- live connection warm-up
- preferred playback format
- playback format fallbacks
- integration language

Defaults:

| Option | Default | Allowed values |
| --- | --- | --- |
| Bridge URL | Current configured URL | Any reachable `http://` or `https://` bridge base URL |
| Bridge API token | Empty | Token configured on the bridge, if any |
| Poll interval | `15` seconds | `5` to `300` seconds |
| Preferred video profile | `quality` | `auto`, `quality`, `stable` |
| Preferred live source | Bridge default, initially `nvr` | `nvr`, `camera` |
| Live connection warm-up | Bridge setting, initially `off` | `off`, `recent`, `always` |
| Preferred playback format | `rtsp` | `auto`, `hls`, `dash`, `mjpeg`, `rtsp` |
| Integration language | `auto` | `auto`, `en`, `uk` |

Changing the bridge URL in options validates the new URL with `GET /api/v1/home-assistant/native/catalog`
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
| `auto` | Bridge RTSP, HLS, DASH, MJPEG |
| `hls` | HLS, DASH, MJPEG, Bridge RTSP |
| `dash` | DASH, HLS, MJPEG, Bridge RTSP |
| `mjpeg` | MJPEG, HLS, DASH, Bridge RTSP |
| `rtsp` | Bridge RTSP, HLS, DASH, MJPEG |

HTTP URLs from the bridge are rewritten through the configured bridge URL. Live
bridge RTSP URLs use that host with the relay port (default `8554`). The bridge
chooses the upstream; Home Assistant receives one stable URL per profile.

When `rtsp` is selected, Home Assistant authenticates to the relay with the bridge
API token if configured. Camera and NVR credentials stay inside the bridge.

## Preferred Live Source in Integration Settings

Open **Settings → Devices & services → DahuaBridge → Configure** and choose
**Preferred live source (default)**. In Ukrainian it is **Бажане джерело прямого
ефіру (за замовчуванням)**, between the video profile and playback format.

- **Camera** prefers each channel's direct camera RTSP connection, with NVR fallback.
- **NVR** prefers the recorder's live RTSP connection, with direct camera fallback.

The bridge verifies the alternate route before switching, retries the preferred
route after recovery, and reports the effective source separately from the saved
preference. Camera fallback requires configured direct credentials. If neither
route works, playback remains unavailable until one recovers. This upstream
failover is independent of the playback-format fallback checkbox.

Opening the form reads the current bridge default. Saving sends a change only
when necessary, persists it in bridge state, and leaves per-camera overrides in
place. Bridge YAML edits and restarts are not needed to switch preferences once
the updated bridge is installed. Installing this update requires rebuilding the
bridge and restarting Home Assistant to load the new integration code.

The **Bridge RTSP (no re-encoding)** playback format forwards the encoded stream
without transcoding it. The former `Direct RTSP` selection now uses this relay.
HA reconnects to the same URL after a source change; it does not choose or store
Camera/NVR fallback URLs. Archived recordings continue to use the NVR through
the existing bridge playback endpoints.

HA must reach the relay's TCP port on the bridge host. Containers on the same
Docker network can use `dahua-bridge:8554` without publishing another NAS port.
An HTTP reverse proxy does not carry RTSP; installations on separate networks
must also make the relay port reachable. The optional `media.rtsp_listen_address`
setting changes the listener, with `:8554` used when omitted.

## Live Connection Warm-up

Open **Settings → Devices & services → DahuaBridge → Configure** and choose
**Live connection warm-up**. The bridge uses the **Preferred video profile**
selected in the same form: `auto`, `quality`, or `stable`.

| Choice | Behavior |
| --- | --- |
| On demand (default) | Open upstream connections when a viewer needs them, using the normal idle timeout. |
| Keep recently viewed streams warm (5 minutes) | Keep a recently used connection available for five minutes after viewing ends. |
| Preconnect selected video profile (continuous) | Connect the selected profile in advance and keep it receiving without viewers. |

This is a global bridge setting. Opening the form reads
`GET /api/v1/settings/live-preconnect`; saving sends the mode and selected profile
together only when they differ. Older bridges hide the unsupported option and
still allow other options to be saved. A failed warm-up save leaves HA options
and connection settings unchanged. Setup and catalog polling do not reapply it.

Warm connections reduce the RTSP connection handshake. The bridge still chooses
Camera/NVR sources and fallback, and all HA live URLs remain unchanged. A new
player may still wait for the camera's next keyframe, HA stream startup, or
HLS/DASH buffering. This does not promise instant playback.

Continuous mode receives camera video even without viewers, using continuous
network bandwidth and a camera connection for each warmed profile. The RTSP
relay does not re-encode it. Bridge HLS/DASH workers remain on demand.

Home Assistant's separate **Preload stream** option also keeps its own native
playback pipeline ready and can reduce native player startup further, at extra
HA resource usage. This integration setting does not change that preference.
See the [Home Assistant camera documentation](https://www.home-assistant.io/integrations/camera/#streaming-video).

## Per-camera Live Source

For NVR channels, the camera's **Settings → Preferred live source** selector in
the surveillance panel offers **Use integration default (NVR/Camera)**, **NVR**,
and **Camera**. Choose the default to follow the integration's global preference,
or choose NVR/Camera to save an override for that channel. Changing the global
default preserves these per-camera overrides. This is independent of the
quality/stable profile and RTSP/HLS/DASH/MJPEG output selection.

The Camera preference can be saved even when direct camera credentials are
missing. The bridge uses the other available source when the preferred source
fails; the panel shows the effective source and fallback reason. Direct camera
streaming requires a configured camera connection and usable credentials.
Archive playback and exports continue to use the NVR.

Automations can change the same setting through the authenticated camera service:

```yaml
action: dahuabridge.set_live_source
target:
  entity_id: camera.west20_nvr_channel_01_camera
data:
  source: camera # camera, nvr, or default to clear this camera's override
```

The integration refreshes the catalog after a successful change. The bridge owns
upstream failover behind its stable stream URLs; source metadata changes do not
reset the Home Assistant stream. Changes made in the bridge administration UI take
effect in Home Assistant on the next catalog refresh. Active native archive
playback is preserved. The `bridge_live_source` camera attribute exposes the
effective input, global default, per-camera override, and camera availability;
`bridge_profiles` exposes stable bridge stream URLs. Current cards use bridge
playback sessions for archive access. The older `recorder_stream_url` field is
supported only for compatibility with earlier bridge catalogs and follows the
existing credential redaction rules.

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
