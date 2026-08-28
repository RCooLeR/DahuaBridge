import { z } from "zod";

import type { LovelaceCardConfig } from "./home-assistant";

const nonBlankTextSchema = z.string().trim().min(1);

export const browserBridgeUrlSchema = z
  .string()
  .trim()
  .pipe(z.url({ protocol: /^https?$/ }));

export const vtoCardConfigSchema = z
  .object({
    device_id: nonBlankTextSchema.optional(),
    label: nonBlankTextSchema.optional(),
    lock_button_entity: nonBlankTextSchema.optional(),
    auto_record_entity: nonBlankTextSchema.optional(),
  })
  .optional();

const configSchema = z.object({
  type: z.literal("custom:dahuabridge-surveillance-panel"),
  title: nonBlankTextSchema.optional(),
  subtitle: nonBlankTextSchema.optional(),
  browser_bridge_url: browserBridgeUrlSchema.optional(),
  event_lookback_hours: z.number().int().positive().max(168).optional(),
  bridge_event_poll_seconds: z.number().int().min(5).max(300).optional(),
  max_events: z.number().int().positive().max(50).optional(),
  vto: vtoCardConfigSchema,
});

export type SurveillancePanelCardConfig = z.infer<typeof configSchema> &
  LovelaceCardConfig;

export function parseConfig(
  config: unknown,
): SurveillancePanelCardConfig {
  return parseCardConfigSchema(configSchema, config);
}

export function parseCardConfigSchema<Schema extends z.ZodType>(
  schema: Schema,
  config: unknown,
): z.output<Schema> {
  const result = schema.safeParse(config);
  if (result.success) {
    return result.data;
  }

  throw new Error(`Invalid DahuaBridge card configuration:\n${z.prettifyError(result.error)}`);
}
