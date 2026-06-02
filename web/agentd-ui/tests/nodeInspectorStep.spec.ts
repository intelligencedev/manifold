import { mount } from "@vue/test-utils";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import NodeInspectorGroup from "@/components/flow/NodeInspectorGroup.vue";
import NodeInspectorSticky from "@/components/flow/NodeInspectorSticky.vue";
import NodeInspectorStep from "@/components/flow/NodeInspectorStep.vue";
import NodeInspectorUtility from "@/components/flow/NodeInspectorUtility.vue";
import type {
  GroupNodeData,
  StepNodeData,
  StickyNoteNodeData,
} from "@/types/flow";
import type { FlowEditorTool } from "@/types/flowEditor";

const updateNodeData = vi.hoisted(() => vi.fn());

vi.mock("@vue-flow/core", () => ({
  useVueFlow: () => ({ updateNodeData }),
}));

const tools: FlowEditorTool[] = [
  {
    name: "web-search",
    parameters: {
      type: "object",
      properties: {
        query: { type: "string", title: "Query" },
      },
    },
  },
];

const stepData: StepNodeData = {
  order: 2,
  label: "Search",
  step: {
    id: "step-1",
    text: "Find something",
    publish_result: false,
    tool: {
      name: "web-search",
      args: { query: "old query" },
    },
  },
};

const utilityData: StepNodeData = {
  order: 3,
  kind: "utility",
  step: {
    id: "utility-1",
    text: "Notes",
    publish_result: false,
    tool: {
      name: "utility_textbox",
      args: {
        label: "Notes",
        text: "old note",
      },
    },
  },
};

const groupData: GroupNodeData = {
  kind: "group",
  label: "Group",
  color: "default",
};

const stickyData: StickyNoteNodeData = {
  kind: "utility",
  note: "old sticky note",
  color: "default",
};

const DropdownSelectStub = {
  props: ["modelValue", "options", "disabled"],
  emits: ["update:modelValue"],
  template: `
    <select
      class="tool-select"
      :value="modelValue"
      :disabled="disabled"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option
        v-for="option in options"
        :key="option.id"
        :value="option.value"
      >
        {{ option.label }}
      </option>
    </select>
  `,
};

function sharedGlobal() {
  return {
    provide: {
      flowEditorMode: ref("design"),
      flowEditorHydrating: ref(false),
    },
  };
}

function mountStepInspector() {
  return mount(NodeInspectorStep, {
    props: {
      nodeId: "step-1",
      data: stepData,
      tools,
    },
    global: {
      ...sharedGlobal(),
      stubs: {
        DropdownSelect: DropdownSelectStub,
        FlowInputBindingsEditor: {
          props: ["schema", "modelValue"],
          emits: ["update:modelValue"],
          template: `
            <button
              class="args-editor"
              type="button"
              @click="$emit('update:modelValue', { query: 'new query' })"
            >
              Update args
            </button>
          `,
        },
      },
    },
  });
}

function mountUtilityInspector() {
  return mount(NodeInspectorUtility, {
    props: {
      nodeId: "utility-1",
      data: utilityData,
    },
    global: {
      ...sharedGlobal(),
      stubs: {
        DropdownSelect: DropdownSelectStub,
        ExpressionPicker: true,
      },
    },
  });
}

function mountGroupInspector() {
  return mount(NodeInspectorGroup, {
    props: {
      nodeId: "group-1",
      data: groupData,
    },
    global: sharedGlobal(),
  });
}

function mountStickyInspector() {
  return mount(NodeInspectorSticky, {
    props: {
      nodeId: "sticky-1",
      data: stickyData,
    },
    global: sharedGlobal(),
  });
}

describe("NodeInspectorStep", () => {
  beforeEach(() => {
    updateNodeData.mockReset();
  });

  it("writes parameter edits to node data without requiring Apply", async () => {
    const wrapper = mountStepInspector();

    expect(wrapper.text()).not.toContain("Apply");

    await wrapper.find(".args-editor").trigger("click");

    expect(updateNodeData).toHaveBeenCalledWith(
      "step-1",
      expect.objectContaining({
        order: 2,
        label: "Search",
        step: expect.objectContaining({
          id: "step-1",
          text: "Find something",
          tool: {
            name: "web-search",
            args: { query: "new query" },
          },
        }),
      }),
    );
  });

  it("writes field edits to node data as they change", async () => {
    const wrapper = mountStepInspector();
    const labelInput = wrapper.find(
      'input[placeholder="Optional (defaults to tool name)"]',
    );

    await labelInput.setValue("Search the docs");

    expect(updateNodeData).toHaveBeenCalledWith(
      "step-1",
      expect.objectContaining({
        label: "Search the docs",
        step: expect.objectContaining({
          text: "Find something",
          tool: {
            name: "web-search",
            args: { query: "old query" },
          },
        }),
      }),
    );
  });
});

describe("node inspectors", () => {
  beforeEach(() => {
    updateNodeData.mockReset();
  });

  it("writes utility edits without requiring Apply", async () => {
    const wrapper = mountUtilityInspector();

    expect(wrapper.text()).not.toContain("Apply");

    await wrapper.find("textarea").setValue("updated note");

    expect(updateNodeData).toHaveBeenCalledWith(
      "utility-1",
      expect.objectContaining({
        step: expect.objectContaining({
          text: "Notes",
          tool: {
            name: "utility_textbox",
            args: {
              label: "Notes",
              text: "updated note",
            },
          },
        }),
      }),
    );
  });

  it("writes group edits without requiring Apply", async () => {
    const wrapper = mountGroupInspector();

    expect(wrapper.text()).not.toContain("Apply");

    await wrapper.find("input").setValue("Planning");

    expect(updateNodeData).toHaveBeenCalledWith(
      "group-1",
      expect.objectContaining({
        kind: "group",
        label: "Planning",
        color: "default",
      }),
    );
  });

  it("writes sticky note edits without requiring Apply", async () => {
    const wrapper = mountStickyInspector();

    expect(wrapper.text()).not.toContain("Apply");

    await wrapper.find("textarea").setValue("updated sticky note");

    expect(updateNodeData).toHaveBeenCalledWith(
      "sticky-1",
      expect.objectContaining({
        kind: "utility",
        note: "updated sticky note",
        color: "default",
      }),
    );
  });
});
