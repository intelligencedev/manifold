import { describe, expect, it } from "vitest";
import type { ExperimentSpec, Run, RunResult } from "@/api/playground";
import {
  buildRunExportReport,
  runExportFilename,
  serializeRunExportReport,
} from "@/lib/playgroundRunExport";

describe("playgroundRunExport", () => {
  const experiment: ExperimentSpec = {
    id: "exp-1",
    name: "Support Eval",
    datasetId: "dataset-1",
    variants: [],
    createdAt: "2026-05-19T00:00:00Z",
  };
  const run: Run = {
    id: "run-1",
    experimentId: "exp-1",
    status: "completed",
    createdAt: "2026-05-19T00:00:00Z",
    plan: {
      shards: [
        {
          id: "shard-1",
          rows: [
            {
              id: "row-1",
              inputs: { customerName: "Alice", issue: "billing, duplicate" },
              expected: "Acknowledge",
            },
          ],
          variants: [],
        },
      ],
    },
  };
  const results: RunResult[] = [
    {
      id: "result-1",
      runId: "run-1",
      rowId: "row-1",
      variantId: "variant-a",
      expected: "Override expected",
      rendered: "Hello Alice\nIssue: billing, duplicate",
      output: 'Output with "quotes"',
    },
  ];

  it("builds a report with inputs from the run plan and result output fields", () => {
    const report = buildRunExportReport(
      experiment,
      run,
      results,
      "2026-05-19T01:02:03Z",
    );

    expect(report).toEqual({
      experimentId: "exp-1",
      experimentName: "Support Eval",
      runId: "run-1",
      exportedAt: "2026-05-19T01:02:03Z",
      rows: [
        {
          rowId: "row-1",
          variantId: "variant-a",
          inputs: { customerName: "Alice", issue: "billing, duplicate" },
          expected: "Override expected",
          renderedPrompt: "Hello Alice\nIssue: billing, duplicate",
          output: 'Output with "quotes"',
        },
      ],
    });
  });

  it("serializes report rows as JSON and CSV", () => {
    const report = buildRunExportReport(
      experiment,
      run,
      results,
      "2026-05-19T01:02:03Z",
    );

    expect(JSON.parse(serializeRunExportReport(report, "json"))).toEqual(
      report,
    );
    expect(serializeRunExportReport(report, "csv")).toBe(
      [
        "rowId,variantId,inputs,expected,renderedPrompt,output",
        'row-1,variant-a,"{""customerName"":""Alice"",""issue"":""billing, duplicate""}",Override expected,"Hello Alice',
        'Issue: billing, duplicate","Output with ""quotes"""',
        "",
      ].join("\n"),
    );
  });

  it("creates a stable filename", () => {
    expect(runExportFilename(experiment, run, "csv")).toBe(
      "support-eval-run-1-results.csv",
    );
  });
});
