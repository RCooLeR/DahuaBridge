import { describe, expect, it } from "vitest";

import {
  createArchiveSearchRequest,
  createPlaybackSeekRequest,
  createPlaybackSessionRequest,
} from "../src/domain/archive";
import { discoverBridgeTopology } from "../src/domain/devices";
import type {
  DeviceRegistryEntry,
  EntityRegistryEntry,
  RegistrySnapshot,
} from "../src/ha/registry";
import type { HomeAssistant } from "../src/types/home-assistant";

function buildSnapshot(
  entityEntries: EntityRegistryEntry[],
  deviceEntries: DeviceRegistryEntry[],
  areaEntries: Array<{ area_id: string; name: string }>,
): RegistrySnapshot {
  const entitiesByDeviceId = new Map<string, EntityRegistryEntry[]>();
  for (const entry of entityEntries) {
    if (!entry.device_id) {
      continue;
    }
    const existing = entitiesByDeviceId.get(entry.device_id) ?? [];
    existing.push(entry);
    entitiesByDeviceId.set(entry.device_id, existing);
  }

  return {
    entitiesByEntityId: new Map(entityEntries.map((entry) => [entry.entity_id, entry] as const)),
    entitiesByUniqueId: new Map(
      entityEntries
        .filter((entry) => typeof entry.unique_id === "string" && entry.unique_id.trim().length > 0)
        .map((entry) => [entry.unique_id as string, entry] as const),
    ),
    entitiesByDeviceId,
    devicesById: new Map(deviceEntries.map((entry) => [entry.id, entry] as const)),
    areasById: new Map(areaEntries.map((entry) => [entry.area_id, entry] as const)),
  };
}

