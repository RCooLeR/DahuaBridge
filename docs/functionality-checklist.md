# DahuaBridge Functionality Checklist

Last reviewed: 2026-04-27

This file is the working checklist for planned functionality. Mark an item with `[x]` only when it is fully implemented and validated enough to consider it closed. Leave it unchecked if it is missing, partial, or only documented.

## Core service

- [x] CLI entrypoint with config file loading
- [x] Structured logging
- [x] MQTT client with publish and subscribe support
- [x] Home Assistant MQTT discovery publishing
- [x] Periodic probe loop per enabled device
- [x] On-demand admin reprobe action
- [x] Event stream loop per event-capable device
- [x] Persisted state store with restore on startup
- [x] Republish restored state back to Home Assistant
- [x] Recent in-memory event buffer for reverse validation
- [x] Event buffer filtering and clear/reset admin actions
- [x] Prometheus metrics endpoint
- [x] Admin HTTP server
- [x] Health and readiness endpoints
- [x] Docker image
- [x] Compose example

## Device support: NVR

- [x] NVR device probe
- [x] NVR identity and firmware inventory
- [x] NVR channel inventory
- [x] NVR disk inventory
- [x] NVR channel encode metadata collection
- [x] NVR snapshot proxy
- [x] NVR event stream ingestion
- [x] NVR motion event normalization
- [x] NVR AI event normalization for human/vehicle/tripwire/intrusion
- [x] NVR Home Assistant discovery for root device
- [x] NVR Home Assistant discovery for channel child devices
- [x] NVR Home Assistant discovery for disk child devices
- [x] NVR MQTT event entities
- [x] NVR MQTT device triggers for channel activity
- [x] NVR recorder health entities beyond current inventory-derived diagnostics
- [x] NVR control actions such as reboot, sync inventory, PTZ, record control, or similar operator commands
- [ ] Full live validation of all supported NVR event payload variants

## Device support: VTO

- [x] VTO device probe
- [x] VTO identity and firmware inventory
- [x] VTO encode metadata collection
- [x] VTO alarm input inventory
- [x] VTO lock inventory
- [x] VTO snapshot proxy
- [x] VTO RTSP stream hints
- [x] VTO event stream ingestion
- [x] VTO doorbell/call/tamper/access event normalization
- [x] VTO Home Assistant discovery for root device
- [x] VTO Home Assistant discovery for lock child devices
- [x] VTO Home Assistant discovery for alarm child devices
- [x] VTO MQTT device triggers for doorbell/call/access/tamper activity
- [x] MQTT command routing for VTO lock open button
- [x] VTO RPC transport for unlock control
- [x] Admin HTTP unlock action for VTO locks
- [ ] Physical VTO unlock validated end-to-end from the final Home Assistant flow
- [ ] Full call acceptance flow
- [ ] Call answer and hangup actions
- [ ] Two-way audio / talkback
- [ ] SIP or equivalent call/media signaling support
- [x] VTO-specific custom Home Assistant integration or richer intercom UI
- [ ] Full live validation of all VTO event payload variants under real activity

## Device support: IPC

- [x] Standalone IPC device probe
- [x] IPC identity and firmware inventory
- [x] IPC snapshot proxy
- [x] IPC RTSP stream hints
- [x] IPC event stream ingestion
- [x] IPC motion event normalization
- [x] IPC AI event normalization for human/vehicle/tripwire/intrusion
- [x] IPC Home Assistant discovery
- [x] IPC MQTT event entities
- [x] IPC MQTT device triggers for motion and AI activity
- [x] Legacy TLS compatibility path for older HTTPS IPC devices
- [ ] Broader live validation across more IPC firmware variants

## Home Assistant integration

- [x] Automatic MQTT device creation for supported Dahua devices
- [x] Automatic MQTT entity creation for supported diagnostics and activity states
- [x] MQTT event entities for automation-first workflows
- [x] MQTT device triggers for NVR channel activity
- [x] MQTT device triggers for IPC activity
- [x] MQTT button entity for VTO lock open
- [x] Generated camera package endpoint
- [x] Generated stream-profile variants for camera packages
- [x] Generated dashboard camera package endpoint
- [x] Documentation for Home Assistant setup
- [x] Fully automatic Home Assistant camera creation without operator copy/paste or UI steps
- [x] Auto-provisioning of Home Assistant ONVIF entities from the bridge
- [x] One-click Home Assistant provisioning flow
- [x] Final dashboard layout and final entity naming pass

## Streams and media

