---
title: Configuring a Project
description: How to create and manage Manifold projects from the web UI
---

# Configuring a Project

This guide walks through how to create and configure a **Project** in the Manifold web UI. A project is the top-level container for your chats, playground experiments, flows, and settings.

You’ll use the **Projects** and **Settings** views in the frontend:

- **Projects**: create and switch between projects, and manage per‑project files.
- **Settings**: configure server-side runtime options that affect all projects.

## 1. Accessing the Projects View

1. Open the Manifold UI in your browser (Docker quickstart: `http://localhost:32180`).
2. In the left sidebar, click **Projects**.

You’ll see:

- A **project selector** at the top-right.
- A **New project name** field and **Create** button.
- A file tree for the currently selected project.

If this is a fresh deployment, you’ll typically see a single default project created by the server.

## 2. Creating a New Project

From the **Projects** view:

1. In the header, type a name into **New project name**, for example: `Support Assistants`.
2. Click **Create**.

The new project is created by the backend and becomes selectable in the **project dropdown** next to the Create button.

> The Projects view also exposes a per‑project file tree. At creation time the tree is empty except for the root directory.

## 3. Selecting and Switching Projects

The **active project** controls which chats, playground assets, specialists, and flows you see across the UI.

To switch the active project:

1. Open **Projects**.
2. Use the **Select project** dropdown in the header to choose a project by name.

When you change the selection:

- The file tree reloads for the newly selected project.
- Other views (Chat, Playground, Specialists, Flow) use that project ID for subsequent API calls.

If data appears “missing” in another view, confirm which project is currently selected in the Projects header.

## 4. Managing Project Files

Each project owns its own small **file workspace**. These files can be used by tools, workflows, or as artifacts you want to keep alongside the project.

In the **left panel** of the Projects view you can:

- Click **Root** to jump to the top of the project’s file tree.
- See the current directory path (e.g. `.` or `datasets/demo`).
- Use the toolbar buttons:
  - **New Folder** – prompts for a folder name and creates a subdirectory under the current path.
  - **Upload** – opens a file picker; selected files are uploaded into the current directory.
  - **Download** – downloads all checked files/folders in the tree (one browser download per item).
  - **Delete** – deletes all checked items after confirmation.

You can navigate the tree and check items using the **FileTree** control. Selecting a file in the tree shows a basic preview in the right-hand panel when supported (for example, text or markdown files).

## 5. Deleting a Project

To remove a project entirely:

1. In the **Select project** dropdown, choose the project you want to delete.
2. Click **Delete** next to the dropdown.
3. Confirm the prompt.

This issues a delete request to the backend. The project’s metadata and associated file workspace are removed. Depending on your deployment and database configuration, other artifacts (such as chats or experiments) may also be deleted or may remain for administrative recovery.

In production, it is common to restrict this operation to administrator accounts.

## 6. Relationship to Settings

- The **Settings** view configures **agentd runtime options** (API base URL, summarization, embeddings, timeouts, logging, etc.) and is not per‑project.
- The **Projects** view is where you create, select, and manage projects and their files.

The combination of a selected project plus the global runtime settings determines how Chat, Specialists, Playground, and Flow behave for your team.

