# Context Archaeology

Context archaeology records decisions as first-class memory objects so Manifold
can later answer why a choice was made, what assumptions supported it, what
alternatives were rejected, and whether those assumptions still hold.

## Model

- `decision.Decision` records the chosen path, rationale, decision actor, status,
  review state, confidence, and source episode.
- `decision.AssumptionLink` ties a decision to belief memory and snapshots the
  belief statement/confidence at link time.
- `decision.Alternative` records paths not taken and rejection reasons.
- `decision.DecisionEvidence` links decisions to episodes, beliefs, transit
  records, artifacts, and other source kinds.
- `decision.Transition` is an append-only audit row for lifecycle changes.
- `artifact.Artifact` stores immutable references to external objects such as git
  commits, chat messages, tickets, documents, and transit key versions.

## Lifecycle

Valid decision status transitions are:

- `proposed -> active | revoked`
- `active -> stale | superseded | revoked`
- `stale -> active | superseded | revoked`
- `superseded` and `revoked` are terminal

Same-status audit rows are permitted only for belief-change review flags.

## Reactivity

The belief notifying store emits non-blocking change events when belief status or
confidence changes. The decision reactor applies these rules:

- Retracted, superseded, or expired load-bearing beliefs mark linked decisions
  `stale`.
- Retracted, superseded, or expired supporting beliefs keep the decision active
  but set `review_state=needs_review`.
- Contextual beliefs add audit notes only.
- Large confidence drops flag load-bearing decisions for review.

The read path also checks current load-bearing assumptions, so a dropped async
event degrades to eventual consistency rather than an incorrect verdict.

## Reconstruction

`archaeology.Service.Reconstruct` searches decisions, loads transitions,
assumptions, alternatives, and evidence, resolves current belief and artifact
state, optionally replays status as of a timestamp, and returns a verdict:

- `holds`
- `needs_review`
- `stale`
- `superseded`
- `revoked`

## Tools

When archaeology is enabled, the agent tool registry exposes:

- `decision_search` for retrieving recorded decisions by text, scope, and status.
- `decision_reconstruct` for loading evidence, assumptions, alternatives, and
  lifecycle transitions behind matching decisions.
- `decision_record` for recording an explicit decision with optional evidence,
  assumption links, and rejected alternatives.
- `decision_review` for accepting/rejecting extracted candidates and updating
  lifecycle state (`reaffirm`, `revoke`, `mark_stale`, `needs_review`,
  `supersede`).

## Archival

Evolving-memory pruning archives removed entries to a cold archive table when the
store supports it. MAGMA lifecycle pruning writes archive nodes for events and
edges before deletion when `archiveBeforeDelete` is enabled. If the archive write
fails, the delete path stops and the hot record remains.

## Configuration

```yaml
archaeology:
  enabled: false
  archive_before_delete: true
  decision_distiller: true
  causal_grounding_required: true
  reactor:
    confidence_floor: 0.35
    confidence_drop_delta: 0.30
```

The feature is disabled by default. When disabled, decision and artifact stores
can exist but no episode-end decision distillation or belief-to-decision reactor
is attached.
