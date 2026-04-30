# Dahua API Notes

This page documents the Dahua device APIs that matter for this repository.

It is based on:

- the `rroller/dahua` Home Assistant integration source:
  - <https://github.com/rroller/dahua>
  - <https://github.com/rroller/dahua/blob/main/custom_components/dahua/client.py>
  - <https://github.com/rroller/dahua/blob/main/custom_components/dahua/vto.py>
  - <https://github.com/rroller/dahua/blob/main/custom_components/dahua/rpc2.py>
- the `vov4uk/Dahua.Api` NetSDK wrapper:
  - <https://github.com/vov4uk/Dahua.Api>
- live requests against the devices currently configured in `bridge/config.yaml`
- local vendor references in `dahua-pdf/`:
  - `DAHUA_IPC_HTTP_API_V1.00x (1).pdf`
  - `pdfcoffee.com_dahua-camera-netsdk-programming-guide-pdf-free.pdf`
  - `696021881-NetSDK-Programming-Manual-Intelligent-AI.pdf`

Verification date:

- April 30, 2026

Verified hardware:

- NVR: `DHI-NVR5232-EI`, firmware `5.001.0000000.2.R`, build `2026-03-31`
- VTO: `DHI-VTO2311R-WP`, firmware `4.510.10IM001.0.R`, build `2024-01-23`
- Direct IPC: `DH-IPC-HFW2849S-S-IL`, firmware `2.880.0000000.20.R`, build `2026-03-12`
- Direct IPC: `DH-IPC-HFW2849S-S-IL-BE`, firmware `2.880.0000000.20.R`, build `2026-03-12`
- Direct IPC: `DH-T4A-PV`, firmware `2.840.0000000.12.R`, build `2025-08-19`
- Direct IPC: `DH-IPC-HFW1430DS1-SAW`, firmware `2.800.0000000.21.R`, build `2024-03-11`
- Direct IPC: `DH-H4C-GE`, firmware `2.810.9992002.0.R`, build `2025-08-01`
- NVR child device only: `IPC-S7XE-10M0WED`, firmware `3.11.0000000.1.R`, build `2026-04-09`

This is not a generic Dahua protocol specification. It is the practical API map that matches the hardware and firmware we actually run.

## 1. API Families

The Dahua surfaces relevant to this repo are:

1. CGI HTTP API with Digest auth
2. HTTP JSON-RPC API on `/RPC2` and `/RPC2_Login`
3. DHIP/TCP JSON protocol on VTO port `5000`
4. Dahua NetSDK on TCP port `37777`

`rroller/dahua` uses all three:

- CGI for most camera and NVR reads and writes
- RPC2 as an alternate JSON API client
- DHIP/TCP for VTO event and call-state handling

Our current bridge uses:

- CGI for most NVR and VTO reads
- HTTP RPC2 for NVR recording search and VTO call/control methods
- CGI long-poll event streams for both NVR and VTO

It does not currently use NetSDK.

## 2. Authentication

### 2.1 CGI

CGI endpoints use HTTP Digest authentication.

Example:

```bash
curl --digest -u '<user>:<password>' \
  'http://<device>/cgi-bin/magicBox.cgi?action=getSystemInfo'
```

### 2.2 HTTP RPC2

RPC2 login is a two-step challenge/response flow:

1. `POST /RPC2_Login` with empty password
2. device returns `realm`, `random`, and a temporary `session`
3. client computes:
   - `HA1 = MD5_UPPER(username:realm:password)`
   - `PASS = MD5_UPPER(username:random:HA1)`
4. client sends a second `global.login`
5. device returns the real session ID and `WebClientHttpSessionID` cookie

Important live detail:

- on the verified NVR, the session from the first login response is not usable for normal RPC calls
- the client must use the second login response session and keep the cookie jar

Working second-login shape:

```json
{
  "method": "global.login",
  "id": 2,
  "session": "<challenge-session>",
  "params": {
    "userName": "<user>",
    "password": "<md5-response>",
    "clientType": "Web3.0",
    "loginType": "Direct",
    "authorityType": "Default"
  }
}
```

### 2.3 DHIP/TCP on VTO

The VTO also supports a DHIP-framed JSON protocol on TCP port `5000`.

`rroller/dahua` uses this for:

