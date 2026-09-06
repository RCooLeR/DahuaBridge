import { rewriteBridgeUrl } from "./bridge-url";

/** Resolve API response links using the request that authorized the response. */
export function authenticatedBridgeResourceUrl(
  target: string | null | undefined,
  requestUrl: string,
  browserBridgeUrl?: string | null,
): string | null {
  if (!target?.trim()) return null;
  try {
    const origin = globalThis.location?.origin ?? "http://localhost";
    const request = new URL(requestUrl, origin);
    const effectiveRequest = new URL(
      rewriteBridgeUrl(requestUrl, browserBridgeUrl) ?? requestUrl,
      origin,
    );
    const resource = new URL(target, request);
    // API responses can name the original bridge or its browser-facing origin.
    // Unrelated links must not receive either a rewritten host or this token.
    if (
      !["http:", "https:"].includes(resource.protocol) ||
      ![request.origin, effectiveRequest.origin].includes(resource.origin) ||
      !resource.pathname.includes("/api/")
    ) {
      return target;
    }
    const resolved = new URL(
      rewriteBridgeUrl(resource.toString(), browserBridgeUrl) ?? resource.toString(),
    );
    // A resource-specific credential takes precedence over inherited request auth.
    if (!resolved.searchParams.has("auth_token") && !resolved.searchParams.has("token")) {
      const key = request.searchParams.has("auth_token") ? "auth_token" : "token";
      const token = request.searchParams.get(key);
      if (token) resolved.searchParams.set(key, token);
    }
    return resolved.toString();
  } catch {
    return target;
  }
}
