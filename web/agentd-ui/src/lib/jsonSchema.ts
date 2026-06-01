const supportedSchemaTypes = new Set([
  "object",
  "array",
  "boolean",
  "number",
  "integer",
  "string",
]);

type JSONSchemaRecord = Record<string, any>;

export function normalizeParameterSchema(
  schema: JSONSchemaRecord | undefined,
): JSONSchemaRecord {
  if (!schema || typeof schema !== "object" || Array.isArray(schema)) {
    return { type: "string" };
  }
  const branch = preferredCompositionBranch(schema);
  const merged = branch ? mergeSchemaBranch(schema, branch) : { ...schema };
  const type = inferredSchemaType(merged);
  return { ...merged, type: type ?? "string" };
}

export function schemaType(schema: JSONSchemaRecord | undefined): string {
  return inferredSchemaType(schema) ?? "string";
}

function preferredCompositionBranch(
  schema: JSONSchemaRecord,
): JSONSchemaRecord | null {
  for (const key of ["anyOf", "oneOf", "allOf"]) {
    const options = schema[key];
    if (!Array.isArray(options)) continue;
    const branch = options.find((option) => {
      if (!option || typeof option !== "object" || Array.isArray(option)) {
        return false;
      }
      return inferredSchemaType(option as JSONSchemaRecord) !== undefined;
    });
    if (branch) return branch as JSONSchemaRecord;
  }
  return null;
}

function mergeSchemaBranch(
  schema: JSONSchemaRecord,
  branch: JSONSchemaRecord,
): JSONSchemaRecord {
  const merged = { ...schema, ...branch };
  if (schema.title && !branch.title) merged.title = schema.title;
  if (schema.description && !branch.description) {
    merged.description = schema.description;
  }
  if (schema.default !== undefined && branch.default === undefined) {
    merged.default = schema.default;
  }
  return merged;
}

function inferredSchemaType(
  schema: JSONSchemaRecord | undefined,
): string | undefined {
  if (!schema) return undefined;
  const direct = directSchemaType(schema.type);
  if (direct) return direct;
  if (schema.properties && typeof schema.properties === "object") {
    return "object";
  }
  if (schema.items) return "array";
  if (Array.isArray(schema.enum) || schema.const !== undefined) {
    return "string";
  }
  return undefined;
}

function directSchemaType(rawType: unknown): string | undefined {
  if (typeof rawType === "string") {
    return supportedSchemaTypes.has(rawType) ? rawType : undefined;
  }
  if (!Array.isArray(rawType)) return undefined;
  for (const type of rawType) {
    if (typeof type === "string" && supportedSchemaTypes.has(type)) {
      return type;
    }
  }
  return undefined;
}
