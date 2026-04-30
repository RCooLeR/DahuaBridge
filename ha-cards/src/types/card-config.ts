import { z } from "zod";

import type { LovelaceCardConfig } from "./home-assistant";

const vtoSchema = z
  .object({
    device_id: z.string().min(1).optional(),
    label: z.string().min(1).optional(),
    lock_button_entity: z.string().min(1).optional(),
    input_volume_entity: z.string().min(1).optional(),
    output_volume_entity: z.string().min(1).optional(),
    muted_entity: z.string().min(1).optional(),
    auto_record_entity: z.string().min(1).optional(),
  })
  .optional();

const configSchema = z.object({
  type: z.literal("custom:dahuabridge-surveillance-panel"),
  title: z.string().min(1).optional(),
  subtitle: z.string().min(1).optional(),
  browser_bridge_url: z.string().min(1).optional(),
  event_lookback_hours: z.number().int().positive().max(168).optional(),
  bridge_event_poll_seconds: z.number().int().min(5).max(300).optional(),
  max_events: z.number().int().positive().max(50).optional(),
  vto: vtoSchema,
});

export type SurveillancePanelCardConfig = z.infer<typeof configSchema> &
  LovelaceCardConfig;

export function parseConfig(
  config: LovelaceCardConfig,
): SurveillancePanelCardConfig {
  return configSchema.parse(config);
}
