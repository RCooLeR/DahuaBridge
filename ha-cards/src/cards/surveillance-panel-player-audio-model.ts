export const DEFAULT_STREAM_VOLUME = 1;

export function clampStreamVolume(value: number): number {
  if (!Number.isFinite(value)) {
    return DEFAULT_STREAM_VOLUME;
  }
  return Math.min(1, Math.max(0, value));
}

export function streamVolumePercent(volume: number): number {
  return Math.round(clampStreamVolume(volume) * 100);
}

export function streamVolumeFromInputValue(value: string): number {
  const parsed = Number.parseFloat(value);
  if (!Number.isFinite(parsed)) {
    return DEFAULT_STREAM_VOLUME;
  }
  return clampStreamVolume(parsed / 100);
}

export function streamVolumeIcon(muted: boolean, volume: number): string {
  const normalizedVolume = clampStreamVolume(volume);
  if (muted || normalizedVolume <= 0) {
    return "mdi:volume-off";
  }
  if (normalizedVolume < 0.5) {
    return "mdi:volume-medium";
  }
  return "mdi:volume-high";
}
