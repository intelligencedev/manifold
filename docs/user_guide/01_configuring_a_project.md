---
title: Configuring a Project
description: Create, manage, and use projects as the working directory for files, datasets, and agent runs.
---

# Configuring a Project

Projects are per-user workspaces where Manifold stores files used by chat, playground experiments, and workflows. The UI exposes a tree view and basic file operations; the backend persists everything under a per-user WORKDIR with optional at-rest encryption.

Placeholder for screenshots: [Projects header + tree panel]

## Where to find it

- App header: Projects in the main navigation.
- Global project selector: top-right of the app header. Chat and other views require a project to be selected.

## Create a project

1) Go to Projects.
2) Enter a name in “New project name” and click Create.
3) The project becomes the current project and its root appears in the tree.

Backend behavior
- Each project is a directory at $WORKDIR/users/<user-id>/projects/<project-id> with metadata in .meta/project.json.
- CreatedAt/UpdatedAt are tracked; size and file count are computed on demand.

## Select, delete projects

- Select: use the Projects select in the page header or the global selector in the app header.
- Delete: click Delete in the Projects header (only for the currently selected project). This removes the entire project directory.

Constraints
- Deleting removes all files; there is no recycle bin.

## Manage files

The left panel is a tree with drag-and-drop support; the right panel previews images or text.

You can:
- Create a folder: New Folder (creates nested dirs as needed).
- Upload files: Upload supports multi-select; files appear under the current directory.
- Download: select checkboxes then Download. Each selected file downloads directly.
- Delete: select checkboxes then Delete. Deleting a folder removes its contents recursively.
- Move/Rename: drag an item onto a destination folder (or the Root bar) to move it. Dragging effectively renames paths; moving a directory into its descendant is refused.

Preview behavior
- Images render inline; common text formats open in an embedded viewer. Other files show an Open link.

Backend rules and safety
- Paths are sanitized under the project root; symlinks are ignored for both listing and deletion.
- Uploads are either plaintext or AES‑GCM encrypted on write when project encryption is enabled.
- Deletions never follow symlinks and reject deleting a symlink.
- Moves validate destination uniqueness and prevent moving a directory into its own subtree.

## Encryption at rest (optional)

If enabled by the operator, Manifold encrypts project files at rest:
- A per-server master key is stored under $WORKDIR/.keystore/master.key.
- Each project has a 32‑byte DEK wrapped by the master key (.meta/enc.json).
- New uploads are written as MGCM v1: header + nonce + AES‑GCM ciphertext.
- Rotation is supported: enc.json can carry new and previous wrapped DEKs while files are re-encrypted.

Note: Encryption is configured server-side; there is no client toggle in the Projects UI.

## Using projects elsewhere

- Chat: you must select a project to enable composer actions (send/attach/voice). The agent runs against project context and attachments are stored in the project.
- Playground: datasets and artifacts live under the server’s playground store but projects remain the working directory for general file workflows.
- Flow: workflows may read/write project files via tools, depending on configured tool access.

Troubleshooting
- “No project available” in selects: an admin may need to grant access or you need to create your first project.
- Drag move blocked: dropping a folder inside itself or an existing destination is not allowed.

Placeholder for screenshots: [Create project + file tree + preview]

