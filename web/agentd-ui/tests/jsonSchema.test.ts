import { describe, expect, it } from "vitest";

import { normalizeParameterSchema, schemaType } from "@/lib/jsonSchema";

describe("json schema helpers", () => {
  it("uses the first supported non-null type from MCP nullable type arrays", () => {
    const schema = normalizeParameterSchema({
      title: "Freshness",
      type: ["null", "string"],
      enum: ["day", "week", "month"],
    });

    expect(schema.type).toBe("string");
    expect(schemaType(schema)).toBe("string");
    expect(schema.enum).toEqual(["day", "week", "month"]);
  });

  it("unwraps MCP anyOf schemas so fields remain editable", () => {
    const schema = normalizeParameterSchema({
      title: "Goggles",
      description: "Optional search goggles",
      anyOf: [{ type: "null" }, { type: "string" }],
      default: null,
    });

    expect(schema.type).toBe("string");
    expect(schema.title).toBe("Goggles");
    expect(schema.description).toBe("Optional search goggles");
  });
});
