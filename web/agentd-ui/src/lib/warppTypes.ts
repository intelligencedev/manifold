// Port-type parsing and drag-time connection compatibility for the WARPP
// editor. Mirrors the backend coercion rules (internal/warpp/value.go).

export interface PortType {
  kind: string;
  elem?: string;
}

const PORT_COLORS: Record<string, string> = {
  text: "#8bd17c",
  number: "#6fb3ff",
  boolean: "#ffb46f",
  json: "#c792ea",
  file: "#62d2c5",
};

const WILDCARD_COLOR = "#9aa4b2";

export function parseType(s: string): PortType {
  const trimmed = (s ?? "").trim();
  if (trimmed.startsWith("dynamic:")) {
    return { kind: "dynamic" };
  }
  if (trimmed.startsWith("list<") && trimmed.endsWith(">")) {
    return { kind: "list", elem: trimmed.slice(5, -1) };
  }
  return { kind: trimmed };
}

export function typeLabel(t: PortType): string {
  if (t.kind === "list") {
    return `list<${t.elem ?? ""}>`;
  }
  return t.kind;
}

export function isWildcard(t: PortType): boolean {
  return (
    t.kind === "T" ||
    t.kind === "dynamic" ||
    (t.kind === "list" && t.elem === "T")
  );
}

export function assignable(
  from: string,
  to: string,
  coercions: [string, string][],
): boolean {
  const f = parseType(from);
  const t = parseType(to);
  if (isWildcard(f) || isWildcard(t)) {
    return true;
  }
  if (f.kind === "list" || t.kind === "list") {
    return f.kind === "list" && t.kind === "list" && f.elem === t.elem;
  }
  if (f.kind === t.kind) {
    return true;
  }
  return coercions.some(([a, b]) => a === f.kind && b === t.kind);
}

export function portColor(typeString: string): string {
  const t = parseType(typeString);
  if (isWildcard(t)) {
    return WILDCARD_COLOR;
  }
  if (t.kind === "list") {
    return PORT_COLORS[t.elem ?? ""] ?? WILDCARD_COLOR;
  }
  return PORT_COLORS[t.kind] ?? WILDCARD_COLOR;
}
