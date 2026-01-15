# Workspace Architecture (Local Filesystem)

This document describes the local-only workspace model used by Manifold. Workspaces are the on-disk project directories managed by the local filesystem.

## Summary

- A workspace is the project directory on disk.
- `Checkout` returns the existing project path.
- `Commit` and `Cleanup` are no-ops because edits are already on disk.

## Directory Layout

```
$WORKDIR/users/<user-id>/projects/<project-id>
```

## Behavior

### Checkout

- Validates project ownership and IDs.
- Returns the project root path.
- Creates the directory if it does not exist.

### Commit

- No-op for local filesystem storage.
- Changes are already persisted on disk.

### Cleanup

- No-op for local filesystem storage.
- Callers are responsible for removing project directories if needed.

## Safety Rules

- Project and session IDs are validated to prevent path traversal.
- All filesystem operations stay within the project root.
- Symlinks are ignored when listing or deleting.

## Concurrency Model

- The filesystem is the source of truth.
- No staging area or object storage indirection is used.
- Concurrent edits rely on client coordination.

## Operational Notes

- Backups should be handled at the host or volume level.
- Disk space monitoring is required in production deployments.
