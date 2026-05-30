<template>
  <div class="flex h-full min-h-0 overflow-hidden">
    <!-- Sidebar navigation -->
    <aside
      class="halo-hairline-r w-60 shrink-0 space-y-4 overflow-y-auto p-4 pr-5"
    >
      <h1 class="text-lg font-semibold text-foreground">Settings</h1>
      <nav class="space-y-1">
        <button
          v-for="s in sections"
          :key="s.key"
          type="button"
          @click="activeSection = s.key"
          :class="[
            'w-full text-left rounded-md px-3 py-2 text-sm transition',
            activeSection === s.key
              ? 'bg-accent text-accent-foreground font-semibold'
              : 'hover:bg-surface-muted/60 text-foreground',
          ]"
        >
          {{ s.label }}
        </button>
      </nav>
      <div class="pt-4 border-t border-border/50 space-y-2">
        <p class="text-xs text-subtle-foreground">
          App settings are stored locally. Runtime configuration is loaded from
          the server.
        </p>
        <div class="flex gap-2 flex-wrap">
          <button
            type="button"
            class="rounded border border-border/70 px-2 py-1 text-xs hover:border-border"
            @click="resetToDefaults"
          >
            Reset App
          </button>
          <button
            type="button"
            class="rounded bg-accent px-2 py-1 text-xs font-semibold text-accent-foreground hover:bg-accent/90"
            @click="persist"
          >
            Save App
          </button>
        </div>
      </div>
    </aside>

    <!-- Main content -->
    <form
      class="flex-1 overflow-auto pl-6 pr-1 space-y-6"
      @submit.prevent="saveAgentdSettings"
    >
      <div class="flex items-center justify-between gap-4 flex-wrap">
        <div class="space-y-1">
          <h2 class="text-xl font-semibold text-foreground">
            {{ currentSectionLabel }}
          </h2>
          <p
            class="text-xs text-subtle-foreground"
            v-if="sectionDescriptions[activeSection]"
          >
            {{ sectionDescriptions[activeSection] }}
          </p>
        </div>
        <div class="flex items-center gap-2 flex-wrap">
          <span v-if="agentdSaveError" class="text-xs text-danger-foreground">{{
            agentdSaveError
          }}</span>
          <span
            v-else-if="agentdSuccess"
            class="text-xs text-accent-foreground"
            >{{ agentdSuccess }}</span
          >
          <button
            type="button"
            class="rounded border border-border/70 px-3 py-2 text-xs font-semibold hover:border-border disabled:opacity-50"
            @click="loadAgentdSettings"
            :disabled="agentdLoading || agentdSaving"
          >
            Reload
          </button>
          <button
            type="submit"
            class="rounded bg-accent px-4 py-2 text-xs font-semibold text-accent-foreground hover:bg-accent/90 disabled:opacity-60"
            :disabled="agentdSaving"
          >
            {{ agentdSaving ? "Saving…" : "Save Changes" }}
          </button>
        </div>
      </div>

      <div
        v-if="agentdLoadError"
        class="flex items-center justify-between gap-3 rounded-md border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground"
      >
        <span>{{ agentdLoadError }}</span>
        <button
          type="button"
          class="rounded border border-danger/40 px-2 py-1 text-xs font-semibold hover:border-danger"
          @click="loadAgentdSettings"
        >
          Retry
        </button>
      </div>
      <div v-if="agentdLoading" class="text-sm text-subtle-foreground">
        Loading configuration…
      </div>

      <!-- General (Application + high-level) -->
      <template v-if="activeSection === 'general'">
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Application (Client)
          </legend>
          <div class="grid gap-4 grid-cols-2">
            <div class="space-y-1">
              <label
                for="api-url"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >API Base URL</label
              >
              <input
                id="api-url"
                v-model="apiUrl"
                type="url"
                placeholder="https://localhost:32180/api"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
              />
            </div>
            <div class="space-y-1">
              <label
                for="ui-theme"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Theme</label
              >
              <DropdownSelect
                id="ui-theme"
                v-model="selectedThemeId"
                :options="themeDropdownOptions"
                size="sm"
                class="w-full"
                aria-label="Theme"
              />
            </div>
          </div>
        </fieldset>
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Runtime Overview
          </legend>
          <p class="text-xs text-subtle-foreground">
            High level runtime identifiers used for telemetry & logs.
          </p>
          <div class="grid gap-4 grid-cols-3">
            <div class="space-y-1">
              <label
                for="otel-service"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Service Name</label
              >
              <input
                id="otel-service"
                type="text"
                v-model="agentdSettings.otelServiceName"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="service-version"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Version</label
              >
              <input
                id="service-version"
                type="text"
                v-model="agentdSettings.serviceVersion"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="environment"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Environment</label
              >
              <input
                id="environment"
                type="text"
                v-model="agentdSettings.environment"
                placeholder="dev"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
          </div>
        </fieldset>
      </template>

      <!-- Summarization -->
      <template v-if="activeSection === 'summarization'">
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Conversation Summarization
          </legend>
          <div class="flex items-center gap-2">
            <input
              id="summary-enabled"
              type="checkbox"
              class="h-4 w-4"
              v-model="agentdSettings.summaryEnabled"
            />
            <label for="summary-enabled" class="text-sm text-foreground"
              >Enable rolling summaries</label
            >
          </div>
          <div class="grid gap-4 grid-cols-4">
            <div class="space-y-1 col-span-2">
              <label
                for="summary-model"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Summary Model</label
              >
              <input
                id="summary-model"
                type="text"
                v-model="agentdSettings.openaiSummaryModel"
                placeholder="gpt-4o-mini"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1 col-span-2">
              <label
                for="summary-url"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Endpoint</label
              >
              <input
                id="summary-url"
                type="url"
                v-model="agentdSettings.openaiSummaryUrl"
                placeholder="https://api.openai.com"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1 col-span-2">
              <label
                for="summary-reserve-buffer"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Reserve Output Tokens</label
              >
              <input
                id="summary-reserve-buffer"
                type="number"
                min="0"
                v-model.number="agentdSettings.summaryReserveBufferTokens"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
              <p class="text-xs text-subtle-foreground">
                Summaries are triggered automatically when the chat approaches the
                model context window. This reserve keeps token budget available
                for the model response.
              </p>
            </div>
            <div class="space-y-1 col-span-2">
              <label
                for="summary-plain-text-context-window"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Plain Summary Context Window</label
              >
              <input
                id="summary-plain-text-context-window"
                type="number"
                min="0"
                v-model.number="agentdSettings.summaryPlainTextContextWindowTokens"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
              <p class="text-xs text-subtle-foreground">
                Optional global trigger budget for portable plain-text summaries.
                Lower values force earlier summarization for summary-only models;
                0 falls back to the summary model context size.
              </p>
            </div>
          </div>
        </fieldset>
      </template>

      <!-- Embeddings -->
      <template v-if="activeSection === 'embeddings'">
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Embedding Provider
          </legend>
          <div class="grid gap-4 grid-cols-3">
            <div class="space-y-1 col-span-3">
              <label
                for="embed-base"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Base URL</label
              >
              <input
                id="embed-base"
                type="url"
                v-model="agentdSettings.embedBaseUrl"
                placeholder="https://api.openai.com"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="embed-model"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Model</label
              >
              <input
                id="embed-model"
                type="text"
                v-model="agentdSettings.embedModel"
                placeholder="text-embedding-3-small"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="embed-path"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Path</label
              >
              <input
                id="embed-path"
                type="text"
                v-model="agentdSettings.embedPath"
                placeholder="/v1/embeddings"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="embed-header"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >API Header</label
              >
              <input
                id="embed-header"
                type="text"
                v-model="agentdSettings.embedApiHeader"
                placeholder="Authorization"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1 col-span-3">
              <label
                for="embed-key"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >API Key</label
              >
              <input
                id="embed-key"
                type="password"
                autocomplete="off"
                v-model="agentdSettings.embedApiKey"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>

            <div class="space-y-1">
              <label
                for="embed-instruction-mode"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Instruction Mode</label
              >
              <DropdownSelect
                id="embed-instruction-mode"
                v-model="agentdSettings.embedInstructionMode"
                :options="embedInstructionModeDropdownOptions"
                class="w-full"
              />
            </div>
            <div class="space-y-1">
              <label
                for="embed-instruction-format"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Instruction Format</label
              >
              <DropdownSelect
                id="embed-instruction-format"
                v-model="agentdSettings.embedInstructionFormat"
                :options="embedInstructionFormatDropdownOptions"
                class="w-full"
              />
            </div>
            <div class="space-y-1 col-span-3">
              <label
                for="embed-default-query-instruction"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Default Query Instruction</label
              >
              <textarea
                id="embed-default-query-instruction"
                v-model="agentdSettings.embedDefaultQueryInstruction"
                rows="2"
                placeholder="Built-in per-surface default"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              ></textarea>
            </div>
            <div class="space-y-1 col-span-3">
              <label
                for="embed-rag-query-instruction"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >RAG Query Instruction</label
              >
              <textarea
                id="embed-rag-query-instruction"
                v-model="agentdSettings.embedRagQueryInstruction"
                rows="2"
                placeholder="Given a search query, retrieve relevant passages that answer the query."
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              ></textarea>
            </div>
            <div class="space-y-1 col-span-3">
              <label
                for="embed-memory-query-instruction"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Evolving Memory Query Instruction</label
              >
              <textarea
                id="embed-memory-query-instruction"
                v-model="agentdSettings.embedEvolvingMemoryQueryInstruction"
                rows="2"
                placeholder="Given the current task, retrieve past experiences, lessons, and strategies relevant to the current task."
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              ></textarea>
            </div>
            <div class="space-y-1 col-span-3">
              <label
                for="embed-transit-query-instruction"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Transit Query Instruction</label
              >
              <textarea
                id="embed-transit-query-instruction"
                v-model="agentdSettings.embedTransitQueryInstruction"
                rows="2"
                placeholder="Given a search query, retrieve relevant stored shared-memory records."
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              ></textarea>
            </div>

            <div class="space-y-1 col-span-3">
              <label
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Additional Headers</label
              >
              <div class="space-y-2">
                <div
                  v-for="(v, k) in agentdSettings.embedApiHeaders"
                  :key="k"
                  class="flex gap-2"
                >
                  <div class="w-48 space-y-1">
                    <label class="text-xs text-subtle-foreground">Header</label>
                    <input
                      type="text"
                      :value="k"
                      readonly
                      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
                    />
                  </div>
                  <div class="flex-1 space-y-1">
                    <label class="text-xs text-subtle-foreground">Value</label>
                    <input
                      type="text"
                      v-model="agentdSettings.embedApiHeaders[k]"
                      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
                    />
                  </div>
                  <div class="flex items-end">
                    <button
                      type="button"
                      class="rounded border border-danger/40 px-2 py-1 text-xs text-danger-foreground"
                      @click="removeEmbedHeader(k)"
                    >
                      Remove
                    </button>
                  </div>
                </div>

                <div class="flex gap-2">
                  <input
                    type="text"
                    v-model="newEmbedHeaderKey"
                    placeholder="Header name (e.g. x-api-key)"
                    class="w-48 rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
                  />
                  <input
                    type="text"
                    v-model="newEmbedHeaderValue"
                    placeholder="Value"
                    class="flex-1 rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
                  />
                  <button
                    type="button"
                    class="rounded bg-accent px-3 py-2 text-xs font-semibold text-accent-foreground"
                    @click="addEmbedHeader"
                  >
                    Add
                  </button>
                </div>
              </div>
            </div>
          </div>
        </fieldset>
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Reranking Provider
          </legend>
          <div class="grid gap-4 grid-cols-3">
            <label class="col-span-3 flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                v-model="agentdSettings.rerankEnabled"
                class="h-4 w-4 rounded border-border text-accent"
              />
              <span>Enabled</span>
            </label>
            <div class="space-y-1 col-span-3">
              <label
                for="rerank-base"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Base URL</label
              >
              <input
                id="rerank-base"
                type="url"
                v-model="agentdSettings.rerankBaseUrl"
                placeholder="http://localhost:8203"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="rerank-model"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Model</label
              >
              <input
                id="rerank-model"
                type="text"
                v-model="agentdSettings.rerankModel"
                placeholder="qwen3-reranker-0.6b"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="rerank-path"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Path</label
              >
              <input
                id="rerank-path"
                type="text"
                v-model="agentdSettings.rerankPath"
                placeholder="/v1/rerank"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="rerank-header"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >API Header</label
              >
              <input
                id="rerank-header"
                type="text"
                v-model="agentdSettings.rerankApiHeader"
                placeholder="Authorization"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1 col-span-3">
              <label
                for="rerank-key"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >API Key</label
              >
              <input
                id="rerank-key"
                type="password"
                autocomplete="off"
                v-model="agentdSettings.rerankApiKey"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1 col-span-3">
              <label
                for="rerank-instruction"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Instruction</label
              >
              <textarea
                id="rerank-instruction"
                v-model="agentdSettings.rerankInstruction"
                rows="2"
                placeholder="Classify whether the document matches the query topic"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              ></textarea>
            </div>
          </div>
        </fieldset>
      </template>

      <!-- Timeouts & Safety -->
      <template v-if="activeSection === 'timeouts'">
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Timeouts (seconds)
          </legend>
          <div class="grid gap-4 grid-cols-3">
            <div class="space-y-1">
              <label
                for="timeout-agent"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Agent Run</label
              >
              <input
                id="timeout-agent"
                type="number"
                min="0"
                v-model.number="agentdSettings.agentRunTimeoutSeconds"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="timeout-stream"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Stream</label
              >
              <input
                id="timeout-stream"
                type="number"
                min="0"
                v-model.number="agentdSettings.streamRunTimeoutSeconds"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="timeout-workflow"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Workflow</label
              >
              <input
                id="timeout-workflow"
                type="number"
                min="0"
                v-model.number="agentdSettings.workflowTimeoutSeconds"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
          </div>
        </fieldset>
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Execution Safety
          </legend>
          <div class="grid gap-4 grid-cols-4">
            <div class="space-y-1 col-span-2">
              <label
                for="block-binaries"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Block Binaries</label
              >
              <input
                id="block-binaries"
                type="text"
                v-model="agentdSettings.blockBinaries"
                placeholder="rm,sudo,chown,chmod,dd,mkfs,mount,umount"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="max-command"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Max Command</label
              >
              <input
                id="max-command"
                type="number"
                min="0"
                v-model.number="agentdSettings.maxCommandSeconds"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="truncate-bytes"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Truncate Bytes</label
              >
              <input
                id="truncate-bytes"
                type="number"
                min="0"
                v-model.number="agentdSettings.outputTruncateBytes"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
          </div>
        </fieldset>
      </template>

      <!-- Observability & Logging -->
      <template v-if="activeSection === 'observability'">
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Telemetry
          </legend>
          <div class="grid gap-4 grid-cols-4">
            <div class="space-y-1 col-span-2">
              <label
                for="otel-endpoint"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >OTLP Endpoint</label
              >
              <input
                id="otel-endpoint"
                type="url"
                v-model="agentdSettings.otelExporterOtlpEndpoint"
                placeholder="http://localhost:4318"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
          </div>
        </fieldset>
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">Logging</legend>
          <div class="grid gap-4 grid-cols-5">
            <div class="space-y-1 col-span-2">
              <label
                for="log-path"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Log Path</label
              >
              <input
                id="log-path"
                type="text"
                v-model="agentdSettings.logPath"
                placeholder="/var/log/agentd.log"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="log-level"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Level</label
              >
              <DropdownSelect
                id="log-level"
                v-model="agentdSettings.logLevel"
                :options="logLevelDropdownOptions"
                class="w-full"
              />
            </div>
            <div class="space-y-1 flex items-center gap-2 col-span-2">
              <input
                id="log-payloads"
                type="checkbox"
                class="h-4 w-4"
                v-model="agentdSettings.logPayloads"
              />
              <label
                for="log-payloads"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Log LLM Payloads</label
              >
            </div>
            <div class="space-y-1 flex items-center gap-2 col-span-2">
              <input
                id="log-raw-prompts"
                type="checkbox"
                class="h-4 w-4"
                v-model="agentdSettings.logRawPrompts"
              />
              <label
                for="log-raw-prompts"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Log Raw LLM Prompts</label
              >
            </div>
          </div>
        </fieldset>
      </template>

      <!-- Web / Search -->
      <template v-if="activeSection === 'web'">
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Web Search
          </legend>
          <div class="grid gap-4 grid-cols-2">
            <div class="space-y-1">
              <label
                for="searxng-url"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >SearXNG URL</label
              >
              <input
                id="searxng-url"
                type="url"
                v-model="agentdSettings.searxngUrl"
                placeholder="http://localhost:8080"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="web-searxng-url"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >UI Override</label
              >
              <input
                id="web-searxng-url"
                type="url"
                v-model="agentdSettings.webSearxngUrl"
                placeholder="http://localhost:8080"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
          </div>
        </fieldset>
      </template>

      <!-- Databases -->
      <template v-if="activeSection === 'databases'">
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Primary Connections
          </legend>
          <div class="grid gap-4 grid-cols-3">
            <div class="space-y-1">
              <label
                for="database-url"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >DATABASE_URL</label
              >
              <input
                id="database-url"
                type="text"
                v-model="agentdSettings.databaseUrl"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="db-url"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >DB_URL</label
              >
              <input
                id="db-url"
                type="text"
                v-model="agentdSettings.dbUrl"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="postgres-dsn"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >POSTGRES_DSN</label
              >
              <input
                id="postgres-dsn"
                type="text"
                v-model="agentdSettings.postgresDsn"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
          </div>
        </fieldset>
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Search Database
          </legend>
          <div class="grid gap-4 grid-cols-3">
            <div class="space-y-1">
              <label
                for="search-backend"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Backend</label
              >
              <input
                id="search-backend"
                type="text"
                v-model="agentdSettings.searchBackend"
                placeholder="postgres"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="search-dsn"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >DSN</label
              >
              <input
                id="search-dsn"
                type="text"
                v-model="agentdSettings.searchDsn"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="search-index"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Index</label
              >
              <input
                id="search-index"
                type="text"
                v-model="agentdSettings.searchIndex"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
          </div>
        </fieldset>
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Vector Database
          </legend>
          <div class="grid gap-4 grid-cols-5">
            <div class="space-y-1">
              <label
                for="vector-backend"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Backend</label
              >
              <input
                id="vector-backend"
                type="text"
                v-model="agentdSettings.vectorBackend"
                placeholder="postgres"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="vector-dsn"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >DSN</label
              >
              <input
                id="vector-dsn"
                type="text"
                v-model="agentdSettings.vectorDsn"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="vector-index"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Index</label
              >
              <input
                id="vector-index"
                type="text"
                v-model="agentdSettings.vectorIndex"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="vector-dimensions"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Dimensions</label
              >
              <input
                id="vector-dimensions"
                type="number"
                min="0"
                v-model.number="agentdSettings.vectorDimensions"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="vector-metric"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Metric</label
              >
              <DropdownSelect
                id="vector-metric"
                v-model="agentdSettings.vectorMetric"
                :options="vectorMetricDropdownOptions"
                class="w-full"
              />
            </div>
          </div>
        </fieldset>
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            Graph Database
          </legend>
          <div class="grid gap-4 grid-cols-2">
            <div class="space-y-1">
              <label
                for="graph-backend"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Backend</label
              >
              <input
                id="graph-backend"
                type="text"
                v-model="agentdSettings.graphBackend"
                placeholder="postgres"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
            <div class="space-y-1">
              <label
                for="graph-dsn"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >DSN</label
              >
              <input
                id="graph-dsn"
                type="text"
                v-model="agentdSettings.graphDsn"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
              />
            </div>
          </div>
        </fieldset>
      </template>

      <!-- MCP Servers -->
      <template v-if="activeSection === 'mcp'">
        <fieldset class="space-y-4">
          <legend class="text-sm font-semibold text-foreground">
            MCP Servers
          </legend>
          <div class="flex justify-between items-center">
            <h3 class="text-sm font-medium text-foreground">
              Configured Servers
            </h3>
            <button
              type="button"
              class="rounded bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground hover:bg-accent/90"
              @click="showAddServerModal = true"
            >
              Add Server
            </button>
          </div>
          <div v-if="mcpLoading" class="text-sm text-subtle-foreground">
            Loading servers…
          </div>
          <div v-if="mcpError" class="text-sm text-danger-foreground">
            {{ mcpError }}
          </div>
          <div
            v-else-if="!mcpServers.length"
            class="text-sm text-subtle-foreground"
          >
            No MCP servers configured.
          </div>
          <div v-else class="space-y-3">
            <div
              v-for="server in mcpServers"
              :key="server.id"
              class="flex items-center justify-between gap-4 p-4 rounded-md border border-border/70 bg-surface-muted/60"
            >
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <p class="text-sm font-medium text-foreground truncate">
                    {{ server.name }}
                  </p>
                  <span
                    v-if="server.oauthClientId"
                    class="rounded bg-accent/20 px-1.5 py-0.5 text-[10px] font-medium text-accent-foreground"
                    >Registered</span
                  >
                </div>
                <p class="text-xs text-subtle-foreground truncate">
                  {{ server.url }}
                </p>
              </div>
              <div class="flex items-center gap-2">
                <button
                  v-if="server.url && !server.hasToken"
                  type="button"
                  class="rounded bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground hover:bg-accent/90"
                  @click="connectServer(server)"
                >
                  Connect
                </button>
                <button
                  v-if="server.url && server.hasToken && server.oauthClientId"
                  type="button"
                  class="rounded border border-border/70 px-3 py-1 text-xs font-semibold text-foreground hover:border-border hover:bg-surface-muted/60"
                  @click="connectServer(server)"
                >
                  Reauthenticate
                </button>
                <button
                  v-if="server.source === 'db'"
                  type="button"
                  class="rounded border border-danger/60 bg-danger/10 px-3 py-1 text-xs font-semibold text-danger-foreground hover:bg-danger/20"
                  @click="deleteServer(server)"
                >
                  Delete
                </button>
              </div>
            </div>
          </div>
        </fieldset>
      </template>
    </form>

    <!-- Add Server Modal -->
    <transition name="modal">
      <div
        v-if="showAddServerModal"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/30"
        @click.self="showAddServerModal = false"
      >
        <div
          class="w-full max-w-md rounded-lg border border-border/70 bg-surface p-6"
        >
          <h3 class="text-lg font-semibold text-foreground mb-4">
            Add MCP Server
          </h3>
          <div class="space-y-4">
            <div class="space-y-1">
              <label
                for="server-name"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Server Name</label
              >
              <input
                id="server-name"
                v-model="newServer.name"
                type="text"
                placeholder="My MCP Server"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
              />
            </div>
            <div class="space-y-1">
              <label
                for="server-url"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Server URL</label
              >
              <input
                id="server-url"
                v-model="newServer.url"
                type="url"
                placeholder="https://mcp-server.local"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
              />
            </div>
            <div class="space-y-1">
              <label
                for="server-oauth-client-id"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >OAuth Client ID (Optional)</label
              >
              <input
                id="server-oauth-client-id"
                v-model="newServer.oauthClientId"
                type="text"
                placeholder="Leave empty for dynamic registration"
                class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
              />
              <p class="text-xs text-subtle-foreground">
                If supported by the server, we will attempt to register a client
                automatically when you connect.
              </p>
            </div>
          </div>
          <div class="flex justify-end gap-2 mt-4">
            <button
              type="button"
              class="rounded border border-border/70 px-3 py-2 text-xs font-semibold hover:border-border"
              @click="showAddServerModal = false"
            >
              Cancel
            </button>
            <button
              type="button"
              class="rounded bg-accent px-3 py-2 text-xs font-semibold text-accent-foreground hover:bg-accent/90"
              @click="addServer"
            >
              Add Server
            </button>
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import {
  fetchAgentdSettings,
  updateAgentdSettings,
  type AgentdSettings,
} from "@/api/client";
import {
  listMCPServers,
  createMCPServer,
  deleteMCPServer,
  startMCPOAuth,
} from "@/api/mcp";
import type { MCPServer, CreateMCPServerRequest } from "@/types/mcp";
import DropdownSelect from "@/components/DropdownSelect.vue";
import { useThemeStore } from "@/stores/theme";
import type { ThemeChoice } from "@/theme/themes";

