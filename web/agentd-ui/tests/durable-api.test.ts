import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  cancelDurableTask,
  fetchDurableQueues,
  fetchDurableTask,
  fetchDurableTaskEvents,
  listDurableTasks,
  retryDurableTask,
} from "@/api/durable";
import { apiClient } from "@/api/client";

vi.mock("@/api/client", () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

const mockedApi = vi.mocked(apiClient);

describe("durable API client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("lists queues through the shared API client", async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: { queues: [{ queue: "default", queued: 1 }] },
    });

    await expect(fetchDurableQueues()).resolves.toEqual([
      { queue: "default", queued: 1 },
    ]);
    expect(mockedApi.get).toHaveBeenCalledWith("/durable/queues");
  });

  it("lists tasks with compact filters", async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: { tasks: [{ id: "dtask_1", queue: "ops", name: "deploy" }] },
    });

    await expect(
      listDurableTasks({ queue: "ops", status: "", name: "", limit: 50 }),
    ).resolves.toEqual([{ id: "dtask_1", queue: "ops", name: "deploy" }]);
    expect(mockedApi.get).toHaveBeenCalledWith("/durable/tasks", {
      params: { queue: "ops", limit: 50 },
    });
  });

  it("fetches task detail, events, cancellation, and retry endpoints", async () => {
    mockedApi.get
      .mockResolvedValueOnce({ data: { task: { id: "dtask_1" } } })
      .mockResolvedValueOnce({
        data: { task_id: "dtask_1", status: "queued", events: [] },
      });
    mockedApi.post.mockResolvedValueOnce({ data: {} });
    mockedApi.post.mockResolvedValueOnce({
      data: { task: { id: "dtask_1", status: "queued" } },
    });

    await expect(fetchDurableTask("dtask_1")).resolves.toEqual({
      id: "dtask_1",
    });
    await expect(fetchDurableTaskEvents("dtask_1")).resolves.toEqual({
      task_id: "dtask_1",
      status: "queued",
      events: [],
    });
    await expect(cancelDurableTask("dtask_1")).resolves.toBeUndefined();
    await expect(retryDurableTask("dtask_1", true)).resolves.toEqual({
      id: "dtask_1",
      status: "queued",
    });

    expect(mockedApi.get).toHaveBeenNthCalledWith(1, "/durable/tasks/dtask_1");
    expect(mockedApi.get).toHaveBeenNthCalledWith(
      2,
      "/durable/tasks/dtask_1/events",
    );
    expect(mockedApi.post).toHaveBeenCalledWith(
      "/durable/tasks/dtask_1/cancel",
    );
    expect(mockedApi.post).toHaveBeenCalledWith(
      "/durable/tasks/dtask_1/retry",
      {
        reset_checkpoints: true,
      },
    );
  });
});
