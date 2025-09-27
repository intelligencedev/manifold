# Playground Tutorial

This tutorial walks through the full workflow for the Manifold playground: creating a versioned prompt, uploading a dataset, configuring an experiment, and running it from the web UI. By the end you will have baseline metrics for a toy experiment and know where to inspect runs and evaluator outputs.

> **Prerequisites**
>
> - `agentd` is running with database access (set `databases.defaultDSN` in `config.yaml` or env).
> - The web UI is accessible (default `http://localhost:8081`).
> - Administration access to the UI (if authentication is enabled).

## 1. Create a Prompt and Version

1. Open the UI and click **Playground → Prompts**.
2. In the **Create Prompt** form:
   - **Name**: `Support Greeting`
   - **Description**: `Greets a customer and echoes their issue.`
   - **Tags**: `support,greeting`
   - Click **Create**.
3. In the prompt list, select **Support Greeting**. On the detail page fill the **Create Version** form:
   - **Version**: `1.0.0`
   - **Template**:
     ```
     Hello {{customerName}},

     Thanks for contacting us about {{issue}}. Let me look into that for you.

     Summary: {{summary}}
     ```
   - **Variables** (JSON):
     ```json
     {
       "customerName": {"type": "string", "description": "Name of the customer", "required": true},
       "issue": {"type": "string", "required": true},
       "summary": {"type": "string", "required": false}
     }
     ```
   - **Guardrails** (optional) – leave blank.
   - Click **Create version**. The version appears in the list with its content hash.

## 2. Upload a Dataset

1. Navigate to **Playground → Datasets**.
2. In **Upload Dataset**, enter:
   - **Name**: `Support Samples`
   - **Tags**: `support`
   - **Rows (JSON array)**:
     ```json
     [
       {
         "id": "ticket-1",
         "inputs": {
           "customerName": "Alice",
           "issue": "billing discrepancy",
           "summary": "Customer was double-charged."
         },
         "expected": "Acknowledge the billing issue and promise a fix.",
         "split": "validation"
       },
       {
         "id": "ticket-2",
         "inputs": {
           "customerName": "Bob",
           "issue": "password reset",
           "summary": "Customer forgot password and requests reset link."
         },
         "expected": "Guide user through secure reset steps.",
         "split": "validation"
       }
     ]
     ```
3. Click **Create dataset**. The dataset appears in the list and is available for experiments.

## 3. Configure an Experiment

1. Go to **Playground → Experiments**.
2. In the **New Experiment** form:
   - **Name**: `Support Greeting Baseline`
   - **Dataset**: select `Support Samples`
   - **Prompt**: select `Support Greeting`
   - **Prompt version**: choose `1.0.0`
   - **Model**: e.g. `gpt-4o`
   - **Slice (optional)**: leave blank to use the initial snapshot
   - Click **Create experiment**.
3. The new experiment is shown in the experiments list. Click **Details** to review the spec and variants.

## 4. Run the Experiment

1. On the experiment detail page, press **Start run**. The UI posts to `/api/v1/playground/experiments/{id}/runs`.
2. Runs appear in the runs table. As the worker executes shards, the status transitions through `running` → `completed` (or `failed`).
3. Use **Refresh** to pull the latest metrics or `Runs` view for a global history (future SSE updates will stream automatically).

## 5. Inspect Results and Metrics

- **Runs Table**: shows the plan status with start/end times.
- **Metrics (coming soon)**: aggregated evaluator scores appear under the run entry once evaluators complete.
- **Artifacts (coming soon)**: rendered prompts and outputs are stored in the configured artifact directory (`workdir/playground-artifacts`).

## API Reference (Quick Shell)

You can drive the same flow via HTTP requests:

```bash
# Create prompt
curl -X POST http://localhost:8081/api/v1/playground/prompts \
  -H 'Content-Type: application/json' \
  -d '{"name":"Support Greeting","description":"Greets customers"}'

# Create prompt version
curl -X POST http://localhost:8081/api/v1/playground/prompts/<prompt-id>/versions \
  -H 'Content-Type: application/json' \
  -d '{"template":"Hello {{name}}", "variables":{"name":{"type":"string"}}}'

# Create dataset (same JSON shown above)
curl -X POST http://localhost:8081/api/v1/playground/datasets -H 'Content-Type: application/json' -d @dataset.json

# Create experiment
curl -X POST http://localhost:8081/api/v1/playground/experiments \
  -H 'Content-Type: application/json' -d @experiment.json

# Start run
curl -X POST http://localhost:8081/api/v1/playground/experiments/<experiment-id>/runs
```

## Troubleshooting

- **Prompt version creation fails**: ensure the Variables JSON is valid and references all `{{ }}` placeholders.
- **Dataset upload error**: verify the payload is JSON with an array of rows; each row should include an `id` or one is auto-generated.
- **Experiment run stuck**: check `agentd` logs for provider errors; the mock adapter is deterministic and returns instantly.
- **Artifacts missing**: make sure the agent process can write to `<workdir>/playground-artifacts`.

The playground is designed to be iterative—create new prompt versions, rerun experiments, and compare outputs quickly. As evaluator support expands, this flow will automatically surface additional quality metrics in the same UI.