- [x] JSON stream catalog endpoint
- [x] Per-stream detail endpoint
- [x] Stream filtering by device ID
- [x] Optional credential inclusion in generated stream references
- [x] Snapshot URL generation for streams
- [x] Multiple RTSP profile variants: default, quality, stable, substream
- [x] Recommended profile heuristic
- [x] Local MJPEG HTTP output for dashboard playback
- [x] Shared worker model for active MJPEG streams
- [x] Media worker inventory endpoint
- [x] Media worker limits, idle shutdown, and metrics
- [x] Lower-bandwidth browser-native video path beyond MJPEG
- [x] WebRTC media output
- [x] HLS or LL-HLS media output
- [x] Audio output path
- [ ] Audio uplink / talkback path
- [ ] Full browser-friendly low-latency interactive media
- [ ] True go2rtc replacement

## ONVIF

- [x] ONVIF profile discovery
- [x] ONVIF H.264 profile selection guidance
- [x] `recommended_ha_integration` exposure in stream inventory
- [x] Broader ONVIF coverage beyond profile discovery and H.264 selection guidance
- [x] More integration tests around ONVIF behavior

## HTTP and operational APIs

- [x] `GET /healthz`
- [x] `GET /readyz`
- [x] `GET /metrics`
- [x] `GET /admin`
- [x] `GET /api/v1/status`
- [x] `GET /api/v1/devices`
- [x] `GET /api/v1/devices/{deviceID}`
- [x] `POST /api/v1/devices/probe-all`
- [x] `POST /api/v1/devices/{deviceID}/probe`
- [x] `GET /api/v1/events`
- [x] `DELETE /api/v1/events`
- [x] `GET /api/v1/streams`
- [x] `GET /api/v1/streams/{streamID}`
- [x] `GET /api/v1/media/workers`
- [x] `GET /api/v1/media/preview/{streamID}`
- [x] `GET /api/v1/media/mjpeg/{streamID}`
- [x] `GET /api/v1/media/hls/{streamID}/{profile}/index.m3u8`
- [x] `GET /api/v1/media/webrtc/{streamID}/{profile}`
- [x] `POST /api/v1/media/webrtc/{streamID}/{profile}/offer`
- [x] `GET /api/v1/nvr/{deviceID}/channels/{channel}/snapshot`
- [x] `POST /api/v1/nvr/{deviceID}/inventory/refresh`
- [x] `POST /api/v1/vto/{deviceID}/locks/{lockIndex}/unlock`
- [x] `POST /api/v1/vto/{deviceID}/call/hangup`
- [x] `GET /api/v1/vto/{deviceID}/intercom`
- [x] `GET /api/v1/vto/{deviceID}/intercom/status`
- [x] `POST /api/v1/vto/{deviceID}/intercom/reset`
- [x] `POST /api/v1/vto/{deviceID}/intercom/uplink/enable`
- [x] `POST /api/v1/vto/{deviceID}/intercom/uplink/disable`
- [x] `GET /api/v1/vto/{deviceID}/snapshot`
- [x] `GET /api/v1/ipc/{deviceID}/snapshot`
- [x] `GET /api/v1/home-assistant/package/cameras.yaml`
- [x] `GET /api/v1/home-assistant/package/cameras_stable.yaml`
- [x] `GET /api/v1/home-assistant/package/cameras_quality.yaml`
- [x] `GET /api/v1/home-assistant/package/cameras_substream.yaml`
- [x] `GET /api/v1/home-assistant/package/cameras_dashboard.yaml`
- [x] `GET /api/v1/home-assistant/dashboard/lovelace.yaml`
- [x] `POST /api/v1/home-assistant/onvif/provision`
- [x] Broader mutating admin APIs for operator actions beyond the current VTO unlock path

## Testing and production hardening

- [x] Unit tests for key config, parsing, driver, discovery, package, store, stream, and media paths
- [ ] Full end-to-end validation with the final real device set
- [ ] Sustained-load tuning on the actual target host
- [ ] Final dashboard-load media tuning
- [x] More integration tests around media worker behavior
- [x] Broader captured-session or live-session integration coverage
- [x] Credential rotation workflows
- [x] Additional rate limiting or abuse protection where needed

## Out of current MVP but still planned direction

- [ ] Full intercom-grade VTO mode inside Home Assistant
- [x] Browser/mobile low-latency live preview without external go2rtc
- [x] Door call session lifecycle handling
- [ ] Unified replacement for `go2rtc` plus custom MQTT scripts
- [ ] Optional custom Home Assistant integration for cameras, calls, unlock, answer, hangup, and speak actions

## Current summary

- Closed foundation: automation-first MQTT bridge, probes, events, snapshots, stream inventory, MJPEG and HLS bridge playback, HTTP admin surface, Docker, persisted state.
- Major open area: intercom-grade media and call control for VTO.
- Major partial area: full interactive browser media, talkback, and final live validation.
