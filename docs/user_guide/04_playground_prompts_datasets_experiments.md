---
title: Playground — Prompts, Datasets, Experiments
description: Define reusable prompt templates, upload small datasets, and run head-to-head experiments.
---

# Playground: Prompts, Datasets, Experiments

The Playground groups three related areas: Prompt definitions with versioning, small JSON datasets for quick evaluation, and Experiments that tie prompts and datasets to run variants and collect runs.

Placeholder for screenshots: [Playground overview tiles]

## Prompts

Overview
- A Prompt is a named definition with metadata and one or more Versions. Each Version has a template string, optional variables/guardrails JSON, and an optional semantic version.

Manage prompts
- Create prompt: fill Name, optional Tags and Description; click Create.
- List: filter by name/tag; navigate to details via the prompt name link.
- Delete: Delete removes the prompt and all its versions.

Prompt details and versions
- Left panel lists versions (most recent first) with semver, created time, and hash.
- Right panel shows the selected version and a form to create a new version.
- Create version: enter semver (optional), template (required), and optional variables/guardrails JSON; click Create version.
- Load into form: loads the selected version’s content to jump-start a new revision.

Integration with Specialists
- In Specialists, “Apply saved prompt version” lets you populate a specialist’s System Prompt from any prompt/version you created here.

Placeholder for screenshots: [Prompts list; Version detail]

## Datasets

Overview
- Datasets are small JSON arrays you can paste/upload for quick experiments. Each row has at minimum an id and inputs, and may include expected, meta, and split fields.

Create dataset
- Provide Name, optional Tags and Description.
- Paste a JSON array of rows in the Rows field. Example:

  [
    { "id": "sample-1", "inputs": { "question": "Hello" }, "expected": "Hi" }
  ]

- Click Create dataset.

Browse and edit
- Select a dataset to open details. Left card: Properties (name/tags/description). Right card: Rows with Table and JSON views.
- Table view shows a preview of up to 50 rows; JSON view allows direct editing of the entire array.
- Save changes writes metadata and updated rows.
- Delete removes the dataset and all rows.

Validation
- Rows must be a JSON array; the UI normalizes rows (ensures id and split, carries inputs/expected/meta through). JSON errors are surfaced inline.

Placeholder for screenshots: [Datasets list; Dataset details table]

## Experiments

Overview
- An Experiment binds a dataset to one or more variants (typically model + prompt version) and records runs.

Create experiment
- Fill Name.
- Choose Dataset, Prompt, then Prompt version (versions list loads on prompt select).
- Enter Model, optional Slice (e.g., validation), and Notes.
- The UI constructs a spec with a single variant referencing the prompt version and model.

Manage experiments and runs
- Experiments list shows each with dataset id and variant count; actions: Details, Start run, Delete.
- Start run triggers an asynchronous run; expand “Show runs” to view run history with status and timestamps.
- The system polls run status per experiment; you can stop polling by collapsing.

Placeholder for screenshots: [New experiment form; Runs list]

Notes
- Playground storage is server-side; it’s separate from Projects. Prompts integrate with Specialists for system prompt templating.