const themeStore = useThemeStore();

const selectedThemeId = computed<ThemeChoice>({
  get: () => themeStore.selection,
  set: (value) => {
    themeStore.setTheme(value);
  },
});

const themeDropdownOptions = computed(() =>
  themeStore.options.map((option) => ({
    id: option.id,
    label: option.label,
    description: option.description,
    value: option.id,
  })),
);

const apiUrl = ref("");

const STORAGE_KEY = "agentd.ui.settings";

type Settings = {
  apiUrl: string;
};

const defaultAgentdSettings: AgentdSettings = {
  openaiSummaryModel: "",
  openaiSummaryUrl: "",
  summaryEnabled: false,
  summaryPlainTextContextWindowTokens: 0,
  summaryReserveBufferTokens: 25000,
  embedBaseUrl: "https://api.openai.com",
  embedModel: "text-embedding-3-small",
  embedApiKey: "",
  embedApiHeader: "Authorization",
  embedApiHeaders: {},
  embedPath: "/v1/embeddings",
  embedInstructionMode: "auto",
  embedInstructionFormat: "qwen",
  embedDefaultQueryInstruction: "",
  embedRagQueryInstruction: "",
  embedEvolvingMemoryQueryInstruction: "",
  embedTransitQueryInstruction: "",
  rerankEnabled: false,
  rerankBaseUrl: "http://localhost:8203",
  rerankModel: "qwen3-reranker-0.6b",
  rerankInstruction: "Classify whether the document matches the query topic",
  rerankApiKey: "",
  rerankApiHeader: "Authorization",
  rerankApiHeaders: {},
  rerankPath: "/v1/rerank",
  agentRunTimeoutSeconds: 0,
  streamRunTimeoutSeconds: 0,
  workflowTimeoutSeconds: 0,
  blockBinaries: "rm,sudo,chown,chmod,dd,mkfs,mount,umount",
  maxCommandSeconds: 30,
  outputTruncateBytes: 65536,
  otelServiceName: "manifold",
  serviceVersion: "0.1.0",
  environment: "dev",
  otelExporterOtlpEndpoint: "http://localhost:4318",
  logPath: "",
  logLevel: "info",
  logPayloads: true,
  logRawPrompts: false,
  searxngUrl: "http://localhost:8080",
  webSearxngUrl: "http://localhost:8080",
  databaseUrl: "",
  dbUrl: "",
  postgresDsn: "",
  searchBackend: "postgres",
  searchDsn: "",
  searchIndex: "",
  vectorBackend: "postgres",
  vectorDsn: "",
  vectorIndex: "",
  vectorDimensions: 1536,
  vectorMetric: "cosine",
  graphBackend: "postgres",
  graphDsn: "",
};