- `global.login`
- `global.keepAlive`
- `eventManager.attach`
- `configManager.getConfig`
- `magicBox.getSoftwareVersion`
- `magicBox.getDeviceType`

The payload is JSON, wrapped in a binary DHIP header.

### 2.4 NetSDK

`vov4uk/Dahua.Api` is not an HTTP API reference. It is a thin C# wrapper over Dahua NetSDK.

The important verified repo facts are:

- login is SDK login to port `37777`
- recording search is SDK `CLIENT_QueryRecordFile`
- record download is SDK `CLIENT_DownloadByRecordFile`
- basic config reads are SDK `CLIENT_GetDevConfig` / `CLIENT_GetNewDevConfig`

This matters as an alternate transport, not as a source of new CGI or RPC2 endpoint shapes.

## 3. Live Verification Summary

### 3.1 NVR

Verified working:

- `magicBox.getSystemInfo`
- `magicBox.getDeviceType`
- `magicBox.getSoftwareVersion`
- `configManager.getConfig` for `RecordMode`, `MotionDetect`, `Lighting_V2`, `Encode`
- idempotent CGI `configManager.cgi?action=setConfig` writes for:
  - `RecordMode[4].Mode=0&RecordMode[4].ModeExtra1=2&RecordMode[4].ModeExtra2=2`
  - `Encode[4].MainFormat[0].AudioEnable=true`
- `snapshot.cgi?channel=N`
- `eventManager.cgi?action=attach&codes=[All]&heartbeat=5`
- CGI `mediaFileFind.cgi` archive search
- RPC2 `configManager.getConfig`
- RPC2 `mediaFileFind.factory.create/findFile/findNextFile/close/destroy`

Verified not working on this hardware/account combination:

- `configManager.cgi?action=getConfig&name=VideoInOptions[4].NightOptions.SwitchMode`
  - returned `Error: Error -1 getting param in name=VideoInOptions[4].NightOptions.SwitchMode`
- `coaxialControlIO.cgi?action=getStatus&channel=5`
  - returned HTTP `400`

Interpretation:

- the configured NVR account is currently write-capable for at least `RecordMode` and `Encode[*].MainFormat[*].AudioEnable`
- camera-centric APIs used by `rroller/dahua` are not automatically valid on NVR child channels
- `Lighting_V2` exists and is readable on the NVR, but camera-centric keys like `VideoInOptions[*].NightOptions.SwitchMode` are still not exposed there

### 3.2 VTO

Verified working:

- CGI `magicBox.getSystemInfo`
- CGI `magicBox.getDeviceType`
- CGI `magicBox.getSoftwareVersion`
- CGI `configManager.getConfig` for:
  - `AccessControl`
  - `T2UServer`
  - `CommGlobal`
  - `Alarm`
  - `Encode`
  - `AudioInputVolume`
  - `AudioOutputVolume`
  - `Sound`
  - `VideoTalkPhoneGeneral`
  - `VideoTalkPhoneBasic`
  - `RecordStoragePoint`
- CGI `eventManager.cgi?action=attach&codes=[All]&heartbeat=5`
- HTTP RPC2:
  - `global.login`
  - `VideoTalkPhone.factory.instance`
  - `VideoTalkPhone.getCallState`
- DHIP/TCP:
  - `global.login`
  - `magicBox.getSoftwareVersion`
  - `magicBox.getDeviceType`
  - `configManager.getConfig`
  - `eventManager.attach`

Verified behavior details:

- `VideoTalkPhone.getCallState` returned `Idle`
- `VideoTalkPhone.answer`, `disconnect`, and `endCall` returned service errors while idle
  - that is expected enough to prove the methods exist, but it is not a full call-flow validation

### 3.3 Direct IPC

Verified working on the configured direct-camera entries:

- CGI `magicBox.getSystemInfo`
- CGI `magicBox.getDeviceType`
- CGI `magicBox.getSoftwareVersion`
- CGI `configManager.getConfig` for:
  - `VideoInOptions`
  - `Lighting_V2`
  - `Encode`
  - `MotionDetect`
  - `ChannelTitle`
  - `AudioInputVolume`
  - `AudioOutputVolume`
  - `RecordStoragePoint`
- CGI `snapshot.cgi`
- CGI `eventManager.cgi?action=attach&codes=[All]&heartbeat=5`
- CGI `devVideoInput.cgi?action=getCaps&channel=1`

