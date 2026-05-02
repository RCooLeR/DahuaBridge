import { describe, expect, it } from "vitest";
import { nothing, type TemplateResult } from "lit";

import { renderCameraEventCountBadges } from "../src/cards/surveillance-panel-event-badges";
import type { CameraViewModel } from "../src/domain/model";

describe("renderCameraEventCountBadges", () => {
  it("renders only non-zero rolling 24h person and vehicle counters", () => {
    const camera = {
      humanCount24h: 2,
      vehicleCount24h: 3,
    } as CameraViewModel;
    const result = renderCameraEventCountBadges(camera, "overlay");

    expect(result).not.toBe(nothing);
    const template = result as TemplateResult;
    expect(template.strings.join("")).toContain("tile-event-counts-");
    expect(template.strings.join("")).toContain("tile-event-count");
    expect(template.values).toHaveLength(3);
    expect(template.values[0]).toBe("overlay");
  });

  it("renders nothing when both counters are zero", () => {
    const camera = {
      humanCount24h: 0,
      vehicleCount24h: 0,
    } as CameraViewModel;

    expect(renderCameraEventCountBadges(camera, "inline")).toBe(nothing);
  });
});
