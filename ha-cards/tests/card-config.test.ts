import { describe, expect, expectTypeOf, it } from "vitest";

import {
  parseConfig,
  type SurveillancePanelCardConfig,
} from "../src/types/card-config";

describe("surveillance panel card configuration", () => {
  it("normalizes user-entered text and accepts HTTP bridge URLs", () => {
    const config = parseConfig({
      type: "custom:dahuabridge-surveillance-panel",
      title: "  Front gate  ",
      browser_bridge_url: "  https://ha.example.test/bridge  ",
      vto: {
        device_id: "  front_vto  ",
      },
    });

    expect(config).toMatchObject({
      title: "Front gate",
      browser_bridge_url: "https://ha.example.test/bridge",
      vto: { device_id: "front_vto" },
    });
    expectTypeOf(config).toMatchTypeOf<SurveillancePanelCardConfig>();
  });

  it("uses Zod's structured error formatter for invalid bridge URLs", () => {
    expect(() =>
      parseConfig({
        type: "custom:dahuabridge-surveillance-panel",
        browser_bridge_url: "bridge.internal:9020",
      }),
    ).toThrow(/Invalid DahuaBridge card configuration:[\s\S]*browser_bridge_url/);
  });

  it("includes nested property paths in configuration failures", () => {
    expect(() =>
      parseConfig({
        type: "custom:dahuabridge-surveillance-panel",
        vto: { device_id: "   " },
      }),
    ).toThrow(/vto\.device_id/);
  });
});
