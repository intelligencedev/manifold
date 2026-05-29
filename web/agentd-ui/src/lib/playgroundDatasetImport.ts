import type { DatasetRow } from "@/api/playground";

type DatasetImportFormat = "json" | "jsonl" | "csv";

const reservedCSVColumns = new Set([
  "id",
  "inputs",
  "expected",
  "meta",
  "split",
]);
const reservedRowFields = new Set([
  "id",
  "inputs",
  "expected",
  "meta",
  "split",
]);

export function parseDatasetRows(
  input: string,
  options: { filename?: string; format?: DatasetImportFormat } = {},
): DatasetRow[] {
  const text = input.trim();
  if (!text) {
    return [];
  }

  const format = options.format ?? formatFromFilename(options.filename);
  if (!format) {
    throw new Error(
      "Unsupported dataset file type. Upload .json, .jsonl, or .csv.",
    );
  }

  const rows =
    format === "json"
      ? parseJSONRows(text)
      : format === "jsonl"
        ? parseJSONLRows(text)
        : parseCSVRows(input);

  return validateDatasetRows(rows);
}

export function formatRowsForEditor(rows: DatasetRow[]): string {
  if (!rows.length) {
    return "[]";
  }
  try {
    return JSON.stringify(rows, null, 2);
  } catch {
    return "[]";
  }
}

function formatFromFilename(
  filename?: string,
): DatasetImportFormat | undefined {
  const lower = filename?.trim().toLowerCase() ?? "";
  if (lower.endsWith(".json")) return "json";
  if (lower.endsWith(".jsonl") || lower.endsWith(".ndjson")) return "jsonl";
  if (lower.endsWith(".csv")) return "csv";
  return undefined;
}

function parseJSONRows(text: string): unknown[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch (err) {
    throw new Error(`Rows must be valid JSON: ${jsonErrorMessage(err)}`);
  }
  if (Array.isArray(parsed)) {
    return parsed;
  }
  if (isPlainObject(parsed) && Array.isArray(parsed.rows)) {
    return parsed.rows;
  }
  throw new Error(
    "JSON dataset must be an array of rows or an object with a rows array.",
  );
}

function parseJSONLRows(text: string): unknown[] {
  const rows: unknown[] = [];
  const lines = text.split(/\r?\n/);
  for (let idx = 0; idx < lines.length; idx += 1) {
    const line = lines[idx].trim();
    if (!line) {
      continue;
    }
    try {
      rows.push(JSON.parse(line));
    } catch (err) {
      throw new Error(
        `JSONL line ${idx + 1} is invalid JSON: ${jsonErrorMessage(err)}`,
      );
    }
  }
  return rows;
}

function parseCSVRows(text: string): unknown[] {
  const records = parseCSVRecords(text);
  if (!records.length) {
    return [];
  }

  const [headerRecord, ...dataRecords] = records;
  const headers = headerRecord.values.map((value) => value.trim());
  if (!headers.length || headers.every((header) => !header)) {
    throw new Error("CSV header row is required.");
  }

  const seenHeaders = new Set<string>();
  for (const header of headers) {
    if (!header) {
      throw new Error("CSV headers cannot be empty.");
    }
    const normalized = header.toLowerCase();
    if (seenHeaders.has(normalized)) {
      throw new Error(`CSV header "${header}" is duplicated.`);
    }
    seenHeaders.add(normalized);
  }

  return dataRecords
    .filter((record) => record.values.some((value) => value.trim() !== ""))
    .map((record, idx) => csvRecordToRow(record, headers, idx));
}

function csvRecordToRow(
  record: CSVRecord,
  headers: string[],
  idx: number,
): Record<string, unknown> {
  if (record.values.length > headers.length) {
    throw new Error(
      `CSV row ${record.line} has ${record.values.length} values but only ${headers.length} headers.`,
    );
  }

  const row: Record<string, unknown> = {};
  const inputs: Record<string, unknown> = {};
  for (let colIdx = 0; colIdx < headers.length; colIdx += 1) {
    const header = headers[colIdx];
    const key = header.toLowerCase();
    const value = record.values[colIdx] ?? "";
    if (!reservedCSVColumns.has(key)) {
      inputs[header] = value;
      continue;
    }

    if (key === "inputs") {
      if (value.trim()) {
        Object.assign(
          inputs,
          parseJSONField(value, "inputs", record.line, true),
        );
      }
      continue;
    }
    if (key === "meta") {
      if (value.trim()) {
        row.meta = parseJSONField(value, "meta", record.line, true);
      }
      continue;
    }
    if (key === "expected") {
      row.expected = parseFlexibleJSONField(value);
      continue;
    }
    if (key === "id") {
      row.id = value;
      continue;
    }
    if (key === "split") {
      row.split = value;
    }
  }

  return {
    ...row,
    inputs,
    id: typeof row.id === "string" && row.id.trim() ? row.id : `row-${idx + 1}`,
  };
}

interface CSVRecord {
  values: string[];
  line: number;
}

function parseCSVRecords(text: string): CSVRecord[] {
  return new CSVRecordParser(text).parse();
}

