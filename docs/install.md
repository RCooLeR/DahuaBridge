# Install Guide

This is the simplest recommended setup:

1. Run the Go bridge from `bridge/`.
2. Install the Home Assistant custom integration from `integration/custom_components/dahuabridge`.
3. Ignore `ha-cards/` for now.

## Before You Start

You need:

- a running MQTT broker
- network access from the bridge host to your Dahua devices
- Docker or Go on the machine where the bridge will run
- a Home Assistant instance that can reach the bridge over HTTP
- `ffmpeg` available if you want bridge-hosted media features

## Step 1: Prepare The Bridge Config

1. Open the `bridge/` directory.
2. Copy `config.example.yaml` to `config.yaml`.

Windows:

```powershell
copy config.example.yaml config.yaml
```

Linux/macOS:

```bash
cp config.example.yaml config.yaml
```

3. Edit `config.yaml`.
4. Set:
   - `mqtt.broker`
   - your Dahua device `id`, `base_url`, `username`, and `password`
   - `home_assistant.public_base_url`

Example:

```yaml
home_assistant:
  public_base_url: http://192.168.1.50:8080
```

Use a URL that both your browser and Home Assistant can actually open.

## Step 2: Start The Bridge

### Option A: Run It Directly

1. Open a terminal in `bridge/`.
2. Start the bridge:

```bash
go run ./cmd/dahuabridge --config config.yaml
```

### Option B: Build And Run Docker

1. Open a terminal in the repo root.
2. Build the image:

```bash
docker build -t dahuabridge ./bridge
```

3. Run the container with:
   - `config.yaml` mounted to `/config/config.yaml`
   - a writable directory mounted to `/data`

If you prefer compose, start from `bridge/compose.example.yaml`.

## Step 3: Check The Bridge

Open these URLs:

1. `http://YOUR_BRIDGE_HOST:8080/healthz`
2. `http://YOUR_BRIDGE_HOST:8080/api/v1/status`
3. `http://YOUR_BRIDGE_HOST:8080/admin`

What you want:

1. `/healthz` returns `ok`
2. `/api/v1/status` returns JSON
3. `/admin` opens and shows your devices

If the admin page does not show your devices, stop here and fix the bridge first.

## Step 4: Install The Home Assistant Integration

1. Copy:

```text
integration/custom_components/dahuabridge
```

2. Into your Home Assistant config directory as:

```text
<home-assistant-config>/custom_components/dahuabridge
```

3. Restart Home Assistant.
4. Open `Settings -> Devices & Services`.
5. Click `Add Integration`.
6. Search for `DahuaBridge`.
7. Enter the bridge URL, for example:

```text
http://192.168.1.50:8080
```

8. Finish setup.

## Step 5: Check Home Assistant

What you want to see:

1. The `DahuaBridge` integration loads without errors.
2. Devices appear from the bridge.
3. A streamable thing like `nvr_channel_01` appears as one Home Assistant device.
4. That one device contains the camera plus the related sensors and buttons.

## Step 6: Clean Up Duplicate Legacy Paths

If you want the cleanest Home Assistant device list:

1. Set this in the bridge config:

```yaml
home_assistant:
  entity_mode: native
```

2. Restart the bridge.
3. Call:

```text
POST /api/v1/home-assistant/mqtt/discovery/remove
```

4. Remove old ONVIF config entries or generic camera package entries if they duplicate the same devices.

See [migration.md](migration.md) for the exact cleanup flow.

## Step 7: Optional Things

After the main setup works, you can also use:

- bridge MQTT auto-discovery
- bridge-generated Home Assistant helper packages
- ONVIF provisioning helpers
- browser preview and media pages from the bridge

These are optional. Do not start with them if you are just trying to get the main system running.

## If Something Is Wrong

1. Check the bridge admin page first.
2. Check that Home Assistant can reach the exact bridge URL you entered.
3. Check bridge logs.
4. In Home Assistant, open the `DahuaBridge` integration and download diagnostics.

## Current Limits

- The native Home Assistant integration is polling-based right now.
- Real runtime validation in your actual Home Assistant instance is still required.
- Full VTO talkback is not part of the basic finished path yet.
