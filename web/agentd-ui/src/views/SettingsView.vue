<template>
  <section class="space-y-6 flex-1 min-h-0 overflow-auto">
    <header class="space-y-2">
      <h1 class="text-2xl font-semibold text-foreground">Settings</h1>
      <p class="text-sm text-subtle-foreground">
        Configure integrations, authentication, and advanced execution knobs for agentd.
      </p>
    </header>

    <div class="grid gap-6 lg:grid-cols-2">
      <!-- Application (client-side) settings — moved to top -->
      <form class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6 lg:col-span-1">
        <header class="space-y-1">
          <h2 class="text-lg font-semibold text-foreground">Application</h2>
          <p class="text-sm text-subtle-foreground">Client-side settings stored in your browser.</p>
        </header>

        <div class="grid gap-2 md:grid-cols-5 md:items-start">
          <div class="md:col-span-2">
            <label class="text-sm font-medium text-muted-foreground" for="api-url">API Base URL</label>
            <p class="text-xs text-faint-foreground">Used during local development when the Go backend is proxied.</p>
          </div>
          <div class="md:col-span-3">
            <input
              id="api-url"
              v-model="apiUrl"
              type="url"
              placeholder="https://localhost:32180/api"
              class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
            />
          </div>
        </div>

        <div class="flex items-center justify-between border-t border-border/60 pt-4">
          <p class="text-sm text-subtle-foreground">
            Changes are stored locally and applied on next reload.
          </p>
          <div class="flex gap-3">
            <button
              type="button"
              class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold text-muted-foreground transition hover:border-border"
              @click="resetToDefaults"
            >
              Reset
            </button>
            <button
              type="button"
              class="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition hover:bg-accent/90"
              @click="persist"
            >
              Save
            </button>
          </div>
        </div>
      </form>

      <!-- Appearance — moved to top -->
      <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6 lg:col-span-1">
        <header class="space-y-1">
          <h2 class="text-lg font-semibold text-foreground">Appearance</h2>
          <p class="text-sm text-subtle-foreground">
            Swap themes or follow your operating system. Changes apply instantly.
          </p>
        </header>
        <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <button
            v-for="option in themeOptions"
            :key="option.id"
            type="button"
            :class="[
              'flex flex-col rounded-xl border px-4 py-3 text-left shadow-sm transition',
              option.id === themeSelection
                ? 'border-accent bg-accent/10'
                : 'border-border/60 bg-surface-muted/40 hover:border-border/80 hover:bg-surface-muted/70',
            ]"
            @click="selectTheme(option.id)"
          >
            <span class="text-sm font-semibold text-foreground">{{ option.label }}</span>
            <span class="text-xs text-subtle-foreground">{{ option.description }}</span>
            <span class="text-[10px] uppercase tracking-wide text-faint-foreground">
              {{ option.id === 'system' ? 'auto' : option.appearance }}
            </span>
          </button>
        </div>
      </section>

      <!-- Agentd settings broken into focused panels -->
      <form class="space-y-6 lg:col-span-2" @submit.prevent="saveAgentdSettings">
        <header class="space-y-1">
          <h2 class="text-lg font-semibold text-foreground">Agentd Configuration</h2>
          <p class="text-sm text-subtle-foreground">Manage summarization, embeddings, timeouts, logging, and database integrations for the agent runtime.</p>
        </header>

        <div v-if="agentdLoadError" class="flex items-center justify-between gap-3 rounded-lg border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground">
          <span>{{ agentdLoadError }}</span>
          <button type="button" class="rounded border border-danger/40 px-2 py-1 text-xs font-semibold text-danger-foreground transition hover:border-danger" @click="loadAgentdSettings">
            Retry
          </button>
        </div>
        <div v-if="agentdLoading" class="text-sm text-subtle-foreground">Loading configuration…</div>

        <template v-else>
          <!-- Summary & Truncation -->
          <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
            <div>
              <h3 class="text-sm font-semibold text-foreground">Summary &amp; Truncation</h3>
              <p class="text-xs text-subtle-foreground">Control conversation summarization and transcript retention.</p>
            </div>
            <label class="inline-flex items-center gap-2 text-sm text-foreground" for="summary-enabled">
              <input id="summary-enabled" type="checkbox" class="h-4 w-4" v-model="agentdSettings.summaryEnabled" />
              <span>Enable rolling summaries</span>
            </label>
            <div class="grid gap-3 sm:grid-cols-2">
              <div class="space-y-1">
                <label for="summary-model" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Summary Model</label>
                <input id="summary-model" type="text" v-model="agentdSettings.openaiSummaryModel" placeholder="gpt-4o-mini" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
              <div class="space-y-1">
                <label for="summary-url" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Summary Endpoint</label>
                <input id="summary-url" type="url" v-model="agentdSettings.openaiSummaryUrl" placeholder="https://api.openai.com" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
              <div class="space-y-1">
                <label for="summary-threshold" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Summarize After (turns)</label>
                <input id="summary-threshold" type="number" min="0" v-model.number="agentdSettings.summaryThreshold" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
              <div class="space-y-1">
                <label for="summary-keep" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Keep Last Turns</label>
                <input id="summary-keep" type="number" min="0" v-model.number="agentdSettings.summaryKeepLast" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
            </div>
          </section>

          <!-- Embedding Service -->
          <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
            <div>
              <h3 class="text-sm font-semibold text-foreground">Embedding Service</h3>
              <p class="text-xs text-subtle-foreground">Configure the embedding provider for vector operations.</p>
            </div>
            <div class="grid gap-3 sm:grid-cols-2">
              <div class="space-y-1 sm:col-span-2">
                <label for="embed-base" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Base URL</label>
                <input id="embed-base" type="url" v-model="agentdSettings.embedBaseUrl" placeholder="https://api.openai.com" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
              <div class="space-y-1">
                <label for="embed-model" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Model</label>
                <input id="embed-model" type="text" v-model="agentdSettings.embedModel" placeholder="text-embedding-3-small" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
              <div class="space-y-1">
                <label for="embed-path" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Path</label>
                <input id="embed-path" type="text" v-model="agentdSettings.embedPath" placeholder="/v1/embeddings" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
              <div class="space-y-1">
                <label for="embed-header" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">API Header</label>
                <input id="embed-header" type="text" v-model="agentdSettings.embedApiHeader" placeholder="Authorization" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
              <div class="space-y-1 sm:col-span-2">
                <label for="embed-key" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">API Key</label>
                <input id="embed-key" type="password" autocomplete="off" v-model="agentdSettings.embedApiKey" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
              </div>
            </div>
          </section>

          <!-- Timeouts and Execution & Safety in two columns -->
          <div class="grid gap-6 xl:grid-cols-2">
            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <div>
                <h3 class="text-sm font-semibold text-foreground">Agent Global Timeouts (seconds)</h3>
                <p class="text-xs text-subtle-foreground">Use 0 to disable a timeout.</p>
              </div>
              <div class="grid gap-3 sm:grid-cols-3">
                <div class="space-y-1">
                  <label for="timeout-agent" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Agent Run</label>
                  <input id="timeout-agent" type="number" min="0" v-model.number="agentdSettings.agentRunTimeoutSeconds" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="timeout-stream" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Stream</label>
                  <input id="timeout-stream" type="number" min="0" v-model.number="agentdSettings.streamRunTimeoutSeconds" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="timeout-workflow" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Workflow</label>
                  <input id="timeout-workflow" type="number" min="0" v-model.number="agentdSettings.workflowTimeoutSeconds" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
              </div>
            </section>

            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <div>
                <h3 class="text-sm font-semibold text-foreground">Execution &amp; Safety</h3>
                <p class="text-xs text-subtle-foreground">Restrict shell execution and output size.</p>
              </div>
              <div class="grid gap-3 sm:grid-cols-2">
                <div class="space-y-1 sm:col-span-2">
                  <label for="block-binaries" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Block Binaries</label>
                  <input id="block-binaries" type="text" v-model="agentdSettings.blockBinaries" placeholder="rm,sudo,chown,chmod,dd,mkfs,mount,umount" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="max-command" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Max Command Seconds</label>
                  <input id="max-command" type="number" min="0" v-model.number="agentdSettings.maxCommandSeconds" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="truncate-bytes" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Output Truncate Bytes</label>
                  <input id="truncate-bytes" type="number" min="0" v-model.number="agentdSettings.outputTruncateBytes" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
              </div>
            </section>
          </div>

          <!-- Observability, Logging, Web Integrations -->
          <div class="grid gap-6 xl:grid-cols-3">
            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <div>
                <h3 class="text-sm font-semibold text-foreground">Observability</h3>
                <p class="text-xs text-subtle-foreground">Export telemetry with descriptive metadata.</p>
              </div>
              <div class="grid gap-3">
                <div class="space-y-1">
                  <label for="otel-service" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Service Name</label>
                  <input id="otel-service" type="text" v-model="agentdSettings.otelServiceName" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="service-version" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Version</label>
                  <input id="service-version" type="text" v-model="agentdSettings.serviceVersion" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="environment" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Environment</label>
                  <input id="environment" type="text" v-model="agentdSettings.environment" placeholder="dev" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="otel-endpoint" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">OTLP Endpoint</label>
                  <input id="otel-endpoint" type="url" v-model="agentdSettings.otelExporterOtlpEndpoint" placeholder="http://localhost:4318" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
              </div>
            </section>

            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <div>
                <h3 class="text-sm font-semibold text-foreground">Logging</h3>
                <p class="text-xs text-subtle-foreground">Tune log verbosity and payload collection.</p>
              </div>
              <div class="grid gap-3">
                <div class="space-y-1">
                  <label for="log-path" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Log Path</label>
                  <input id="log-path" type="text" v-model="agentdSettings.logPath" placeholder="/var/log/agentd.log" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="log-level" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Log Level</label>
                  <select id="log-level" v-model="agentdSettings.logLevel" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40">
                    <option v-for="level in logLevelOptions" :key="level" :value="level">{{ level }}</option>
                  </select>
                </div>
                <label class="inline-flex items-center gap-2 text-sm text-foreground" for="log-payloads">
                  <input id="log-payloads" type="checkbox" class="h-4 w-4" v-model="agentdSettings.logPayloads" />
                  <span>Log LLM payloads</span>
                </label>
              </div>
            </section>

            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <div>
                <h3 class="text-sm font-semibold text-foreground">Web Integrations</h3>
                <p class="text-xs text-subtle-foreground">Expose search services to the UI and tools.</p>
              </div>
              <div class="grid gap-3">
                <div class="space-y-1">
                  <label for="searxng-url" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">SearXNG URL</label>
                  <input id="searxng-url" type="url" v-model="agentdSettings.searxngUrl" placeholder="http://localhost:8080" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="web-searxng-url" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">UI Override</label>
                  <input id="web-searxng-url" type="url" v-model="agentdSettings.webSearxngUrl" placeholder="http://localhost:8080" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
              </div>
            </section>
          </div>

          <!-- Databases split into four panels -->
          <div class="grid gap-6 xl:grid-cols-2">
            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <h4 class="text-sm font-semibold text-foreground">Primary Connections</h4>
              <div class="space-y-3">
                <div class="space-y-1">
                  <label for="database-url" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">DATABASE_URL</label>
                  <input id="database-url" type="text" v-model="agentdSettings.databaseUrl" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="db-url" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">DB_URL</label>
                  <input id="db-url" type="text" v-model="agentdSettings.dbUrl" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="postgres-dsn" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">POSTGRES_DSN</label>
                  <input id="postgres-dsn" type="text" v-model="agentdSettings.postgresDsn" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
              </div>
            </section>

            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <h4 class="text-sm font-semibold text-foreground">Search Database</h4>
              <div class="space-y-3">
                <div class="space-y-1">
                  <label for="search-backend" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Backend</label>
                  <input id="search-backend" type="text" v-model="agentdSettings.searchBackend" placeholder="postgres" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="search-dsn" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">DSN</label>
                  <input id="search-dsn" type="text" v-model="agentdSettings.searchDsn" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="search-index" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Index</label>
                  <input id="search-index" type="text" v-model="agentdSettings.searchIndex" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
              </div>
            </section>
          </div>

          <div class="grid gap-6 xl:grid-cols-2">
            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <h4 class="text-sm font-semibold text-foreground">Vector Database</h4>
              <div class="space-y-3">
                <div class="space-y-1">
                  <label for="vector-backend" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Backend</label>
                  <input id="vector-backend" type="text" v-model="agentdSettings.vectorBackend" placeholder="postgres" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="vector-dsn" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">DSN</label>
                  <input id="vector-dsn" type="text" v-model="agentdSettings.vectorDsn" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="vector-index" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Index</label>
                  <input id="vector-index" type="text" v-model="agentdSettings.vectorIndex" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="vector-dimensions" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Dimensions</label>
                  <input id="vector-dimensions" type="number" min="0" v-model.number="agentdSettings.vectorDimensions" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="vector-metric" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Metric</label>
                  <select id="vector-metric" v-model="agentdSettings.vectorMetric" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40">
                    <option v-for="metric in vectorMetricOptions" :key="metric" :value="metric">{{ metric }}</option>
                  </select>
                </div>
              </div>
            </section>

            <section class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6">
              <h4 class="text-sm font-semibold text-foreground">Graph Database</h4>
              <div class="space-y-3">
                <div class="space-y-1">
                  <label for="graph-backend" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">Backend</label>
                  <input id="graph-backend" type="text" v-model="agentdSettings.graphBackend" placeholder="postgres" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
                <div class="space-y-1">
                  <label for="graph-dsn" class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">DSN</label>
                  <input id="graph-dsn" type="text" v-model="agentdSettings.graphDsn" class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground transition focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
                </div>
              </div>
            </section>
          </div>
        </template>

        <!-- Actions card -->
        <div class="rounded-2xl border border-border/70 bg-surface p-4 flex flex-wrap items-center justify-between">
          <p class="text-xs text-subtle-foreground">Saved values apply to new requests. Some changes may require restarting background workers.</p>
          <div class="flex flex-wrap items-center gap-3">
            <span v-if="agentdSaveError" class="text-xs text-danger-foreground">{{ agentdSaveError }}</span>
            <span v-else-if="agentdSuccess" class="text-xs text-accent-foreground">{{ agentdSuccess }}</span>
            <button type="button" class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold text-muted-foreground transition hover:border-border disabled:opacity-60" @click="loadAgentdSettings" :disabled="agentdLoading || agentdSaving">Reload</button>
            <button type="submit" class="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition hover:bg-accent/90 disabled:opacity-70" :disabled="agentdSaving">{{ agentdSaving ? 'Saving…' : 'Save Changes' }}</button>
          </div>
        </div>
      </form>

      <section v-if="isAdmin" class="space-y-4 rounded-2xl border border-border/70 bg-surface p-6 lg:col-span-2">
        <header class="space-y-1">
          <h2 class="text-lg font-semibold text-foreground">Users</h2>
          <p class="text-sm text-subtle-foreground">Create, modify, and delete users. Admin only.</p>
        </header>
        <div class="flex items-center justify-between">
          <button type="button" class="rounded-lg bg-accent px-3 py-2 text-sm font-semibold text-accent-foreground hover:bg-accent/90" @click="startCreate">New user</button>
          <div class="text-xs text-faint-foreground">Total: {{ users.length }}</div>
        </div>
        <div class="grid gap-4 lg:grid-cols-3">
          <div class="overflow-x-auto lg:col-span-2 min-w-0">
            <table class="w-full table-fixed border-collapse text-left text-sm">
              <thead>
                <tr class="border-b border-border/60 text-muted-foreground">
                  <th class="py-2 pr-2">Email</th>
                  <th class="py-2 pr-2">Name</th>
                  <th class="py-2 pr-2">Roles</th>
                  <th class="py-2 pr-2">Provider</th>
                  <th class="py-2 pr-2"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="u in users" :key="u.id" class="border-b border-border/40">
                  <td class="py-2 pr-2 truncate">{{ u.email }}</td>
                  <td class="py-2 pr-2 truncate">{{ u.name }}</td>
                  <td class="py-2 pr-2 truncate">{{ u.roles?.join(', ') }}</td>
                  <td class="py-2 pr-2 truncate">{{ u.provider }}</td>
                  <td class="py-2 flex gap-2">
                    <button class="rounded border border-border/70 px-2 py-1 text-xs hover:border-border" @click="edit(u)">Edit</button>
                    <button class="rounded border border-border/70 px-2 py-1 text-xs text-danger hover:border-border" @click="remove(u)">Delete</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="editing" class="rounded-xl border border-border/60 bg-surface-muted/40 p-4 lg:col-span-1 lg:sticky lg:top-6 self-start">
            <h3 class="mb-2 text-sm font-semibold text-foreground">{{ form.id ? 'Edit user' : 'New user' }}</h3>
            <div class="grid gap-3 md:grid-cols-2">
              <div>
                <label class="text-xs text-muted-foreground">Email</label>
                <input v-model="form.email" type="email" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
              <div>
                <label class="text-xs text-muted-foreground">Name</label>
                <input v-model="form.name" type="text" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
              <div>
                <label class="text-xs text-muted-foreground">Roles (comma-separated)</label>
                <input v-model="rolesInput" type="text" placeholder="admin, user" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
              <div>
                <label class="text-xs text-muted-foreground">Provider</label>
                <input v-model="form.provider" type="text" placeholder="oidc" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
              <div class="md:col-span-2">
                <label class="text-xs text-muted-foreground">Subject</label>
                <input v-model="form.subject" type="text" class="mt-1 w-full rounded border border-border/70 bg-surface px-2 py-1 text-sm" />
              </div>
            </div>
            <div class="mt-3 flex gap-2">
              <button class="rounded bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground hover:bg-accent/90" @click="save">Save</button>
              <button class="rounded border border-border/70 px-3 py-1 text-xs hover:border-border" @click="cancel">Cancel</button>
              <span class="text-xs text-faint-foreground" v-if="errorMsg">{{ errorMsg }}</span>
            </div>
          </div>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useThemeStore } from '@/stores/theme'