const agentdSettings = ref<AgentdSettings>({ ...defaultAgentdSettings });
const agentdLoading = ref(false);
const agentdSaving = ref(false);
const agentdLoadError = ref("");
const agentdSaveError = ref("");
const agentdSuccess = ref("");

// Helpers for embedding headers UI
const newEmbedHeaderKey = ref("");
const newEmbedHeaderValue = ref("");
function addEmbedHeader() {
  const k = newEmbedHeaderKey.value.trim();
  if (!k) return;
  agentdSettings.value.embedApiHeaders = {
    ...agentdSettings.value.embedApiHeaders,
    [k]: newEmbedHeaderValue.value,
  };
  newEmbedHeaderKey.value = "";
  newEmbedHeaderValue.value = "";
}
function removeEmbedHeader(key: string) {
  const h = { ...agentdSettings.value.embedApiHeaders };
  delete h[key];
  agentdSettings.value.embedApiHeaders = h;
}

const logLevelOptions = ["trace", "debug", "info", "warn", "error"];
const vectorMetricOptions = ["cosine", "dot", "euclidean"];
const embedInstructionModeOptions = ["auto", "enabled", "disabled"];
const embedInstructionFormatOptions = ["qwen"];

const logLevelDropdownOptions = logLevelOptions.map((level) => ({
  id: level,
  label: level,
  value: level,
}));
const vectorMetricDropdownOptions = vectorMetricOptions.map((metric) => ({
  id: metric,
  label: metric,
  value: metric,
}));
const embedInstructionModeDropdownOptions = embedInstructionModeOptions.map(
  (mode) => ({
    id: mode,
    label: mode,
    value: mode,
  }),
);
const embedInstructionFormatDropdownOptions = embedInstructionFormatOptions.map(
  (format) => ({
    id: format,
    label: format,
    value: format,
  }),
);

