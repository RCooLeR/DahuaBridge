import { describe, expect, it, vi } from "vitest";

import { fetchRegistrySnapshot } from "../src/ha/registry";
import type { HomeAssistant } from "../src/types/home-assistant";

describe("registry snapshot", () => {
  it("shares simultaneous registry loads for the same Home Assistant connection", async () => {
    const sendMessagePromiseMock = vi.fn(
      (_message: { type: string; [key: string]: unknown }) => Promise.resolve([]),
    );
    const sendMessagePromise: NonNullable<
      NonNullable<HomeAssistant["connection"]>["sendMessagePromise"]
    > = async <T,>(message: { type: string; [key: string]: unknown }) =>
      (await sendMessagePromiseMock(message)) as T;
    const connection = { sendMessagePromise };
    const hassA = buildHass(connection);
    const hassB = buildHass(connection);

    const [snapshotA, snapshotB] = await Promise.all([
      fetchRegistrySnapshot(hassA),
      fetchRegistrySnapshot(hassB),
    ]);

    expect(snapshotA).toBe(snapshotB);
    expect(sendMessagePromiseMock).toHaveBeenCalledTimes(3);
  });
});

function buildHass(
  connection: NonNullable<HomeAssistant["connection"]>,
): HomeAssistant {
  return {
    states: {},
    connection,
    callService: vi.fn(),
  };
}
