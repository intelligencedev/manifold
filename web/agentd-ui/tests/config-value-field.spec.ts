import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ConfigValueField from "@/components/settings/ConfigValueField.vue";

describe("ConfigValueField LLM clients", () => {
  it("shows one supported provider configuration at a time", async () => {
    const wrapper = mount(ConfigValueField, {
      props: {
        label: "Evolving memory client",
        modelValue: {
          provider: "openai",
          openai: { model: "gpt-5-mini", apiKey: "[REDACTED]" },
          anthropic: { model: "claude-sonnet-4-6", maxTokens: 4096 },
          google: { model: "gemini-2.5-pro", timeoutSeconds: 30 },
        },
      },
    });

    const provider = wrapper.get("select[data-llm-provider]");
    expect(
      provider.findAll("option").map((option) => option.attributes("value")),
    ).toEqual([
      "openai",
      "anthropic",
      "google",
      "openrouter",
      "llamacpp",
      "local",
    ]);
    expect(wrapper.find("#config-maxtokens").exists()).toBe(false);

    await provider.setValue("anthropic");
    await wrapper.setProps({
      modelValue: {
        provider: "anthropic",
        openai: { model: "gpt-5-mini", apiKey: "[REDACTED]" },
        anthropic: { model: "claude-sonnet-4-6", maxTokens: 4096 },
        google: { model: "gemini-2.5-pro", timeoutSeconds: 30 },
      },
    });

    expect(wrapper.find("#config-maxtokens").exists()).toBe(true);
    expect(wrapper.findAll("#config-model")).toHaveLength(1);
  });
});