type NumericSettingKey =
  | "summaryPlainTextContextWindowTokens"
  | "summaryReserveBufferTokens"
  | "agentRunTimeoutSeconds"
  | "streamRunTimeoutSeconds"
  | "workflowTimeoutSeconds"
  | "maxCommandSeconds"
  | "outputTruncateBytes"
  | "vectorDimensions";

type BooleanSettingKey =
  | "summaryEnabled"
  | "rerankEnabled"
  | "logPayloads"
  | "logRawPrompts";

const numericSettingKeys: NumericSettingKey[] = [
  "summaryPlainTextContextWindowTokens",
  "summaryReserveBufferTokens",
  "agentRunTimeoutSeconds",
  "streamRunTimeoutSeconds",
  "workflowTimeoutSeconds",
  "maxCommandSeconds",
  "outputTruncateBytes",
  "vectorDimensions",
];
const booleanSettingKeys: BooleanSettingKey[] = [
  "summaryEnabled",
  "rerankEnabled",
  "logPayloads",
  "logRawPrompts",
];

function toNumber(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function toBoolean(value: unknown, fallback: boolean): boolean {
  if (typeof value === "boolean") {
    return value;
  }
  if (typeof value === "string") {
    const trimmed = value.trim().toLowerCase();
    if (trimmed === "true" || trimmed === "1" || trimmed === "yes") {
      return true;
    }
    if (trimmed === "false" || trimmed === "0" || trimmed === "no") {
      return false;
    }
  }
  return fallback;
}

function normalizeAgentdSettings(
  input?: Partial<AgentdSettings>,
): AgentdSettings {
  const merged = {
    ...defaultAgentdSettings,
    ...(input ?? {}),
  } as AgentdSettings & {
    summaryThreshold?: unknown;
    summaryKeepLast?: unknown;
  };
  delete merged.summaryThreshold;
  delete merged.summaryKeepLast;
  for (const key of numericSettingKeys) {
    merged[key] = toNumber(input?.[key], defaultAgentdSettings[key]);
  }
  for (const key of booleanSettingKeys) {
    merged[key] = toBoolean(input?.[key], defaultAgentdSettings[key]);
  }
  return merged;
}

async function loadAgentdSettings() {
  agentdLoading.value = true;
  agentdLoadError.value = "";
  try {
    const data = await fetchAgentdSettings();
    agentdSettings.value = normalizeAgentdSettings(data);
  } catch (error: any) {
    console.warn("Failed to load agentd settings", error);
    agentdLoadError.value =
      error?.response?.data ?? "Unable to load agent configuration";
    agentdSettings.value = normalizeAgentdSettings(agentdSettings.value);
  } finally {
    agentdLoading.value = false;
  }
}

async function saveAgentdSettings() {
  if (agentdSaving.value) {
    return;
  }
  agentdSaving.value = true;
  agentdSaveError.value = "";
  agentdSuccess.value = "";
  try {
    const payload = normalizeAgentdSettings(agentdSettings.value);
    const saved = await updateAgentdSettings(payload);
    // Some servers respond to PUT with 204 No Content. In that case, `saved` will
    // be undefined/empty and we were previously resetting the form back to defaults.
    // Prefer the server echo when present; otherwise reload from GET, and finally
    // fall back to the payload the user submitted so the UI reflects their choices.
    const looksLikeSettings =
      saved &&
      typeof saved === "object" &&
      "openaiSummaryModel" in (saved as any);
    if (looksLikeSettings) {
      agentdSettings.value = normalizeAgentdSettings(
        saved as Partial<AgentdSettings>,
      );
    } else {
      try {
        await loadAgentdSettings();
      } catch {
        agentdSettings.value = normalizeAgentdSettings(payload);
      }
    }
    agentdSuccess.value = "Saved";
    window.setTimeout(() => {
      agentdSuccess.value = "";
    }, 3000);
  } catch (error: any) {
    console.error("Failed to save agentd settings", error);
    const status = error?.response?.status;
    if (
      error?.code === "READ_ONLY" ||
      status === 405 ||
      status === 404 ||
      status === 501
    ) {
      // Backend does not expose a write endpoint for agentd config.
      // Keep the current UI values and show a clear message.
      agentdSaveError.value =
        "Configuration is read-only on this server. Update config.yaml / environment and restart agentd.";
    } else {
      agentdSaveError.value = error?.response?.data ?? "Save failed";
    }
  } finally {
    agentdSaving.value = false;
  }
}

// MCP Management
const mcpServers = ref<MCPServer[]>([]);
const mcpLoading = ref(false);
const mcpError = ref("");
const showAddServerModal = ref(false);
const newServer = ref<CreateMCPServerRequest>({
  name: "",
  url: "",
  oauthClientId: "",
});

async function loadMCPServers() {
  mcpLoading.value = true;
  mcpError.value = "";
  try {
    mcpServers.value = await listMCPServers();
  } catch (e: any) {
    mcpError.value = e.message || "Failed to load MCP servers";
  } finally {
    mcpLoading.value = false;
  }
}

async function addServer() {
  if (!newServer.value.name) return;
  try {
    await createMCPServer(newServer.value);
    showAddServerModal.value = false;
    newServer.value = { name: "", url: "", oauthClientId: "" };
    loadMCPServers();
  } catch (e: any) {
    alert("Failed to add server: " + (e.response?.data || e.message));
  }
}

async function deleteServer(server: MCPServer) {
  if (!confirm(`Delete server ${server.name}?`)) return;
  try {
    await deleteMCPServer(server.name);
    loadMCPServers();
  } catch (e: any) {
    alert("Failed to delete server: " + (e.response?.data || e.message));
  }
}

async function connectServer(server: MCPServer) {
  if (!server.url) return;
  try {
    const res = await startMCPOAuth(server.id, server.url);
    if (res.redirectUrl) {
      window.open(res.redirectUrl, "mcp_oauth", "width=600,height=700");
    }
  } catch (e: any) {
    alert("Failed to start OAuth: " + (e.response?.data || e.message));
  }
}

function handleMessage(event: MessageEvent) {
  if (event.data?.type === "mcp-oauth-success") {
    loadMCPServers();
  }
}

// theme selection UI removed; theme controlled via header toggle

// Sections (sidebar navigation)
type SectionKey =
  | "general"
  | "summarization"
  | "embeddings"
  | "timeouts"
  | "observability"
  | "web"
  | "databases"
  | "mcp";
const sections: { key: SectionKey; label: string }[] = [
  { key: "general", label: "General" },
  { key: "summarization", label: "Summarization" },
  { key: "embeddings", label: "Embeddings" },
  { key: "timeouts", label: "Timeouts & Safety" },
  { key: "observability", label: "Observability & Logging" },
  { key: "web", label: "Search & Web" },
  { key: "databases", label: "Databases" },
  { key: "mcp", label: "MCP Servers" },
];
const activeSection = ref<SectionKey>("general");
const sectionDescriptions: Record<SectionKey, string> = {
  general: "Client-local app settings and runtime identifiers.",
  summarization: "Control conversation summarization cadence and retention.",
  embeddings: "Configure embedding provider parameters.",
  timeouts: "Global execution time limits and shell safety.",
  observability: "Telemetry export and logging verbosity.",
  web: "Search service integration exposed to tools/UI.",
  databases: "Primary, search, vector, and graph database connection settings.",
  mcp: "Manage Model Context Protocol servers and connections.",
};
const currentSectionLabel = computed(
  () => sections.find((s) => s.key === activeSection.value)?.label || "",
);

onMounted(() => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as Settings;
      apiUrl.value = parsed.apiUrl;
    }
  } catch (error) {
    console.warn("Unable to parse stored settings", error);
  }
  loadAgentdSettings();
  loadMCPServers();
  window.addEventListener("message", handleMessage);
});

onUnmounted(() => {
  window.removeEventListener("message", handleMessage);
});

function persist() {
  const payload: Settings = {
    apiUrl: apiUrl.value,
  };
  localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
}

function resetToDefaults() {
  localStorage.removeItem(STORAGE_KEY);
  apiUrl.value = "";
}

// Appearance panel removed

// User management removed per redesign (admin UI not part of Settings now)
</script>
