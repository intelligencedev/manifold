<template>
  <div class="flex h-full min-h-0 flex-col space-y-4">
    <div class="flex flex-wrap items-center gap-3">
      <label class="text-sm text-muted-foreground">
        Workflow
        <select
          v-model="selectedIntent"
          class="ap-input ml-2 rounded bg-surface-muted/60 px-2 py-1 text-sm text-foreground"
        >
          <option disabled value="">Select workflow</option>
          <option v-for="wf in workflowList" :key="wf.intent" :value="wf.intent">
            {{ wf.intent }}
          </option>
        </select>
      </label>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-muted text-foreground hover:bg-muted/80 plain-link"
        title="Create new workflow"
        aria-label="New workflow"
        @click="onNew"
      >
        New
      </button>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-accent text-accent-foreground hover:bg-accent/90 plain-link"
        :disabled="!canSave"
        title="Save workflow"
        aria-label="Save workflow"
        @click="onSave"
      >
        Save
      </button>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-danger text-danger-foreground hover:bg-danger/90 plain-link"
        :disabled="!canDelete"
        title="Delete workflow"
        aria-label="Delete workflow"
        @click="onDelete"
      >
        Delete
      </button>

      <!-- Import button placed to the left of Export -->
      <input
        ref="importInput"
        type="file"
        accept="application/json,.json"
        class="hidden"
        @change="onImportSelected"
      />
      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-muted text-foreground hover:bg-muted/80 plain-link"
        title="Import workflow from JSON"
        aria-label="Import workflow"
        @click="triggerImport"
      >
        Import
      </button>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-muted text-foreground hover:bg-muted/80 plain-link"
        :disabled="!canExport"
        title="Export workflow as JSON"
        aria-label="Export workflow"
        @click="exportWorkflow"
      >
        Export
      </button>

      <button
        class="inline-flex items-center gap-2 rounded px-3 py-1 text-sm font-medium transition focus:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50 disabled:cursor-not-allowed bg-primary text-primary-foreground hover:bg-primary/90 plain-link"
        :disabled="!canRun"
        title="Run workflow"
        aria-label="Run workflow"
        @click="onRun"
      >
        <span v-if="!running">Run</span>
        <span v-else class="inline-flex items-center gap-1">
          <svg class="h-3 w-3 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle class="opacity-25" cx="12" cy="12" r="10" />
            <path class="opacity-75" d="M4 12a8 8 0 018-8" />
          </svg>
          Running…
        </span>
      </button>
      <button
        v-if="running"
        class="rounded bg-muted px-3 py-1 text-sm font-medium text-foreground transition hover:bg-muted/80 plain-link"
        @click="onCancelRun"
      >
        Cancel
      </button>
      <div class="flex flex-wrap items-center gap-3 ml-auto">
        <span v-if="loading" class="text-sm text-subtle-foreground">Loading…</span>
        <span v-else-if="error" class="text-sm text-danger-foreground">{{ error }}</span>
        <span v-else class="text-sm text-faint-foreground">Tools: {{ tools.length }}</span>
        <span
          v-if="runOutput"
          class="text-xs italic text-subtle-foreground truncate max-w-[320px]"
          :title="runOutput"
        >
          Result: {{ runOutput }}
        </span>
        <div class="flex items-center gap-1">
          <span class="text-[10px] uppercase tracking-wide text-faint-foreground">Mode</span>
          <div class="inline-flex overflow-hidden rounded border border-border/60 text-xs">
            <button
              type="button"
              class="px-2 py-1 transition"
              :class="
                editorMode === 'design'
                  ? 'bg-accent text-accent-foreground'
                  : 'text-subtle-foreground hover:text-foreground'
              "
              @click="setEditorMode('design')"
            >
              Design
            </button>
            <button
              type="button"
              class="border-l border-border/60 px-2 py-1 transition disabled:opacity-40"
              :class="
                editorMode === 'run'
                  ? 'bg-accent text-accent-foreground'
                  : 'text-subtle-foreground hover:text-foreground'
              "
              :disabled="!hasRunTrace && !running"
              @click="setEditorMode('run')"
            >
              Run
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Workflow metadata summary -->
    <div v-if="activeWorkflow" class="text-xs text-subtle-foreground -mt-2">
      <span v-if="activeWorkflow?.description">Description: {{ activeWorkflow?.description }}</span>
      <span v-if="activeWorkflow?.keywords?.length" class="ml-3">Keywords: {{ activeWorkflow?.keywords?.join(', ') }}</span>
    </div>

    <div v-if="runLogs.length" class="max-h-[3.6rem] overflow-y-auto rounded border border-border/50 bg-surface-muted px-3 py-2 text-xs font-mono leading-relaxed space-y-0.5">
      <div v-for="(l,i) in runLogs" :key="i" class="whitespace-pre-wrap break-words">{{ l }}</div>
    </div>

    <div
      class="flex flex-1 min-h-0 flex-col gap-4 overflow-auto lg:flex-row lg:items-stretch lg:overflow-hidden"
    >
      <aside class="lg:w-72">
        <div class="ap-panel ap-hover flex min-h-0 flex-col rounded-xl bg-surface p-4 lg:h-full">
          <!-- Conditional: Node Configuration when a single node is selected, else show Tool Palette -->
          <template v-if="selectedCount === 1 && selectedNode && !paletteOverride">
            <div class="flex items-center justify-between gap-2 mb-2">
              <div class="flex items-center gap-2">
                <button
                  class="inline-flex h-6 w-6 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
                  title="Back to palette"
                  aria-label="Back to palette"
                  @click="backToPalette"
                >
                  <!-- simple left arrow -->
                  <svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="15 18 9 12 15 6"></polyline>
                  </svg>
                </button>
                <h2 class="text-sm font-semibold text-foreground">Node Configuration</h2>
              </div>
            </div>
            <div class="text-[11px] text-subtle-foreground mb-3">
              <div class="font-mono flex items-center gap-1">
                <span class="text-foreground/70">ID:</span>
                <span class="truncate" :title="selectedNode.id">{{ selectedNode.id }}</span>
              </div>
            </div>
            <div class="lg:flex-1 lg:min-h-0 overflow-y-auto pr-1">
              <NodeInspectorStep
                v-if="selectedNode.type === 'warppStep'"
                :node-id="selectedNode.id"
                :data="selectedNode.data as any"
                :tools="tools"
              />
              <NodeInspectorUtility
                v-else-if="selectedNode.type === 'warppUtility'"
                :node-id="selectedNode.id"
                :data="selectedNode.data as any"
              />
              <NodeInspectorSticky
                v-else-if="selectedNode.type === 'warppSticky'"
                :node-id="selectedNode.id"
                :data="selectedNode.data as any"
              />
              <div v-else class="text-xs text-subtle-foreground">
                This node type has no configurable parameters.
              </div>
            </div>
          </template>
          <template v-else>
            <div class="flex items-center justify-between gap-2">
              <h2 class="text-sm font-semibold text-foreground">Tool Palette</h2>
              <span class="text-[10px] uppercase tracking-wide text-faint-foreground">Drag to add</span>
            </div>
            <p class="mt-1 text-xs text-subtle-foreground">
              Drag onto the canvas to add workflow steps, utilities, or group containers.
            </p>
            <div class="mt-3 max-h-[40vh] space-y-3 overflow-y-auto pr-1 lg:flex-1 lg:min-h-0 lg:max-h-none">
              <div class="space-y-2">
                <h3 class="text-[11px] font-semibold uppercase tracking-wide text-faint-foreground">Utility Nodes</h3>
                <p class="text-[10px] text-subtle-foreground">
                  Utility nodes provide editor-only helpers for WARPP workflows.
                </p>
                <!-- Group Container and Sticky Note are utility items and appear first -->
                <div
                  class="cursor-grab rounded ap-ring bg-surface-muted px-3 py-2 text-sm font-medium text-foreground transition hover:bg-surface truncate"
                  draggable="true"
                  title="Group nodes to keep steps organized"
                  @dragstart="onGroupDragStart"
                  @dragend="onPaletteDragEnd"
                >
                  Group Container
                </div>
                <div
                  class="cursor-grab rounded ap-ring bg-surface-muted px-3 py-2 text-sm font-medium text-foreground transition hover:bg-surface truncate"
                  draggable="true"
                  title="Sticky note (editor-only)"
                  @dragstart="onStickyDragStart"
                  @dragend="onPaletteDragEnd"
                >
                  Sticky Note
                </div>
                <!-- Other utility tools from backend follow -->
                <div
                  v-for="tool in utilityTools"
                  :key="tool.name"
                  class="cursor-grab rounded ap-ring bg-surface-muted px-3 py-2 text-sm font-medium text-foreground transition hover:bg-surface truncate"
                  draggable="true"
                  :title="tool.description ?? tool.name"
                  @dragstart="(event: DragEvent) => onPaletteDragStart(event, tool)"
                  @dragend="onPaletteDragEnd"
                >
                  {{ prettyUtilityLabel(tool.name) }}
                </div>
              </div>
              <template v-if="workflowTools.length">
                <div class="space-y-2">
                  <h3 class="text-[11px] font-semibold uppercase tracking-wide text-faint-foreground">
                    Workflow Tools
                  </h3>
                  <div
                    v-for="tool in workflowTools"
                    :key="tool.name"
                    class="cursor-grab rounded ap-ring bg-surface-muted px-3 py-2 text-sm font-medium text-foreground transition hover:bg-surface truncate"
                    draggable="true"
                    :title="tool.description ?? tool.name"
                    @dragstart="(event: DragEvent) => onPaletteDragStart(event, tool)"
                    @dragend="onPaletteDragEnd"
                  >
                    {{ tool.name }}
                  </div>
                </div>
              </template>
              <div
                v-if="!tools.length && !loading"
                class="rounded border border-dashed border-border/60 bg-surface-muted/60 p-3 text-xs text-subtle-foreground"
              >
                No tools available for this configuration.
              </div>
            </div>
          </template>
        </div>
      </aside>

      <div class="flex-1 min-h-0">
        <div
          ref="flowWrapper"
          class="flex h-full min-h-0 w-full overflow-hidden rounded-xl border bg-surface"
          :class="isDraggingFromPalette ? 'border-accent/60' : 'border-border/70'"
        >
          <VueFlow
            v-model:nodes="nodes"
            v-model:edges="edges"
            :fit-view="true"
            :zoom-on-scroll="true"
            :zoom-on-double-click="false"
            :node-types="nodeTypes"
            :default-edge-options="{ type: currentEdgeStyle }"
            class="h-full w-full"
            @dragover="onDragOver"
            @drop="onDrop"
            @connect="onConnect"
          >
            <Background />

            <!-- Themed Controls (replaces default Controls) -->
            <Panel position="bottom-left">
              <div
                class="ap-chip flex items-center gap-1 rounded-md p-1"
              >
                <!-- Expand/Collapse all -->
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  :aria-pressed="nodesCollapsed"
                  :aria-label="nodesCollapsed ? 'Expand all nodes' : 'Collapse all nodes'"
                  :title="nodesCollapsed ? 'Expand all' : 'Collapse all'"
                  @click="toggleCollapseAll"
                >
                  <CollapseIcon v-if="!nodesCollapsed" class="h-4 w-4" />
                  <ExpandIcon v-else class="h-4 w-4" />
                </button>
                <!-- Auto layout buttons -->
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  aria-label="Auto layout (vertical)"
                  title="Auto layout (vertical)"
                  @click="onAutoLayout('TB')"
                >
                  <LayoutIcon class="h-4 w-4 rotate-90" />
                </button>
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  aria-label="Auto layout (horizontal)"
                  title="Auto layout (horizontal)"
                  @click="onAutoLayout('LR')"
                >
                  <LayoutIcon class="h-4 w-4" />
                </button>
                <span class="mx-0.5 h-5 w-px bg-border/60" aria-hidden="true"></span>
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  aria-label="Zoom in"
                  @click="onZoomIn"
                >
                  <ZoomInIcon class="h-4 w-4" />
                </button>
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  aria-label="Zoom out"
                  @click="onZoomOut"
                >
                  <ZoomOutIcon class="h-4 w-4" />
                </button>
                <span class="mx-0.5 h-5 w-px bg-border/60" aria-hidden="true"></span>
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  aria-label="Fit view"
                  @click="onFitView"
                >
                  <FullScreenIcon class="h-4 w-4" />
                </button>
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded px-2 py-2 text-[10px] font-semibold uppercase tracking-wide text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  :aria-label="edgeStyleAriaLabel"
                  :title="edgeStyleButtonTitle"
                  @click="cycleEdgeStyle"
                >
                  <svg
                    class="h-4 w-4 shrink-0"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  >
                    <path d="M3 18c3.5 0 4.5-12 8-12s4.5 6 8 6" />
                    <circle cx="3" cy="18" r="1.5" fill="currentColor" stroke="none" />
                    <circle cx="19" cy="12" r="1.5" fill="currentColor" stroke="none" />
                  </svg>
                  <span class="leading-none">{{ edgeStyleLabel }}</span>
                </button>
                <span class="mx-0.5 h-5 w-px bg-border/60" aria-hidden="true"></span>
                <button
                  type="button"
                  class="inline-flex items-center justify-center rounded p-2 text-subtle-foreground hover:bg-surface-muted/80 hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                  :aria-pressed="nodesLocked"
                  :aria-label="nodesLocked ? 'Unlock node positions' : 'Lock node positions'"
                  @click="toggleNodeLock"
                >
                  <UnlockedIcon v-if="!nodesLocked" class="h-4 w-4" />
                  <LockedIcon v-else class="h-4 w-4" />
                </button>
              </div>
            </Panel>

            <!-- Themed MiniMap -->
            <MiniMap
              v-if="showMiniMap"
              class="ap-chip rounded-md p-1"
              :position="'bottom-right'"
              :pannable="true"
              :zoomable="true"
              :width="MINI_MAP_WIDTH"
              :height="MINI_MAP_HEIGHT"
              :mask-color="'rgb(var(--color-surface) / 0.85)'"
              :mask-stroke-color="'rgb(var(--color-border) / 0.7)'"
              :mask-stroke-width="1"
              :mask-border-radius="8"
              :node-color="miniMapNodeColor"
              :node-stroke-color="miniMapNodeStroke"
              :node-border-radius="6"
              :node-stroke-width="1"
            />

            <!-- Close button overlay for MiniMap (top-left of the MiniMap) -->
            <Panel
              v-if="showMiniMap"
              position="bottom-right"
              :style="{
                transform: `translate(calc(-${MINI_MAP_WIDTH}px + ${MINI_MAP_INSET}px), calc(-${MINI_MAP_HEIGHT}px + ${MINI_MAP_INSET}px))`,
              }"
            >
              <button
                type="button"
                class="ap-chip inline-flex h-6 w-6 items-center justify-center rounded text-subtle-foreground hover:text-foreground"
                aria-label="Hide minimap"
                title="Hide minimap"
                @click="showMiniMap = false"
              >
                ×
              </button>
            </Panel>

            <!-- Collapsed show button when MiniMap hidden -->
            <Panel v-if="!showMiniMap" position="bottom-right">
              <button
                type="button"
                class="ap-chip inline-flex items-center justify-center rounded-md p-1.5 text-subtle-foreground hover:text-foreground"
                aria-label="Show minimap"
                title="Show minimap"
                @click="showMiniMap = true"
              >
                <MapShowIcon class="h-5 w-5 -scale-x-100" />
              </button>
            </Panel>

            <Panel position="top-right">
              <button
                type="button"
                class="ap-chip inline-flex items-center justify-center rounded-md p-1.5 text-subtle-foreground hover:text-foreground"
                aria-label="Workflow help"
                title="Workflow help"
                @click="openHelpModal"
              >
                <HelpIcon class="h-5 w-5" />
              </button>
            </Panel>
          </VueFlow>
        </div>
      </div>
    </div>
    <div
      v-if="resultModal"
      class="fixed inset-0 z-50 flex items-center justify-center px-4 py-8"
    >
      <div class="absolute inset-0 bg-surface/70 backdrop-blur-sm" @click="closeResultModal"></div>
      <div
        class="relative z-10 flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-border/70 bg-surface shadow-2xl"
      >
        <div class="flex items-start justify-between gap-4 border-b border-border/60 px-6 py-4">
          <div class="space-y-1">
            <h3 class="text-base font-semibold text-foreground">{{ modalStepTitle }}</h3>
            <p v-if="modalStepId" class="text-xs text-subtle-foreground">ID: {{ modalStepId }}</p>
            <p v-if="modalStatusLabel" class="text-xs text-subtle-foreground">Status: {{ modalStatusLabel }}</p>
          </div>
          <button
            type="button"
            class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-medium text-foreground hover:bg-surface-muted/80"
            @click="closeResultModal"
          >
            Close
          </button>
        </div>
        <div class="flex-1 overflow-y-auto px-6 py-4 text-sm text-foreground">
          <div v-if="activeModalTrace" class="space-y-5">
            <section v-if="activeModalTrace?.renderedArgs">
              <button
                type="button"
                class="flex w-full items-center justify-between gap-2 text-left"
                @click="collapsedArgs = !collapsedArgs"
                :aria-expanded="!collapsedArgs"
                aria-controls="modal-args"
              >
                <h4 class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Rendered Arguments</h4>
                <svg class="h-3.5 w-3.5 text-subtle-foreground transition-transform" :class="collapsedArgs ? '-rotate-90' : 'rotate-0'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>
              </button>
              <pre
                v-show="!collapsedArgs"
                id="modal-args"
                class="mt-1 rounded border border-border/60 bg-surface-muted p-3 text-xs text-foreground/90 whitespace-pre-wrap break-words"
              >{{ formatJSON(activeModalTrace?.renderedArgs) }}</pre>
            </section>
            <section v-if="activeModalTrace?.delta">
              <button
                type="button"
                class="flex w-full items-center justify-between gap-2 text-left"
                @click="collapsedDelta = !collapsedDelta"
                :aria-expanded="!collapsedDelta"
                aria-controls="modal-delta"
              >
                <h4 class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Delta</h4>
                <svg class="h-3.5 w-3.5 text-subtle-foreground transition-transform" :class="collapsedDelta ? '-rotate-90' : 'rotate-0'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>
              </button>
              <pre
                v-show="!collapsedDelta"
                id="modal-delta"
                class="mt-1 rounded border border-border/60 bg-surface-muted p-3 text-xs text-foreground/90 whitespace-pre-wrap break-words"
              >{{ formatJSON(activeModalTrace?.delta) }}</pre>
            </section>
            <section v-if="activeModalTrace?.payload">
              <button
                type="button"
                class="flex w-full items-center justify-between gap-2 text-left"
                @click="collapsedPayload = !collapsedPayload"
                :aria-expanded="!collapsedPayload"
                aria-controls="modal-payload"
              >
                <h4 class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Payload</h4>
                <svg class="h-3.5 w-3.5 text-subtle-foreground transition-transform" :class="collapsedPayload ? '-rotate-90' : 'rotate-0'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>
              </button>
              <pre
                v-show="!collapsedPayload"
                id="modal-payload"
                class="mt-1 rounded border border-border/60 bg-surface-muted p-3 text-xs text-foreground/90 whitespace-pre-wrap break-words"
              >{{ formatJSON(activeModalTrace?.payload) }}</pre>
            </section>
            <p v-if="activeModalTrace?.error" class="rounded border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger-foreground">
              {{ activeModalTrace?.error }}
            </p>
          </div>
          <div v-else class="text-sm text-subtle-foreground">
            Trace data not yet available.
          </div>
        </div>
      </div>
    </div>

    <!-- Help modal -->
    <div v-if="showHelpModal" class="fixed inset-0 z-50 flex items-center justify-center px-4 py-8">
      <div class="absolute inset-0 bg-surface/70 backdrop-blur-sm" @click="closeHelpModal"></div>
      <div class="relative z-10 w-full max-w-lg overflow-hidden rounded-xl border border-border/70 bg-surface shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="warpp-help-title">
        <div class="flex items-center justify-between border-b border-border/60 px-5 py-3">
          <h3 id="warpp-help-title" class="text-base font-semibold text-foreground">Flow Help</h3>
        </div>
        <div class="px-5 py-4 space-y-4 text-sm text-foreground/90">
          <p class="text-xs uppercase tracking-wide text-faint-foreground">Controls and Hotkeys</p>
          <ul class="list-disc space-y-2 pl-5 text-sm text-subtle-foreground">
            <li><span class="font-medium text-foreground">Shift + click &amp; drag</span> draws a selection box so you can move or delete multiple nodes together.</li>
            <li><span class="font-medium text-foreground">Cmd/Ctrl + click</span> adds or removes individual nodes from the current selection.</li>
            <li><span class="font-medium text-foreground">Backspace/Delete</span> removes the currently selected nodes or edges.</li>
            <li><span class="font-medium text-foreground">Drag on empty canvas space</span> to pan the view; use your mouse wheel or trackpad gestures to zoom.</li>
          </ul>
        </div>
        <div class="flex items-center justify-end gap-2 border-t border-border/60 px-5 py-3">
          <button
            type="button"
            class="rounded px-3 py-1 text-sm font-medium bg-accent text-accent-foreground hover:bg-accent/90"
            @click="closeHelpModal"
          >
            Got it
          </button>
        </div>
      </div>
    </div>

    <!-- Metadata modal -->
    <div v-if="showMetaModal" class="fixed inset-0 z-50 flex items-center justify-center px-4 py-8">
      <div class="absolute inset-0 bg-surface/70 backdrop-blur-sm" @click="closeMetaModal"></div>
      <div class="relative z-10 w-full max-w-xl overflow-hidden rounded-xl border border-border/70 bg-surface shadow-2xl">
        <div class="flex items-center justify-between border-b border-border/60 px-5 py-3">
          <h3 class="text-base font-semibold text-foreground">Workflow Metadata</h3>
          <button type="button" class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-medium text-foreground hover:bg-surface-muted/80" @click="closeMetaModal">Cancel</button>
        </div>
        <div class="px-5 py-4 space-y-4">
          <div>
            <label class="block text-sm font-medium text-foreground mb-1" for="wf-desc">Description</label>
            <textarea id="wf-desc" v-model="metaDescription" rows="4" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground" placeholder="Describe this workflow and its purpose"></textarea>
            <p class="mt-1 text-[10px] text-faint-foreground">Provide a concise, multi-line description.</p>
          </div>
          <div>
            <label class="block text-sm font-medium text-foreground mb-1" for="wf-keywords">Keywords</label>
            <input id="wf-keywords" v-model="metaKeywords" type="text" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground" placeholder="comma, separated, keywords" />
            <p class="mt-1 text-[10px] text-faint-foreground">Enter a comma-separated list. Both description and keywords are required.</p>
          </div>
        </div>
        <div class="flex items-center justify-end gap-2 border-t border-border/60 px-5 py-3">
          <button type="button" class="rounded px-3 py-1 text-sm font-medium bg-muted text-foreground hover:bg-muted/80" @click="closeMetaModal">Cancel</button>
          <button type="button" class="rounded px-3 py-1 text-sm font-medium bg-accent text-accent-foreground hover:bg-accent/90 disabled:opacity-50 disabled:cursor-not-allowed" :disabled="metaSaveDisabled" @click="onSubmitMetadata">Save</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, provide, ref, watch, markRaw } from 'vue'