Verified direct-camera writes:

- `VideoInOptions[0].NightOptions.SwitchMode=<current-value>` returned `OK`
- `Encode[0].MainFormat[0].AudioEnable=<current-value>` returned `OK`
- `ChannelTitle[0].Name=<current-value>` returned `OK`
- `Lighting_V2[0][0][0].Mode=<current-value>` returned `OK` on:
  - `DH-IPC-HFW2849S-S-IL`
  - `DH-IPC-HFW2849S-S-IL-BE`
  - `DH-T4A-PV`
  - `DH-H4C-GE`

Verified behavior details:

- direct camera `setConfig` keys must omit the `table.` prefix shown in `getConfig` output
- `devVideoInput.cgi?action=getCaps` worked with `channel=1`, not `channel=0`
- `coaxialControlIO.cgi?action=getStatus&channel=1` returned HTTP `400` on all verified direct IPCs
- `DH-IPC-HFW1430DS1-SAW` exposed readable `Lighting_V2[...]` config but returned HTTP `400` for `Lighting_V2[0][0][0].Mode=Auto`
- `DH-H4C-GE` redirected HTTP to HTTPS, and its HTTPS CGI surface worked with `curl` even though Python `requests` failed the TLS handshake in this environment
- `IPC-S7XE-10M0WED` on `192.168.150.90` was visible through NVR inventory for channels `5` and `6`, but direct HTTP from the bridge host timed out and direct HTTPS on `443` was unreachable during live verification

Verified HTTP RPC2 on direct IPC:

- `global.login` with `clientType=Web3.0` worked on `DH-T4A-PV` and `DH-IPC-HFW1430DS1-SAW`
- `configManager.getConfig` for `VideoInOptions` worked on those two models

## 4. CGI API

All CGI examples below use Digest auth.

### 4.1 `magicBox.cgi`

Purpose:

- system identity
- firmware
- machine name
- vendor

Common calls:

```text
/cgi-bin/magicBox.cgi?action=getSystemInfo
/cgi-bin/magicBox.cgi?action=getDeviceType
/cgi-bin/magicBox.cgi?action=getSoftwareVersion
/cgi-bin/magicBox.cgi?action=getMachineName
/cgi-bin/magicBox.cgi?action=getVendor
```

Live NVR sample:

```text
deviceType=31
processor=ST7108
serialNumber=BD04CE2PAJ3B9BF
updateSerial=DH-NVR5232-4KS3/I
```

Live VTO sample:

```text
deviceType=DHI-VTO2311R-WP
processor=SSC338BQ
serialNumber=AD0578CPAJ36130
```

### 4.2 `configManager.cgi?action=getConfig`

Purpose:

- structured config reads
- capability detection
- current control state

Pattern:

```text
/cgi-bin/configManager.cgi?action=getConfig&name=<ConfigName>
```

Important verified NVR config names:

- `RecordMode`
- `MotionDetect`
- `Lighting_V2`
- `Encode`

Important verified VTO config names:

- `AccessControl`
- `CommGlobal`
- `Alarm`
- `Encode`
- `AudioInputVolume`
- `AudioOutputVolume`
- `Sound`
- `VideoTalkPhoneGeneral`
- `VideoTalkPhoneBasic`
- `RecordStoragePoint`
- `T2UServer`

Important verified direct IPC config names:

- `VideoInOptions`
- `Lighting_V2`
- `Encode`
- `MotionDetect`
- `ChannelTitle`
- `AudioInputVolume`
- `AudioOutputVolume`
- `RecordStoragePoint`

Live NVR `RecordMode` sample:

```text
table.RecordMode[0].Mode=0
table.RecordMode[0].ModeExtra1=2
table.RecordMode[0].ModeExtra2=2
```

Live VTO `VideoTalkPhoneGeneral` sample:

```text
table.VideoTalkPhoneGeneral.AutoRecordEnable=true
table.VideoTalkPhoneGeneral.AutoRecordTime=11
```

### 4.3 `configManager.cgi?action=setConfig`

Purpose:

- persistent config writes

Pattern:

```text
/cgi-bin/configManager.cgi?action=setConfig&Some.Key=value
```

Important syntax detail:

- `getConfig` returns keys prefixed with `table.`
- `setConfig` expects the key without that prefix

Example:

