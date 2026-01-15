# Project Storage (Local Filesystem)

Manifold stores project files on the local filesystem under the configured `WORKDIR`.

## Layout

Each project is a directory at:

```
$WORKDIR/users/<user-id>/projects/<project-id>
```

Metadata lives at:

```
$WORKDIR/users/<user-id>/projects/<project-id>/.meta/project.json
```

## Configuration

Set `WORKDIR` in `.env` and ensure the same path is used by `workdir` in [config.yaml](../config.yaml).

No storage backend configuration is required for local deployments.

## Operational Notes

- Deleting a project removes the directory immediately.
- Files are read and written directly on disk.
- All file paths are validated to stay inside the project directory.