import type { ThemeChoice } from '@/theme/themes'
import {
  listUsers,
  createUser,
  updateUser,
  deleteUser,
  fetchAgentdSettings,
  updateAgentdSettings,
  type AgentdSettings,
} from '@/api/client'

const apiUrl = ref('')

const STORAGE_KEY = 'agentd.ui.settings'

type Settings = {
  apiUrl: string
}

const defaultAgentdSettings: AgentdSettings = {
  openaiSummaryModel: '',
  openaiSummaryUrl: '',
  summaryEnabled: false,
  summaryThreshold: 40,
  summaryKeepLast: 12,
  embedBaseUrl: 'https://api.openai.com',
  embedModel: 'text-embedding-3-small',
  embedApiKey: '',
  embedApiHeader: 'Authorization',
  embedPath: '/v1/embeddings',
  agentRunTimeoutSeconds: 0,
  streamRunTimeoutSeconds: 0,
  workflowTimeoutSeconds: 0,
  blockBinaries: 'rm,sudo,chown,chmod,dd,mkfs,mount,umount',
  maxCommandSeconds: 30,
  outputTruncateBytes: 65536,
  otelServiceName: 'manifold',
  serviceVersion: '0.1.0',
  environment: 'dev',
  otelExporterOtlpEndpoint: 'http://localhost:4318',
  logPath: '',
  logLevel: 'info',
  logPayloads: true,
  searxngUrl: 'http://localhost:8080',
  webSearxngUrl: 'http://localhost:8080',
  databaseUrl: '',
  dbUrl: '',
  postgresDsn: '',
  searchBackend: 'postgres',
  searchDsn: '',
  searchIndex: '',
  vectorBackend: 'postgres',
  vectorDsn: '',
  vectorIndex: '',
  vectorDimensions: 1536,
  vectorMetric: 'cosine',
  graphBackend: 'postgres',
  graphDsn: '',
}