import { VueFlow, type Edge, type Node, useVueFlow, type Connection, Panel, type GraphNode } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { MiniMap } from '@vue-flow/minimap'

import WarppStepNode from '@/components/flow/WarppStepNode.vue'
import WarppUtilityNode from '@/components/flow/WarppUtilityNode.vue'
import WarppStickyNoteNode from '@/components/flow/WarppStickyNoteNode.vue'
import WarppGroupNode from '@/components/flow/WarppGroupNode.vue'
import NodeInspectorStep from '@/components/flow/NodeInspectorStep.vue'
import NodeInspectorUtility from '@/components/flow/NodeInspectorUtility.vue'
import NodeInspectorSticky from '@/components/flow/NodeInspectorSticky.vue'
import ZoomInIcon from '@/components/icons/ZoomIn.vue'
import ZoomOutIcon from '@/components/icons/ZoomOut.vue'
import FullScreenIcon from '@/components/icons/FullScreen.vue'
import LockedIcon from '@/components/icons/LockedBold.vue'
import UnlockedIcon from '@/components/icons/UnlockedBold.vue'
import MapShowIcon from '@/components/icons/MapShow.vue'
import HelpIcon from '@/components/icons/Help.vue'
import LayoutIcon from '@/components/icons/FlowLayout.vue'
import CollapseIcon from '@/components/icons/Collapse.vue'
import ExpandIcon from '@/components/icons/Expand.vue'
import dagre from 'dagre'
import { fetchWarppTools, fetchWarppWorkflow, fetchWarppWorkflows, saveWarppWorkflow, deleteWarppWorkflow } from '@/api/warpp'
import type { WarppStep, WarppTool, WarppWorkflow, WarppStepTrace, WarppGroupUIEntry, WarppWorkflowUI, WarppNoteUIEntry } from '@/types/warpp'
import type { StepNodeData, GroupNodeData } from '@/types/flow'
import { useWarppRunStore } from '@/stores/warpp'
import {
  WARPP_STEP_NODE_DIMENSIONS,
  WARPP_UTILITY_NODE_DIMENSIONS,
  WARPP_GROUP_NODE_DIMENSIONS,
  WARPP_STEP_NODE_COLLAPSED,
  WARPP_UTILITY_NODE_COLLAPSED,
} from '@/constants/warppNodes'

