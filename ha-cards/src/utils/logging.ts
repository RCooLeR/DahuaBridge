export function logCardInfo(message: string, details?: Record<string, unknown>): void {
  if (typeof console === "undefined" || typeof console.log !== "function") {
    return;
  }
  console.log("[DahuaBridge]", message, details ?? {});
}

export function logCardWarn(message: string, details?: Record<string, unknown>): void {
  if (typeof console === "undefined" || typeof console.warn !== "function") {
    return;
  }
  console.warn("[DahuaBridge]", message, details ?? {});
}

export function redactUrlForLog(targetUrl: string): string {
  try {
    const parsed = new URL(targetUrl, window.location.origin);
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
