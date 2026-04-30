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
    return buildRewrittenUrl(browserBase, parsedTarget.pathname, parsedTarget.search, parsedTarget.hash);
  } catch {
    if (normalizedTarget.startsWith("/")) {
      return buildRewrittenUrl(browserBase, normalizedTarget, "", "");
    }
    return new URL(normalizedTarget, ensureTrailingSlash(normalizedBrowserBridgeUrl)).toString();
  }
}

function buildRewrittenUrl(
  browserBase: URL,
  targetPath: string,
  search: string,
  hash: string,
): string {
  const rewritten = new URL(browserBase.toString());
  const normalizedTargetPath = targetPath === "/" && !search && !hash ? "" : targetPath;
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

function ensureTrailingSlash(value: string): string {
  return value.endsWith("/") ? value : `${value}/`;
}

function joinUrlPaths(basePath: string, targetPath: string): string {
  const normalizedBase = trimTrailingSlash(basePath);
  if (!targetPath) {
    return normalizedBase || "/";
  }
  const normalizedTarget = targetPath.startsWith("/") ? targetPath : `/${targetPath}`;
  return `${normalizedBase}${normalizedTarget}` || "/";
}