```text
GET /cgi-bin/configManager.cgi?action=getConfig&name=Encode
...
table.Encode[0].MainFormat[0].AudioEnable=true
```

```text
GET /cgi-bin/configManager.cgi?action=setConfig&Encode[0].MainFormat[0].AudioEnable=true
OK
```

Important reality on the current NVR:

- the configured `assistant` account returned `OK` for idempotent `RecordMode[...]` and `Encode[...].AudioEnable` writes on April 30, 2026
- this proves the account is not globally read-only anymore
- it does not prove that every NVR config family is writable

Implication for this repo:

- NVR channel controls that depend on `setConfig` should still be gated per operation, not by a blanket read-only assumption

Implication for direct IPC:

- direct camera writes are viable, but the exact writable key set is model-specific

### 4.4 `eventManager.cgi?action=attach`

Purpose:

- long-lived event stream

Pattern:

```text
/cgi-bin/eventManager.cgi?action=attach&codes=[All]&heartbeat=5
```

Response format:

- multipart-ish boundary stream using `--myboundary`
- heartbeats are plain `Heartbeat`
- events are lines like `Code=...;action=...;index=...`

Live NVR sample:

```text
--myboundary
Content-Type: text/plain
Content-Length: 9

Heartbeat
```

Live VTO sample:

```text
Code=SIPRegis;action=Pulse;index=0
```

Practical note:

- the VTO does not require DHIP for events on this firmware because CGI `eventManager` also works

### 4.5 `snapshot.cgi`

Purpose:

- JPEG snapshot capture

NVR pattern:

```text
/cgi-bin/snapshot.cgi?channel=<1-based-channel>
```

VTO pattern:

```text
/cgi-bin/snapshot.cgi
```

Live NVR result:

- `Content-Type: image/jpeg`
- channel `5` returned a valid JPEG

### 4.6 `mediaFileFind.cgi`

Purpose:

- CGI archive search
- useful as fallback when RPC2 is unavailable

Verified sequence:

1. `factory.create`
2. `findFile`
3. `findNextFile`
4. `close`

Example:

```text
/cgi-bin/mediaFileFind.cgi?action=factory.create
/cgi-bin/mediaFileFind.cgi?action=findFile&object=<handle>&condition.Channel=5&condition.StartTime=2026-04-29 00:00:00&condition.EndTime=2026-04-30 23:59:59
/cgi-bin/mediaFileFind.cgi?action=findNextFile&object=<handle>&count=3
/cgi-bin/mediaFileFind.cgi?action=close&object=<handle>
```

Live result:

- `factory.create` returned a numeric handle
- `findFile` returned `OK`
- `findNextFile` returned item lines for actual `.dav` recordings on the NVR

### 4.7 `devVideoInput.cgi?action=getCaps`

Purpose:

- direct-camera capability discovery for video-input and day/night features

Verified direct IPC pattern:

```text
/cgi-bin/devVideoInput.cgi?action=getCaps&channel=1
```

Verified behavior:

- `channel=1` returned structured caps
- `channel=0` returned HTTP `400`

Useful direct-camera fields observed live:

- `caps.DayNightColor=true`
- `caps.NightOptions=true`
- `caps.Defog=true` or `false` depending on model
- `caps.ExposureMode=...`
- `caps.BrightnessCompensation=true`

### 4.8 Camera-Centric CGI Calls From `rroller/dahua`

These calls exist in `rroller/dahua`, but they split into two different realities for our hardware.

On verified direct IPCs, these camera-side keys are real:

- `VideoInOptions[0].NightOptions.SwitchMode`
- `Encode[0].MainFormat[0].AudioEnable`
- `ChannelTitle[0].Name`
- some `Lighting_V2[...]` keys, depending on model

On NVR child channels, they are not automatically valid:

- `coaxialControlIO.cgi?action=getStatus&channel=1`
- `coaxialControlIO.cgi?action=control...`
- `configManager.cgi?action=setConfig&VideoInOptions[ch].NightOptions.SwitchMode=...`
- `configManager.cgi?action=setConfig&Lighting_V2[ch][pm][0].Mode=...`
- `configManager.cgi?action=setConfig&RecordMode[ch].Mode=...`

Reality on the current NVR:

- `coaxialControlIO` for channel `5` returned HTTP `400`
- `VideoInOptions[4].NightOptions.SwitchMode` was not exposed
- idempotent `RecordMode[...]` and `Encode[...].AudioEnable` writes returned `OK`

