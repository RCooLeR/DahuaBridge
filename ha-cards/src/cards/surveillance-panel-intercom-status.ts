import type { BridgeIntercomSnapshot } from "../ha/bridge-intercom";
import type { Localizer } from "../localization";

export function localizeIntercomStatus(
  snapshot: BridgeIntercomSnapshot,
  t: Localizer,
): string {
  if (snapshot.statusText === "Waiting for bridge media") {
    return t("vto.micWaitingBridge");
  }

  const reconnectMatch = snapshot.statusText.match(/^Reconnecting mic in (\d+)s$/u);
  if (reconnectMatch) {
    return t("vto.micReconnectingIn", { seconds: reconnectMatch[1] ?? "1" });
  }

  switch (snapshot.phase) {
    case "idle":
      return t("vto.micInactive");
    case "error":
      return t("vto.micUnavailable");
    case "negotiating":
      return t("vto.micNegotiating");
    case "connecting":
      return t("vto.micConnecting");
    case "connected":
      return t("vto.micConnected");
    case "reconnecting":
      return t("vto.micReconnecting");
    default:
      return t("vto.micState", { state: snapshot.phase });
  }
}

export function localizeIntercomError(error: string, t: Localizer): string {
  switch (error) {
    case "Bridge intercom offer URL is unavailable for this VTO.":
      return t("error.intercomOfferUnavailable");
    case "Browser microphone setup failed.":
      return t("error.browserMicSetupFailed");
    case "Browser microphone capture is not available in this browser.":
      return t("error.browserMicCaptureUnavailable");
    default:
      return error;
  }
}
