# Expression Binding Demo

This example workflow demonstrates how to reference upstream node outputs in downstream node inputs using expression bindings.

## Workflow Structure

```
[prompt-input] → [search-step] → [display-results]
  (Textbox)      (Web Search)       (Textbox)
```

1. **prompt-input** — A utility textbox containing a research topic
2. **search-step** — A web search whose `query` parameter is bound to the textbox's output text
3. **display-results** — A utility textbox that displays the raw search results payload

## Expression Syntax

Expressions are strings that reference data from upstream nodes or the workflow run input.

### Reference upstream node output

```
={{$node.<step-id>.output}}           → full output object
={{$node.<step-id>.output.<field>}}   → specific field from output
```

### Reference workflow run input

```
={{$run.input}}         → full run input object
={{$run.input.query}}   → the query/prompt passed when running the workflow
```

### How it works

When a tool node executes, its JSON response is parsed and the top-level keys become output fields. For example, a `utility_textbox` tool returns:

```json
{"text": "Hello world", "output_attr": ""}
```

Its output map becomes:
- `payload` — raw JSON string
- `json` — parsed JSON object
- `text` — `"Hello world"` (spread from response)

So `={{$node.my-textbox.output.text}}` resolves to `"Hello world"`.

### Legacy syntax

Older workflows may use `${A.<step-id>.json.<path>}`. This is automatically converted to the expression format when imported. Both syntaxes are supported.

## Import Instructions

1. Open the Flow Editor in the Manifold UI
2. Click the **Import** button (or use the menu)
3. Select `expression_binding_demo.json`
4. The workflow will appear with three connected nodes
5. Click **Run** to execute

After running, you can click the `{x}` button on any input field to open the expression picker and see available upstream outputs.
