import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EditSpecialistRoot from "@/components/specialists/EditSpecialistRoot.vue";
import type { Specialist } from "@/api/client";

const apiMocks = vi.hoisted(() => ({
  upsertSpecialist: vi.fn(),
}));

vi.mock("@/api/client", () => ({
  upsertSpecialist: apiMocks.upsertSpecialist,
}));

vi.mock("@/api/playground", () => ({
  listPrompts: async () => [],
  listPromptVersions: async () => [],
}));

vi.mock("@/api/flow", () => ({
  fetchFlowTools: async () => [
    { name: "agent_response", description: "Return the final answer" },
    { name: "fetch", description: "Fetch a URL" },
    { name: "search", description: "Search the web" },
  ],
}));

const providerDefaults = {
  openai: {
    provider: "openai",
    baseURL: "https://api.openai.com/v1",
    model: "gpt-5",
  },
};

function baseSpecialist(overrides: Partial<Specialist> = {}): Specialist {
  return {
    id: 1,
    name: "coder",
    description: "Code specialist",
    provider: "openai",
    baseURL: "https://api.openai.com/v1",
    model: "gpt-5",
    summaryContextWindowTokens: 0,
    enableTools: true,
    imageGeneration: false,
    autoDiscover: null,
    paused: false,
    allowTools: [],
    system: "",
    extraHeaders: {},
    extraParams: {},
    teams: [],
    ...overrides,
  };
}

function mountEditor(initial: Specialist) {
  return mount(EditSpecialistRoot, {
    props: {
      initial,
      providerOptions: ["openai"],
      providerDefaults,
      availableTeams: [],
    },
    global: {
      stubs: {
        CodeEditor: {
          props: ["modelValue"],
          emits: ["update:modelValue", "blur"],
          template:
            '<textarea :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @blur="$emit(\'blur\')" />',
        },
        KeyValueTableEditor: {
          props: ["modelValue"],
          emits: ["update:modelValue", "editJson", "blur"],
          template: "<div />",
        },
      },
    },
  });
}

async function clickSave(wrapper: ReturnType<typeof mountEditor>) {
  const saveButton = wrapper
    .findAll("button")
    .find((button) => button.text() === "Save");
  expect(saveButton).toBeTruthy();
  await saveButton!.trigger("click");
  await flushPromises();
}

describe("EditSpecialistRoot harness settings", () => {
  beforeEach(() => {
    apiMocks.upsertSpecialist.mockReset();
    apiMocks.upsertSpecialist.mockImplementation(async (sp: Specialist) => ({
      ...sp,
      id: sp.id ?? 1,
    }));
  });

  it("submits null harness when the per-specialist override is disabled", async () => {
    const wrapper = mountEditor(baseSpecialist());
    await flushPromises();

    await clickSave(wrapper);

    const payload = apiMocks.upsertSpecialist.mock.calls[0][0] as Specialist;
    expect(payload.harness).toBeNull();
  });

  it("submits workflow harness settings from the specialist editor", async () => {
    const wrapper = mountEditor(baseSpecialist());
    await flushPromises();

    await wrapper.find("#sp-harness-override").setValue(true);
    await flushPromises();
    await wrapper.find("select#sp-harness-mode").setValue("workflow");
    await wrapper.find("#sp-harness-max-retries").setValue("6");
    await wrapper.find("#sp-harness-max-tool-errors").setValue("4");
    await wrapper
      .find("#sp-harness-terminal-tools")
      .setValue("agent_response\nfinalize");
    await wrapper.find("#sp-harness-required-steps").setValue("search, fetch");
    await wrapper.find("#sp-harness-compact-keep").setValue("5");
    await wrapper.find("#sp-harness-phase-thresholds").setValue("0.5, 0.8");

    await clickSave(wrapper);

    const payload = apiMocks.upsertSpecialist.mock.calls[0][0] as Specialist;
    expect(payload.harness).toEqual({
      enabled: true,
      mode: "workflow",
      rescueEnabled: true,
      maxRetriesPerStep: 6,
      maxToolErrors: 4,
      terminalTools: ["agent_response", "finalize"],
      requiredSteps: ["search", "fetch"],
      toolPrerequisites: {},
      compact: {
        enabled: true,
        keepRecentSteps: 5,
        phaseThresholds: [0.5, 0.8],
      },
    });
  });
});
