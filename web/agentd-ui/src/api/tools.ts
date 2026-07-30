import { baseURL } from "./clientCore";

export interface ToolCatalogEntry {
  name: string;
  description?: string;
  parameters?: Record<string, unknown>;
}

// fetchToolCatalog returns every registered tool schema, for building
// specialist tool allow-lists.
export async function fetchToolCatalog(): Promise<ToolCatalogEntry[]> {
  const resp = await fetch(`${baseURL.replace(/\/$/, "")}/tools/catalog`);
  if (!resp.ok) {
    throw new Error(`request failed (${resp.status})`);
  }
  return (await resp.json()) as ToolCatalogEntry[];
}
