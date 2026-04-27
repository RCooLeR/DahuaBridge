# Project Status

Last updated: 2026-04-27

This file is the working end-state plan for the whole repository after the repo split into:

- `bridge/`
- `integration/`
- `ha-cards/`

Use this as the short answer to "where are we now?" and "what still has to be finished?"

## Current Position

- [x] Repository split completed: bridge code under `bridge/`, Home Assistant integration under `integration/`, future cards under `ha-cards/`
- [x] Bridge builds from `bridge/` as a standalone Go/Docker subtree
- [x] Home Assistant custom integration exists and is installable from `integration/custom_components/dahuabridge`
- [x] Native bridge catalog endpoint exists for unified Home Assistant device grouping
- [ ] Real Home Assistant runtime validation of the custom integration is still pending
- [ ] Final intercom-grade VTO media/talkback work is still pending
- [ ] Final real-device validation matrix is still pending

## Repo Layout

- [x] Minimal root layout with only repo-level files plus `bridge/`, `integration/`, `ha-cards/`
- [x] `bridge/` contains the full standalone bridge source, docs, Dockerfile, and config examples
- [x] `integration/` contains the standalone Home Assistant custom integration source
- [x] `ha-cards/` reserved for future custom cards
- [ ] Final cleanup of any local machine temp artifacts before release snapshot
  Note: one Windows-locked local temp folder may still remain under `bridge/.gotmp-*` after test runs

## Bridge: Core Service

- [x] CLI entrypoint and config loading
- [x] Structured logging
- [x] MQTT client with publish and subscribe support
- [x] MQTT discovery/state publishing
- [x] Probe loops for enabled devices
- [x] Event stream loops for supported devices
- [x] Persisted state store and restore
- [x] Republish restored state on startup
- [x] In-memory recent event buffer
- [x] Admin HTTP server
- [x] Health/readiness/metrics endpoints
- [x] Docker image and compose example
- [ ] Final production hardening pass under sustained real deployment

## Bridge: Device Support

### NVR

- [x] Identity, firmware, channel, disk, and encode metadata probe
- [x] Snapshot proxy
- [x] Event ingestion and normalization for motion, human, vehicle, tripwire, intrusion
- [x] MQTT discovery for NVR root, channels, and disks
- [x] MQTT event entities and device triggers
- [x] Inventory refresh control
- [ ] Full live validation across real NVR event payload variants

### IPC

- [x] Identity, firmware, and encode metadata probe
- [x] Snapshot proxy
- [x] Event ingestion and normalization for motion and AI events
- [x] MQTT discovery and device triggers
- [x] Legacy TLS compatibility path
- [ ] Broader live validation across more IPC firmware variants

### VTO

- [x] Identity, firmware, encode metadata, alarm input, and lock inventory
- [x] Snapshot proxy
- [x] Event ingestion and normalization for call, doorbell, tamper, access
- [x] MQTT discovery for root, locks, and alarms
- [x] Lock control path and admin unlock action
- [x] VTO call answer action
- [x] VTO call hangup action
- [x] Browser intercom page
- [x] Bridge-side intercom session reset and RTP export toggles
- [ ] Physical unlock validation end-to-end
- [ ] Full accepted-call flow validation on live ringing calls
- [ ] Direct VTO talkback / two-way audio
- [ ] SIP or equivalent signaling if needed for complete call control
- [ ] Full live validation across real VTO event payload variants

## Bridge: Streams And Media

- [x] Stream catalog endpoint
- [x] Per-stream detail endpoint
- [x] Stream filtering by device
- [x] Snapshot URL generation
- [x] RTSP profile variants: default, quality, stable, substream
- [x] Recommended profile heuristic
- [x] Local MJPEG output
- [x] Shared ffmpeg worker model
- [x] Worker inventory endpoint
- [x] HLS output
- [x] WebRTC playback output
- [x] Audio playback path where source audio exists
- [x] Browser/mobile preview pages
- [ ] Audio uplink connected through to actual device talkback path
- [ ] Fully interactive low-latency browser intercom media
- [ ] True `go2rtc` replacement scope

