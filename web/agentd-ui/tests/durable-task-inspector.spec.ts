import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import TaskInspector from "@/components/durable/TaskInspector.vue";
import type {
  DurableEvent,
  DurableTask,
  DurableTaskEventsResponse,
} from "@/api/durable";

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

function makeEvent(
  id: number,
  name: string,
  payload: DurableEvent["payload"],
): DurableEvent {
  return {
    id,
    task_id: "dtask_1",
    queue: "flow_v2",
    name,
    sequence: id,
    payload,
    occurred_at: "2026-05-18T13:01:00Z",
  };
}

function makeEventPage(events: DurableEvent[]): DurableTaskEventsResponse {
  return {
    task_id: "dtask_1",
    status: "running",
    events,
    limit: 2,
    first_sequence: events[0]?.sequence,
    last_sequence: events[events.length - 1]?.sequence,
    has_more_before: true,
    has_more_after: true,
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

  it("renders collapsible timeline events and highlights failed events", () => {
    const wrapper = mount(TaskInspector, {
      props: {
        task: makeTask("failed"),
        events: [
          makeEvent(1, "flow.run_started", { status: "running" }),
          makeEvent(2, "flow.node_failed", {
            status: "failed",
            error: "tool exploded",
          }),
        ],
      },
    });

    const eventRows = wrapper.findAll("ol details");
    expect(eventRows).toHaveLength(2);
    expect(eventRows[0].attributes("open")).toBeUndefined();
    expect(eventRows[1].attributes("open")).toBeDefined();
    expect(eventRows[1].classes()).toContain("border-danger/60");
    expect(eventRows[1].text()).toContain("failed");

    const eventPayloads = wrapper.findAll("ol pre");
    expect(eventPayloads).toHaveLength(2);
    expect(eventPayloads[0].classes()).toContain("whitespace-pre-wrap");
    expect(eventPayloads[0].classes()).toContain("break-words");
    expect(eventPayloads[0].classes()).not.toContain("overflow-auto");
    expect(
      eventPayloads[0]
        .classes()
        .some((className) => className.startsWith("max-h")),
    ).toBe(false);
  });

  it("emits event pagination actions from the timeline controls", async () => {
    const events = [
      makeEvent(2, "flow.step", { status: "running" }),
      makeEvent(3, "flow.step", { status: "running" }),
    ];
    const wrapper = mount(TaskInspector, {
      props: {
        task: makeTask("running"),
        events,
        eventsPage: makeEventPage(events),
        showClose: false,
      },
    });

    const buttons = wrapper.findAll("button");
    await buttons.find((button) => button.text() === "Older")?.trigger("click");
    await buttons.find((button) => button.text() === "Newer")?.trigger("click");
    await buttons
      .find((button) => button.text() === "Latest")
      ?.trigger("click");

    expect(wrapper.text()).toContain("#2 - #3");
    expect(wrapper.emitted("eventsOlder")).toHaveLength(1);
    expect(wrapper.emitted("eventsNewer")).toHaveLength(1);
    expect(wrapper.emitted("eventsLatest")).toHaveLength(1);
  });
});
