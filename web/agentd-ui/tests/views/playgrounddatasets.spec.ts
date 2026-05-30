import { fireEvent, render } from "@testing-library/vue";
import { describe, expect, it, vi } from "vitest";
import PlaygroundDatasetsView from "@/views/playground/PlaygroundDatasetsView.vue";

vi.mock("@/api/playground", () => ({
  createPrompt: async (payload: any) => payload,
  getPrompt: async (id: string) => ({ id, name: "Prompt", createdAt: "" }),
  deletePrompt: async () => {},
  createPromptVersion: async (_id: string, payload: any) => payload,
  listPrompts: async () => [],
  listPromptVersions: async () => [],
  createDataset: async (payload: any) => ({
    id: "dataset-1",
    name: payload.dataset.name,
    rows: payload.rows,
    createdAt: "2026-05-19T00:00:00Z",
  }),
  updateDataset: async (id: string, payload: any) => ({
    id,
    name: payload.dataset.name,
    rows: payload.rows,
    createdAt: "2026-05-19T00:00:00Z",
  }),
  getDataset: async (id: string) => ({
    id,
    name: "Dataset",
    rows: [],
    createdAt: "2026-05-19T00:00:00Z",
  }),
  deleteDataset: async () => {},
  listDatasets: async () => [],
  listExperiments: async () => [],
  createExperiment: async (spec: any) => spec,
  getExperiment: async (id: string) => ({
    id,
    name: "Experiment",
    datasetId: "dataset-1",
    variants: [],
    createdAt: "2026-05-19T00:00:00Z",
  }),
  deleteExperiment: async () => {},
  startExperimentRun: async () => ({
    id: "run-1",
    experimentId: "experiment-1",
    plan: { shards: [] },
    status: "completed",
    createdAt: "2026-05-19T00:00:00Z",
  }),
  listExperimentRuns: async () => [],
  listRunResults: async () => [],
}));

describe("PlaygroundDatasetsView", () => {
  it("shows a validation error when an imported dataset file is invalid", async () => {
    const { findByLabelText, findByText } = render(PlaygroundDatasetsView);

    const input = (await findByLabelText(/Import file/)) as HTMLInputElement;
    const file = new File(["id\nrow-1"], "bad.csv", { type: "text/csv" });

    await fireEvent.change(input, { target: { files: [file] } });

    expect(await findByText(/Row 1 must include inputs/)).toBeInTheDocument();
  });
});
