import { fireEvent, render, waitFor } from "@testing-library/vue";
import { describe, expect, it, vi, beforeEach } from "vitest";
import PlaygroundExperimentsView from "@/views/playground/PlaygroundExperimentsView.vue";

const playgroundMocks = vi.hoisted(() => ({
  createExperiment: vi.fn(async (spec: any) => spec),
}));

vi.mock("@/api/client", () => ({
  listSpecialists: async () => [
    { name: "orchestrator", model: "gpt-5", paused: false },
    { name: "runner", model: "special-model", paused: false },
    { name: "paused-runner", model: "special-model", paused: true },
  ],
}));

vi.mock("@/api/playground", () => ({
  createPrompt: async (payload: any) => payload,
  getPrompt: async (id: string) => ({
    id,
    name: "Greeting",
    createdAt: "2026-05-19T00:00:00Z",
  }),
  deletePrompt: async () => {},
  createPromptVersion: async (_id: string, payload: any) => payload,
  listPrompts: async () => [
    {
      id: "prompt-1",
      name: "Greeting",
      createdAt: "2026-05-19T00:00:00Z",
    },
  ],
  listPromptVersions: async () => [
    {
      id: "version-1",
      promptId: "prompt-1",
      semver: "1.0.0",
      template: "Hello {{name}}",
      createdAt: "2026-05-19T00:00:00Z",
    },
  ],
  createDataset: async (payload: any) => ({
    id: payload.dataset?.id || "dataset-new",
    name: payload.dataset?.name || "Dataset",
    createdAt: "2026-05-19T00:00:00Z",
  }),
  updateDataset: async (id: string, payload: any) => ({
    id,
    name: payload.dataset?.name || "Dataset",
    rows: payload.rows,
    createdAt: "2026-05-19T00:00:00Z",
  }),
  getDataset: async (id: string) => ({
    id,
    name: "Samples",
    rows: [],
    createdAt: "2026-05-19T00:00:00Z",
  }),
  deleteDataset: async () => {},
  listDatasets: async () => [
    {
      id: "dataset-1",
      name: "Samples",
      createdAt: "2026-05-19T00:00:00Z",
    },
  ],
  listExperiments: async () => [
    {
      id: "existing-exp",
      name: "Existing",
      datasetId: "dataset-1",
      variants: [{ id: "variant-0", promptVersionId: "version-1", model: "" }],
      execution: { specialistName: "runner" },
      createdAt: "2026-05-19T00:00:00Z",
    },
  ],
  createExperiment: playgroundMocks.createExperiment,
  getExperiment: async (id: string) => ({
    id,
    name: "Existing",
    datasetId: "dataset-1",
    variants: [],
    createdAt: "2026-05-19T00:00:00Z",
  }),
  deleteExperiment: async () => {},
  startExperimentRun: async () => ({
    id: "run-1",
    experimentId: "existing-exp",
    plan: { shards: [] },
    status: "completed",
    createdAt: "2026-05-19T00:00:00Z",
  }),
  listExperimentRuns: async () => [],
  listRunResults: async () => [],
}));

describe("PlaygroundExperimentsView", () => {
  beforeEach(() => {
    playgroundMocks.createExperiment.mockClear();
  });

  it("submits a selected specialist runner and disables the direct model input", async () => {
    const {
      findAllByText,
      findByLabelText,
      findByRole,
      findByText,
      queryByText,
    } = render(PlaygroundExperimentsView);

    expect(await findAllByText(/Runner: Specialist: runner/)).not.toHaveLength(
      0,
    );
    expect(queryByText(/paused-runner/)).toBeNull();

    const name = (await findByLabelText("Name")) as HTMLInputElement;
    const dataset = (await findByLabelText("Dataset")) as HTMLSelectElement;
    const prompt = (await findByLabelText("Prompt")) as HTMLSelectElement;
    const model = (await findByLabelText("Model")) as HTMLInputElement;
    const runner = (await findByLabelText(
      "Specialist runner",
    )) as HTMLSelectElement;

    expect(model).toBeRequired();
    await fireEvent.update(runner, "runner");
    expect(model).toBeDisabled();

    await fireEvent.update(name, "Tool experiment");
    await fireEvent.update(dataset, "dataset-1");
    await fireEvent.update(prompt, "prompt-1");
    await findByText("1.0.0");

    const version = (await findByLabelText(
      "Prompt version",
    )) as HTMLSelectElement;
    await fireEvent.update(version, "version-1");
    await fireEvent.click(
      await findByRole("button", { name: /Create experiment/i }),
    );

    await waitFor(() => {
      expect(playgroundMocks.createExperiment).toHaveBeenCalledTimes(1);
    });
    const submitted = playgroundMocks.createExperiment.mock.calls[0][0];
    expect(submitted.execution).toEqual({ specialistName: "runner" });
    expect(submitted.variants[0].model).toBe("");
  });
});
