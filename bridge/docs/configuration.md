# Configuration

The bridge is configured with `config.yaml`.

The best reference template is:

- `../config.example.yaml`

This page focuses on the sections that matter most for real deployments.

## `log`

Controls process logging.

Important fields:

- `level`
- `pretty`

## `http`

Controls the built-in HTTP server.

Important fields:

- `listen_address`
- `metrics_path`
- `health_path`
- `read_timeout`
- `write_timeout`
- `idle_timeout`
- `admin_rate_limit_*`
- `snapshot_rate_limit_*`
- `media_rate_limit_*`

Use this section when you need to:

- change ports
- run behind a reverse proxy
- tune rate limits

## `media`

Controls bridge-hosted media.

Important fields:

- `enabled`
- `ffmpeg_path`
- `ffmpeg_log_level`
- `input_preset`
- `video_encoder`
- `clip_path`
- `idle_timeout`
- `start_timeout`
- `max_workers`
- `frame_rate`
- `stable_frame_rate`
- `substream_frame_rate`
- `jpeg_quality`
- `threads`
- `scale_width`
- `read_buffer_size`
- `hls_segment_time`
- `hls_list_size`
- `hls_tmp_dir`
- `hls_keep_after_exit`
- `hwaccel_args`
- `webrtc_ice_servers`
- `webrtc_uplink_targets`

This section controls:

- live MJPEG
- HLS
- WebRTC helper paths
- stream-backed snapshots
- bridge-owned clip recording

Important operational notes:

- `enabled: true` is required for MJPEG, HLS, WebRTC helper pages, generic stream snapshots, and bridge-owned MP4 clip recording
- `clip_path` defaults to `/data/clips` when omitted
- `webrtc_uplink_targets` only matter for VTO browser microphone export / external RTP export
- `webrtc_ice_servers` only matter for WebRTC clients that need STUN or TURN

See:

- [media-and-recording.md](media-and-recording.md)

## `archive`

Controls the bridge-side archive index and future cache/pretranscode work.

Important fields:

- `enabled`
- `db_path`
- `cache_dir`
- `prefetch_days`
- `retain_days`
- `max_parallel_jobs`
- `prefetch_smd`
- `prefetch_ivs`
- `cron`

This section controls:

- SQLite indexing of recorder archive files
- SQLite indexing of SMD/IVS event-backed archive items
- event-to-file linking
- persisted transcode job and asset metadata
- scheduled background sync

Important operational notes:

- `enabled: true` turns on the archive background service
- `db_path` is the SQLite file used for normalized archive metadata
- `cache_dir` is reserved for archive cache and future pre-transcoded assets
- `prefetch_days` defines how far back the bridge indexes recorder data
- `retain_days` defines when old indexed rows are pruned
- `cron` uses a 5-field cron shape such as `5,35 * * * *`
- the current implementation indexes native archive files plus SMD/IVS event-backed archive results

## `home_assistant`

Controls bridge behavior related to Home Assistant.

Important fields:

- `enabled`
- `node_id`
- `entity_mode`
- `camera_snapshot_source`
- `public_base_url`
- `api_base_url`
- `access_token`
- `request_timeout`

For the supported setup:

```yaml
home_assistant:
  entity_mode: native
```

`public_base_url` is especially important because it is used when the bridge generates URLs in the native catalog.

Critical values:

- `entity_mode` must be `native`
- `camera_snapshot_source` must be `device` or `logo`
- `public_base_url` should be the exact base URL that Home Assistant and browsers can really open
- `api_base_url` and `access_token` are only needed when the bridge itself must call back into Home Assistant

## `state_store`

Controls persistent bridge state.

Important fields:

- `enabled`
- `path`
- `flush_interval`

## `devices`

Device inventory is grouped by type:

- `nvr`
- `ipc`
- `vto`

Each entry typically defines:

- `id`
- `name`
- `manufacturer`
- `model`
- `base_url`
- `username`
- `password`
- `poll_interval`
- `request_timeout`
- `insecure_skip_tls`
- `enabled`

NVR entries also commonly use:

- `channel_allowlist`
- `onvif_*`
- `allow_config_writes`, default `false`; set to `true` only when the bridge is allowed to change NVR config values such as record mode or stream-audio state
- `direct_ipc_credentials`, when direct-camera API calls are needed for a channel; `/admin/test-bridge` uses these credentials for direct IPC lighting, audio, and raw PTZ CGI diagnostics

For heavy use of `/admin/test-bridge`, raise `http.admin_rate_limit_per_minute` and `http.admin_rate_limit_burst` enough for repeated button testing.

## Configuration Advice

- choose stable device IDs and do not rename them casually
- keep `public_base_url` aligned with real browser and Home Assistant reachability
- only enable hardware acceleration after confirming it works in your environment
- use `media.enabled: false` only if you do not want bridge-hosted snapshots, HLS, MJPEG, WebRTC helpers, or bridge-owned clip recording
- use `archive.enabled: true` when you want DB-first archive search results and persisted archive metadata
- keep `channel_allowlist`, `lock_allowlist`, and `alarm_allowlist` narrow if your devices expose unused placeholders
- leave `allow_config_writes` disabled unless NVR config mutation has been approved for that installation

## Next Step

- [features.md](features.md)
- [api-reference.md](api-reference.md)