const agentdSettings = ref<AgentdSettings>({ ...defaultAgentdSettings })
const agentdLoading = ref(false)
const agentdSaving = ref(false)
const agentdLoadError = ref('')
const agentdSaveError = ref('')
const agentdSuccess = ref('')

const logLevelOptions = ['trace', 'debug', 'info', 'warn', 'error']
const vectorMetricOptions = ['cosine', 'dot', 'euclidean']

type NumericSettingKey =
  | 'summaryThreshold'
  | 'summaryKeepLast'
  | 'agentRunTimeoutSeconds'
  | 'streamRunTimeoutSeconds'
  | 'workflowTimeoutSeconds'
  | 'maxCommandSeconds'
  | 'outputTruncateBytes'
  | 'vectorDimensions'

type BooleanSettingKey = 'summaryEnabled' | 'logPayloads'

const numericSettingKeys: NumericSettingKey[] = [
  'summaryThreshold',
  'summaryKeepLast',
  'agentRunTimeoutSeconds',
  'streamRunTimeoutSeconds',
  'workflowTimeoutSeconds',
  'maxCommandSeconds',
  'outputTruncateBytes',
  'vectorDimensions',
]
const booleanSettingKeys: BooleanSettingKey[] = ['summaryEnabled', 'logPayloads']