## Bridge: Home Assistant Support

### MQTT Path

- [x] Automatic MQTT device creation
- [x] Automatic MQTT entity creation
- [x] MQTT event entities for automation workflows
- [x] MQTT triggers for NVR/IPC/VTO activity
- [x] MQTT buttons for VTO lock and call/session actions

### Generated Helper Endpoints

- [x] Camera package generation
- [x] Dashboard package generation
- [x] Lovelace dashboard generation

### ONVIF Path

- [x] ONVIF profile discovery
- [x] ONVIF H.264 guidance
- [x] ONVIF recommendation exposure in stream inventory
- [x] Home Assistant ONVIF provisioning flow
- [x] Migration guidance away from ONVIF/generic-camera/MQTT split toward native integration
- [ ] Keep or downgrade this path after native integration proves itself in real HA

## Native Home Assistant Integration (`integration/`)

### Foundation

- [x] Custom integration scaffold
- [x] Config flow
- [x] Options flow for scan interval
- [x] Options changes reload the integration cleanly
- [x] Coordinator-based polling model
- [x] Device registry grouping based on bridge device identity
- [x] Brand assets
- [x] Home Assistant diagnostics hook with redacted bridge/catalog URLs
- [x] Home Assistant migration helpers for native-only cleanup

### Entity Coverage

- [x] Camera entities
- [x] Binary sensor entities
- [x] Sensor entities
- [x] Button entities
- [x] Root device action buttons like `Probe Now`
- [x] NVR root `Refresh Inventory` button
- [x] VTO answer/hangup/reset/export/unlock actions exposed through buttons
- [x] Native catalog consumes merged bridge state plus selected stream metadata
- [x] Camera entities use absolute bridge URLs for stream/snapshot/preview
- [ ] Full parity with the useful existing MQTT surface
- [ ] Real-world entity naming/enable-default review inside Home Assistant

### Unified Device Goal

- [x] Architectural path exists for `nvr_channel_01` to own camera plus related sensors in one HA device
- [x] Bridge-native catalog endpoint supports this model
- [ ] Real Home Assistant confirmation that the final grouping behaves exactly as intended
- [ ] Migration guidance from MQTT/ONVIF/generic-camera split to native integration as primary path

## HTTP / Admin Surface

- [x] Admin page
- [x] Device inventory APIs
- [x] Event APIs
- [x] Stream/media APIs
- [x] NVR snapshot APIs
- [x] IPC snapshot APIs
- [x] VTO snapshot APIs
- [x] VTO unlock, answer, hangup, intercom status, reset, uplink control APIs
- [x] Home Assistant package endpoints
- [x] Native Home Assistant catalog endpoint
- [x] Home Assistant migration plan and MQTT-discovery cleanup endpoints
- [ ] Final compatibility validation of all important APIs against real users of the project

## Testing / Validation

- [x] Broad unit and integration-style tests across bridge packages
- [x] Targeted tests for native catalog and HTTP exposure
- [x] Python syntax/compile validation for the Home Assistant integration
- [ ] Real Home Assistant installation test
- [ ] Real Dahua end-to-end validation with final device set
- [ ] Long-running load/stability validation on target host
- [ ] Final cleanup of Windows-specific temporary test-process issues during local development runs

## Release-Ready End State

- [x] Bridge subtree can be copied and built on its own
- [x] Integration subtree can be copied into Home Assistant on its own
- [x] Project has a clear architecture for unified HA devices
- [ ] Native integration proven in a real Home Assistant instance
- [ ] VTO intercom-grade features proven or explicitly cut from release scope
- [ ] Final decision on what remains primary vs optional: MQTT, ONVIF, native integration
- [ ] Final release snapshot with docs aligned to the validated setup

## Short Answer

What is effectively done:

- [x] Core bridge
- [x] NVR / IPC / most VTO bridge plumbing
- [x] Browser media and admin surface
- [x] Repo restructure
- [x] Native HA integration foundation

What is still truly open:

- [ ] Native HA integration real-world validation and polish
- [ ] Full VTO talkback / accepted-call media
- [ ] Final physical validation on real hardware