describe("discoverBridgeTopology", () => {
  it("maps device data through registry-linked customized entity ids", () => {
    const now = new Date().toISOString();
    const hass: HomeAssistant = {
      states: {
        "camera.west20_nvr_channel_01_camera": {
          entity_id: "camera.west20_nvr_channel_01_camera",
          state: "recording",
          attributes: {
            friendly_name: "Entrance Gate",
            bridge_device_id: "west20_nvr_channel_01",
            bridge_root_device_id: "west20_nvr",
            bridge_device_kind: "nvr_channel",
            bridge_base_url: "http://bridge.local:9205",
            bridge_events_url: "http://bridge.local:9205/api/v1/events",
            bridge_controls: {
              ptz: {
                supported: true,
                pan: true,
                tilt: true,
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/ptz",
              },
              aux: {
                supported: true,
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
                features: ["warning_light", "siren"],
                outputs: ["light", "aux"],
              },
              recording: {
                supported: true,
                active: true,
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/recording",
              },
              audio: {
                supported: true,
                mute: true,
                volume: true,
                playback_supported: true,
                playback_siren: true,
                playback_quick_reply: true,
                playback_formats: ["wav", "pcm"],
                playback_file_count: 3,
              },
              validation_notes: ["ptz_profile_validated"],
            },
            bridge_features: [
              {
                key: "recording",
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/recording",
                active: true,
              },
              {
                key: "light",
                label: "White Light",
                group: "deterrence",
                kind: "toggle",
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
                supported: true,
                parameter_key: "output",
                parameter_value: "light",
                actions: ["start", "stop"],
              },
              {
                key: "warning_light",
                label: "Warning Light",
                group: "deterrence",
                kind: "action",
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
                supported: true,
                parameter_key: "output",
                parameter_value: "warning_light",
                actions: ["start", "stop", "pulse"],
              },
              {
                key: "siren",
                label: "Siren",
                group: "deterrence",
                kind: "action",
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/channels/1/aux",
                supported: true,
                parameter_key: "output",
                parameter_value: "siren",
                actions: ["start", "stop", "pulse"],
              },
              {
                key: "archive_search",
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/recordings",
                supported: true,
              },
              {
                key: "archive_playback",
                url: "http://bridge.local:9205/api/v1/nvr/west20_nvr/playback/sessions",
                supported: true,
              },
            ],
            stream_source:
              "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
            bridge_profiles: {
              quality: {
                name: "quality",
                stream_url: "rtsp://bridge.local/quality",
                local_hls_url:
                  "http://bridge.local:9205/api/v1/media/hls/west20_nvr_channel_01/quality",
                local_mjpeg_url:
                  "http://bridge.local:9205/api/v1/media/mjpeg/west20_nvr_channel_01/quality",
                subtype: 0,
                frame_rate: 25,
                source_width: 2560,
                source_height: 1440,
                recommended: true,
              },
            },
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.entry_gate_online": {
          entity_id: "binary_sensor.entry_gate_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.entry_gate_manufacturer_actual": {
          entity_id: "sensor.entry_gate_manufacturer_actual",
          state: "Dahua",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.entry_gate_model_actual": {
          entity_id: "sensor.entry_gate_model_actual",
          state: "IPC-HFW5442",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.entry_gate_serial_actual": {
          entity_id: "sensor.entry_gate_serial_actual",
          state: "CH01SERIAL",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.entry_gate_firmware_actual": {
          entity_id: "sensor.entry_gate_firmware_actual",
          state: "V2.800",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.entry_gate_recommended_ha_actual": {
          entity_id: "sensor.entry_gate_recommended_ha_actual",
          state: "native",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.entry_gate_recommended_reason_actual": {
          entity_id: "sensor.entry_gate_recommended_reason_actual",
          state: "Bridge HLS preferred",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.entry_gate_onvif_profile_name_actual": {
          entity_id: "sensor.entry_gate_onvif_profile_name_actual",
          state: "Profile_1",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.entry_gate_onvif_profile_token_actual": {
          entity_id: "sensor.entry_gate_onvif_profile_token_actual",
          state: "token-1",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.entry_gate_onvif_h264_actual": {
          entity_id: "binary_sensor.entry_gate_onvif_h264_actual",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.recorder_model_actual": {
          entity_id: "sensor.recorder_model_actual",
          state: "DHI-NVR5432",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_serial": {
          entity_id: "sensor.west20_nvr_serial",
          state: "NVRSERIAL",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_firmware": {
          entity_id: "sensor.west20_nvr_firmware",
          state: "V4.001",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_channel_count": {
          entity_id: "sensor.west20_nvr_channel_count",
          state: "16",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.west20_nvr_disk_count": {
          entity_id: "sensor.west20_nvr_disk_count",
          state: "2",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.dev_sda_total_bytes": {
          entity_id: "sensor.dev_sda_total_bytes",
          state: "2000000",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.dev_sda_used_bytes": {
          entity_id: "sensor.dev_sda_used_bytes",
          state: "500000",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.dev_sda_used_percent": {
          entity_id: "sensor.dev_sda_used_percent",
          state: "25",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.dev_sda_online": {
          entity_id: "binary_sensor.dev_sda_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.drive_a_model_actual": {
          entity_id: "sensor.drive_a_model_actual",
          state: "Seagate IronWolf",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "camera.driveway_ipc_camera": {
          entity_id: "camera.driveway_ipc_camera",
          state: "streaming",
          attributes: {
            friendly_name: "Driveway IPC",
            bridge_device_id: "driveway_ipc",
            bridge_root_device_id: "driveway_ipc",
            bridge_device_kind: "ipc",
            bridge_base_url: "http://bridge.local:9205",
            stream_source: "http://bridge.local:9205/api/v1/ipc/driveway_ipc/stream",
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.driveway_ipc_online": {
          entity_id: "binary_sensor.driveway_ipc_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "camera.front_vto_camera": {
          entity_id: "camera.front_vto_camera",
          state: "streaming",
          attributes: {
            friendly_name: "Front VTO",
            bridge_device_id: "front_vto",
            bridge_root_device_id: "front_vto",
            bridge_device_kind: "vto",
            bridge_base_url: "http://bridge.local:9205",
            bridge_events_url: "http://bridge.local:9205/api/v1/events",
            stream_source: "http://bridge.local:9205/api/v1/media/hls/front_vto/quality",
            bridge_local_intercom_url:
              "http://bridge.local:9205/api/v1/media/intercom/front_vto/quality",
            bridge_intercom: {
              answer_url: "http://bridge.local:9205/api/v1/vto/front_vto/call/answer",
              hangup_url: "http://bridge.local:9205/api/v1/vto/front_vto/call/hangup",
              lock_urls: [
                "http://bridge.local:9205/api/v1/vto/front_vto/locks/0/unlock",
              ],
              output_volume_url:
                "http://bridge.local:9205/api/v1/vto/front_vto/audio/output-volume",
              input_volume_url:
                "http://bridge.local:9205/api/v1/vto/front_vto/audio/input-volume",
              mute_url: "http://bridge.local:9205/api/v1/vto/front_vto/audio/mute",
              recording_url: "http://bridge.local:9205/api/v1/vto/front_vto/recording",
              bridge_session_reset_url:
                "http://bridge.local:9205/api/v1/vto/front_vto/intercom/reset",
              external_uplink_enable_url:
                "http://bridge.local:9205/api/v1/vto/front_vto/intercom/uplink/enable",
              external_uplink_disable_url:
                "http://bridge.local:9205/api/v1/vto/front_vto/intercom/uplink/disable",
              supports_hangup: true,
              supports_unlock: true,
              supports_bridge_session_reset: true,
              supports_browser_microphone: true,
              supports_bridge_audio_uplink: true,
              supports_bridge_audio_output: true,
              supports_external_audio_export: true,
              supports_vto_output_volume_control: true,
              supports_vto_input_volume_control: true,
              supports_vto_mute_control: true,
              supports_vto_recording_control: true,
              supports_vto_call_answer: true,
              supports_vto_talkback: true,
              supports_full_call_acceptance: true,
              validation_notes: ["door_station_profile_validated"],
            },
          },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.front_vto_online": {
          entity_id: "binary_sensor.front_vto_online",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.front_station_model_actual": {
          entity_id: "sensor.front_station_model_actual",
          state: "DHI-VTO2202F",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.front_vto_lock_count": {
          entity_id: "sensor.front_vto_lock_count",
          state: "1",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.front_station_ring": {
          entity_id: "binary_sensor.front_station_ring",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "button.answer_front_station": {
          entity_id: "button.answer_front_station",
          state: "unknown",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "button.hangup_front_station": {
          entity_id: "button.hangup_front_station",
          state: "unknown",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "number.front_station_output_level": {
          entity_id: "number.front_station_output_level",
          state: "72",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "number.front_station_input_level": {
          entity_id: "number.front_station_input_level",
          state: "61",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "switch.front_station_muted_toggle": {
          entity_id: "switch.front_station_muted_toggle",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "switch.front_station_auto_record_toggle": {
          entity_id: "switch.front_station_auto_record_toggle",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "button.front_station_unlock": {
          entity_id: "button.front_station_unlock",
          state: "unknown",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.front_door_lock_state": {
          entity_id: "sensor.front_door_lock_state",
          state: "closed",
          attributes: { friendly_name: "Front Door State" },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.front_door_lock_sensor_enabled": {
          entity_id: "binary_sensor.front_door_lock_sensor_enabled",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.front_door_lock_mode": {
          entity_id: "sensor.front_door_lock_mode",
          state: "normal",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.front_lock_model_actual": {
          entity_id: "sensor.front_lock_model_actual",
          state: "Relay Lock",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.vestibule_alarm_sense_method": {
          entity_id: "sensor.vestibule_alarm_sense_method",
          state: "NO",
          attributes: { friendly_name: "Vestibule Alarm Sense Method" },
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.vestibule_alarm_enabled": {
          entity_id: "binary_sensor.vestibule_alarm_enabled",
          state: "on",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "binary_sensor.vestibule_alarm_active": {
          entity_id: "binary_sensor.vestibule_alarm_active",
          state: "off",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
        "sensor.vestibule_alarm_model_actual": {
          entity_id: "sensor.vestibule_alarm_model_actual",
          state: "Alarm Contact",
          attributes: {},
          last_changed: now,
          last_updated: now,
        },
      },
      callService: async () => undefined,
    };

    const registrySnapshot = buildSnapshot(
      [
        {
          entity_id: "camera.west20_nvr_channel_01_camera",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_camera",
        },
        {
          entity_id: "binary_sensor.entry_gate_online",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_online",
        },
        {
          entity_id: "sensor.entry_gate_manufacturer_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_manufacturer",
        },
        {
          entity_id: "sensor.entry_gate_model_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_model",
        },
        {
          entity_id: "sensor.entry_gate_serial_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_serial",
        },
        {
          entity_id: "sensor.entry_gate_firmware_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_firmware",
        },
        {
          entity_id: "sensor.entry_gate_recommended_ha_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_recommended_ha_integration",
        },
        {
          entity_id: "sensor.entry_gate_recommended_reason_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_recommended_ha_reason",
        },
        {
          entity_id: "sensor.entry_gate_onvif_profile_name_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_onvif_profile_name",
        },
        {
          entity_id: "sensor.entry_gate_onvif_profile_token_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_onvif_profile_token",
        },
        {
          entity_id: "binary_sensor.entry_gate_onvif_h264_actual",
          device_id: "dev-channel-1",
          unique_id: "west20_nvr_channel_01_onvif_h264_available",
        },
        {
          entity_id: "sensor.recorder_model_actual",
          device_id: "dev-nvr",
          unique_id: "west20_nvr_model",
        },
        {
          entity_id: "camera.driveway_ipc_camera",
          device_id: "dev-ipc",
          unique_id: "driveway_ipc_camera",
        },
        {
          entity_id: "camera.front_vto_camera",
          device_id: "dev-vto",
          unique_id: "front_vto_camera",
        },
        {
          entity_id: "sensor.front_station_model_actual",
          device_id: "dev-vto",
          unique_id: "front_vto_model",
        },
        {
          entity_id: "binary_sensor.front_station_ring",
          device_id: "dev-vto",
          unique_id: "front_vto_doorbell",
        },
        {
          entity_id: "button.answer_front_station",
          device_id: "dev-vto",
          unique_id: "front_vto_answer_call",
        },
        {
          entity_id: "button.hangup_front_station",
          device_id: "dev-vto",
          unique_id: "front_vto_hangup_call",
        },
        {
          entity_id: "number.front_station_output_level",
          device_id: "dev-vto",
          unique_id: "front_vto_output_volume",
        },
        {
          entity_id: "number.front_station_input_level",
          device_id: "dev-vto",
          unique_id: "front_vto_input_volume",
        },
        {
          entity_id: "switch.front_station_muted_toggle",
          device_id: "dev-vto",
          unique_id: "front_vto_muted",
        },
        {
          entity_id: "switch.front_station_auto_record_toggle",
          device_id: "dev-vto",
          unique_id: "front_vto_auto_record_enabled",
        },
        {
          entity_id: "button.front_station_unlock",
          device_id: "dev-vto",
          unique_id: "front_vto_unlock_1",
        },
        {
          entity_id: "sensor.dev_sda_total_bytes",
          device_id: "dev-disk-1",
          unique_id: "west20_nvr_disk_00_total_bytes",
        },
        {
          entity_id: "sensor.dev_sda_used_bytes",
          device_id: "dev-disk-1",
          unique_id: "west20_nvr_disk_00_used_bytes",
        },
        {
          entity_id: "sensor.dev_sda_used_percent",
          device_id: "dev-disk-1",
          unique_id: "west20_nvr_disk_00_used_percent",
        },
        {
          entity_id: "binary_sensor.dev_sda_online",
          device_id: "dev-disk-1",
          unique_id: "west20_nvr_disk_00_online",
        },
        {
          entity_id: "sensor.drive_a_model_actual",
          device_id: "dev-disk-1",
          unique_id: "west20_nvr_disk_00_model",
        },
        {
          entity_id: "sensor.front_door_lock_state",
          device_id: "dev-vto-lock-1",
          unique_id: "front_vto_lock_00_state",
        },
        {
          entity_id: "binary_sensor.front_door_lock_sensor_enabled",
          device_id: "dev-vto-lock-1",
          unique_id: "front_vto_lock_00_sensor_enabled",
        },
        {
          entity_id: "sensor.front_door_lock_mode",
          device_id: "dev-vto-lock-1",
          unique_id: "front_vto_lock_00_lock_mode",
        },
        {
          entity_id: "sensor.front_lock_model_actual",
          device_id: "dev-vto-lock-1",
          unique_id: "front_vto_lock_00_model",
        },
        {
          entity_id: "sensor.vestibule_alarm_sense_method",
          device_id: "dev-vto-alarm-1",
          unique_id: "front_vto_alarm_00_sense_method",
        },
        {
          entity_id: "binary_sensor.vestibule_alarm_enabled",
          device_id: "dev-vto-alarm-1",
          unique_id: "front_vto_alarm_00_enabled",
        },
        {
          entity_id: "binary_sensor.vestibule_alarm_active",
          device_id: "dev-vto-alarm-1",
          unique_id: "front_vto_alarm_00_active",
        },
        {
          entity_id: "sensor.vestibule_alarm_model_actual",
          device_id: "dev-vto-alarm-1",
          unique_id: "front_vto_alarm_00_model",
        },
      ],
      [
        {
          id: "dev-channel-1",
          area_id: "area-entrance",
          via_device_id: "dev-nvr",
          name: "CH 01 Entrance Gate",
        },
        { id: "dev-nvr", name: "West20 NVR" },
        { id: "dev-disk-1", via_device_id: "dev-nvr", name: "Drive A" },
        { id: "dev-ipc", area_id: "area-driveway", name: "Driveway IPC" },
        { id: "dev-vto", area_id: "area-entry", name: "Front VTO" },
        { id: "dev-vto-lock-1", via_device_id: "dev-vto", name: "Front Door Lock" },
        { id: "dev-vto-alarm-1", via_device_id: "dev-vto", name: "Vestibule Alarm" },
      ],
      [
        { area_id: "area-entrance", name: "Entrance" },
        { area_id: "area-driveway", name: "Driveway" },
        { area_id: "area-entry", name: "Entry" },
      ],
    );

    const topology = discoverBridgeTopology(hass, registrySnapshot);

    expect(topology.nvrs).toHaveLength(1);
    expect(topology.nvrs[0]?.label).toBe("West20 NVR");
    expect(topology.nvrs[0]?.roomLabel).toBe("Entrance");
    expect(topology.nvrs[0]?.metadata.model).toBe("DHI-NVR5432");
    expect(topology.nvrs[0]?.diagnostics).toMatchObject({
      channelCount: 16,
      diskCount: 2,
    });

    expect(topology.nvrs[0]?.channels[0]).toMatchObject({
      deviceId: "west20_nvr_channel_01",
      online: true,
      roomLabel: "Entrance",
      metadata: {
        manufacturer: "Dahua",
        model: "IPC-HFW5442",
        serial: "CH01SERIAL",
        firmware: "V2.800",
        buildDate: null,
      },
    });
    expect(topology.nvrs[0]?.channels[0]?.diagnostics).toMatchObject({
      catalog: {
        recommendedHaIntegration: "native",
        recommendedHaReason: "Bridge HLS preferred",
      },
      onvif: {
        h264Available: true,
        profileName: "Profile_1",
        profileToken: "token-1",
      },
    });
    expect(topology.nvrs[0]?.channels[0]?.capabilities.audio).toMatchObject({
      supported: true,
      mute: true,
      volume: true,
      playback: {
        supported: true,
        siren: true,
        quickReply: true,
        formats: ["wav", "pcm"],
        fileCount: 3,
      },
    });
    expect(topology.nvrs[0]?.channels[0]?.capabilities.validationNotes).toEqual([
      "ptz_profile_validated",
    ]);
    expect(topology.nvrs[0]?.channels[0]?.capabilities.aux?.targets).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "light",
          label: "White Light",
          outputKey: "light",
          toggleSupported: true,
        }),
        expect.objectContaining({
          key: "warning_light",
          label: "Warning Light",
          outputKey: "warning_light",
          toggleSupported: true,
        }),
        expect.objectContaining({
          key: "siren",
          label: "Siren",
          outputKey: "siren",
          toggleSupported: true,
        }),
      ]),
    );

    expect(topology.nvrs[0]?.drives[0]).toMatchObject({
      deviceId: "west20_nvr_disk_00",
      label: "Drive A",
      totalBytes: 2000000,
      usedBytes: 500000,
      usedPercent: 25,
      metadata: {
        manufacturer: null,
        model: "Seagate IronWolf",
        serial: null,
        firmware: null,
        buildDate: null,
      },
    });

    expect(topology.vtos[0]).toMatchObject({
      deviceId: "front_vto",
      roomLabel: "Entry",
      callState: "ringing",
      muted: true,
      autoRecordEnabled: true,
      answerButtonEntityId: "button.answer_front_station",
      hangupButtonEntityId: "button.hangup_front_station",
      outputVolumeEntityId: "number.front_station_output_level",
      inputVolumeEntityId: "number.front_station_input_level",
      mutedEntityId: "switch.front_station_muted_toggle",
      autoRecordEntityId: "switch.front_station_auto_record_toggle",
    });
    expect(topology.vtos[0]?.locks[0]).toMatchObject({
      deviceId: "front_vto_lock_00",
      stateText: "closed",
      sensorEnabled: true,
      lockMode: "normal",
      unlockButtonEntityId: "button.front_station_unlock",
      metadata: {
        manufacturer: null,
        model: "Relay Lock",
        serial: null,
        firmware: null,
        buildDate: null,
      },
    });
    expect(topology.vtos[0]?.alarms[0]).toMatchObject({
      deviceId: "front_vto_alarm_00",
      enabled: true,
      active: false,
      senseMethod: "NO",
      metadata: {
        manufacturer: null,
        model: "Alarm Contact",
        serial: null,
        firmware: null,
        buildDate: null,
      },
    });
  });

  it("builds typed archive and playback request models", () => {
    expect(
      createArchiveSearchRequest(
        2,
        "2026-04-28T00:00:00Z",
        "2026-04-28T01:00:00Z",
      ),
    ).toEqual({
      channel: 2,
      startTime: "2026-04-28T00:00:00Z",
      endTime: "2026-04-28T01:00:00Z",
      limit: 100,
    });

    expect(
      createPlaybackSessionRequest(
        2,
        "2026-04-28T00:00:00Z",
        "2026-04-28T01:00:00Z",
        "2026-04-28T00:20:00Z",
      ),
    ).toEqual({
      channel: 2,
      startTime: "2026-04-28T00:00:00Z",
      endTime: "2026-04-28T01:00:00Z",
      seekTime: "2026-04-28T00:20:00Z",
    });

    expect(createPlaybackSeekRequest("2026-04-28T00:45:00Z")).toEqual({
      seekTime: "2026-04-28T00:45:00Z",
    });
  });
});