Conclusion:

- `rroller/dahua` camera control paths are useful references for direct IPC implementation
- they still cannot be copied onto NVR child channels without per-device verification

## 5. HTTP RPC2 API

### 5.1 Login

Endpoints:

- `POST /RPC2_Login`
- `POST /RPC2`

Verified client type:

- `Web3.0` worked on both the NVR and VTO

`Dahua3.0-Web3.0` also worked on the NVR in ad hoc testing, but our bridge already uses `Web3.0` successfully for VTO and NVR RPC2.

### 5.2 NVR Recording Search via RPC2

This is the most important verified NVR RPC2 flow.

Verified sequence:

1. `mediaFileFind.factory.create`
2. `mediaFileFind.findFile`
3. `mediaFileFind.findNextFile`
4. `mediaFileFind.close`
5. `mediaFileFind.destroy`

Important live details:

- `mediaFileFind.factory.create` returned the object handle in `result`, not in `params.object`
- `findNextFile` returned results under `params.infos`, not `params.items`
- `Channel` is zero-based in the RPC2 search condition

Working example payloads:

```json
{
  "method": "mediaFileFind.factory.create",
  "id": 3,
  "session": "<session>"
}
```

```json
{
  "method": "mediaFileFind.findFile",
  "id": 4,
  "session": "<session>",
  "object": 2499970352,
  "params": {
    "condition": {
      "StartTime": "2026-4-29 00:00:00",
      "EndTime": "2026-4-30 23:59:59",
      "Events": ["*"],
      "Flags": null,
      "Types": ["dav"],
      "Channel": 4,
      "VideoStream": "Main"
    }
  }
}
```

Live result sample:

```json
{
  "found": 3,
  "infos": [
    {
      "Channel": 4,
      "StartTime": "2026-04-29 00:00:00",
      "EndTime": "2026-04-29 00:30:02",
      "Type": "dav",
      "VideoStream": "Main",
      "FilePath": "/mnt/dvr/2026-04-29/4/dav/00/1/0/134484/00.00.00-00.30.02[R][0@0][0].dav"
    }
  ]
}
```

### 5.3 NVR `configManager.getConfig`

Verified call:

```json
{
  "method": "configManager.getConfig",
  "id": 3,
  "session": "<session>",
  "params": {
    "name": "RecordMode"
  }
}
```

Live result:

- returned `params.table` with one object per channel

### 5.4 `system.multicall`

The current NVR accepts the outer `system.multicall` request shape:

```json
{
  "method": "system.multicall",
  "params": [
    {"method": "...", "params": {...}},
    {"method": "...", "params": {...}}
  ]
}
```

But on the verified NVR:

- read-only `configManager.getConfig` calls inside `system.multicall` returned inner `result=false`
- this means multicall cannot be assumed to behave like plain single-call RPC2 for all methods

Treat `system.multicall` as a method-specific surface that must be validated per operation.

### 5.5 VTO HTTP RPC2

Verified methods on the VTO:

- `VideoTalkPhone.factory.instance`
- `VideoTalkPhone.getCallState`

Live sample:

```json
{
  "method": "VideoTalkPhone.getCallState",
  "object": 20737192
}
```

Live result:

```json
{
  "params": {
    "callState": "Idle"
  },
  "result": true
}
```

Methods that exist but returned service errors while idle:

- `VideoTalkPhone.answer`
- `VideoTalkPhone.disconnect`
- `VideoTalkPhone.endCall`

That is still enough to prove the surface exists on this VTO firmware.

### 5.6 Direct IPC HTTP RPC2

Verified on `DH-T4A-PV` and `DH-IPC-HFW1430DS1-SAW`:

- two-step `global.login`
- `configManager.getConfig`

Verified call:

```json
{
  "method": "configManager.getConfig",
  "id": 3,
  "session": "<session>",
  "params": {
    "name": "VideoInOptions"
  }
}
```

Practical note:

- direct IPC HTTP RPC2 exists, but CGI already covers the current direct-camera control needs
- the `DH-H4C-GE` HTTPS RPC2 path was not validated with the current Python client because of a TLS-handshake mismatch in this environment

## 6. VTO DHIP/TCP API

`rroller/dahua` uses the DHIP/TCP path for VTO event and detail handling.

