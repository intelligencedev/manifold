import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import TaskInspector from "@/components/durable/TaskInspector.vue";
import type { DurableTask } from "@/api/durable";

function makeTask(status: DurableTask["status"]): DurableTask {
  return {
    id: "dtask_1",
    queue: "flow_v2",
    name: "flow_v2.run",
    user_id: 0,
    status,
    attempt: 3,
    available_at: "2026-05-18T13:00:00Z",
    created_at: "2026-05-18T13:00:00Z",
    updated_at: "2026-05-18T13:01:00Z",
  };
}

describe("TaskInspector", () => {
  it("hides close control when used as a detail view", () => {
    const wrapper = mount(TaskInspector, {
      props: {
        task: makeTask("completed"),
        events: [],
        showClose: false,
      },
    });

    expect(wrapper.text()).not.toContain("Close");
  });

  it("emits retry with checkpoint reset choice for failed tasks", async () => {
    const wrapper = mount(TaskInspector, {
      props: {
        task: makeTask("failed"),
        events: [],
      },
    });

    await wrapper.find('input[type="checkbox"]').setValue(true);
    const retryButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "Retry");
    expect(retryButton).toBeTruthy();
    await retryButton?.trigger("click");

    expect(wrapper.emitted("retry")).toEqual([["dtask_1", true]]);
  });
});