type LayoutEntry = {
  x: number
  y: number
  width?: number
  height?: number
}

type LayoutMap = Record<string, LayoutEntry>

type WarppNodeData = StepNodeData | GroupNodeData
type WarppNode = Node<WarppNodeData>
type StepNode = Node<StepNodeData>
type GroupNode = Node<GroupNodeData>
type SelectableWarppNode = WarppNode & { selected?: boolean }

const DRAG_DATA_TYPE = 'application/warpp-tool'
const GROUP_DRAG_TOKEN = '__warpp-group__'
const STICKY_DRAG_TOKEN = '__warpp-sticky__'
const DEFAULT_LAYOUT_START_X = 140
const DEFAULT_LAYOUT_START_Y = 160
const DEFAULT_LAYOUT_HORIZONTAL_GAP = 320
const UTILITY_TOOL_PREFIX = 'utility_'

// Dagre layout sizing
// Use measured node sizes when available; these are fallbacks when not yet measured
const DAGRE_NODE_BASE_WIDTH = WARPP_STEP_NODE_DIMENSIONS.defaultWidth
const DAGRE_NODE_BASE_HEIGHT = WARPP_STEP_NODE_DIMENSIONS.defaultHeight

type NodeKind = 'step' | 'utility' | 'group'

function getDefaultDimensions(kind: NodeKind) {
  if (kind === 'group') return WARPP_GROUP_NODE_DIMENSIONS
  return kind === 'utility' ? WARPP_UTILITY_NODE_DIMENSIONS : WARPP_STEP_NODE_DIMENSIONS
}

function isGroupNode(node: WarppNode): boolean {
  return node.data?.kind === 'group' || node.type === 'warppGroup'
}

function isStepLikeNode(node: WarppNode): boolean {
  return !isGroupNode(node)
}

function toPx(value: number) {
  return `${Math.round(value)}px`
}

