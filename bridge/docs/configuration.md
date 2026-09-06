# Configuration

The bridge reads `config.yaml`. Use `../config.example.yaml` as the template; it now contains only settings used by the running bridge.

Unknown legacy YAML keys are ignored by the loader. Remove old keys from local configs anyway so the file reflects what the process actually uses.

## Active Sections

| Section | Used for |
| --- | --- |
| `log` | process logging |
| `http` | built-in API/admin/metrics server and rate limits |
| `media` | bridge-hosted snapshots, MJPEG, HLS, WebRTC, and bridge-owned MP4 clips |
| `archive` | SQLite archive/event index, archive export, and background SMD/IVS sync |
| `home_assistant` | bridge-generated URLs consumed by the native HA integration |
| `imou` | optional Imou Open Platform channel overrides |
| `state_store` | persisted probe state and Imou auth state |
| `devices` | NVR, IPC, and VTO inventory |

## `log`

- `level`: log verbosity.
- `pretty`: human-readable console formatting.

## `http`

- `listen_address`: bind address for the HTTP server.
- `metrics_path`: Prometheus metrics route.
- `health_path`: liveness route.
- `read_timeout`, `write_timeout`, `idle_timeout`: HTTP server timeouts.
- `auth_token`: optional bearer-style API token. Empty disables bridge-level HTTP auth.
- `auth_token_env`: environment variable read when `auth_token` is empty; defaults to `DAHUABRIDGE_HTTP_AUTH_TOKEN`.
- `auth_query_token`: allow `?auth_token=...` / `?token=...` on browser-facing URLs when auth is enabled.
- `allowed_origins`: optional CORS allowlist. Empty keeps the permissive default for trusted reverse-proxy deployments.
- `trusted_proxies`: proxy IPs/CIDRs whose forwarded client-IP headers are trusted for rate limiting.
- `max_request_body_bytes`: request body cap for API/admin routes.
- `admin_rate_limit_*`: admin/API action limiter.
- `snapshot_rate_limit_*`: snapshot limiter.
- `media_rate_limit_*`: media endpoint limiter.

When `auth_token` or `auth_token_env` is set, most API/admin/media routes require
`Authorization: Bearer <token>` or `X-DahuaBridge-Token: <token>`. Health,
readiness, status, metrics, CORS preflight, and static admin assets stay open for
monitoring and browser startup. Query-token auth exists for Home Assistant camera
resource URLs and should be used only behind HTTPS.

Set `trusted_proxies` only to proxies you control. If it is empty, rate limiting
uses the direct TCP peer address and ignores forwarded-IP headers.

## `media`

- `enabled`: enables bridge-hosted media.
- `rtsp_listen_address`: live RTSP relay listener, default `:8554`; TCP transport,
  shared upstreams, no video re-encoding. HA must reach this port directly or
  over the shared Docker network. It uses the HTTP API token for RTSP authentication.
- `ffmpeg_path`: ffmpeg executable.
- `ffmpeg_log_level`: child ffmpeg log verbosity.
- `input_preset`: RTSP input flags, `low_latency` or `stable`.
- `video_encoder`: `software` or Intel `qsv`.
- `clip_path`: finished bridge MP4 clip directory.
- `idle_timeout`: unused worker shutdown delay.
- `start_timeout`: first-frame or initial-playlist timeout.
- `max_workers`: active media worker/session cap.
- `frame_rate`: default transcode output rate for quality/default profile work.
- `stable_frame_rate`: transcode output rate for the `stable` profile.
- `jpeg_quality`: MJPEG quality argument.
- `threads`: ffmpeg thread count.
- `scale_width`: output width; `0` disables scaling.
- `read_buffer_size`: MJPEG parser buffer size.
- `hls_segment_time`: HLS/DASH segment duration.
- `hls_list_size`: live playlist segment count.
- `hls_tmp_dir`: HLS/DASH working directory.
- `hls_keep_after_exit`: keep playback HLS/DASH outputs temporarily after worker exit.
- `hwaccel_args`: optional ffmpeg hardware acceleration input args.
- `webrtc_ice_servers`: optional STUN/TURN config for WebRTC.
- `webrtc_uplink_targets`: optional UDP targets for VTO browser microphone RTP export.

Legacy note: `hls_temp_path` is still accepted as an alias for `hls_tmp_dir`, but new configs should use `hls_tmp_dir`.

## `archive`