class CSVRecordParser {
  private readonly records: CSVRecord[] = [];
  private values: string[] = [];
  private field = "";
  private inQuotes = false;
  private fieldQuoted = false;
  private line = 1;
  private recordLine = 1;

  constructor(private readonly text: string) {}

  parse(): CSVRecord[] {
    for (let idx = 0; idx < this.text.length; idx += 1) {
      const nextIdx = this.consumeChar(idx);
      idx = nextIdx;
    }
    this.finish();
    return this.records.filter((record) =>
      record.values.some((value) => value.trim() !== ""),
    );
  }

  private consumeChar(idx: number): number {
    const char = this.text[idx];
    const next = this.text[idx + 1];

    if (this.inQuotes) {
      if (char === '"') {
        if (next === '"') {
          this.field += '"';
          return idx + 1;
        } else {
          this.inQuotes = false;
        }
      } else {
        this.field += char;
        if (char === "\n") {
          this.line += 1;
        }
      }
      return idx;
    }

    if (char === '"') {
      if (this.field.trim().length > 0) {
        throw new Error(
          `CSV quote appears inside an unquoted field at line ${this.line}.`,
        );
      }
      this.inQuotes = true;
      this.fieldQuoted = true;
      return idx;
    }

    if (char === ",") {
      this.pushField();
      return idx;
    }

    if (char === "\n") {
      this.pushRecord();
      this.line += 1;
      this.recordLine = this.line;
      return idx;
    }

    if (char === "\r") {
      if (next === "\n") {
        return idx;
      }
      this.pushRecord();
      this.line += 1;
      this.recordLine = this.line;
      return idx;
    }

    if (this.fieldQuoted) {
      if (char.trim().length > 0) {
        throw new Error(
          `CSV has text after a closing quote at line ${this.line}.`,
        );
      }
      return idx;
    }
    this.field += char;
    return idx;
  }

  private finish() {
    if (this.inQuotes) {
      throw new Error(
        `CSV has an unterminated quoted field starting near line ${this.recordLine}.`,
      );
    }
    if (this.field.length > 0 || this.values.length > 0) {
      this.pushRecord();
    }
  }

  private pushField() {
    this.values.push(this.field);
    this.field = "";
    this.fieldQuoted = false;
  }

  private pushRecord() {
    this.pushField();
    this.records.push({ values: this.values, line: this.recordLine });
    this.values = [];
    this.recordLine = this.line;
  }
}

function validateDatasetRows(rows: unknown[]): DatasetRow[] {
  const normalized = rows.map((row, idx) => normalizeDatasetRow(row, idx));
  const seen = new Set<string>();
  for (const row of normalized) {
    if (seen.has(row.id)) {
      throw new Error(`Duplicate row id "${row.id}".`);
    }
    seen.add(row.id);
  }
  return normalized;
}

function normalizeDatasetRow(row: unknown, idx: number): DatasetRow {
  if (!isPlainObject(row)) {
    throw new Error(`Row ${idx + 1} must be an object.`);
  }

  const id =
    typeof row.id === "string" && row.id.trim().length > 0
      ? row.id.trim()
      : `row-${idx + 1}`;
  const inputs = normalizeInputs(row, idx);
  const split =
    typeof row.split === "string" && row.split.trim().length > 0
      ? row.split.trim()
      : "train";

  if (row.meta !== undefined && !isPlainObject(row.meta)) {
    throw new Error(`Row ${idx + 1} meta must be an object.`);
  }

  return {
    id,
    inputs,
    expected: row.expected,
    meta: row.meta as Record<string, unknown> | undefined,
    split,
  };
}

function normalizeInputs(
  row: Record<string, unknown>,
  idx: number,
): Record<string, unknown> {
  if (row.inputs !== undefined) {
    if (!isPlainObject(row.inputs)) {
      throw new Error(`Row ${idx + 1} inputs must be an object.`);
    }
    const inputs = { ...row.inputs };
    if (Object.keys(inputs).length === 0) {
      throw new Error(
        `Row ${idx + 1} must include inputs or at least one input column.`,
      );
    }
    return inputs;
  }

  const inputs: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(row)) {
    if (!reservedRowFields.has(key)) {
      inputs[key] = value;
    }
  }
  if (Object.keys(inputs).length === 0) {
    throw new Error(
      `Row ${idx + 1} must include inputs or at least one input column.`,
    );
  }
  return inputs;
}

function parseJSONField(
  value: string,
  field: string,
  line: number,
  requireObject: boolean,
): unknown {
  try {
    const parsed = JSON.parse(value);
    if (requireObject && !isPlainObject(parsed)) {
      throw new Error(`${field} must be a JSON object`);
    }
    return parsed;
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    throw new Error(`CSV row ${line} ${field} is invalid JSON: ${message}`);
  }
}

function parseFlexibleJSONField(value: string): unknown {
  const trimmed = value.trim();
  if (!trimmed) {
    return undefined;
  }
  if (!/^(true|false|null|-?\d|\{|\[|")/.test(trimmed)) {
    return value;
  }
  try {
    return JSON.parse(trimmed);
  } catch {
    return value;
  }
}

function jsonErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function isPlainObject(value: unknown): value is Record<string, any> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}
