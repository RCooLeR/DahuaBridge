import type { CameraStreamViewModel } from "../domain/model";

export type BridgeIntercomPhase =
  | "idle"
  | "negotiating"
  | "connecting"
  | "connected"
  | "reconnecting"
  | "error";

export interface BridgeIntercomSnapshot {
  enabled: boolean;
  phase: BridgeIntercomPhase;
  statusText: string;
  error: string;
}

interface BridgeIntercomSessionOptions {
  onChange: (snapshot: BridgeIntercomSnapshot) => void;
  fetchImpl?: typeof fetch;
  createPeerConnection?: (configuration?: RTCConfiguration) => RTCPeerConnection;
  getUserMedia?: (constraints: MediaStreamConstraints) => Promise<MediaStream>;
  setTimeoutImpl?: typeof globalThis.setTimeout;
  clearTimeoutImpl?: typeof globalThis.clearTimeout;
}

const INITIAL_SNAPSHOT: BridgeIntercomSnapshot = {
  enabled: false,
  phase: "idle",
  statusText: "Mic inactive",
  error: "",
};

export function buildWebRtcOfferUrl(baseUrl: string | null): string | null {
  const normalizedBaseUrl = baseUrl?.trim();
  if (!normalizedBaseUrl) {
    return null;
  }

  try {
    const parsed = new URL(normalizedBaseUrl, globalThis.location?.href);
    parsed.pathname = normalizeOfferPath(parsed.pathname);
    return parsed.toString();
  } catch {
    return normalizeOfferPath(normalizedBaseUrl);
  }
}

export function resolveIntercomOfferUrl(stream: Pick<CameraStreamViewModel, "localIntercomUrl" | "profiles" | "preferredVideoProfile">): string | null {
  if (stream.preferredVideoProfile) {
    const preferredProfile =
      stream.profiles.find((profile) => profile.key === stream.preferredVideoProfile) ?? null;
    if (preferredProfile?.localWebRtcUrl) {
      return buildWebRtcOfferUrl(preferredProfile.localWebRtcUrl);
    }
  }

  const recommendedProfile =
    stream.profiles.find((profile) => profile.recommended && profile.localWebRtcUrl) ??
    stream.profiles.find((profile) => profile.localWebRtcUrl) ??
    null;
  if (recommendedProfile?.localWebRtcUrl) {
    return buildWebRtcOfferUrl(recommendedProfile.localWebRtcUrl);
  }

  return buildWebRtcOfferUrl(stream.localIntercomUrl);
}

export class BridgeIntercomSessionController {
  private readonly fetchImpl: typeof fetch;
  private readonly createPeerConnection: (configuration?: RTCConfiguration) => RTCPeerConnection;
  private readonly getUserMedia: ((constraints: MediaStreamConstraints) => Promise<MediaStream>) | null;
  private readonly setTimeoutImpl: typeof globalThis.setTimeout;
  private readonly clearTimeoutImpl: typeof globalThis.clearTimeout;
  private reconnectTimer: ReturnType<typeof globalThis.setTimeout> | null = null;
  private peer: RTCPeerConnection | null = null;
  private micStream: MediaStream | null = null;
  private reconnectAttempts = 0;
  private desiredEnabled = false;
  private currentOfferUrl: string | null = null;
  private connectionVersion = 0;
  private snapshot: BridgeIntercomSnapshot = INITIAL_SNAPSHOT;

  constructor(private readonly options: BridgeIntercomSessionOptions) {
    this.fetchImpl = options.fetchImpl ?? fetch;
    this.createPeerConnection =
      options.createPeerConnection ??
      ((configuration) => new RTCPeerConnection(configuration));
    this.getUserMedia =
      options.getUserMedia ??
      globalThis.navigator?.mediaDevices?.getUserMedia?.bind(globalThis.navigator.mediaDevices) ??
      null;
    this.setTimeoutImpl = options.setTimeoutImpl ?? globalThis.setTimeout.bind(globalThis);
    this.clearTimeoutImpl = options.clearTimeoutImpl ?? globalThis.clearTimeout.bind(globalThis);
  }