function parseDimension(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = parseFloat(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return undefined
}

function readNodeSize(node: WarppNode) {
  const kind: NodeKind = node.data?.kind === 'utility' ? 'utility' : node.data?.kind === 'group' ? 'group' : 'step'
  const defaults = getDefaultDimensions(kind)
  const graphNode = node as unknown as GraphNode
  const style = typeof node.style === 'function' ? (node.style as any)(graphNode) : node.style
  const styledWidth = parseDimension((style as any)?.width)
  const styledHeight = parseDimension((style as any)?.height)
  const dimsWidth = graphNode?.dimensions?.width
  const dimsHeight = graphNode?.dimensions?.height
  // If node is collapsed and no explicit style width/height are present, use collapsed footprint
  const collapsed = (node.data as any)?.collapsed === true
  const width =
    styledWidth ??
    dimsWidth ??
    (collapsed ? (kind === 'utility' ? WARPP_UTILITY_NODE_COLLAPSED.width : WARPP_STEP_NODE_COLLAPSED.width) : defaults.defaultWidth)
  const height =
    styledHeight ??
    dimsHeight ??
    (collapsed ? (kind === 'utility' ? WARPP_UTILITY_NODE_COLLAPSED.height : WARPP_STEP_NODE_COLLAPSED.height) : defaults.defaultHeight)
  return { width, height }
}

function buildNodeStyle(kind: NodeKind, stored?: LayoutEntry) {
  const defaults = getDefaultDimensions(kind)
  const storedWidth = typeof stored?.width === 'number' ? stored.width : undefined
  const storedHeight = typeof stored?.height === 'number' ? stored.height : undefined
  const width = storedWidth && storedWidth >= defaults.minWidth ? storedWidth : defaults.defaultWidth
  const style: Record<string, string> = {
    width: toPx(width),
    zIndex: kind === 'group' ? '0' : '10',
  }
  if (storedHeight && storedHeight >= defaults.minHeight) {
    style.height = toPx(storedHeight)
  }
  if (kind === 'group' && !style.height) {
    style.height = toPx(defaults.defaultHeight)
  }
  return style
}

type UiSnapshot = {
  layout: LayoutMap
  parents: Record<string, string>
  groups: WarppGroupUIEntry[]
  memberships: Record<string, string | undefined>
  notes: WarppNoteUIEntry[]
}

function collectUiState(allNodes: WarppNode[]): UiSnapshot {
  const layout: LayoutMap = {}
  const parents: Record<string, string> = {}
  const groups: WarppGroupUIEntry[] = []
  const notes: WarppNoteUIEntry[] = []
  const memberships: Record<string, string | undefined> = {}
  const groupRects = new Map<string, { left: number; top: number; right: number; bottom: number }>()

  allNodes.forEach((node) => {
    const position = node.position ?? { x: 0, y: 0 }
    const size = readNodeSize(node)
    const entry: LayoutEntry = { x: position.x, y: position.y }
    if (Number.isFinite(size.width)) entry.width = size.width
    if (Number.isFinite(size.height)) entry.height = size.height
    layout[node.id] = entry

    if (isGroupNode(node)) {
      const data = node.data as GroupNodeData
      groups.push({ id: node.id, label: data.label, collapsed: data.collapsed, color: data.color })
      const width = entry.width ?? WARPP_GROUP_NODE_DIMENSIONS.defaultWidth
      const height = entry.height ?? WARPP_GROUP_NODE_DIMENSIONS.defaultHeight
      groupRects.set(node.id, {
        left: position.x,
        top: position.y,
        right: position.x + width,
        bottom: position.y + height,
      })
    } else if ((node.type === 'warppSticky') || (node.data as any)?.note !== undefined) {
      const data = (node.data as any) as { label?: string; color?: string; note?: string }
      notes.push({ id: node.id, label: data.label, color: data.color, note: data.note })
    }
  })

  const rectEntries = Array.from(groupRects.entries())
  allNodes.forEach((node) => {
    if (!isStepLikeNode(node)) return
    const position = node.position ?? { x: 0, y: 0 }
    const size = readNodeSize(node)
    const centerX = position.x + size.width / 2
    const centerY = position.y + size.height / 2
    const match = rectEntries.find(([, rect]) => centerX >= rect.left && centerX <= rect.right && centerY >= rect.top && centerY <= rect.bottom)
    if (match) {
      parents[node.id] = match[0]
      memberships[node.id] = match[0]
    } else {
      memberships[node.id] = undefined
    }
  })

  return { layout, parents, groups, memberships, notes }
}

// markRaw prevents Vue from proxying the nodeTypes object/components which can
// interfere with Vue Flow's dynamic component resolution in some cases
const nodeTypes = markRaw({
  warppStep: markRaw(WarppStepNode),
  warppUtility: markRaw(WarppUtilityNode),
  warppGroup: markRaw(WarppGroupNode),
  warppSticky: markRaw(WarppStickyNoteNode),
})

const { project, zoomIn, zoomOut, fitView, nodesDraggable, updateNode } = useVueFlow()

const flowWrapper = ref<HTMLDivElement | null>(null)
const isDraggingFromPalette = ref(false)
// MiniMap visibility and sizing
// Start minimap collapsed by default; press the show button to view it
const showMiniMap = ref(false)
const MINI_MAP_WIDTH = 180
const MINI_MAP_HEIGHT = 120
const MINI_MAP_INSET = 8

const nodes = ref<WarppNode[]>([])
const edges = ref<Edge[]>([])
const isHydrating = ref(false)

const workflowList = ref<WarppWorkflow[]>([])
const selectedIntent = ref<string>('')
const activeWorkflow = ref<WarppWorkflow | null>(null)

const tools = ref<WarppTool[]>([])
provide('warppTools', tools)
provide('warppHydrating', isHydrating)
const editorMode = ref<'design' | 'run'>('design')
// Pinia store for run state (persists across navigation)
const warppRunStore = useWarppRunStore()
provide('warppMode', editorMode)
// Wrap store values in computed to maintain reactivity when providing to children
const runTraceComputed = computed(() => warppRunStore.runTrace)
provide('warppRunTrace', runTraceComputed)

const loading = ref(false)
const error = ref('')
const saving = ref(false)
// Pinia unwraps refs by default, so we need to access the underlying $state
// or re-wrap in computed to maintain reactivity across navigation
const running = computed(() => warppRunStore.running)
const runOutput = computed(() => warppRunStore.runOutput)
const runLogs = computed(() => warppRunStore.runLogs)
provide('warppRunning', running)
provide('warppRunOutput', runOutput)
provide('warppRunLogs', runLogs)
let runTraceTimers: ReturnType<typeof setTimeout>[] = []
// Provide collapse/expand-all signals for nodes to react to
const collapseAllSeq = ref(0)
const expandAllSeq = ref(0)
provide('warppCollapseAllSeq', collapseAllSeq)
provide('warppExpandAllSeq', expandAllSeq)
// Track global collapsed state for control icon
const nodesCollapsed = ref(false)
const resultModal = ref<{ stepId: string; title: string } | null>(null)
// Collapsible state for sections inside the result modal
const collapsedArgs = ref(false)
const collapsedDelta = ref(false)
const collapsedPayload = ref(false)
const activeModalTrace = computed(() => {
  if (!resultModal.value) return undefined
  return warppRunStore.runTrace[resultModal.value.stepId]
})
function openResultModal(stepId: string, title: string) {
  const hasTrace = warppRunStore.runTrace[stepId]
  if (!hasTrace) return
  resultModal.value = { stepId, title }
  // Reset collapsible sections when opening
  collapsedArgs.value = false
  collapsedDelta.value = false
  collapsedPayload.value = false
}
function closeResultModal() {
  resultModal.value = null
  // Reset collapsed state when closing
  collapsedArgs.value = false
  collapsedDelta.value = false
  collapsedPayload.value = false
}
provide('warppOpenResultModal', openResultModal)
provide('warppCloseResultModal', closeResultModal)
provide('warppRequestUngroup', removeGroup)
const dirty = ref(false)
// Track unsaved, locally-created workflows by intent
const localWorkflows = ref(new Map<string, WarppWorkflow>())

// Persist UI metadata (layout, parents, groups) client-side per intent so groups survive
// backend omissions. This is a temporary resilience layer and can be removed when the
// server persists full UI state.
const UI_CACHE_KEY = 'warpp.ui.cache.v2'
type UiCacheRecord = Record<string, { layout?: LayoutMap; parents?: Record<string, string>; groups?: WarppGroupUIEntry[]; notes?: WarppNoteUIEntry[] }>
function readUiCache(): UiCacheRecord {
  try {
    const raw = localStorage.getItem(UI_CACHE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    return typeof parsed === 'object' && parsed ? (parsed as UiCacheRecord) : {}
  } catch {
    return {}
  }
}
function writeUiCache(cache: UiCacheRecord) {
  try {
    localStorage.setItem(UI_CACHE_KEY, JSON.stringify(cache))
  } catch {
    // ignore storage failures
  }
}
function getCachedUi(intent: string): { layout?: LayoutMap; parents?: Record<string, string>; groups?: WarppGroupUIEntry[]; notes?: WarppNoteUIEntry[] } {
  const cache = readUiCache()
  return cache[intent] ?? {}
}
function setCachedUi(intent: string, ui: { layout?: LayoutMap; parents?: Record<string, string>; groups?: WarppGroupUIEntry[]; notes?: WarppNoteUIEntry[] }) {
  const cache = readUiCache()
  cache[intent] = {
    ...(cache[intent] ?? {}),
    ...(ui || {}),
  }
  writeUiCache(cache)
}

// File import element
const importInput = ref<HTMLInputElement | null>(null)

const toolMap = computed(() => {
  const map = new Map<string, WarppTool>()
  tools.value.forEach((tool) => {
    map.set(tool.name, tool)
  })
  return map
})

// Selection state for showing Node Configuration panel
const selectedNodes = computed(() => nodes.value.filter((n) => (n as SelectableWarppNode).selected))
const selectedCount = computed(() => selectedNodes.value.length)
const selectedNode = computed(() => (selectedCount.value === 1 ? selectedNodes.value[0] : null))
const paletteOverride = ref(false)
function backToPalette() {
  paletteOverride.value = true
}
watch(
  () => selectedNode.value?.id,
  (next, prev) => {
    if (next !== prev) paletteOverride.value = false
  },
)
watch(selectedCount, (c) => {
  if (c !== 1) paletteOverride.value = false
})

const workflowTools = computed(() => tools.value.filter((tool) => !isUtilityToolName(tool.name)))
const utilityTools = computed(() => tools.value.filter((tool) => isUtilityToolName(tool.name)))
const hasRunTrace = computed(() => {
  const rec = warppRunStore.runTrace
  if (!rec || typeof rec !== 'object') return false
  try {
    return Object.keys(rec).length > 0
  } catch {
    return false
  }
})
const modalStepTitle = computed(() => {
  if (!resultModal.value) return ''
  return resultModal.value.title || activeModalTrace.value?.text || resultModal.value.stepId
})
const modalStepId = computed(() => resultModal.value?.stepId ?? '')
const modalStatusLabel = computed(() => {
  const status = activeModalTrace.value?.status
  if (!status) return ''
  switch (status) {
    case 'completed':
      return 'Completed'
    case 'skipped':
      return 'Skipped'
    case 'noop':
      return 'Not executed'
    case 'error':
      return 'Error'
    default:
      return status
  }
})

const canSave = computed(() => !!activeWorkflow.value && !saving.value && dirty.value)
const canRun = computed(() => !!activeWorkflow.value && !saving.value && !running.value && nodes.value.length > 0)
const canExport = computed(() => !!activeWorkflow.value)
const canDelete = computed(() => !!activeWorkflow.value && !saving.value && !running.value)

// Node lock state: when true, nodes cannot be dragged
const nodesLocked = ref(false)
// Keep Vue Flow's global draggable flag in sync with our lock state
watch(
  nodesLocked,
  (locked) => {
    nodesDraggable.value = !locked
  },
  { immediate: true },
)

function onZoomIn() {
  zoomIn()
}
function onZoomOut() {
  zoomOut()
}
function onFitView() {
  try {
    fitView({ padding: 0.15 })
  } catch (e) {
    // ignore
  }
}
// schedule a fitView on next tick to ensure nodes/edges are rendered
async function scheduleFitView() {
  await nextTick()
  // small delay to let VueFlow compute bounds
  requestAnimationFrame(() => {
    onFitView()
  })
}
function toggleNodeLock() {
  nodesLocked.value = !nodesLocked.value
}

const EDGE_STYLES = ['default', 'smoothstep', 'step', 'straight', 'simplebezier'] as const
type EdgeStyle = (typeof EDGE_STYLES)[number]
const EDGE_STYLE_LABELS: Record<EdgeStyle, string> = {
  default: 'Default',
  smoothstep: 'Smoothstep',
  step: 'Step',
  straight: 'Straight',
  simplebezier: 'Simple Bezier',
}

const edgeStyleIndex = ref(0)
const currentEdgeStyle = computed<EdgeStyle>(() => EDGE_STYLES[edgeStyleIndex.value])
const edgeStyleLabel = computed(() => EDGE_STYLE_LABELS[currentEdgeStyle.value])
const edgeStyleButtonTitle = computed(() => `Edge style: ${edgeStyleLabel.value}. Click to cycle`)
const edgeStyleAriaLabel = computed(() => `Switch edge style (current: ${edgeStyleLabel.value})`)

function normalizeEdgesWithCurrentStyle(list: Edge[]): Edge[] {
  const type = currentEdgeStyle.value
  return list.map((edge) => ({ ...edge, type }))
}

function applyEdgeStyleToExistingEdges() {
  if (!edges.value.length) return
  edges.value = normalizeEdgesWithCurrentStyle(edges.value)
}

function cycleEdgeStyle() {
  edgeStyleIndex.value = (edgeStyleIndex.value + 1) % EDGE_STYLES.length
}

watch(currentEdgeStyle, () => {
  applyEdgeStyleToExistingEdges()
})

// Expand/Collapse all nodes via provided signals
function collapseAll() {
  collapseAllSeq.value += 1
  nodesCollapsed.value = true
}
function expandAll() {
  expandAllSeq.value += 1
  nodesCollapsed.value = false
}
function toggleCollapseAll() {
  if (nodesCollapsed.value) expandAll()
  else collapseAll()
}

type DagreDirection = 'TB' | 'LR'

function onAutoLayout(direction: DagreDirection) {
  // We don't need to refresh the UI snapshot here; we'll avoid triggering
  // membership/delta translation during layout and finalize it afterward.
  
  // Separate nodes into top-level (including groups) and children (inside groups)
  const topLevelNodes: WarppNode[] = []
  const childNodes: Map<string, WarppNode[]> = new Map()
  
  for (const n of nodes.value) {
    if (isGroupNode(n)) {
      // Group nodes are always top-level
      topLevelNodes.push(n)
    } else if (isStepLikeNode(n)) {
      const data = n.data as StepNodeData
      if (data.groupId) {
        // This node is inside a group
        if (!childNodes.has(data.groupId)) {
          childNodes.set(data.groupId, [])
        }
        childNodes.get(data.groupId)!.push(n)
      } else {
        // This node is top-level
        topLevelNodes.push(n)
      }
    } else {
      // Unknown node type, treat as top-level
      topLevelNodes.push(n)
    }
  }
  
  // Build dagre graph with only top-level nodes
  const g = new dagre.graphlib.Graph()
  // Slightly increase separations to account for handles/margins
  g.setGraph({ rankdir: direction, nodesep: 60, ranksep: 80, marginx: 24, marginy: 24 })
  g.setDefaultEdgeLabel(() => ({}))

  // Add only top-level nodes with measured sizes
  for (const n of topLevelNodes) {
    const size = readNodeSize(n)
    g.setNode(n.id, { width: size.width || DAGRE_NODE_BASE_WIDTH, height: size.height || DAGRE_NODE_BASE_HEIGHT })
  }
  
  // Add edges between top-level nodes only. If an endpoint is a child, lift it to its group id.
  const edgeKey = (a: string, b: string) => `${a}->${b}`
  const added = new Set<string>()
  for (const e of edges.value) {
    const sourceNode = nodes.value.find((n) => n.id === e.source)
    const targetNode = nodes.value.find((n) => n.id === e.target)
    if (!sourceNode || !targetNode) continue

    const sourceGroupId = isStepLikeNode(sourceNode) ? (sourceNode.data as StepNodeData).groupId : undefined
    const targetGroupId = isStepLikeNode(targetNode) ? (targetNode.data as StepNodeData).groupId : undefined

    // Skip edges entirely inside the same group
    if (sourceGroupId && targetGroupId && sourceGroupId === targetGroupId) continue

    // Lift endpoints to their top-level containers (group id if present, else node id)
    const liftedSource = sourceGroupId ?? sourceNode.id
    const liftedTarget = targetGroupId ?? targetNode.id

    // Avoid self-loops after lifting
    if (liftedSource === liftedTarget) continue

    // Avoid duplicates
    const key = edgeKey(liftedSource, liftedTarget)
    if (added.has(key)) continue
    added.add(key)

    g.setEdge(liftedSource, liftedTarget)
  }

  try {
    dagre.layout(g)
  } catch (e) {
    console.warn('dagre layout failed', e)
    return
  }

  // Build map of group movements
  const groupDeltas = new Map<string, { dx: number; dy: number }>()
  topLevelNodes.forEach((n) => {
    if (!isGroupNode(n)) return
    const oldPos = n.position ?? { x: 0, y: 0 }
    const dagrePos = g.node(n.id) as { x: number; y: number } | undefined
    if (!dagrePos) return
    
    const size = readNodeSize(n)
    const width = size.width || DAGRE_NODE_BASE_WIDTH
    const height = size.height || DAGRE_NODE_BASE_HEIGHT
    const newPos = {
      x: dagrePos.x - width / 2,
      y: dagrePos.y - height / 2,
    }
    
    groupDeltas.set(n.id, {
      dx: newPos.x - oldPos.x,
      dy: newPos.y - oldPos.y,
    })
  })

  // Apply positions: top-level nodes get dagre positions, children move with their groups
  const positioned = nodes.value.map((n) => {
    // Check if this is a child node
    if (isStepLikeNode(n)) {
      const data = n.data as StepNodeData
      if (data.groupId) {
        // This is a child node - move it by the same delta as its parent group
        const delta = groupDeltas.get(data.groupId)
        if (delta) {
          const oldPos = n.position ?? { x: 0, y: 0 }
          return { ...n, position: { x: oldPos.x + delta.dx, y: oldPos.y + delta.dy } }
        }
        // If no delta found, keep original position
        return n
      }
    }
    
    // This is a top-level node (ungrouped or group) - use dagre position
    const pos = g.node(n.id) as { x: number; y: number } | undefined
    if (!pos) return n
    const size = readNodeSize(n)
    const width = size.width || DAGRE_NODE_BASE_WIDTH
    const height = size.height || DAGRE_NODE_BASE_HEIGHT
    const x = pos.x - width / 2
    const y = pos.y - height / 2
    return { ...n, position: { x, y } }
  })

  // Prevent refreshUiSnapshot from translating children a second time
  // while we commit the new positions.
  isApplyingLayout.value = true
  nodes.value = positioned
  // Update latest snapshot and previousGroupPositions without translating
  // children or mutating memberships.
  refreshUiSnapshot()
  isApplyingLayout.value = false
  // Fit view after positioning
  scheduleFitView()
}

function isUtilityToolName(name?: string | null): boolean {
  if (typeof name !== 'string') return false
  return /^utility[_-]/.test(name)
}

function prettyUtilityLabel(name: string): string {
  if (!isUtilityToolName(name)) return name
  const readable = name.replace(/^utility[_-]/, '')
  return readable.replace(/[_-]+/g, ' ').replace(/\b\w/g, (ch) => ch.toUpperCase())
}

function clearRunTraceTimers() {
  runTraceTimers.forEach((id) => clearTimeout(id))
  runTraceTimers = []
}

function resetRunView() {
  clearRunTraceTimers()
  editorMode.value = 'design'
  closeResultModal()
}

function applyRunTrace(entries: WarppStepTrace[]) {
  clearRunTraceTimers()
  warppRunStore.runTrace = {}
  if (!entries.length) {
    return
  }
  entries.forEach((entry, index) => {
    const delay = Math.min(index * 150, 1500)
    const timer = setTimeout(() => {
      warppRunStore.runTrace = { ...warppRunStore.runTrace, [entry.stepId]: entry }
    }, delay)
    runTraceTimers.push(timer)
  })
}

function setEditorMode(mode: 'design' | 'run') {
  if (mode === editorMode.value) return
  if (mode === 'run' && !hasRunTrace.value && !running.value) {
    return
  }
  editorMode.value = mode
  // Freeze node dimensions when entering run mode to prevent auto-resize from content changes
  if (mode === 'run') {
    try {
      // Use current measured size to set explicit width/height styles
      nodes.value.forEach((n) => {
        const size = readNodeSize(n)
        updateNode(n.id, (node) => {
          const baseStyle: Record<string, any> =
            typeof node.style === 'function' ? ((node.style as any)(node) ?? {}) : { ...(node.style as any ?? {}) }
          return {
            style: {
              ...baseStyle,
              width: toPx(size.width),
              // Also lock height to current size; content will scroll within
              height: toPx(size.height),
            },
          }
        })
      })
    } catch (e) {
      console.warn('Failed to freeze node sizes for run mode', e)
    }
  }
  if (mode === 'design') {
    closeResultModal()
  }
}

function formatJSON(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch (err) {
    console.warn('Failed to stringify value for modal', err)
    return String(value)
  }
}

// Metadata modal state
const showMetaModal = ref(false)
const metaDescription = ref('')
const metaKeywords = ref('')
const metaSaveDisabled = computed(() => metaDescription.value.trim().length === 0 || parseKeywords(metaKeywords.value).length === 0)

const showHelpModal = ref(false)

function openMetaModal() {
  if (!activeWorkflow.value) return
  // Pre-fill from current workflow
  metaDescription.value = activeWorkflow.value.description ?? ''
  metaKeywords.value = (activeWorkflow.value.keywords ?? []).join(', ')
  showMetaModal.value = true
}
function closeMetaModal() {
  showMetaModal.value = false
}

function openHelpModal() {
  showHelpModal.value = true
}

function closeHelpModal() {
  showHelpModal.value = false
}

function parseKeywords(input: string): string[] {
  return input
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
}

async function onSubmitMetadata() {
  if (!activeWorkflow.value) return
  if (metaSaveDisabled.value) return
  const desc = metaDescription.value.trim()
  const kws = parseKeywords(metaKeywords.value)
  const saved = await performSave(desc, kws)
  if (saved) {
    closeMetaModal()
  }
}

// MiniMap styling helpers (use theme CSS variables)
function miniMapNodeColor() {
  // Base fill uses surface-muted for cohesion; selection handled by library styles
  return 'rgb(var(--color-surface-muted))'
}
function miniMapNodeStroke() {
  return 'rgb(var(--color-border))'
}

onMounted(async () => {
  loading.value = true
  try {
    const [toolResp, workflows] = await Promise.all([
      fetchWarppTools().catch((err) => {
        console.error('warpp tools', err)
        return [] as WarppTool[]
      }),
      fetchWarppWorkflows(),
    ])
    tools.value = toolResp
    workflowList.value = workflows
    if (selectedIntent.value) {
      await loadWorkflow(selectedIntent.value)
    } else if (workflows.length > 0) {
      selectedIntent.value = workflows[0].intent
      // ensure initial selection loads immediately instead of waiting for watcher timing
      await nextTick()
      await loadWorkflow(selectedIntent.value)
    }
  } catch (err: any) {
    error.value = err?.message ?? 'Failed to load workflows'
  } finally {
    loading.value = false
    // initial fit once the initial load settles
    scheduleFitView()
  }
})

watch(selectedIntent, async (intent) => {
  resetRunView()
  if (!intent) {
    nodes.value = []
    edges.value = []
    activeWorkflow.value = null
    return
  }
  // If this is a locally-created unsaved workflow, hydrate from local instead of fetching
  const local = localWorkflows.value.get(intent)
  if (local) {
    error.value = ''
    isHydrating.value = true
    try {
      activeWorkflow.value = local
      nodes.value = []
      edges.value = []
      dirty.value = false
    } finally {
      await nextTick()
      isHydrating.value = false
    }
    return
  }
  loading.value = true
  error.value = ''
  try {
    await loadWorkflow(intent)
  } catch (err: any) {
    error.value = err?.message ?? 'Failed to load workflow'
  } finally {
    loading.value = false
  }
})

// Throttled sync to avoid heavy recomputation on each keystroke inside node editors
let syncScheduled = false
watch(
  nodes,
  () => {
    if (isHydrating.value) return
    if (syncScheduled) return
    syncScheduled = true
    requestAnimationFrame(() => {
      syncScheduled = false
      if (isHydrating.value) return
      syncWorkflowFromNodes()
      dirty.value = true
    })
  },
  { deep: true },
)

// Keep workflow.depends_on in sync when edges change (add/remove/reconnect)
watch(
  edges,
  () => {
    if (isHydrating.value) return
    if (syncScheduled) return
    syncScheduled = true
    requestAnimationFrame(() => {
      syncScheduled = false
      if (isHydrating.value) return
      syncWorkflowFromNodes()
      dirty.value = true
    })
  },
  { deep: true },
)

function workflowToNodes(wf: WarppWorkflow): WarppNode[] {
  const layout = wf.ui?.layout ?? {}
  const parents = wf.ui?.parents ?? {}
  const groupsMeta = wf.ui?.groups ?? []
  const notesMeta = (wf.ui as any)?.notes ?? latestUiSnapshot.value.notes ?? []

  const groupNodes: WarppNode[] = groupsMeta.map((group, idx) => {
    // Prefer saved layout; if missing or partial, merge with last client snapshot to
    // retain width/height captured during editing.
    const clientStored = latestUiSnapshot.value?.layout?.[group.id]
    const savedStored = layout[group.id]
    const stored: LayoutEntry | undefined = savedStored
      ? { ...(clientStored ?? {}), ...savedStored }
      : clientStored ?? undefined
    const position = resolveNodePosition(stored, idx)
    const style = buildNodeStyle('group', stored)
    return {
      id: group.id,
      type: 'warppGroup',
      position,
      style,
      draggable: true,
      selectable: true,
      data: {
        kind: 'group',
        label: group.label || 'Group',
        collapsed: group.collapsed ?? false,
        color: group.color,
      },
    }
  })

  const stepNodes: WarppNode[] = wf.steps.map((step, idx) => {
    const stored = layout[step.id]
    const position = resolveNodePosition(stored, idx)
    const utility = isUtilityToolName(step.tool?.name)
    const style = buildNodeStyle(utility ? 'utility' : 'step', stored)
    return {
      id: step.id,
      type: utility ? 'warppUtility' : 'warppStep',
      position,
      style,
      draggable: true,
      selectable: true,
      data: {
        order: idx,
        step: JSON.parse(JSON.stringify(step)) as WarppStep,
        kind: utility ? 'utility' : 'step',
        groupId: parents[step.id],
      },
    }
  })

  const noteNodes: WarppNode[] = (notesMeta as any[]).map((note: any, idx: number) => {
    const stored = layout[note.id]
    const position = resolveNodePosition(stored, idx + wf.steps.length)
    const style = buildNodeStyle('utility', stored)
    return {
      id: note.id,
      type: 'warppSticky',
      position,
      style,
      draggable: true,
      selectable: true,
      connectable: false,
      data: {
        kind: 'utility',
        label: note.label ?? 'Sticky Note',
        note: note.note ?? '',
        color: note.color,
        groupId: parents[note.id],
      } as any,
    }
  })

  return [...groupNodes, ...stepNodes, ...noteNodes]
}

function resolveNodePosition(stored: LayoutEntry | undefined, index: number) {
  if (stored && Number.isFinite(stored.x) && Number.isFinite(stored.y)) {
    return { x: stored.x, y: stored.y }
  }
  return {
    x: DEFAULT_LAYOUT_START_X + index * DEFAULT_LAYOUT_HORIZONTAL_GAP,
    y: DEFAULT_LAYOUT_START_Y,
  }
}

function workflowToEdges(wf: WarppWorkflow): Edge[] {
  const out: Edge[] = []
  // Prefer explicit depends_on if present on any step
  const hasDag = wf.steps.some((s) => Array.isArray(s.depends_on) && s.depends_on.length > 0)
  if (hasDag) {
    for (const step of wf.steps) {
      const deps = step.depends_on ?? []
      for (const dep of deps) {
        out.push({ id: `e-${dep}-${step.id}`, source: dep, target: step.id })
      }
    }
    return normalizeEdgesWithCurrentStyle(out)
  }
  // Fallback to sequential
  for (let i = 1; i < wf.steps.length; i += 1) {
    const prev = wf.steps[i - 1]
    const curr = wf.steps[i]
    out.push({ id: `e-${prev.id}-${curr.id}`, source: prev.id, target: curr.id })
  }
  return normalizeEdgesWithCurrentStyle(out)
}

async function loadWorkflow(intent: string) {
  isHydrating.value = true
  try {
    const wf = await fetchWarppWorkflow(intent)
    // Merge server UI with locally cached UI for resilience (especially for groups)
    const cached = getCachedUi(intent)
    const mergedUi: WarppWorkflowUI = {
      ...(wf.ui ?? {}),
      layout: { ...(cached.layout ?? {}), ...((wf.ui?.layout ?? {}) as LayoutMap) },
      parents: (wf.ui?.parents ?? cached.parents) as any,
      groups: (wf.ui?.groups ?? cached.groups) as any,
      notes: (wf.ui?.notes ?? cached.notes) as any,
    }
    const mergedWf: WarppWorkflow = { ...wf, ui: mergedUi }
    // Seed latest snapshot so hydration can preserve sizes
  latestUiSnapshot.value = { layout: mergedUi.layout ?? {}, parents: mergedUi.parents ?? {}, groups: mergedUi.groups ?? [], memberships: {}, notes: mergedUi.notes ?? [] }
    const nextNodes = workflowToNodes(mergedWf)
    const nextEdges = workflowToEdges(mergedWf)

    activeWorkflow.value = mergedWf
    edges.value = nextEdges
    nodes.value = nextNodes
  } finally {
    await nextTick()
    isHydrating.value = false
    // Fit view after nodes are rendered
    scheduleFitView()
  }
}

function getOrderedStepNodes(): StepNode[] {
  return nodes.value
    .filter((node) => isStepLikeNode(node) && Boolean((node.data as any)?.step))
    .map((node) => node as StepNode)
    .sort((a, b) => (a.data?.order ?? 0) - (b.data?.order ?? 0))
}

function nextStepOrder(): number {
  return nodes.value.filter((node) => isStepLikeNode(node) && Boolean((node.data as any)?.step)).length
}

function translateGroupChildren(groupId: string, dx: number, dy: number): boolean {
  if (!dx && !dy) return false
  let changed = false
  const translated = nodes.value.map<WarppNode>((node) => {
    if (!isStepLikeNode(node)) return node
    const data = node.data as StepNodeData
    if (data.groupId !== groupId) return node
    const position = node.position ?? { x: 0, y: 0 }
    const newPosition = { x: position.x + dx, y: position.y + dy }
    changed = true
    return { ...node, position: newPosition } as WarppNode
  })
  if (changed) {
    nodes.value = translated
  }
  return changed
}

function applyMembership(snapshot: UiSnapshot) {
  const membership = snapshot.memberships
  let changed = false
  const nextNodes = nodes.value.map<WarppNode>((node) => {
    if (!isStepLikeNode(node)) return node
    const data = node.data as StepNodeData
    const desired = membership[node.id]
    const current = data.groupId
    if (current === desired || (!current && !desired)) {
      return node
    }
    const nextData: StepNodeData = { ...data }
    if (desired) nextData.groupId = desired
    else delete nextData.groupId
    changed = true
    return { ...node, data: nextData } as WarppNode
  })
  if (changed) {
    nodes.value = nextNodes
  }
}

const latestUiSnapshot = ref<UiSnapshot>({ layout: {}, parents: {}, groups: [], memberships: {}, notes: [] })
const previousGroupPositions = new Map<string, { x: number; y: number }>()
// Guard to avoid double-moving children while auto-layout commits positions
const isApplyingLayout = ref(false)

function removeGroup(groupId: string) {
  let changed = false
  const updatedNodes: WarppNode[] = []
  nodes.value.forEach((node) => {
    if (node.id === groupId && isGroupNode(node)) {
      changed = true
      return
    }
    if (isStepLikeNode(node)) {
      const data = node.data as StepNodeData
      if (data.groupId === groupId) {
        const nextData: StepNodeData = { ...data }
        delete nextData.groupId
        updatedNodes.push({ ...node, data: nextData } as WarppNode)
        changed = true
        return
      }
    }
    updatedNodes.push(node)
  })
  if (!changed) return
  nodes.value = updatedNodes
  previousGroupPositions.delete(groupId)
  refreshUiSnapshot()
  dirty.value = true
}

function refreshUiSnapshot(): UiSnapshot {
  let snapshot = collectUiState(nodes.value)

  // When applying auto-layout, do NOT translate children or mutate memberships.
  if (!isApplyingLayout.value) {
    const deltas: Array<{ id: string; dx: number; dy: number }> = []
    snapshot.groups.forEach((group) => {
      const entry = snapshot.layout[group.id]
      if (!entry) return
      const prev = previousGroupPositions.get(group.id)
      if (prev) {
        const dx = entry.x - prev.x
        const dy = entry.y - prev.y
        if (dx || dy) {
          deltas.push({ id: group.id, dx, dy })
        }
      }
    })
    if (deltas.length) {
      deltas.forEach(({ id, dx, dy }) => translateGroupChildren(id, dx, dy))
      snapshot = collectUiState(nodes.value)
    }
  }

  previousGroupPositions.clear()
  snapshot.groups.forEach((group) => {
    const entry = snapshot.layout[group.id]
    if (entry) {
      previousGroupPositions.set(group.id, { x: entry.x, y: entry.y })
    }
  })

  if (!isApplyingLayout.value) {
    applyMembership(snapshot)
  }
  latestUiSnapshot.value = snapshot
  return snapshot
}

function syncWorkflowFromNodes() {
  if (isHydrating.value) return
  if (!activeWorkflow.value) return
  const snapshot = refreshUiSnapshot()
  const orderedNodes = getOrderedStepNodes()
  const incoming: Record<string, string[]> = {}
  for (const e of edges.value) {
    if (!incoming[e.target]) incoming[e.target] = []
    incoming[e.target].push(e.source)
  }
  const steps = orderedNodes.map((node) => {
    const data = node.data as StepNodeData
    const step = { ...(data.step ?? ({} as WarppStep)) }
    step.id = node.id
    step.depends_on = (incoming[node.id] ?? []).slice()
    return step
  })
  const uiLayout = snapshot.layout
  const uiParents = snapshot.parents
  const uiGroups = snapshot.groups
  const uiNotes = snapshot.notes
  activeWorkflow.value = {
    ...activeWorkflow.value,
    steps,
    ui: {
      ...(activeWorkflow.value.ui ?? {}),
      layout: uiLayout,
      parents: Object.keys(uiParents).length ? uiParents : undefined,
      groups: uiGroups.length ? uiGroups : undefined,
      notes: uiNotes.length ? uiNotes : undefined,
    },
  }
  // Persist UI snapshot locally for this intent for resilience across reloads
  try {
    const intent = activeWorkflow.value.intent
    setCachedUi(intent, { layout: uiLayout, parents: uiParents, groups: uiGroups, notes: uiNotes })
  } catch {}
}

function onDragOver(event: DragEvent) {
  if (!event.dataTransfer?.types.includes(DRAG_DATA_TYPE)) {
    return
  }
  event.preventDefault()
  event.dataTransfer.dropEffect = 'copy'
}

function onDrop(event: DragEvent) {
  if (!event.dataTransfer?.types.includes(DRAG_DATA_TYPE)) {
    return
  }
  event.preventDefault()
  isDraggingFromPalette.value = false

  const token = event.dataTransfer.getData(DRAG_DATA_TYPE)
  if (!token || !flowWrapper.value) {
    return
  }

  const bounds = flowWrapper.value.getBoundingClientRect()
  const position = project({
    x: event.clientX - bounds.left,
    y: event.clientY - bounds.top,
  })

  if (token === GROUP_DRAG_TOKEN) {
    appendNode(createGroupNode(position))
    dirty.value = true
    return
  }
  if (token === STICKY_DRAG_TOKEN) {
    appendNode(createStickyNode(position))
    dirty.value = true
    return
  }

  const tool = toolMap.value.get(token)
  if (!tool) {
    return
  }

  addToolNode(tool, position)
}

function onPaletteDragStart(event: DragEvent, tool: WarppTool) {
  if (!event.dataTransfer) {
    return
  }
  isDraggingFromPalette.value = true
  event.dataTransfer.setData(DRAG_DATA_TYPE, tool.name)
  event.dataTransfer.setData('text/plain', tool.name)
  event.dataTransfer.effectAllowed = 'copyMove'
}

function onGroupDragStart(event: DragEvent) {
  if (!event.dataTransfer) return
  isDraggingFromPalette.value = true
  event.dataTransfer.setData(DRAG_DATA_TYPE, GROUP_DRAG_TOKEN)
  event.dataTransfer.setData('text/plain', 'Group Container')
  event.dataTransfer.effectAllowed = 'copyMove'
}

function onStickyDragStart(event: DragEvent) {
  if (!event.dataTransfer) return
  isDraggingFromPalette.value = true
  event.dataTransfer.setData(DRAG_DATA_TYPE, STICKY_DRAG_TOKEN)
  event.dataTransfer.setData('text/plain', 'Sticky Note')
  event.dataTransfer.effectAllowed = 'copyMove'
}

function onPaletteDragEnd() {
  isDraggingFromPalette.value = false
}

function onConnect(connection: Connection) {
  const { source, target } = connection
  if (!source || !target) return
  if (source === target) return // no self-loop
  // prevent duplicate edges
  if (edges.value.some((e) => e.source === source && e.target === target)) return
  const id = `e-${source}-${target}-${Math.random().toString(36).slice(2, 8)}`
  edges.value = normalizeEdgesWithCurrentStyle([...edges.value, { id, source, target }])
}

function addToolNode(tool: WarppTool, position: { x: number; y: number }) {
  if (!activeWorkflow.value) {
    return
  }
  if (isUtilityToolName(tool.name)) {
    appendNode(createUtilityNode(tool, position))
    return
  }
  appendNode(createWorkflowNode(tool, position))
}

function findGroupAtPoint(point: { x: number; y: number }): string | undefined {
  const snapshot = refreshUiSnapshot()
  for (const group of snapshot.groups) {
    const entry = snapshot.layout[group.id]
    if (!entry) continue
    const width = entry.width ?? WARPP_GROUP_NODE_DIMENSIONS.defaultWidth
    const height = entry.height ?? WARPP_GROUP_NODE_DIMENSIONS.defaultHeight
    if (point.x >= entry.x && point.x <= entry.x + width && point.y >= entry.y && point.y <= entry.y + height) {
      return group.id
    }
  }
  return undefined
}

function generateGroupId(): string {
  let candidate = ''
  do {
    const unique =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? (crypto.randomUUID?.() ?? Math.random().toString(36).slice(2, 10))
        : Math.random().toString(36).slice(2, 10)
    candidate = `group-${unique.slice(0, 8)}`
  } while (nodes.value.some((node) => node.id === candidate))
  return candidate
}

function createGroupNode(position: { x: number; y: number }): WarppNode {
  const id = generateGroupId()
  const style = buildNodeStyle('group')
  return {
    id,
    type: 'warppGroup',
    position,
    style,
    draggable: true,
    selectable: true,
    data: {
      kind: 'group',
      label: 'Group',
      collapsed: false,
    },
  }
}

function generateStickyId(): string {
  let candidate = ''
  do {
    const unique =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? (crypto.randomUUID?.() ?? Math.random().toString(36).slice(2, 10))
        : Math.random().toString(36).slice(2, 10)
    candidate = `note-${unique.slice(0, 8)}`
  } while (nodes.value.some((node) => node.id === candidate))
  return candidate
}

function createStickyNode(position: { x: number; y: number }): WarppNode {
  const id = generateStickyId()
  const style = buildNodeStyle('utility')
  // Ensure the sticky note has an explicit initial height since its children are absolutely positioned
  const dims = getDefaultDimensions('utility')
  style.height = toPx(dims.defaultHeight)
  const groupId = findGroupAtPoint(position)
  return {
    id,
    type: 'warppSticky',
    position,
    style,
    draggable: true,
    selectable: true,
    connectable: false,
    data: {
      kind: 'utility',
      label: 'Note',
      note: '',
      groupId: groupId ?? undefined,
    } as any,
  }
}

function createWorkflowNode(tool: WarppTool, position: { x: number; y: number }): WarppNode {
  const id = generateStepId(tool.name)
  const order = nextStepOrder()
  const step: WarppStep = {
    id,
    text: tool.description ?? tool.name,
    publish_result: false,
    tool: { name: tool.name },
  }
  const style = buildNodeStyle('step')
  const groupId = findGroupAtPoint(position)
  return {
    id,
    type: 'warppStep',
    position,
    style,
    data: {
      order,
      step,
      kind: 'step',
      groupId: groupId ?? undefined,
    },
  } as WarppNode
}

function createUtilityNode(tool: WarppTool, position: { x: number; y: number }): WarppNode {
  const id = generateStepId(tool.name)
  const order = nextStepOrder()
  const displayName = prettyUtilityLabel(tool.name)
  const step: WarppStep = {
    id,
    text: displayName,
    publish_result: false,
    tool: { name: tool.name, args: { label: displayName, text: '', output_attr: '' } },
  }
  const style = buildNodeStyle('utility')
  const groupId = findGroupAtPoint(position)
  return {
    id,
    type: 'warppUtility',
    position,
    style,
    data: {
      order,
      step,
      kind: 'utility',
      groupId: groupId ?? undefined,
    },
  } as WarppNode
}

function appendNode(node: WarppNode) {
  const previousStep = [...nodes.value].reverse().find((candidate) => isStepLikeNode(candidate))
  nodes.value = [...nodes.value, node]
  const prevHasStep = previousStep && Boolean((previousStep!.data as any)?.step)
  const newHasStep = isStepLikeNode(node) && Boolean((node.data as any)?.step)
  if (newHasStep && prevHasStep && isStepLikeNode(previousStep!)) {
    edges.value = normalizeEdgesWithCurrentStyle([
      ...edges.value,
      { id: `e-${previousStep.id}-${node.id}`, source: previousStep.id, target: node.id },
    ])
  }
  refreshUiSnapshot()
}

function generateStepId(toolName: string): string {
  const base =
    toolName
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/(^-|-$)/g, '') || 'step'
  let candidate = ''
  do {
    const unique =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? (crypto.randomUUID?.() ?? Math.random().toString(36).slice(2, 10))
        : Math.random().toString(36).slice(2, 10)
    candidate = `${base}-${unique.slice(0, 8)}`
  } while (nodes.value.some((node) => node.id === candidate))
  return candidate
}

async function onSave(): Promise<WarppWorkflow | null> {
  if (!activeWorkflow.value) return null
  // Open metadata modal for user to confirm/edit description and keywords
  openMetaModal()
  return null
}

// Core save logic (optionally overriding description/keywords)
async function performSave(description?: string, keywords?: string[]): Promise<WarppWorkflow | null> {
  if (!activeWorkflow.value) return null
  if (!dirty.value && description === undefined && keywords === undefined) return activeWorkflow.value
  saving.value = true
  error.value = ''
  try {
    // Ensure any in-flight UI updates (resize/label edits/drag) are flushed before snapshot
    await nextTick()
    const snapshot = refreshUiSnapshot()
    const orderedNodes = getOrderedStepNodes()
    const incoming: Record<string, string[]> = {}
    for (const e of edges.value) {
      if (!incoming[e.target]) incoming[e.target] = []
      incoming[e.target].push(e.source)
    }
    const steps = orderedNodes.map((node) => {
      const data = node.data as StepNodeData
      const step = { ...(data.step ?? ({} as WarppStep)) }
      step.id = node.id
      step.depends_on = (incoming[node.id] ?? []).slice()
      return step as WarppStep
    })
    const layout = snapshot.layout
    const parents = snapshot.parents
    const groups = snapshot.groups
    const payload: WarppWorkflow = {
      ...activeWorkflow.value,
      description: description ?? activeWorkflow.value.description,
      keywords: keywords ?? activeWorkflow.value.keywords,
      steps,
      ui: {
        ...(activeWorkflow.value.ui ?? {}),
        layout,
        parents: Object.keys(parents).length ? parents : undefined,
        groups: groups.length ? groups : undefined,
      },
    }
    runLogs.value.push('[save] PUT /api/warpp/workflows/' + encodeURIComponent(payload.intent))
    console.log('[DEBUG] Payload groups:', payload.ui?.groups)
    console.log('[DEBUG] Payload layout keys:', Object.keys(payload.ui?.layout ?? {}))
  // Capture the exact snapshot we used for layout so we can seed hydration fallback
  const preSaveSnapshot = snapshot
  const saved = await saveWarppWorkflow(payload)
    console.log('[DEBUG] Server returned groups:', saved.ui?.groups)
    console.log('[DEBUG] Server returned layout keys:', Object.keys(saved.ui?.layout ?? {}))
    const payloadUi = payload.ui ?? {}
    const savedUi = saved.ui ?? {}
    const mergedLayout: Record<string, LayoutEntry> | undefined = (() => {
      if (!payloadUi.layout && !savedUi.layout) return undefined
      return { ...(payloadUi.layout ?? {}), ...(savedUi.layout ?? {}) }
    })()
    const mergedUi: WarppWorkflow['ui'] = {
      ...payloadUi,
      ...savedUi,
      ...(mergedLayout ? { layout: mergedLayout } : {}),
    }
    // CRITICAL: Force groups and parents from payload if server omitted them
    // This ensures group nodes survive save/reload even when backend doesn't persist ui.groups
    if (!savedUi.groups?.length && payloadUi.groups?.length) {
      mergedUi.groups = payloadUi.groups
    }
    if (!savedUi.parents && payloadUi.parents) {
      mergedUi.parents = payloadUi.parents
    }
    if (!savedUi.layout && payloadUi.layout) {
      mergedUi.layout = payloadUi.layout
    }
    const normalizedSaved: WarppWorkflow = {
      ...saved,
      ui: mergedUi,
    }
    console.log('[DEBUG] Merged UI groups:', normalizedSaved.ui?.groups)
    console.log('[DEBUG] Merged UI layout keys:', Object.keys(normalizedSaved.ui?.layout ?? {}))
    // Also update the client-side UI cache immediately with the merged UI
    try {
      setCachedUi(normalizedSaved.intent, {
        layout: mergedUi.layout,
        parents: mergedUi.parents,
        groups: mergedUi.groups,
      })
    } catch {}
    runLogs.value.push('[save] 200 OK')
    // If this workflow was locally-created, clear the local marker
    localWorkflows.value.delete(payload.intent)
    const listIdx = workflowList.value.findIndex((wf) => wf.intent === normalizedSaved.intent)
    if (listIdx !== -1) workflowList.value.splice(listIdx, 1, normalizedSaved)
    else workflowList.value.push(normalizedSaved)
    isHydrating.value = true
    try {
      // Seed latest snapshot so hydration preserves sizes/positions even if server omits them
      latestUiSnapshot.value = preSaveSnapshot
      activeWorkflow.value = normalizedSaved
      nodes.value = workflowToNodes(normalizedSaved)
      edges.value = workflowToEdges(normalizedSaved)
      dirty.value = false
    } finally {
      await nextTick()
      isHydrating.value = false
    }
    // Fit after successful save rehydrate
    scheduleFitView()
  return normalizedSaved
  } catch (err: any) {
    const msg = err?.message ?? 'Failed to save workflow'
    error.value = msg
    runLogs.value.push('[save] error: ' + msg)
    return null
  } finally {
    saving.value = false
  }
}

async function onRun() {
  if (!activeWorkflow.value) return
  warppRunStore.error = ''
  warppRunStore.runOutput = ''
  warppRunStore.runLogs = []
  editorMode.value = 'run'
  clearRunTraceTimers()
  warppRunStore.runTrace = {}
  const intent = activeWorkflow.value.intent
  warppRunStore.runLogs.push(`▶ Starting run for intent "${intent}"`)
  // Capture need to save at start (canSave may change mid-process)
  const needSave = canSave.value
  if (needSave) {
    warppRunStore.runLogs.push('… Saving workflow before run')
    const saved = await performSave()
    if (saved) warppRunStore.runLogs.push('✓ Save complete')
    else warppRunStore.runLogs.push('✗ Save failed – proceeding with current in-memory workflow')
  }
  try {
    const res = await warppRunStore.startRun(intent, `Run workflow: ${intent}`)
    applyRunTrace(res.trace ?? [])
  } catch (err: any) {
    resetRunView()
  }
}

function onCancelRun() {
  warppRunStore.cancelRun()
}

async function onDelete() {
  if (!activeWorkflow.value) return
  const intent = activeWorkflow.value.intent
  const confirmed = window.confirm(`Delete workflow "${intent}"? This cannot be undone.`)
  if (!confirmed) return
  try {
    await deleteWarppWorkflow(intent)
    // Remove from local list/maps and reset selection
    localWorkflows.value.delete(intent)
    const idx = workflowList.value.findIndex(w => w.intent === intent)
    if (idx !== -1) workflowList.value.splice(idx, 1)
    if (selectedIntent.value === intent) {
      selectedIntent.value = workflowList.value[0]?.intent ?? ''
    }
    activeWorkflow.value = null
    nodes.value = []
    edges.value = []
    dirty.value = false
  } catch (err: any) {
    alert(err?.message ?? 'Failed to delete workflow')
  }
}

function exportWorkflow() {
  if (!activeWorkflow.value) return
  // Build latest payload mirroring save logic (without network)
  const snapshot = refreshUiSnapshot()
  const orderedNodes = getOrderedStepNodes()
  const incoming: Record<string, string[]> = {}
  for (const e of edges.value) {
    if (!incoming[e.target]) incoming[e.target] = []
    incoming[e.target].push(e.source)
  }
  const steps = orderedNodes.map((node) => {
    const data = node.data as StepNodeData
    const step = { ...(data.step ?? ({} as WarppStep)) }
    step.id = node.id
    step.depends_on = (incoming[node.id] ?? []).slice()
    return step as WarppStep
  })
  const layout = snapshot.layout
  const parents = snapshot.parents
  const groups = snapshot.groups
  const payload: WarppWorkflow = {
    ...activeWorkflow.value,
    steps,
    ui: {
      ...(activeWorkflow.value.ui ?? {}),
      layout,
      parents: Object.keys(parents).length ? parents : undefined,
      groups: groups.length ? groups : undefined,
    },
  }

  // Safe stringify with cycle protection and function stripping
  const seen = new WeakSet()
  const json = JSON.stringify(
    payload,
    (_k, val) => {
      if (typeof val === 'function' || typeof val === 'symbol') return undefined
      if (val && typeof val === 'object') {
        if (seen.has(val)) return undefined
        seen.add(val)
      }
      return val
    },
    2,
  )

  const ts = new Date().toISOString().replace(/[:]/g, '-')
  // payload may come from older formats that used `name`; assert to any to access safely
  const base = (payload.intent || (payload as any).name || 'workflow')
  const filename = `${base}-${ts}.json`

  const blob = new Blob([json], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

function normalizeIntent(input: string): string {
  // Conservative normalization: trim and collapse spaces, restrict to [a-z0-9._-]
  // Keep it readable and filesystem-friendly
  const t = input.trim().toLowerCase()
  const collapsed = t.replace(/\s+/g, '-')
  const safe = collapsed.replace(/[^a-z0-9._-]/g, '-')
  return safe.replace(/^-+|-+$/g, '').slice(0, 64) || 'workflow'
}

async function onNew() {
  const name = window.prompt('Enter a name for the new workflow (intent):', '')
  if (name === null) return
  const intent = normalizeIntent(name)
  if (!intent) {
    alert('Please enter a valid name')
    return
  }
  if (workflowList.value.some((w) => w.intent === intent) || localWorkflows.value.has(intent)) {
    alert('A workflow with that name already exists')
    return
  }
  const wf: WarppWorkflow = { intent, description: '', steps: [] }
  // Track locally and show in dropdown immediately
  localWorkflows.value.set(intent, wf)
  workflowList.value.push(wf)
  // Switch to the new workflow view
  isHydrating.value = true
  try {
    selectedIntent.value = intent
    activeWorkflow.value = wf
    nodes.value = []
    edges.value = []
    dirty.value = false
  } finally {
    await nextTick()
    isHydrating.value = false
  }
}

function triggerImport() {
  if (importInput.value) {
    // reset value so selecting the same file twice still triggers change
    importInput.value.value = ''
    importInput.value.click()
  }
}

async function onImportSelected(event: Event) {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (input) input.value = ''
  if (!file) return

  let data: any
  try {
    const text = await file.text()
    data = JSON.parse(text)
  } catch (e: any) {
    alert('Invalid JSON file')
    return
  }

  // Accept legacy shape { name } or current { intent }
  let intent: string = normalizeIntent(String(data?.intent || data?.name || ''))
  if (!intent) {
    const provided = window.prompt('Enter an intent for this imported workflow:', file.name.replace(/\.json$/i, ''))
    if (provided === null) return
    intent = normalizeIntent(provided)
    if (!intent) {
      alert('Invalid intent')
      return
    }
  }

  // Ensure uniqueness; suggest a suffix if needed
  const exists = (i: string) => workflowList.value.some(w => w.intent === i) || localWorkflows.value.has(i)
  if (exists(intent)) {
    let base = intent.replace(/-import(-\d+)?$/, '')
    let candidate = `${base}-import`
    let n = 2
    while (exists(candidate)) {
      candidate = `${base}-import-${n++}`
    }
    const rename = window.prompt('A workflow with this intent already exists. Enter a new name:', candidate)
    if (rename === null) return
    const newIntent = normalizeIntent(rename)
    if (!newIntent) {
      alert('Invalid intent')
      return
    }
    intent = newIntent
    if (exists(intent)) {
      alert('A workflow with that name already exists')
      return
    }
  }

  const steps = Array.isArray(data?.steps) ? data.steps : []
  const wf: WarppWorkflow = {
    intent,
    description: typeof data?.description === 'string' ? data.description : '',
    keywords: Array.isArray(data?.keywords) ? data.keywords : undefined,
    max_concurrency: typeof data?.max_concurrency === 'number' ? data.max_concurrency : undefined,
    fail_fast: typeof data?.fail_fast === 'boolean' ? data.fail_fast : undefined,
    steps: steps.map((s: any) => ({
      id: String(s?.id ?? ''),
      text: String(s?.text ?? String(s?.id ?? '')),
      guard: typeof s?.guard === 'string' ? s.guard : undefined,
      publish_result: typeof s?.publish_result === 'boolean' ? s.publish_result : undefined,
      publish_mode: s?.publish_mode === 'immediate' || s?.publish_mode === 'topo' ? s.publish_mode : undefined,
      continue_on_error: typeof s?.continue_on_error === 'boolean' ? s.continue_on_error : undefined,
      tool: s?.tool && typeof s.tool?.name === 'string' ? { name: s.tool.name, args: s.tool.args } : undefined,
      depends_on: Array.isArray(s?.depends_on) ? s.depends_on.filter((x: any) => typeof x === 'string') : undefined,
    })),
    ui:
      data?.ui && typeof data.ui === 'object'
        ? {
            layout:
              data.ui.layout && typeof data.ui.layout === 'object' && !Array.isArray(data.ui.layout)
                ? { ...data.ui.layout }
                : undefined,
            parents:
              data.ui.parents && typeof data.ui.parents === 'object' && !Array.isArray(data.ui.parents)
                ? { ...data.ui.parents }
                : undefined,
            groups: Array.isArray(data.ui.groups)
              ? data.ui.groups
                  .filter((g: any) => g && typeof g.id === 'string')
                  .map((g: any) => ({ id: String(g.id), label: String(g.label ?? 'Group'), collapsed: !!g.collapsed }))
              : undefined,
          }
        : undefined,
  }

  // Basic validation
  if (!wf.steps.every(s => s.id)) {
    alert('Invalid workflow: each step must have an id')
    return
  }

  // Track locally and show in dropdown
  localWorkflows.value.set(intent, wf)
  workflowList.value.push({ ...wf })

  // Switch selection to the imported workflow
  isHydrating.value = true
  try {
    selectedIntent.value = intent
    await nextTick()
    activeWorkflow.value = wf
    nodes.value = workflowToNodes(wf)
    edges.value = workflowToEdges(wf)
    dirty.value = false
  } finally {
    await nextTick()
    isHydrating.value = false
    scheduleFitView()
  }
}
</script>

<style scoped>
/* Make workflow header buttons appear as plain text links with underline on hover */
.plain-link {
  background: none !important;
  border: none !important;
  padding: 0 !important;
  color: inherit !important;
  font: inherit !important;
  text-decoration: none !important;
  cursor: pointer !important;
}
.plain-link:hover {
  text-decoration: underline !important;
  background: none !important;
}
</style>

<style>
/* ensure flow canvas fills area */
.vue-flow__container {
  height: 100%;
}
</style>
