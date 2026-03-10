# Project Storage (Local Filesystem)

Manifold stores project files on the local filesystem under the configured `WORKDIR`.

This is the active deployment model in the current repository. The runtime project and workspace path is filesystem-backed.

## Layout

Each project is a directory at:

```text
$WORKDIR/users/<user-id>/projects/<project-id>
```

Metadata lives at:

```text
$WORKDIR/users/<user-id>/projects/<project-id>/.meta/project.json
```

## Configuration

Set `WORKDIR` in `.env` or in the process environment.

In the current example configuration, `WORKDIR` is provided through `.env` and does not need a matching `workdir:` field in `config.yaml` unless you explicitly want YAML to supply it.

No storage backend configuration is required for local deployments.

## Operational Notes

- Deleting a project removes the directory immediately.
- Files are read and written directly on disk.
- All file paths are validated to stay inside the project directory.
