import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";
import openapiTS from "openapi-typescript";
import ts from "typescript";

const root = path.resolve(process.cwd());
const localSpec = path.resolve(root, "../docs/openapi/openapi.json");
const output = path.resolve(root, "src/api/schema.d.ts");

const schemaAst = await openapiTS(pathToFileURL(localSpec));
const schema = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed }).printList(
  ts.ListFormat.MultiLine,
  ts.factory.createNodeArray(schemaAst),
  ts.createSourceFile("schema.d.ts", "", ts.ScriptTarget.Latest, false, ts.ScriptKind.TS)
);
await fs.mkdir(path.dirname(output), { recursive: true });
await fs.writeFile(output, schema);
console.log(`Wrote ${output}`);
