import type { ExperimentSpec, Run, RunResult } from "@/api/playground";

export type RunExportFormat = "json" | "csv";

export interface RunExportRecord {
  rowId: string;
  variantId: string;
  inputs: Record<string, unknown>;
  expected: unknown;
  renderedPrompt: string;
  output: string;
}

export interface RunExportReport {
  experimentId: string;
  experimentName?: string;
  runId: string;
  exportedAt: string;
  rows: RunExportRecord[];
}

interface PlannedRow {
  id?: string;
  inputs?: Record<string, unknown>;
  expected?: unknown;
}

export function buildRunExportReport(
  experiment: ExperimentSpec | null,
  run: Run,
  results: RunResult[],
  exportedAt = new Date().toISOString(),
): RunExportReport {
  const rowsByID = plannedRowsByID(run);
  return {
    experimentId: run.experimentId,
    experimentName: experiment?.name,
    runId: run.id,
    exportedAt,
    rows: results.map((result) => {
      const planned = rowsByID.get(result.rowId);
      const expected =
        result.expected !== undefined
          ? result.expected
          : planned?.expected !== undefined
            ? planned.expected
            : null;
      return {
        rowId: result.rowId,
        variantId: result.variantId,
        inputs: planned?.inputs ? { ...planned.inputs } : {},
        expected,
        renderedPrompt: result.rendered ?? "",
        output: result.output ?? "",
      };
    }),
  };
}

export function serializeRunExportReport(
  report: RunExportReport,
  format: RunExportFormat,
): string {
  return format === "json" ? serializeJSON(report) : serializeCSV(report.rows);
}

export function runExportFilename(
  experiment: ExperimentSpec | null,
  run: Run,
  format: RunExportFormat,
): string {
  const prefix = experiment?.name?.trim() || run.experimentId || "experiment";
  const safePrefix = prefix
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return `${safePrefix || "experiment"}-${run.id}-results.${format}`;
}

function plannedRowsByID(run: Run): Map<string, PlannedRow> {
  const rows = new Map<string, PlannedRow>();
  for (const shard of run.plan?.shards ?? []) {
    for (const row of shard.rows ?? []) {
      const planned = row as PlannedRow;
      if (typeof planned.id === "string" && planned.id.trim()) {
        rows.set(planned.id, planned);
      }
    }
  }
  return rows;
}

function serializeJSON(report: RunExportReport): string {
  return JSON.stringify(report, null, 2);
}

function serializeCSV(rows: RunExportRecord[]): string {
  const headers: Array<keyof RunExportRecord> = [
    "rowId",
    "variantId",
    "inputs",
    "expected",
    "renderedPrompt",
    "output",
  ];
  const lines = [
    headers.join(","),
    ...rows.map((row) =>
      headers.map((header) => csvCell(row[header])).join(","),
    ),
  ];
  return `${lines.join("\n")}\n`;
}

function csvCell(value: unknown): string {
  let text: string;
  if (value === null || value === undefined) {
    text = "";
  } else if (typeof value === "string") {
    text = value;
  } else {
    text = JSON.stringify(value);
  }
  if (/[",\r\n]/.test(text)) {
    return `"${text.replace(/"/g, '""')}"`;
  }
  return text;
}
