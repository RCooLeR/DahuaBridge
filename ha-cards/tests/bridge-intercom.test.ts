import { describe, expect, it, vi } from "vitest";

import {
  BridgeIntercomSessionController,
  buildWebRtcOfferUrl,
  resolveIntercomOfferUrl,
} from "../src/ha/bridge-intercom";

describe("bridge intercom helpers", () => {
  it("builds an offer URL from the bridge intercom helper page path", () => {
    expect(
      buildWebRtcOfferUrl("https://ha.example.com/bridge/api/v1/media/intercom/front_vto/quality"),
    ).toBe("https://ha.example.com/bridge/api/v1/media/webrtc/front_vto/quality/offer");
  });

  it("keeps existing webrtc offer URLs stable", () => {
    expect(
      buildWebRtcOfferUrl("https://ha.example.com/bridge/api/v1/media/webrtc/front_vto/quality/offer"),
    ).toBe("https://ha.example.com/bridge/api/v1/media/webrtc/front_vto/quality/offer");
  });

  it("prefers a profile-level webrtc URL and falls back to the stream intercom URL", () => {
    expect(
      resolveIntercomOfferUrl({
        localIntercomUrl: "https://ha.example.com/bridge/api/v1/media/intercom/front_vto/quality",
        preferredVideoProfile: "quality",
        profiles: [
          {
            key: "quality",
            name: "Quality",
            streamUrl: null,
            localDashUrl: null,
            localMjpegUrl: null,
            localHlsUrl: null,
            localWebRtcUrl: "https://ha.example.com/bridge/api/v1/media/webrtc/front_vto/quality",
            subtype: 0,
            rtspTransport: null,
            frameRate: 15,
            resolution: "1280x720",
            recommended: true,
          },
        ],
      }),
    ).toBe("https://ha.example.com/bridge/api/v1/media/webrtc/front_vto/quality/offer");

    expect(
      resolveIntercomOfferUrl({
        localIntercomUrl: "https://ha.example.com/bridge/api/v1/media/intercom/front_vto/quality",
        preferredVideoProfile: null,
        profiles: [],
      }),
    ).toBe("https://ha.example.com/bridge/api/v1/media/webrtc/front_vto/quality/offer");
  });
});

describe("BridgeIntercomSessionController", () => {
  it("enables and disables the browser microphone session", async () => {
    const snapshots: string[] = [];
    const micTrack = { stop: vi.fn() };
    const micStream = {
      getAudioTracks: () => [micTrack],
      getTracks: () => [micTrack],
    } as unknown as MediaStream;
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({ type: "answer", sdp: "answer" }),
    }) as Response);
    const getUserMedia = vi.fn(async () => micStream);
    const peer = new FakePeerConnection();
    const controller = new BridgeIntercomSessionController({
      onChange: (snapshot) => {
        snapshots.push(`${snapshot.phase}:${snapshot.statusText}`);
      },
      fetchImpl,
      getUserMedia,
      createPeerConnection: () => peer as unknown as RTCPeerConnection,
    });

    await controller.enable("https://ha.example.com/bridge/api/v1/media/intercom/front_vto/quality");

    expect(getUserMedia).toHaveBeenCalledWith({ audio: true });
    expect(fetchImpl).toHaveBeenCalledWith(
      "https://ha.example.com/bridge/api/v1/media/webrtc/front_vto/quality/offer",
      expect.objectContaining({
        method: "POST",
      }),
    );
    expect(controller.currentSnapshot()).toMatchObject({
      enabled: true,
      phase: "connected",
      statusText: "Mic connected",
      error: "",
    });

    await controller.disable();

    expect(micTrack.stop).toHaveBeenCalledTimes(1);
    expect(peer.closed).toBe(true);
    expect(controller.currentSnapshot()).toMatchObject({
      enabled: false,
      phase: "idle",
      statusText: "Mic inactive",
      error: "",
    });
    expect(snapshots).toContain("connected:Mic connected");
  });
});

class FakePeerConnection {
  connectionState: RTCPeerConnectionState = "new";
  iceGatheringState: RTCIceGatheringState = "complete";
  localDescription: RTCSessionDescriptionInit | null = null;
  onconnectionstatechange: (() => void) | null = null;
  closed = false;

  addTransceiver(): void {}

  addTrack(): void {}

  async createOffer(): Promise<RTCSessionDescriptionInit> {
    return { type: "offer", sdp: "offer" };
  }

  async setLocalDescription(description: RTCSessionDescriptionInit): Promise<void> {
    this.localDescription = description;
  }

  async setRemoteDescription(): Promise<void> {
    this.connectionState = "connected";
    this.onconnectionstatechange?.();
  }

  addEventListener(): void {}

  removeEventListener(): void {}

  close(): void {
    this.closed = true;
    this.connectionState = "closed";
  }
}
