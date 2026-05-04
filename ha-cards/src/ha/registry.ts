import type { HomeAssistant } from "../types/home-assistant";

export interface EntityRegistryEntry {
  entity_id: string;
  device_id?: string | null;
  area_id?: string | null;
  unique_id?: string | null;
}

export interface DeviceRegistryEntry {
  id: string;
  area_id?: string | null;
  via_device_id?: string | null;
  name?: string | null;
  name_by_user?: string | null;
}

export interface AreaRegistryEntry {
  area_id: string;
  name: string;
}

export interface RegistrySnapshot {
  entitiesByEntityId: Map<string, EntityRegistryEntry>;
  entitiesByUniqueId: Map<string, EntityRegistryEntry>;
  entitiesByDeviceId: Map<string, EntityRegistryEntry[]>;
  devicesById: Map<string, DeviceRegistryEntry>;
  areasById: Map<string, AreaRegistryEntry>;
}

interface RegistrySnapshotCacheEntry {
  snapshot: RegistrySnapshot | null;
  fetchedAt: number;
  inFlight: Promise<RegistrySnapshot | null> | null;
}

type RegistryCall = <T>(message: { type: string; [key: string]: unknown }) => Promise<T>;

const REGISTRY_SNAPSHOT_CACHE_MS = 5 * 60_000;
const registrySnapshotCache = new WeakMap<object, RegistrySnapshotCacheEntry>();

export async function fetchRegistrySnapshot(
  hass: HomeAssistant,
): Promise<RegistrySnapshot | null> {
  const call = registryCallForHass(hass);
  if (!call) {
    return null;
  }

  const cacheKey = registryCacheKeyForHass(hass);
  const now = Date.now();
  const cached = registrySnapshotCache.get(cacheKey);
  if (cached?.inFlight) {
    return cached.inFlight;
  }
  if (cached && now - cached.fetchedAt < REGISTRY_SNAPSHOT_CACHE_MS) {
    return cached.snapshot;
  }

  const inFlight = loadRegistrySnapshot(call);
  registrySnapshotCache.set(cacheKey, {
    snapshot: cached?.snapshot ?? null,
    fetchedAt: cached?.fetchedAt ?? 0,
    inFlight,
  });

  try {
    const snapshot = await inFlight;
    registrySnapshotCache.set(cacheKey, {
      snapshot,
      fetchedAt: Date.now(),
      inFlight: null,
    });
    return snapshot;
  } catch (error) {
    if (cached) {
      registrySnapshotCache.set(cacheKey, {
        snapshot: cached.snapshot,
        fetchedAt: cached.fetchedAt,
        inFlight: null,
      });
    } else {
      registrySnapshotCache.delete(cacheKey);
    }
    throw error;
  }
}

function registryCallForHass(hass: HomeAssistant): RegistryCall | null {
  return (
    hass.callWS?.bind(hass) ??
    hass.connection?.sendMessagePromise?.bind(hass.connection) ??
    null
  );
}

function registryCacheKeyForHass(hass: HomeAssistant): object {
  return hass.connection ?? hass;
}

async function loadRegistrySnapshot(
  call: RegistryCall,
): Promise<RegistrySnapshot | null> {
  const [entityEntries, deviceEntries, areaEntries] = await Promise.all([
    call<EntityRegistryEntry[]>({ type: "config/entity_registry/list" }),
    call<DeviceRegistryEntry[]>({ type: "config/device_registry/list" }),
    call<AreaRegistryEntry[]>({ type: "config/area_registry/list" }),
  ]);

  return {
    entitiesByEntityId: new Map(
      entityEntries.map((entry) => [entry.entity_id, entry] as const),
    ),
    entitiesByUniqueId: new Map(
      entityEntries
        .filter((entry) => typeof entry.unique_id === "string" && entry.unique_id.trim().length > 0)
        .map((entry) => [entry.unique_id as string, entry] as const),
    ),
    entitiesByDeviceId: buildEntitiesByDeviceId(entityEntries),
    devicesById: new Map(deviceEntries.map((entry) => [entry.id, entry] as const)),
    areasById: new Map(areaEntries.map((entry) => [entry.area_id, entry] as const)),
  };
}

function buildEntitiesByDeviceId(
  entityEntries: EntityRegistryEntry[],
): Map<string, EntityRegistryEntry[]> {
  const grouped = new Map<string, EntityRegistryEntry[]>();
  for (const entry of entityEntries) {
    const deviceId = entry.device_id ?? null;
    if (!deviceId) {
      continue;
    }
    const existing = grouped.get(deviceId) ?? [];
    existing.push(entry);
    grouped.set(deviceId, existing);
  }
  return grouped;
}

export function areaNameForEntity(
  snapshot: RegistrySnapshot | null | undefined,
  entityId: string,
): string | null {
  if (!snapshot) {
    return null;
  }

  const entityEntry = snapshot.entitiesByEntityId.get(entityId);
  if (!entityEntry) {
    return null;
  }

  const areaId =
    entityEntry.area_id ??
    findAreaIdForDeviceChain(snapshot, entityEntry.device_id ?? null);
  if (!areaId) {
    return null;
  }

  return snapshot.areasById.get(areaId)?.name ?? null;
}

export function areaNameForDevice(
  snapshot: RegistrySnapshot | null | undefined,
  deviceId: string | null | undefined,
): string | null {
  if (!snapshot || !deviceId) {
    return null;
  }

  const areaId = findAreaIdForDeviceChain(snapshot, deviceId);
  if (!areaId) {
    return null;
  }

  return snapshot.areasById.get(areaId)?.name ?? null;
}

function findAreaIdForDeviceChain(
  snapshot: RegistrySnapshot,
  deviceId: string | null,
): string | null {
  let currentDeviceId = deviceId;
  while (currentDeviceId) {
    const device = snapshot.devicesById.get(currentDeviceId);
    if (!device) {
      return null;
    }
    if (device.area_id) {
      return device.area_id;
    }
    currentDeviceId = device.via_device_id ?? null;
  }
  return null;
}

export function deviceNameForEntity(
  snapshot: RegistrySnapshot | null | undefined,
  entityId: string,
): string | null {
  if (!snapshot) {
    return null;
  }

  const entityEntry = snapshot.entitiesByEntityId.get(entityId);
  if (!entityEntry?.device_id) {
    return null;
  }

  const device = snapshot.devicesById.get(entityEntry.device_id);
  if (!device) {
    return null;
  }

  return device.name_by_user ?? device.name ?? null;
}

export function deviceNameForDevice(
  snapshot: RegistrySnapshot | null | undefined,
  deviceId: string | null | undefined,
): string | null {
  if (!snapshot || !deviceId) {
    return null;
  }

  const device = snapshot.devicesById.get(deviceId);
  if (!device) {
    return null;
  }

  return device.name_by_user ?? device.name ?? null;
}

export function rootDeviceName(
  snapshot: RegistrySnapshot | null | undefined,
  entityId: string,
): string | null {
  if (!snapshot) {
    return null;
  }

  const entityEntry = snapshot.entitiesByEntityId.get(entityId);
  if (!entityEntry?.device_id) {
    return null;
  }

  let currentDevice = snapshot.devicesById.get(entityEntry.device_id) ?? null;
  let lastNamed: string | null = null;
  while (currentDevice) {
    const currentName = currentDevice.name_by_user ?? currentDevice.name ?? "";
    if (currentName.trim()) {
      lastNamed = currentName.trim();
    }

    if (!currentDevice.via_device_id) {
      break;
    }
    currentDevice = snapshot.devicesById.get(currentDevice.via_device_id) ?? null;
  }

  return lastNamed;
}
