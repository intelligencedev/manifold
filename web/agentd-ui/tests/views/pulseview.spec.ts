import { render, screen, waitFor, within } from "@testing-library/vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import PulseView from "@/views/PulseView.vue";
import type { MatrixRoom, MatrixTask } from "@/api/matrix";

const matrixApiMocks = vi.hoisted(() => ({
  listMatrixRooms: vi.fn(),
  fetchMatrixRoomMessages: vi.fn(),
  listMatrixRoomTasks: vi.fn(),
  createMatrixRoomTask: vi.fn(),
  updateMatrixRoomTask: vi.fn(),
  deleteMatrixRoomTask: vi.fn(),
  setMatrixRoomTaskEnabled: vi.fn(),
  runMatrixRoomTaskNow: vi.fn(),
}));

vi.mock("@/api/matrix", () => matrixApiMocks);

const RouterLinkStub = {
  props: ["to"],
  template:
    '<a :data-route-name="to && to.name" :data-task-id="to && to.params && to.params.taskId"><slot /></a>',
};

describe("PulseView durable run states", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    matrixApiMocks.listMatrixRooms.mockResolvedValue([roomFixture()]);
    matrixApiMocks.fetchMatrixRoomMessages.mockResolvedValue([]);
    matrixApiMocks.listMatrixRoomTasks.mockResolvedValue([
      taskFixture({
        id: "queued",
        title: "Queued task",
        activeRunId: "durable-queued",
        activeRunStatus: "queued",
      }),
      taskFixture({
        id: "running",
        title: "Running task",
        activeRunId: "durable-running",
        activeRunStatus: "running",
      }),
      taskFixture({
        id: "waiting",
        title: "Waiting task",
        activeRunId: "durable-waiting",
        activeRunStatus: "waiting",
      }),
      taskFixture({
        id: "failed",
        title: "Failed task",
        lastRunId: "durable-failed",
        lastRunStatus: "failed",
      }),
      taskFixture({
        id: "completed",
        title: "Completed task",
        lastRunId: "durable-completed",
        lastRunStatus: "completed",
      }),
      taskFixture({
        id: "due",
        title: "Due task",
        due: true,
      }),
    ]);
  });

  it("renders durable task states, links, and active run button labels", async () => {
    render(PulseView, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
        },
      },
    });

    await screen.findByText("Queued task");

    expectCardState("Queued task", "queued", "Queued…", true);
    expectCardState("Running task", "running", "Running…", true);
    expectCardState("Waiting task", "waiting", "Waiting…", true);
    expectCardState("Failed task", "failed", "Run now", false);
    expectCardState("Completed task", "completed", "Run now", false);
    expectCardState("Due task", "due", "Run now", false);

    await waitFor(() => {
      expect(durableRunLink("Queued task")).toMatchObject({
        routeName: "durable-task",
        taskId: "durable-queued",
      });
      expect(durableRunLink("Completed task")).toMatchObject({
        routeName: "durable-task",
        taskId: "durable-completed",
      });
    });
  });
});

function expectCardState(
  title: string,
  state: string,
  buttonLabel: string,
  disabled: boolean,
) {
  const card = taskCard(title);
  expect(within(card).getByText(state)).toBeInTheDocument();
  const button = within(card).getByRole("button", { name: buttonLabel });
  if (disabled) {
    expect(button).toBeDisabled();
  } else {
    expect(button).toBeEnabled();
  }
}

function durableRunLink(title: string) {
  const link = within(taskCard(title))
    .getByText(/^Durable run:/)
    .closest("a");
  return {
    routeName: link?.getAttribute("data-route-name"),
    taskId: link?.getAttribute("data-task-id"),
  };
}

function taskCard(title: string) {
  const card = screen.getByText(title).closest(".halo-surface");
  if (!card) {
    throw new Error(`Task card not found for ${title}`);
  }
  return card as HTMLElement;
}

function roomFixture(): MatrixRoom {
  return {
    roomId: "!pulse:test",
    defaultTarget: "orchestrator",
    allowUnmentioned: false,
    mentions: {},
    maxConcurrent: 1,
    messageRetention: 100,
    sessionId: "session-1",
    stats: {
      roomId: "!pulse:test",
      messageCount: 0,
    },
    routes: [],
    taskCount: 6,
    enabledTaskCount: 6,
  };
}

function taskFixture(overrides: Partial<MatrixTask>): MatrixTask {
  return {
    id: "task",
    roomId: "!pulse:test",
    routeTarget: "orchestrator",
    title: "Pulse task",
    prompt: "Summarize the current state",
    scheduleType: "interval",
    scheduleLabel: "Every 5m",
    intervalSeconds: 300,
    intervalHuman: "5m",
    enabled: true,
    roomEnabled: true,
    due: false,
    lastRunAt: "",
    nextRunAt: "2026-06-10T12:05:00Z",
    nextRunHuman: "2026-06-10T12:05:00Z",
    createdAt: "2026-06-10T12:00:00Z",
    updatedAt: "2026-06-10T12:00:00Z",
    ...overrides,
  };
}