- `enabled`: starts the archive service.
- `db_path`: SQLite database for `smd_ivs_events`, `nvr_recording_chunks`, and export metadata.
- `temp_dir`: staging directory for recorder DAV downloads and iframe-prefix work.
- `prefetch_days`: recent history window to index.
- `retain_days`: indexed row retention window.
- `max_parallel_jobs`: archive MP4 asset prefetch concurrency cap.
- `prefetch_smd`: index SMD events.
- `prefetch_ivs`: index IVS events.
- `export_event_mp4`: create automatic MP4 backup clips for SMD/IVS events. Set to `false` to keep DB event counts/search while disabling automatic event MP4 creation.
- `export_delay`: delay after an SMD/IVS event ends before automatic MP4 export starts. Defaults to `1h`.
- `export_ivs`: IVS event channels allowed for automatic MP4 export. Empty means all channels.
- `export_smd_person`: SMD person/human event channels allowed for automatic MP4 export. Empty means all channels.
- `export_smd_transport`: SMD transport/vehicle event channels allowed for automatic MP4 export. Empty means all channels.
- `export_smd_animal`: SMD animal event channels allowed for automatic MP4 export. Empty means all channels.
- `cron`: 5-field cron schedule for chunk sync.

SMD/IVS list APIs read from SQLite first. Event filters match normalized event codes and Dahua event-type strings stored in the database, including values such as `Event.smdTypeHuman`.

## `home_assistant`

- `public_base_url`: the browser- and Home Assistant-reachable base URL for this bridge.

When the bridge is mounted under a reverse-proxy prefix, include that prefix here. With an nginx rule such as `location /dahua-bridge/ { ... }`, use `https://ha.example.com/dahua-bridge`, not `https://ha.example.com`.

The bridge no longer publishes MQTT discovery and does not call the Home Assistant API. Native integration discovery uses `/api/v1/home-assistant/native/catalog`.

## `imou`

- `enabled`: enables Imou override support.
- `app_id`, `app_secret`: Imou Open Platform credentials. Environment variables `DAHUABRIDGE_IMOU_APP_ID` and `DAHUABRIDGE_IMOU_APP_SECRET` are also supported.
- `data_center`: `fk`, `sg`, or `or`.
- `endpoint`: optional explicit API endpoint override.
- `request_timeout`: Imou HTTP timeout.
- `alarm_poll_interval`: cloud event polling interval.
- `event_active_window`: synthetic active window after cloud alarm events.

## `state_store`

- `enabled`: writes probe/auth state to disk.
- `path`: JSON state file path.
- `flush_interval`: periodic write interval.

## `devices`

Common fields:

- `id`: stable bridge ID used in URLs and integration identifiers.
- `name`: display name.
- `manufacturer`, `model`: UI/device registry metadata.
- `base_url`: Dahua HTTP/HTTPS endpoint.
- `username`, `password`: primary device credentials.
- `rpc_username`, `rpc_password`: optional RPC credentials when RPC archive/config operations require a different account.
- `onvif_enabled`, `onvif_username`, `onvif_password`, `onvif_service_url`: optional ONVIF probing.
- `poll_interval`: probe interval.
- `request_timeout`: per-device HTTP timeout.
- `insecure_skip_tls`: allow self-signed/broken HTTPS certs for trusted devices.
- `enabled`: keep an entry in the file while disabling it.

NVR-specific fields:

- `channel_allowlist`: expose only selected 1-based channels.
- `channel_aux_control_overrides`: correct siren/light/wiper capability mapping.
- `channel_ptz_control_overrides`: hide/show PTZ when firmware reports it incorrectly.
- `channel_recording_control_overrides`: override recorder-mode capability/state metadata.
- `channel_imou_overrides`: map NVR channels to Imou cloud devices for events/lights/siren.
- `direct_ipc_credentials`: call the real IPC directly for controls behind an NVR; also supplies the camera login for the optional **Camera** live-source setting. Select the live source in the UI; the selection is persisted in the state store rather than this YAML.
- `allow_config_writes`: permit NVR recorder-mode config mutations; default is false.

VTO-specific fields:

- `lock_allowlist`: expose only selected locks.
- `alarm_allowlist`: expose only selected alarm inputs.

## Removed Legacy Settings

These settings were removed because no live bridge flow used them:

- `mqtt.*`
- `home_assistant.enabled`
- `home_assistant.node_id`
- `home_assistant.entity_mode`
- `home_assistant.camera_snapshot_source`
- `home_assistant.api_base_url`
- `home_assistant.access_token`
- `home_assistant.request_timeout`
- `media.substream_frame_rate`
- `archive.cache_dir`

## Next Step

- [features.md](features.md)
- [api-reference.md](api-reference.md)
