import { describe, expect, it } from "vitest";
import { nothing, type TemplateResult } from "lit";

import { renderCameraEventCountBadges } from "../src/cards/surveillance-panel-event-badges";
import type { CameraViewModel } from "../src/domain/model";

describe("renderCameraEventCountBadges", () => {
  it("renders only non-zero rolling 24h event counters", () => {
    const camera = {
      humanCount24h: 2,
      vehicleCount24h: 3,
      ivsCount24h: 0,
    } as CameraViewModel;
    const result = renderCameraEventCountBadges(camera, "overlay");

    expect(result).not.toBe(nothing);
    const template = result as TemplateResult;
    expect(template.strings.join("")).toContain("tile-event-counts-");
    expect(template.strings.join("")).toContain("tile-event-count");
    expect(template.values[0]).toBe("overlay");
  });

  it("renders IVS-only counters", () => {
    const camera = {
      humanCount24h: 0,
      vehicleCount24h: 0,
      ivsCount24h: 4,
    } as CameraViewModel;

    expect(renderCameraEventCountBadges(camera, "inline")).not.toBe(nothing);
  });

  it("renders generic SMD/IVS total counters when category counts are unavailable", () => {
    const camera = {
      eventCount24h: 90,
      humanCount24h: 0,
      vehicleCount24h: 0,
      ivsCount24h: 0,
    } as CameraViewModel;

    expect(renderCameraEventCountBadges(camera, "inline")).not.toBe(nothing);
  });

  it("renders nothing when all counters are zero", () => {
    const camera = {
      eventCount24h: 0,
      humanCount24h: 0,
      vehicleCount24h: 0,
      ivsCount24h: 0,
    } as CameraViewModel;

    expect(renderCameraEventCountBadges(camera, "inline")).toBe(nothing);
  });
});
