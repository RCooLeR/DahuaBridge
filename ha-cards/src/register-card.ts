export interface CustomCardMetadata extends Record<string, unknown> {
  type: string;
  name: string;
  description: string;
  preview?: boolean;
}

interface CustomCardHost {
  customCards?: Array<Record<string, unknown>>;
}

/** Register a Lovelace card without duplicating its picker entry on resource reload. */
export function registerCustomCard(
  registry: Pick<CustomElementRegistry, "define" | "get">,
  host: CustomCardHost,
  tagName: string,
  constructor: CustomElementConstructor,
  metadata: CustomCardMetadata,
): void {
  if (!registry.get(tagName)) {
    registry.define(tagName, constructor);
  }

  const cards = host.customCards ?? [];
  if (!cards.some((card) => card.type === metadata.type)) {
    cards.push(metadata);
  }
  host.customCards = cards;
}
