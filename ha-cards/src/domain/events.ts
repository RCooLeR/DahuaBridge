import { binarySensorEntityId, sensorEntityId } from "../ha/entity-id";
import { entityById, entityBooleanState } from "../ha/state";
import type { BridgeEvent } from "../ha/bridge-events";
import type { HomeAssistant } from "../types/home-assistant";
import { createLocalizer, type Localizer } from "../localization";

export type EventSeverity = "critical" | "warning" | "info" | "success";

export interface TimelineEventDetail {
  label: string;
  value: string;
}

export interface TimelineEvent {
  id: string;
  deviceId: string;
  rootDeviceId: string;
  title: string;
  context: string;
  roomLabel: string | null;
  deviceKind: string | null;
  icon: string;
  severity: EventSeverity;
  active: boolean;
  statusText: string;
  actionText: string;
  sourceCode?: string;
  details: TimelineEventDetail[];
  timestamp: number;
}

const CAMERA_EVENT_SPECS = [
  {
    key: "motion",
    titleKey: "event.motion",
    icon: "mdi:motion-sensor",
    severity: "warning",
  },
  {
    key: "human",
    titleKey: "event.human",
    icon: "mdi:account",
    severity: "info",
  },
  {
    key: "vehicle",
    titleKey: "event.vehicle",
    icon: "mdi:car",
    severity: "info",
  },
  {
    key: "tripwire",
    titleKey: "event.tripwire",
    icon: "mdi:vector-line",
    severity: "warning",
  },
  {
    key: "intrusion",
    titleKey: "event.intrusion",
    icon: "mdi:shield-alert",
    severity: "critical",
  },
] as const satisfies ReadonlyArray<{
  key: string;
  titleKey: Parameters<Localizer>[0];
  icon: string;
  severity: EventSeverity;
}>;

export function collectCameraEvents(
  hass: HomeAssistant,
  deviceId: string,
  label: string,
  roomLabel: string | null,
  deviceKind: string | null,
  lookbackMs: number,
  t: Localizer = createLocalizer("en"),
): TimelineEvent[] {
  const now = Date.now();

  return CAMERA_EVENT_SPECS.flatMap((spec) => {
    const entity = entityById(hass, binarySensorEntityId(deviceId, spec.key));
    if (!entity) {
      return [];
    }

    const timestamp = Date.parse(entity.last_changed);
    if (!Number.isFinite(timestamp)) {
      return [];
    }

    const active = entityBooleanState(hass, entity.entity_id);
    if (!active && now - timestamp > lookbackMs) {
      return [];
    }

    return [
      {
        id: `${deviceId}:${spec.key}:${entity.last_changed}`,
        deviceId,
        rootDeviceId: deviceId,
        title: t(spec.titleKey),
        context: label,
        roomLabel,
        deviceKind,
        icon: spec.icon,
        severity: spec.severity,
        active,
        statusText: active ? t("events.activeNow") : t("events.recentEvent"),
        actionText: active ? t("events.state") : t("events.cleared"),
        sourceCode: spec.key,
        details: [],
        timestamp,
      },
    ];
  });
}

export function collectVtoEvents(
  hass: HomeAssistant,
  deviceId: string,
  label: string,
  roomLabel: string | null,
  lookbackMs: number,
  t: Localizer = createLocalizer("en"),
): TimelineEvent[] {
  const specs = [
    {
      key: "doorbell",
      titleKey: "event.doorbell" as const,
      icon: "mdi:bell-ring",
      severity: "warning" as const,
    },
    {
      key: "call",
      titleKey: "event.call" as const,
      icon: "mdi:phone",
      severity: "info" as const,
    },
    {
      key: "access",
      titleKey: "event.access" as const,
      icon: "mdi:door-open",
      severity: "success" as const,
    },
    {
      key: "tamper",
      titleKey: "event.tamper" as const,
      icon: "mdi:shield-alert",
      severity: "critical" as const,
    },
  ];

  const events = specs.flatMap((spec) => {
    const entity = entityById(hass, binarySensorEntityId(deviceId, spec.key));
    if (!entity) {
      return [];
    }

    const timestamp = Date.parse(entity.last_changed);
    if (!Number.isFinite(timestamp)) {
      return [];
    }

    const active = entityBooleanState(hass, entity.entity_id);
    if (!active && Date.now() - timestamp > lookbackMs) {
      return [];
    }

    return [
      {
        id: `${deviceId}:${spec.key}:${entity.last_changed}`,
        deviceId,
        rootDeviceId: deviceId,
        title: t(spec.titleKey),
        context: label,
        roomLabel,
        deviceKind: "vto",
        icon: spec.icon,
        severity: spec.severity,
        active,
        statusText: active ? t("events.activeNow") : t("events.recentEvent"),
        actionText: active ? t("events.state") : t("events.cleared"),
        sourceCode: spec.key,
        details: [],
        timestamp,
      },
    ];
  });

  const callStart = entityById(
    hass,
    sensorEntityId(deviceId, "last_call_started_at"),
  )?.state;
  if (callStart) {
    const timestamp = Date.parse(callStart);
    if (Number.isFinite(timestamp) && Date.now() - timestamp <= lookbackMs) {
      events.push({
        id: `${deviceId}:call_started:${callStart}`,
        deviceId,
        rootDeviceId: deviceId,
        title: t("event.callStarted"),
        context: label,
        roomLabel,
        deviceKind: "vto",
        icon: "mdi:phone-in-talk",
        severity: "info",
        active: false,
        statusText: t("events.recentEvent"),
        actionText: t("events.start"),
        sourceCode: "call_started",
        details: [],
        timestamp,
      });
    }
  }

  return events.sort((left, right) => right.timestamp - left.timestamp);
}