  currentSnapshot(): BridgeIntercomSnapshot {
    return this.snapshot;
  }

  async enable(offerUrl: string): Promise<void> {
    const normalizedOfferUrl = buildWebRtcOfferUrl(offerUrl);
    if (!normalizedOfferUrl) {
      this.publish({
        enabled: false,
        phase: "error",
        statusText: "Mic unavailable",
        error: "Bridge intercom offer URL is unavailable for this VTO.",
      });
      return;
    }

    this.desiredEnabled = true;
    this.currentOfferUrl = normalizedOfferUrl;
    const version = ++this.connectionVersion;

    try {
      await this.connect(version, false);
    } catch (error) {
      this.desiredEnabled = false;
      this.currentOfferUrl = null;
      this.clearReconnectTimer();
      this.closePeer();
      this.stopMicStream();
      this.publish({
        enabled: false,
        phase: "error",
        statusText: "Mic unavailable",
        error: error instanceof Error ? error.message : "Browser microphone setup failed.",
      });
    }
  }

  async disable(): Promise<void> {
    this.desiredEnabled = false;
    this.currentOfferUrl = null;
    this.connectionVersion += 1;
    this.clearReconnectTimer();
    this.closePeer();
    this.stopMicStream();
    this.reconnectAttempts = 0;
    this.publish(INITIAL_SNAPSHOT);
  }

  private async connect(version: number, isReconnect: boolean): Promise<void> {
    const offerUrl = this.currentOfferUrl;
    if (!this.desiredEnabled || !offerUrl) {
      return;
    }

    this.clearReconnectTimer();
    this.closePeer();
    this.publish({
      enabled: true,
      phase: isReconnect ? "reconnecting" : "negotiating",
      statusText: isReconnect ? "Reconnecting mic" : "Negotiating mic",
      error: "",
    });

    const peer = this.createPeerConnection({ iceServers: [] });
    this.peer = peer;
    peer.onconnectionstatechange = () => {
      this.handleConnectionStateChange(peer);
    };
    peer.addTransceiver("video", { direction: "recvonly" });
    peer.addTransceiver("audio", { direction: "recvonly" });

    if (!this.getUserMedia) {
      throw new Error("Browser microphone capture is not available in this browser.");
    }
    if (!this.micStream) {
      this.micStream = await this.getUserMedia({ audio: true });
    }
    if (!this.isCurrentConnection(version, peer)) {
      peer.close();
      return;
    }

    for (const track of this.micStream.getAudioTracks()) {
      peer.addTrack(track, this.micStream);
    }

    this.publish({
      enabled: true,
      phase: "connecting",
      statusText: "Connecting mic",
      error: "",
    });

    const offer = await peer.createOffer();
    if (!this.isCurrentConnection(version, peer)) {
      peer.close();
      return;
    }
    await peer.setLocalDescription(offer);
    if (!this.isCurrentConnection(version, peer)) {
      peer.close();
      return;
    }
    await waitForIceComplete(peer);
    if (!this.isCurrentConnection(version, peer)) {
      peer.close();
      return;
    }

    const response = await this.fetchImpl(offerUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(peer.localDescription),
    });
    if (!response.ok) {
      this.closePeer();
      throw new Error(await response.text());
    }

    const answer = (await response.json()) as RTCSessionDescriptionInit;
    if (!this.isCurrentConnection(version, peer)) {
      peer.close();
      return;
    }
    await peer.setRemoteDescription(answer);
    if (!this.isCurrentConnection(version, peer)) {
      peer.close();
      return;
    }

