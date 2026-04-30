export function cameraEntityId(deviceId: string): string {
  return `camera.${deviceId}_camera`;
}

export function binarySensorEntityId(deviceId: string, key: string): string {
  return `binary_sensor.${deviceId}_${key}`;
}

export function sensorEntityId(deviceId: string, key: string): string {
  return `sensor.${deviceId}_${key}`;
}

export function buttonEntityId(deviceId: string, key: string): string {
  return `button.${deviceId}_${key}`;
}

export function switchEntityId(deviceId: string, key: string): string {
  return `switch.${deviceId}_${key}`;
}

export function numberEntityId(deviceId: string, key: string): string {
  return `number.${deviceId}_${key}`;
}

export function parseChannelNumber(deviceId: string): number | null {
  const match = deviceId.match(/_channel_(\d+)$/);
  if (!match) {
    return null;
  }

  const parsed = Number.parseInt(match[1], 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

export function normalizeBridgeOrigin(urlValue: unknown): string | null {
  if (typeof urlValue !== "string" || !urlValue.trim()) {
    return null;
  }

  try {
    const parsed = new URL(urlValue);
    return parsed.origin;
  } catch {
    return null;
  }
}