function toNumber(value: unknown, fallback: number): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function toBoolean(value: unknown, fallback: boolean): boolean {
  if (typeof value === 'boolean') {
    return value
  }
  if (typeof value === 'string') {
    const trimmed = value.trim().toLowerCase()
    if (trimmed === 'true' || trimmed === '1' || trimmed === 'yes') {
      return true
    }
    if (trimmed === 'false' || trimmed === '0' || trimmed === 'no') {
      return false
    }
  }
  return fallback
}

function normalizeAgentdSettings(input?: Partial<AgentdSettings>): AgentdSettings {
  const merged: AgentdSettings = { ...defaultAgentdSettings, ...(input ?? {}) }
  for (const key of numericSettingKeys) {
    merged[key] = toNumber(input?.[key], defaultAgentdSettings[key])
  }
  for (const key of booleanSettingKeys) {
    merged[key] = toBoolean(input?.[key], defaultAgentdSettings[key])
  }
  return merged
}

async function loadAgentdSettings() {
  agentdLoading.value = true
  agentdLoadError.value = ''
  try {
    const data = await fetchAgentdSettings()
    agentdSettings.value = normalizeAgentdSettings(data)
  } catch (error: any) {
    console.warn('Failed to load agentd settings', error)
    agentdLoadError.value = error?.response?.data ?? 'Unable to load agent configuration'
    agentdSettings.value = normalizeAgentdSettings(agentdSettings.value)
  } finally {
    agentdLoading.value = false
  }
}

