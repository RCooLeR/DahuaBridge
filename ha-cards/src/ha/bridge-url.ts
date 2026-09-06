export function normalizeBrowserBridgeUrl(value: string | undefined): string | null {
  if (!value) {
    return null;
  }

  try {
    const parsed = new URL(value);
    if (!parsed.hostname) {
      return null;
    }
    parsed.pathname = trimTrailingSlash(parsed.pathname);
    return parsed.toString().replace(/\/$/, "");
  } catch {
    return null;
  }
}

export function rewriteBridgeUrl(
  target: string | null | undefined,
  browserBridgeUrl: string | null | undefined,
): string | null {
  const normalizedTarget = normalizeTarget(target);
  if (!normalizedTarget) {
    return null;
  }

  const normalizedBrowserBridgeUrl = normalizeBrowserBridgeUrl(
    browserBridgeUrl ?? undefined,
  );
  if (!normalizedBrowserBridgeUrl) {
    return normalizedTarget;
  }

  const browserBase = new URL(normalizedBrowserBridgeUrl);

  try {
    const parsedTarget = new URL(normalizedTarget);
    if (parsedTarget.protocol !== "http:" && parsedTarget.protocol !== "https:") {
      return normalizedTarget;
    }
    return buildRewrittenUrl(browserBase, parsedTarget.pathname, parsedTarget.search, parsedTarget.hash);
  } catch {
    const parsedTarget = new URL(normalizedTarget, "https://dahuabridge.invalid/");
    return buildRewrittenUrl(browserBase, parsedTarget.pathname, parsedTarget.search, parsedTarget.hash);
  }
}

export function isBridgeRtspRelayUrl(value: string | null | undefined): boolean {
  if (!value) {
    return false;
  }
  try {
    const url = new URL(value);
    return url.protocol === "rtsp:" && url.pathname.includes("/api/v1/rtsp/");
  } catch {
    return false;
  }
}

export function buildBridgeEndpointUrl(
  baseUrl: string | null | undefined,
  targetPath: string | null | undefined,
): string | null {
  const normalizedPath = normalizeTarget(targetPath);
  if (!normalizedPath) {
    return null;
  }
  const relativePath = normalizedPath.startsWith("/") ? normalizedPath : `/${normalizedPath}`;
  return rewriteBridgeUrl(relativePath, baseUrl);
}

function buildRewrittenUrl(
  browserBase: URL,
  targetPath: string,
  search: string,
  hash: string,
): string {
  const rewritten = new URL(browserBase.toString());
  const normalizedTargetPath =
    targetPath === "/" && !search && !hash
      ? ""
      : stripExistingBasePath(targetPath, browserBase.pathname);
  rewritten.pathname = joinUrlPaths(browserBase.pathname, normalizedTargetPath);
  rewritten.search = search;
  rewritten.hash = hash;
  return rewritten.toString();
}

function normalizeTarget(target: string | null | undefined): string | null {
  if (typeof target !== "string") {
    return null;
  }
  const trimmed = target.trim();
  return trimmed ? trimmed : null;
}

function trimTrailingSlash(pathname: string): string {
  if (!pathname || pathname === "/") {
    return "";
  }
  return pathname.replace(/\/+$/, "");
}

function joinUrlPaths(basePath: string, targetPath: string): string {
  const normalizedBase = trimTrailingSlash(basePath);
  if (!targetPath) {
    return normalizedBase || "/";
  }
  const normalizedTarget = targetPath.startsWith("/") ? targetPath : `/${targetPath}`;
  return `${normalizedBase}${normalizedTarget}` || "/";
}

function stripExistingBasePath(targetPath: string, basePath: string): string {
  const normalizedBase = trimTrailingSlash(basePath);
  if (!normalizedBase) {
    return targetPath;
  }

  const normalizedTarget = targetPath.startsWith("/") ? targetPath : `/${targetPath}`;
  if (normalizedTarget === normalizedBase) {
    return "";
  }
  if (normalizedTarget.startsWith(`${normalizedBase}/`)) {
    return normalizedTarget.slice(normalizedBase.length);
  }
  return targetPath;
}
