# Install Guide

This is the simplest recommended setup:

1. Run the Go bridge from `bridge/`.
2. Install the Home Assistant custom integration from `integration/custom_components/dahuabridge`.
3. Add the integration in Home Assistant and point it at the bridge.

## Before You Start

You need:

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
   - your Dahua device `id`, `base_url`, `username`, and `password`
   - `home_assistant.public_base_url`

Example:

```yaml
home_assistant:
  public_base_url: http://192.168.1.50:9205
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

If you want Intel QSV acceleration:

1. Pass `/dev/dri` into the container.
2. Give the container access to the `render` / `video` groups if needed.
3. Keep `media.hwaccel_args` enabled only after you confirm it actually works.
4. If QSV fails, set:

```yaml
media:
  hwaccel_args: []
```

### Option C: Baked Config Image

Use this if you do not want config/data volumes at all.

1. In `bridge/`, copy:

```text
config.example.yaml -> config.image.yaml
```

2. Edit `config.image.yaml` with your real settings.
3. Build:

```bash
docker build -t dahuabridge-baked -f bridge/Dockerfile.baked ./bridge
```

4. Run:

```bash
docker run -d --name dahuabridge -p 9205:9205 dahuabridge-baked
```

5. If you prefer compose, start from `bridge/compose.baked.example.yaml`.

Important:

- `config.image.yaml` becomes part of the image.
- Do not publish that image to a public registry if it contains real secrets.
- Bridge state will live in the container writable layer unless your config changes the path.

## Step 3: Check The Bridge

Open these URLs:

1. `http://YOUR_BRIDGE_HOST:9205/healthz`
2. `http://YOUR_BRIDGE_HOST:9205/api/v1/status`
3. `http://YOUR_BRIDGE_HOST:9205/admin`

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
http://192.168.1.50:9205
```

8. Finish setup.

## Step 5: Check Home Assistant

What you want to see:

1. The `DahuaBridge` integration loads without errors.
2. Devices appear from the bridge.
3. A streamable thing like `nvr_channel_01` appears as one Home Assistant device.
4. That one device contains the camera plus the related sensors and buttons.

## Step 6: Native-Only Defaults

The bridge example config now defaults to the native integration path:

```yaml
mqtt:
  enabled: false

home_assistant:
  entity_mode: native
```

If you are reusing an older config, keep `mqtt.enabled: false` and `home_assistant.entity_mode: native`.

## Step 7: Optional Things

After the main setup works, you can also use:

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