Verified working methods on the current VTO:

- `global.login`
- `magicBox.getSoftwareVersion`
- `magicBox.getDeviceType`
- `configManager.getConfig`
- `eventManager.attach`

Verified live result:

```json
{
  "method": "eventManager.attach",
  "params": {
    "codes": ["All"]
  }
}
```

Response:

```json
{
  "id": 8,
  "params": {
    "SID": 513
  },
  "result": true
}
```

Practical conclusion:

- DHIP works
- CGI event attach also works
- we do not need to migrate VTO event streaming to DHIP unless CGI event behavior becomes insufficient

## 7. NetSDK Notes From `vov4uk/Dahua.Api` And Vendor PDFs

These sources do not change the verified HTTP paths above, but they do confirm the SDK-side shape if we ever need a native fallback.

`vov4uk/Dahua.Api` uses:

- `CLIENT_Login(host, 37777, username, password)`
- `CLIENT_GetNewDevConfig(..., CFG_CMD_RTSP, ...)`
- `CLIENT_GetNewDevConfig(..., CFG_CMD_CHANNELTITLE, ...)`
- `CLIENT_GetDevConfig(..., TIMECFG, ...)`
- `CLIENT_SetDevConfig(..., TIMECFG, ...)`
- `CLIENT_QueryRecordFile(...)`
- `CLIENT_DownloadByRecordFile(...)`
- `CLIENT_GetDownloadPos(...)`
- `CLIENT_StopDownload(...)`

The local HTTP API PDF matches the direct-camera CGI keys we verified live:

- `VideoInOptions`
- `VideoInOptions[0].NightOptions.SwitchMode`
- `Encode[0].MainFormat[0].AudioEnable`
- `ChannelTitle[0].Name`
- `devVideoInput.cgi?action=getCaps&channel=1`

The local NetSDK programming guide adds two important recording details:

- SDK record search can use either:
  - `CLIENT_QueryRecordFile`
  - `CLIENT_FindFile` + `CLIENT_FindNextFile` + `CLIENT_FindClose`
- SDK recording workflows should set `DH_RECORD_STREAM_TYPE` with `CLIENT_SetDeviceMode` before record search/download

Practical conclusion:

- NetSDK is a credible fallback path for recordings and downloads
- it is not a good first migration target while CGI and RPC2 already work on the NVR

## 8. What `rroller/dahua` Gets Right For Our Hardware

- CGI `magicBox` and `configManager.getConfig` are still the core discovery surfaces
- VTO uses a richer call/event surface than simple CGI config reads
- some controls are firmware-family specific and need per-device branching

## 9. What Does Not Transfer Cleanly From `rroller/dahua`

- camera-centric `coaxialControlIO` assumptions do not hold for our NVR channels
- `VideoInOptions[*].NightOptions.SwitchMode` is not exposed on this NVR
- plain CGI `setConfig` writes do not work with our configured NVR account
- `Lighting_V2` write behavior is not uniform even across direct IPC models
- VTO event handling in `rroller/dahua` is DHIP-centric, but our bridge currently succeeds with CGI event streaming

## 10. Rules For This Repository

Use these rules when implementing bridge behavior:

1. Treat NVR recording search as:
   - primary: RPC2 `mediaFileFind`
   - fallback: CGI `mediaFileFind.cgi`
2. Treat NVR channel writes as available only for operations that have been verified on the current firmware and credential set.
3. For mapped direct IPC channels, prefer direct camera `setConfig` over NVR-side guesses.
4. Strip the `table.` prefix from `getConfig` output before building `setConfig` writes.
5. Do not assume camera APIs from `rroller/dahua` apply to NVR child channels.
6. Treat `Lighting_V2` as model-specific even on direct IPCs; on the current hardware it is writable on `DH-IPC-HFW2849S-S-IL`, `DH-IPC-HFW2849S-S-IL-BE`, `DH-T4A-PV`, and `DH-H4C-GE`, but not on `DH-IPC-HFW1430DS1-SAW`.
7. For direct IPC capability probes, use `devVideoInput.cgi?action=getCaps&channel=1`.
8. Treat VTO as two separate control families:
   - CGI config surfaces for inventory and config-backed state
   - RPC2 for call-state and call-control methods
9. Keep per-device capability validation tied to live probes and not to model-name guesses alone.
