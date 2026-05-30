import { describe, expect, it } from "vitest";
import { parseDatasetRows } from "@/lib/playgroundDatasetImport";

describe("playgroundDatasetImport", () => {
  it("parses JSON rows and maps non-reserved fields to inputs", () => {
    const rows = parseDatasetRows(
      JSON.stringify([
        {
          id: "ticket-1",
          customerName: "Alice",
          issue: "billing",
          expected: "Acknowledge",
          split: "validation",
        },
      ]),
      { format: "json" },
    );

    expect(rows).toEqual([
      {
        id: "ticket-1",
        inputs: { customerName: "Alice", issue: "billing" },
        expected: "Acknowledge",
        meta: undefined,
        split: "validation",
      },
    ]);
  });

  it("parses JSON objects with a rows array", () => {
    const rows = parseDatasetRows(
      JSON.stringify({
        rows: [{ id: "row-1", inputs: { question: "Hello" } }],
      }),
      { filename: "dataset.json" },
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].inputs).toEqual({ question: "Hello" });
  });

  it("parses JSONL rows and reports line errors", () => {
    const rows = parseDatasetRows(
      '{"id":"row-1","question":"Hello"}\n{"id":"row-2","question":"Hi"}',
      { filename: "dataset.jsonl" },
    );

    expect(rows.map((row) => row.id)).toEqual(["row-1", "row-2"]);
    expect(() =>
      parseDatasetRows('{"id":"row-1"}\n{bad}', { format: "jsonl" }),
    ).toThrow(/JSONL line 2 is invalid JSON/);
  });

  it("parses CSV rows with non-reserved columns as inputs", () => {
    const rows = parseDatasetRows(
      [
        "id,customerName,issue,expected,split,meta",
        'ticket-1,Alice,"billing, duplicate",Acknowledge,validation,"{""source"":""seed""}"',
        "ticket-2,Bob,password reset,,test,",
      ].join("\n"),
      { filename: "support.csv" },
    );

    expect(rows).toEqual([
      {
        id: "ticket-1",
        inputs: { customerName: "Alice", issue: "billing, duplicate" },
        expected: "Acknowledge",
        meta: { source: "seed" },
        split: "validation",
      },
      {
        id: "ticket-2",
        inputs: { customerName: "Bob", issue: "password reset" },
        expected: undefined,
        meta: undefined,
        split: "test",
      },
    ]);
  });

  it("merges CSV inputs JSON with non-reserved columns", () => {
    const rows = parseDatasetRows(
      'id,inputs,question\nrow-1,"{""topic"":""support""}",Hello',
      { format: "csv" },
    );

    expect(rows[0].inputs).toEqual({ topic: "support", question: "Hello" });
  });

  it("rejects malformed or ambiguous datasets", () => {
    expect(() =>
      parseDatasetRows(
        '[{"id":"dup","question":"A"},{"id":"dup","question":"B"}]',
        {
          format: "json",
        },
      ),
    ).toThrow(/Duplicate row id "dup"/);

    expect(() => parseDatasetRows("id,id\n1,2", { format: "csv" })).toThrow(
      /CSV header "id" is duplicated/,
    );

    expect(() => parseDatasetRows("id\nrow-1", { format: "csv" })).toThrow(
      /must include inputs/,
    );

    expect(() =>
      parseDatasetRows("id,meta\nrow-1,bad", { format: "csv" }),
    ).toThrow(/CSV row 2 meta is invalid JSON/);

    expect(() => parseDatasetRows("[]", { filename: "dataset.txt" })).toThrow(
      /Unsupported dataset file type/,
    );
  });
});