    if (peer.connectionState !== "connected") {
      this.publish({
        enabled: true,
        phase: "connecting",
        statusText: "Waiting for bridge media",
        error: "",
      });
    }
  }

  private handleConnectionStateChange(peer: RTCPeerConnection): void {
    if (this.peer !== peer || !this.desiredEnabled) {
      return;
    }

    switch (peer.connectionState) {
      case "connected":
        this.reconnectAttempts = 0;
        this.publish({
          enabled: true,
          phase: "connected",
          statusText: "Mic connected",
          error: "",
        });
        return;
      case "new":
        this.publish({
          enabled: true,
          phase: "negotiating",
          statusText: "Negotiating mic",
          error: "",
        });
        return;
      case "connecting":
        this.publish({
          enabled: true,
          phase: "connecting",
          statusText: "Connecting mic",
          error: "",
        });
        return;
      case "disconnected":
      case "failed":
        this.scheduleReconnect(`connection ${peer.connectionState}`);
        return;
      case "closed":
        this.scheduleReconnect("connection closed");
        return;
      default:
        this.publish({
          enabled: true,
          phase: "connecting",
          statusText: `Mic ${peer.connectionState}`,
          error: "",
        });
    }
  }

  private scheduleReconnect(reason: string): void {
    if (!this.desiredEnabled || !this.currentOfferUrl || this.reconnectTimer !== null) {
      return;
    }

    this.reconnectAttempts += 1;
    const delay = reconnectDelayMilliseconds(this.reconnectAttempts);
    this.publish({
      enabled: true,
      phase: "reconnecting",
      statusText: `Reconnecting mic in ${Math.max(1, Math.round(delay / 1000))}s`,
      error: "",
    });
    this.reconnectTimer = this.setTimeoutImpl(() => {
      this.reconnectTimer = null;
      const version = ++this.connectionVersion;
      void this.connect(version, true).catch((error) => {
        if (!this.desiredEnabled) {
          return;
        }
        this.publish({
          enabled: true,
          phase: "reconnecting",
          statusText: "Retrying mic",
          error: error instanceof Error ? error.message : String(error),
        });
        this.scheduleReconnect(reason);
      });
    }, delay);
  }

  private isCurrentConnection(version: number, peer: RTCPeerConnection): boolean {
    return (
      this.desiredEnabled &&
      this.currentOfferUrl !== null &&
      this.connectionVersion === version &&
      this.peer === peer
    );
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer === null) {
      return;
    }
    this.clearTimeoutImpl(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  private closePeer(): void {
    if (!this.peer) {
      return;
    }
    const peer = this.peer;
    this.peer = null;
    try {
      peer.onconnectionstatechange = null;
      peer.close();
    } catch {
      return;
    }
  }

  private stopMicStream(): void {
    if (!this.micStream) {
      return;
    }
    for (const track of this.micStream.getTracks()) {
      track.stop();
    }
    this.micStream = null;
  }

  private publish(snapshot: BridgeIntercomSnapshot): void {
    this.snapshot = snapshot;
    this.options.onChange(snapshot);
  }
}

function normalizeOfferPath(pathOrUrl: string): string {
  const trimmed = pathOrUrl.trim();
  if (!trimmed) {
    return trimmed;
  }

  const withoutOffer = trimmed.replace(/\/offer\/?$/, "");
  const intercomPathMatch = withoutOffer.match(/^(.*\/api\/v1\/media\/)(intercom|webrtc)(\/.+)$/);
  if (intercomPathMatch) {
    return `${intercomPathMatch[1]}webrtc${intercomPathMatch[3]}/offer`;
  }
  return `${withoutOffer.replace(/\/+$/, "")}/offer`;
}

function reconnectDelayMilliseconds(attempt: number): number {
  return Math.min(1000 * 2 ** Math.min(attempt, 4), 10_000);
}

async function waitForIceComplete(peer: RTCPeerConnection): Promise<void> {
  if (peer.iceGatheringState === "complete") {
    return;
  }

  await new Promise<void>((resolve) => {
    const onChange = () => {
      if (peer.iceGatheringState === "complete") {
        peer.removeEventListener("icegatheringstatechange", onChange);
        resolve();
      }
    };
    peer.addEventListener("icegatheringstatechange", onChange);
  });
}
