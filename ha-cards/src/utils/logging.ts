const cardLogState = new Map<string, Map<string, Record<string, unknown>>>();

export function logCardInfo(message: string, details?: Record<string, unknown>): void {
  writeCardLog("log", message, details);
}

export function logCardWarn(message: string, details?: Record<string, unknown>): void {
  writeCardLog("warn", message, details);
}

export function setCardLogState(
  scope: string,
  key: string,
  state: Record<string, unknown>,
): void {
  const normalizedScope = scope.trim();
  const normalizedKey = key.trim();
  if (!normalizedScope || !normalizedKey) {
    return;
  }

  const scopedState = ensureScopedCardLogState(normalizedScope);
  scopedState.set(normalizedKey, { ...state });
  writeCardLog("log", "app state", {
    scope: normalizedScope,
    key: normalizedKey,
    action: "set",
    app_state: snapshotCardLogState(),
  });
}

export function clearCardLogState(scope: string, key: string): void {
  const normalizedScope = scope.trim();
  const normalizedKey = key.trim();
  if (!normalizedScope || !normalizedKey) {
    return;
  }

  const scopedState = cardLogState.get(normalizedScope);
  if (!scopedState?.delete(normalizedKey)) {
    return;
  }
  if (scopedState.size === 0) {
    cardLogState.delete(normalizedScope);
  }

  writeCardLog("log", "app state", {
    scope: normalizedScope,
    key: normalizedKey,
    action: "clear",
    app_state: snapshotCardLogState(),
  });
}

export function redactUrlForLog(targetUrl: string): string {
  try {
    const origin =
      typeof window !== "undefined" && window.location?.origin
        ? window.location.origin
        : "http://localhost";
    const parsed = new URL(targetUrl, origin);
    for (const key of [...parsed.searchParams.keys()]) {
      if (shouldRedactUrlParam(key)) {
        parsed.searchParams.set(key, "[redacted]");
      }
    }
    return parsed.toString();
  } catch {
    return targetUrl;
  }
}

function shouldRedactUrlParam(key: string): boolean {
  const normalized = key.trim().toLowerCase();
  return (
    normalized.includes("password") ||
    normalized.includes("passwd") ||
    normalized.includes("pwd") ||
    normalized.includes("token") ||
    normalized.includes("secret")
  );
}

function writeCardLog(
  method: "log" | "warn",
  message: string,
  details?: Record<string, unknown>,
): void {
  if (typeof console === "undefined" || typeof console[method] !== "function") {
    return;
  }
  console[method]("[DahuaBridge]", {
    event: message,
    details: details ?? {},
  });
}

function ensureScopedCardLogState(
  scope: string,
): Map<string, Record<string, unknown>> {
  let scopedState = cardLogState.get(scope);
  if (!scopedState) {
    scopedState = new Map<string, Record<string, unknown>>();
    cardLogState.set(scope, scopedState);
  }
  return scopedState;
}

function snapshotCardLogState(): Record<string, Record<string, Record<string, unknown>>> {
  const snapshot: Record<string, Record<string, Record<string, unknown>>> = {};
  const scopes = [...cardLogState.keys()].sort((left, right) =>
    left.localeCompare(right),
  );
  for (const scope of scopes) {
    const scopedState = cardLogState.get(scope);
    if (!scopedState || scopedState.size === 0) {
      continue;
    }
    snapshot[scope] = {};
    const keys = [...scopedState.keys()].sort((left, right) =>
      left.localeCompare(right),
    );
    for (const key of keys) {
      snapshot[scope][key] = { ...scopedState.get(key) };
    }
  }
  return snapshot;
}
