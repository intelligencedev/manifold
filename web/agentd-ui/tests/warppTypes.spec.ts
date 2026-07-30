import { describe, expect, it } from "vitest";

import { assignable, isWildcard, parseType, portColor } from "@/lib/warppTypes";

const coercions: [string, string][] = [
  ["number", "text"],
  ["boolean", "text"],
];

describe("parseType", () => {
  it("parses scalars, lists, wildcards", () => {
    expect(parseType("text")).toEqual({ kind: "text" });
    expect(parseType("list<json>")).toEqual({ kind: "list", elem: "json" });
    expect(parseType("T")).toEqual({ kind: "T" });
    expect(parseType("list<T>")).toEqual({ kind: "list", elem: "T" });
    expect(parseType("dynamic:as")).toEqual({ kind: "dynamic" });
  });
});

describe("assignable", () => {
  it("identity and coercions", () => {
    expect(assignable("text", "text", coercions)).toBe(true);
    expect(assignable("number", "text", coercions)).toBe(true);
    expect(assignable("boolean", "text", coercions)).toBe(true);
    expect(assignable("text", "number", coercions)).toBe(false);
    expect(assignable("json", "text", coercions)).toBe(false);
  });
  it("lists match on element", () => {
    expect(assignable("list<text>", "list<text>", coercions)).toBe(true);
    expect(assignable("list<number>", "list<text>", coercions)).toBe(false);
    expect(assignable("text", "list<text>", coercions)).toBe(false);
  });
  it("wildcards accept anything", () => {
    expect(assignable("json", "T", coercions)).toBe(true);
    expect(assignable("list<json>", "list<T>", coercions)).toBe(true);
    expect(assignable("dynamic:as", "text", coercions)).toBe(true);
    expect(isWildcard(parseType("list<T>"))).toBe(true);
  });
  it("port colors are stable and list follows element", () => {
    expect(portColor("text")).toBe(portColor("text"));
    expect(portColor("list<number>")).toBe(portColor("number"));
    expect(portColor("text")).not.toBe(portColor("json"));
  });
});