export function bridgeEventsToTimeline(
  events: BridgeEvent[],
  contextByDeviceId: ReadonlyMap<string, {
    label: string;
    roomLabel: string | null;
    deviceKind: string | null;
  }>,
  lookbackMs: number,
  t: Localizer = createLocalizer("en"),
): TimelineEvent[] {
  const now = Date.now();

  return events
    .flatMap((event) => {
      const timestamp = Date.parse(event.occurred_at);
      if (!Number.isFinite(timestamp) || now - timestamp > lookbackMs) {
        return [];
      }

      const targetDeviceId = event.child_id?.trim() || event.device_id.trim();
      const contextMeta =
        contextByDeviceId.get(targetDeviceId) ??
        contextByDeviceId.get(event.device_id) ??
        null;
      const normalized = normalizeBridgeEvent(event.code, event.action, t);
      const details = bridgeEventDetails(event, t);

      return [
        {
          id: [
            event.device_id,
            event.child_id ?? "",
            event.code,
            event.action,
            event.occurred_at,
            event.index ?? "",
          ].join(":"),
          deviceId: targetDeviceId,
          rootDeviceId: event.device_id,
          title: normalized.title,
          context: contextMeta?.label ?? humanizeSnakeLikeValue(targetDeviceId),
          roomLabel: contextMeta?.roomLabel ?? null,
          deviceKind: contextMeta?.deviceKind ?? null,
          icon: normalized.icon,
          severity: normalized.severity,
          active: normalized.active,
          statusText: normalized.statusText,
          actionText: bridgeActionText(event.action, t),
          sourceCode: event.code,
          details,
          timestamp,
        },
      ];
    })
    .sort((left, right) => right.timestamp - left.timestamp);
}

export function mergeTimelineEvents(
  bridgeTimeline: TimelineEvent[],
  syntheticTimeline: TimelineEvent[],
): TimelineEvent[] {
  const grouped = new Map<string, TimelineEvent>();

  for (const event of [...bridgeTimeline, ...syntheticTimeline].sort(
    (left, right) => right.timestamp - left.timestamp,
  )) {
    const key = timelineMergeKey(event);
    const existing = grouped.get(key);
    if (!existing || timelineEventScore(event) > timelineEventScore(existing)) {
      grouped.set(key, event);
      continue;
    }
    if (timelineEventScore(event) === timelineEventScore(existing) && event.timestamp > existing.timestamp) {
      grouped.set(key, event);
    }
  }

  return [...grouped.values()].sort((left, right) => right.timestamp - left.timestamp);
}

function normalizeBridgeEvent(
  code: string,
  action: string,
  t: Localizer,
): Pick<TimelineEvent, "title" | "icon" | "severity" | "active" | "statusText"> {
  const normalizedCode = code.trim().toLowerCase();
  const active = isActiveBridgeEventAction(action);

  switch (normalizedCode) {
    case "videomotion":
      return eventMeta(
        active ? t("event.motion") : t("event.motionCleared"),
        "mdi:motion-sensor",
        "warning",
        active,
        t,
      );
    case "smartmotionhuman":
      return eventMeta(
        active ? t("event.humanDetected") : t("event.humanCleared"),
        "mdi:account",
        "info",
        active,
        t,
      );
    case "smartmotionvehicle":
      return eventMeta(
        active ? t("event.vehicleDetected") : t("event.vehicleCleared"),
        "mdi:car",
        "info",
        active,
        t,
      );
    case "crosslinedetection":
      return eventMeta(
        active ? t("event.tripwire") : t("event.tripwireCleared"),
        "mdi:vector-line",
        "warning",
        active,
        t,
      );
    case "crossregiondetection":
      return eventMeta(
        active ? t("event.intrusion") : t("event.intrusionCleared"),
        "mdi:shield-alert",
        "critical",
        active,
        t,
      );
    case "doorbell":
      return eventMeta(
        active ? t("event.doorbellPressed") : t("event.doorbellCleared"),
        "mdi:bell-ring",
        "warning",
        active,
        t,
      );
    case "call":
      return eventMeta(
        active ? t("event.callStarted") : t("event.callEnded"),
        active ? "mdi:phone-in-talk" : "mdi:phone-hangup",
        "info",
        active,
        t,
      );
    case "accessctl":
    case "accesscontrol":
      return eventMeta(
        active ? t("event.accessGranted") : t("event.accessClosed"),
        "mdi:door-open",
        "success",
        active,
        t,
      );
    case "alarmlocal":
      return eventMeta(
        active ? t("event.alarmTriggered") : t("event.alarmCleared"),
        "mdi:alarm-light",
        "critical",
        active,
        t,
      );
    case "tamper":
      return eventMeta(
        active ? t("event.tamperDetected") : t("event.tamperCleared"),
        "mdi:shield-alert",
        "critical",
        active,
        t,
      );
    default:
      return eventMeta(
        humanizeSnakeLikeValue(code),
        "mdi:information-outline",
        "info",
        active,
        t,
      );
  }
}