async function saveAgentdSettings() {
  if (agentdSaving.value) {
    return
  }
  agentdSaving.value = true
  agentdSaveError.value = ''
  agentdSuccess.value = ''
  try {
    const payload: AgentdSettings = { ...agentdSettings.value }
    const saved = await updateAgentdSettings(payload)
    // Some servers respond to PUT with 204 No Content. In that case, `saved` will
    // be undefined/empty and we were previously resetting the form back to defaults.
    // Prefer the server echo when present; otherwise reload from GET, and finally
    // fall back to the payload the user submitted so the UI reflects their choices.
    const looksLikeSettings =
      saved && typeof saved === 'object' && 'openaiSummaryModel' in (saved as any)
    if (looksLikeSettings) {
      agentdSettings.value = normalizeAgentdSettings(saved as Partial<AgentdSettings>)
    } else {
      try {
        await loadAgentdSettings()
      } catch {
        agentdSettings.value = normalizeAgentdSettings(payload)
      }
    }
    agentdSuccess.value = 'Saved'
    window.setTimeout(() => {
      agentdSuccess.value = ''
    }, 3000)
  } catch (error: any) {
    console.error('Failed to save agentd settings', error)
    const status = error?.response?.status
    if (error?.code === 'READ_ONLY' || status === 405 || status === 404 || status === 501) {
      // Backend does not expose a write endpoint for agentd config.
      // Keep the current UI values and show a clear message.
      agentdSaveError.value = 'Configuration is read-only on this server. Update config.yaml / environment and restart agentd.'
    } else {
      agentdSaveError.value = error?.response?.data ?? 'Save failed'
    }
  } finally {
    agentdSaving.value = false
  }
}

