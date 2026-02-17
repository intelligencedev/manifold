import { describe, expect, it } from "vitest";
import {
  resolveLeadingSpecialistMention,
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
    expect(resolveLeadingSpecialistMention("@unknown write a haiku", ["known"])).toEqual({
      specialist: null,
      prompt: "@unknown write a haiku",
    });
  });
});

describe("stripLeadingSpecialistMention", () => {
  it("strips punctuation separators after the mention token", () => {
    expect(
      stripLeadingSpecialistMention("@orchestrator-max: write a haiku", "orchestrator-max"),
    ).toBe("write a haiku");
  });
});