function eventMeta(
  title: string,
  icon: string,
  severity: EventSeverity,
  active: boolean,
  t: Localizer,
): Pick<TimelineEvent, "title" | "icon" | "severity" | "active" | "statusText"> {
  return {
    title,
    icon,
    severity,
    active,
    statusText: active ? t("events.activeNow") : t("events.recovered"),
  };
}

function bridgeActionText(action: string, t: Localizer): string {
  const normalized = action.trim().toLowerCase();
  switch (normalized) {
    case "start":
      return t("events.start");
    case "stop":
    case "clear":
    case "cleared":
      return t("events.cleared");
    case "state":
      return t("events.state");
    default:
      return humanizeSnakeLikeValue(action);
  }
}

function isActiveBridgeEventAction(action: string): boolean {
  const normalized = action.trim().toLowerCase();
  return normalized === "start" || normalized === "pulse" || normalized === "state";
}

function timelineMergeKey(event: TimelineEvent): string {
  const minuteBucket = Math.floor(event.timestamp / 60_000);
  return [
    event.rootDeviceId,
    event.deviceId,
    canonicalEventCode(event.sourceCode ?? event.title),
    event.active ? "active" : "cleared",
    String(minuteBucket),
  ].join("|");
}

function timelineEventScore(event: TimelineEvent): number {
  return (
    event.details.length * 10 +
    (event.roomLabel ? 3 : 0) +
    (event.deviceKind ? 3 : 0) +
    (event.sourceCode ? 2 : 0) +
    (event.context ? 1 : 0)
  );
}

function canonicalEventCode(value: string): string {
  const normalized = value.trim().toLowerCase();
  switch (normalized) {
    case "videomotion":
      return "motion";
    case "smartmotionhuman":
      return "human";
    case "smartmotionvehicle":
      return "vehicle";
    case "crosslinedetection":
      return "tripwire";
    case "crossregiondetection":
      return "intrusion";
    case "doorbell":
      return "doorbell";
    case "call":
    case "call_started":
      return "call";
    case "accessctl":
    case "accesscontrol":
    case "access":
      return "access";
    case "alarmlocal":
      return "alarm";
    case "tamper":
      return "tamper";
    default:
      return normalized.replace(/[^a-z0-9]+/g, "_");
  }
}

function humanizeSnakeLikeValue(value: string): string {
  return value
    .trim()
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (match) => match.toUpperCase());
}

function bridgeEventDetails(event: BridgeEvent, t: Localizer): TimelineEventDetail[] {
  const details: TimelineEventDetail[] = [];
  const data = event.data ?? {};

  pushDetail(details, t("events.source"), firstEventDataValue(data, [
    "CallSrc",
    "CallSource",
    "Source",
    "From",
    "FromNo",
    "RoomNo",
    "VillaNo",
    "UnitNo",
    "FloorNo",
  ]));
  pushDetail(details, t("events.rule"), firstEventDataValue(data, [
    "Name",
    "RuleName",
    "ProfileName",
  ]));
  pushDetail(details, t("events.region"), firstEventDataValue(data, [
    "RegionName",
    "Region",
  ]));
  pushDetail(details, t("events.object"), firstEventDataValue(data, [
    "ObjectType",
    "ObjectClass",
    "Type",
  ]));

  if (event.channel && event.channel > 0) {
    pushDetail(details, t("events.channel"), String(event.channel));
  }

  return details.slice(0, 3);
}

function pushDetail(
  details: TimelineEventDetail[],
  label: string,
  value: string | null,
): void {
  if (!value) {
    return;
  }
  if (details.some((detail) => detail.label === label && detail.value === value)) {
    return;
  }
  details.push({ label, value });
}

function firstEventDataValue(
  data: Record<string, unknown>,
  keys: string[],
): string | null {
  for (const key of keys) {
    const exact = data[key];
    if (typeof exact === "string" && exact.trim()) {
      return exact.trim();
    }

    for (const [candidateKey, candidateValue] of Object.entries(data)) {
      if (
        candidateKey.toLowerCase() === key.toLowerCase() &&
        typeof candidateValue === "string" &&
        candidateValue.trim()
      ) {
        return candidateValue.trim();
      }
    }
  }

  return null;
}