const themeStore = useThemeStore()
const themeOptions = computed(() => themeStore.options)
const themeSelection = computed(() => themeStore.selection)

// Admin-only Users section state
type User = {
  id: number
  email: string
  name: string
  provider?: string
  subject?: string
  roles: string[]
}
const users = ref<User[]>([])
const isAdmin = ref(false)
const editing = ref(false)
const form = ref<Partial<User>>({})
const rolesInput = ref('')
const errorMsg = ref('')

onMounted(() => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored) as Settings
      apiUrl.value = parsed.apiUrl
    }
  } catch (error) {
    console.warn('Unable to parse stored settings', error)
  }
  loadAgentdSettings()
  // Determine admin by probing a protected admin endpoint indirectly: list users.
  // If it succeeds, current user is authenticated and has access (auth middleware already enforces auth).
  refreshUsers()
})

function persist() {
  const payload: Settings = {
    apiUrl: apiUrl.value,
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
}

function resetToDefaults() {
  localStorage.removeItem(STORAGE_KEY)
  apiUrl.value = ''
}

function selectTheme(choice: ThemeChoice) {
  themeStore.setTheme(choice)
}

async function refreshUsers() {
  try {
    const data = await listUsers()
    users.value = data
    isAdmin.value = true // if call succeeds, treat as admin-capable page
  } catch (e) {
    // Not admin or not authenticated; keep hidden
    isAdmin.value = false
  }
}

function startCreate() {
  form.value = { id: 0, email: '', name: '', provider: 'oidc', subject: '', roles: ['user'] }
  rolesInput.value = (form.value.roles || []).join(', ')
  errorMsg.value = ''
  editing.value = true
}

function edit(u: User) {
  form.value = { ...u }
  rolesInput.value = (u.roles || []).join(', ')
  errorMsg.value = ''
  editing.value = true
}

async function save() {
  if (!form.value) return
  const payload: any = {
    email: form.value.email,
    name: form.value.name,
    provider: form.value.provider,
    subject: form.value.subject,
    roles: rolesInput.value.split(',').map((s) => s.trim()).filter(Boolean),
  }
  try {
    if (!form.value.id || form.value.id === 0) {
      await createUser(payload)
    } else {
      await updateUser(form.value.id, payload)
    }
    editing.value = false
    await refreshUsers()
  } catch (e: any) {
    errorMsg.value = e?.response?.data || 'Save failed'
  }
}

function cancel() {
  editing.value = false
}

async function remove(u: User) {
  if (!confirm(`Delete user ${u.email}?`)) return
  try {
    await deleteUser(u.id)
    await refreshUsers()
  } catch (e) {
    // ignore
  }
}
</script>
