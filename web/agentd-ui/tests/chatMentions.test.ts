import { describe, expect, it } from "vitest";
import {
  resolveLeadingChatMention,
  resolveLeadingSpecialistMention,
  stripLeadingChatMention,
  stripLeadingSpecialistMention,
} from "@/utils/chatMentions";

describe("resolveLeadingSpecialistMention", () => {
  it("resolves a leading specialist mention and strips it from the prompt", () => {
    expect(
      resolveLeadingSpecialistMention("@orchestrator-max write a haiku", [
        "orchestrator",
        "orchestrator-max",
      ]),
    ).toEqual({
      specialist: "orchestrator-max",
      prompt: "write a haiku",
    });
  });

  it("matches mentions case-insensitively", () => {
    expect(
      resolveLeadingSpecialistMention("@Orchestrator-Max write a haiku", [
        "orchestrator-max",
      ]),
    ).toEqual({
      specialist: "orchestrator-max",
      prompt: "write a haiku",
    });
  });

  it("treats unknown mentions as regular prompt text", () => {
    expect(
      resolveLeadingSpecialistMention("@unknown write a haiku", ["known"]),
    ).toEqual({
      specialist: null,
      prompt: "@unknown write a haiku",
    });
  });
});

describe("stripLeadingSpecialistMention", () => {
  it("strips punctuation separators after the mention token", () => {
    expect(
      stripLeadingSpecialistMention(
        "@orchestrator-max: write a haiku",
        "orchestrator-max",
      ),
    ).toBe("write a haiku");
  });
});

describe("resolveLeadingChatMention", () => {
  it("resolves a leading team mention and strips it from the prompt", () => {
    expect(
      resolveLeadingChatMention("@ops write a rollout plan", [
        { kind: "specialist", name: "writer" },
        { kind: "team", name: "ops" },
      ]),
    ).toEqual({
      kind: "team",
      name: "ops",
      prompt: "write a rollout plan",
    });
  });

  it("prefers teams when specialist and team names collide", () => {
    expect(
      resolveLeadingChatMention("@ops write a rollout plan", [
        { kind: "specialist", name: "ops" },
        { kind: "team", name: "ops" },
      ]),
    ).toEqual({
      kind: "team",
      name: "ops",
      prompt: "write a rollout plan",
    });
  });

  it("treats unknown target mentions as regular prompt text", () => {
    expect(
      resolveLeadingChatMention("@unknown write a plan", [
        { kind: "team", name: "ops" },
      ]),
    ).toEqual({
      kind: null,
      name: null,
      prompt: "@unknown write a plan",
    });
  });
});

describe("stripLeadingChatMention", () => {
  it("strips a leading team or specialist mention by target name", () => {
    expect(stripLeadingChatMention("@ops: write a plan", "ops")).toBe(
      "write a plan",
    );
  });
});
