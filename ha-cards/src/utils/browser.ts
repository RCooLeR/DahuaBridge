import { logCardInfo, redactUrlForLog } from "./logging";

export function openExternalUrl(targetUrl: string): void {
  const normalizedUrl = targetUrl.trim();
  if (!normalizedUrl) {
    return;
  }

  logCardInfo("card open external url", {
    url: redactUrlForLog(normalizedUrl),
  });

  const documentRef = globalThis.document;
  if (documentRef?.body && typeof documentRef.createElement === "function") {
    const link = documentRef.createElement("a");
    link.href = normalizedUrl;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.style.display = "none";
    documentRef.body.append(link);
    link.click();
    link.remove();
    return;
  }

  if (typeof window !== "undefined" && typeof window.open === "function") {
    window.open(normalizedUrl, "_blank", "noopener,noreferrer");
  }
}
