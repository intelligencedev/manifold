<template>
  <div class="flex h-full min-h-0 flex-col overflow-hidden">
    <!-- Sticky header: title + subtitle + close + tabs -->
    <div
      class="sticky top-0 z-10 border-b border-border/50 bg-surface/90 backdrop-blur-sm"
    >
      <div class="flex items-start justify-between gap-3 px-4 pb-3 pt-4">
        <div class="min-w-0">
          <h2 class="text-base font-semibold text-foreground">
            {{ headerTitle }}
          </h2>
          <p
            v-if="headerSubtitle"
            class="mt-0.5 text-xs text-subtle-foreground"
          >
            {{ headerSubtitle }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <span
            v-if="isDirty"
            class="rounded-full border border-border/60 bg-surface-muted/30 px-2 py-1 text-xs font-semibold text-subtle-foreground"
            >Unsaved</span
          >
          <button
            type="button"
            class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
            @click="onCancel"
          >
            Close
          </button>
        </div>
      </div>

      <div
        role="tablist"
        aria-label="Edit Specialist"
        class="flex flex-wrap gap-2 px-4 pb-3"
      >
        <button
          v-for="t in tabs"
          :key="t.id"
          role="tab"
          :id="`tab-${t.id}`"
          :aria-controls="`panel-${t.id}`"
          :aria-selected="activeTab === t.id ? 'true' : 'false'"
          :tabindex="activeTab === t.id ? 0 : -1"
          type="button"
          class="rounded-full border px-3 py-1.5 text-xs font-semibold transition"
          :class="
            activeTab === t.id
              ? 'border-border/80 bg-surface-muted/60 text-foreground'
              : 'border-border/50 text-subtle-foreground hover:border-border'
          "
          @click="activeTab = t.id"
          @keydown="onTabKeydown($event, t.id)"
        >
          <span>{{ t.label }}</span>
          <span
            v-if="tabHasErrors(t.id)"
            class="ml-2 inline-flex h-1.5 w-1.5 rounded-full bg-danger"
          ></span>
        </button>
      </div>
    </div>

    <!-- Scrollable body (single scroll region) -->
    <div
      class="flex flex-1 min-h-0 flex-col overflow-auto px-4 pb-6 pt-4 scrollbar-inset"
    >
      <div
        v-if="actionError"
        class="mb-4 rounded-2xl border border-danger/60 bg-danger/10 p-3 text-danger-foreground text-sm"
      >
        {{ actionError }}
      </div>
      <div
        v-if="successMsg"
        class="mb-4 rounded-2xl border border-border/60 bg-surface-muted/30 p-3 text-sm text-foreground"
      >
        {{ successMsg }}
      </div>

      <!-- BASICS -->
      <div
        v-show="activeTab === 'basics'"
        role="tabpanel"
        :id="'panel-basics'"
        :aria-labelledby="'tab-basics'"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <div
          v-if="submitAttempted && errorsByTab.basics.length"
          class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground"
        >
          <p class="font-semibold">Fix the following to continue:</p>
          <ul class="mt-2 list-disc pl-5">
            <li v-for="e in errorsByTab.basics" :key="e">{{ e }}</li>
          </ul>
        </div>

        <FormSection
          title="Identity"
          helper="Give the specialist a stable name and a short description."
        >
          <div class="flex flex-col gap-3">
            <div class="flex flex-col gap-1">
              <label
                for="sp-name"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Name</label
              >
              <input
                id="sp-name"
                v-model.trim="draft.name"
                class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                :disabled="lockName"
                @blur="touch('name')"
              />
              <p
                v-if="fieldError('name')"
                class="text-xs text-danger-foreground"
              >
                {{ fieldError("name") }}
              </p>
            </div>

            <div class="flex flex-col gap-1">
              <label
                for="sp-description"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Description</label
              >
              <textarea
                id="sp-description"
                v-model="draft.description"
                rows="4"
                class="w-full resize-y overflow-auto rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                @blur="touch('description')"
              ></textarea>
            </div>
          </div>
        </FormSection>

        <FormSection
          title="Status"
          helper="Paused specialists are not available for use and do not consume orchestration context."
        >
          <div class="flex flex-col gap-3">
            <label
              class="inline-flex items-center justify-between gap-3 rounded border border-border/60 bg-surface-muted/20 px-3 py-2"
            >
              <span class="text-sm text-foreground">Enabled</span>
              <input
                id="sp-enabled"
                type="checkbox"
                class="h-4 w-4"
                :checked="!draft.paused"
                @change="
                  draft.paused = !($event.target as HTMLInputElement).checked
                "
              />
            </label>
            <label
              class="inline-flex items-center justify-between gap-3 rounded border border-border/60 bg-surface-muted/20 px-3 py-2"
            >
              <span class="text-sm text-foreground">Paused</span>
              <input
                id="sp-paused"
                v-model="draft.paused"
                type="checkbox"
                class="h-4 w-4"
              />
            </label>
          </div>
        </FormSection>

        <FormSection
          title="Groups"
          helper="Assign this specialist to one or more teams. Specialists can belong to multiple groups."
        >
          <div class="flex flex-col gap-3">
            <input
              v-model="groupSearch"
              type="text"
              placeholder="Search groups"
              class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm text-foreground"
            />
            <div class="rounded-lg border border-border/60 bg-surface">
              <div
                v-if="!availableGroups.length"
                class="px-3 py-3 text-sm text-subtle-foreground"
              >
                No groups created yet.
              </div>
              <div
                v-else-if="!filteredGroupOptions.length"
                class="px-3 py-3 text-sm text-subtle-foreground"
              >
                No groups match your search.
              </div>
              <label
                v-for="g in filteredGroupOptions"
                :key="g"
                class="flex cursor-pointer items-start gap-3 border-t border-border/40 px-3 py-2 transition-colors first:border-t-0 hover:bg-surface-muted/40"
              >
                <input
                  class="mt-1 h-4 w-4 shrink-0"
                  type="checkbox"
                  :checked="selectedGroupsSet.has(g)"
                  @change="setGroupSelected(g, ($event.target as HTMLInputElement).checked)"
                />
                <div class="min-w-0">
                  <p class="text-sm font-medium text-foreground">{{ g }}</p>
                </div>
              </label>
            </div>
          </div>
        </FormSection>

        <FormSection
          title="Runtime"
          helper="Select a provider and model. Optionally override the default endpoint."
        >
          <div class="flex flex-col gap-3">
            <div class="grid gap-3 md:grid-cols-2">
              <div class="flex flex-col gap-1">
                <label
                  for="sp-provider"
                  class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                  >Provider</label
                >
                <DropdownSelect
                  id="sp-provider"
                  v-model="draft.provider"
                  :options="providerDropdownOptions"
                  class="w-full text-sm"
                  @update:modelValue="onProviderChange"
                />
                <p
                  v-if="fieldError('provider')"
                  class="text-xs text-danger-foreground"
                >
                  {{ fieldError("provider") }}
                </p>
              </div>
              <div class="flex flex-col gap-1">
                <label
                  for="sp-model"
                  class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                  >Model</label
                >
                <input
                  id="sp-model"
                  v-model.trim="draft.model"
                  class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                  @blur="touch('model')"
                />
                <p
                  v-if="fieldError('model')"
                  class="text-xs text-danger-foreground"
                >
                  {{ fieldError("model") }}
                </p>
              </div>
            </div>

            <div
              class="rounded border border-border/60 bg-surface-muted/20 p-3"
            >
              <label
                class="inline-flex items-center gap-2 text-sm text-foreground"
              >
                <input
                  v-model="draft.useDefaultEndpoint"
                  type="checkbox"
                  class="h-4 w-4"
                />
                <span>Use default endpoint (recommended)</span>
              </label>
              <p class="mt-1 text-xs text-subtle-foreground">
                Stores only overrides when you provide a custom endpoint.
              </p>

              <div
                v-if="!draft.useDefaultEndpoint"
                class="mt-3 flex flex-col gap-1"
              >
                <label
                  for="sp-baseurl"
                  class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                  >Custom endpoint</label
                >
                <input
                  id="sp-baseurl"
                  v-model.trim="draft.customBaseURL"
                  class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                  placeholder="https://…"
                  @blur="touch('baseURL')"
                />
                <p
                  v-if="fieldError('baseURL')"
                  class="text-xs text-danger-foreground"
                >
                  {{ fieldError("baseURL") }}
                </p>
              </div>
              <div v-else class="mt-3 text-xs text-subtle-foreground">
                Default:
                <span class="font-mono">{{ defaultBaseURL || "—" }}</span>
              </div>
            </div>
          </div>
        </FormSection>

        <FormSection
          title="Credentials"
          helper="Secrets are never re-displayed after save."
        >
          <div class="flex flex-col gap-3">
            <div
              class="flex items-center justify-between gap-3 rounded border border-border/60 bg-surface-muted/20 px-3 py-2"
            >
              <div>
                <p class="text-sm font-medium text-foreground">API key</p>
                <p class="text-xs text-subtle-foreground">
                  {{ credentialStatus }}
                </p>
              </div>
              <button
                type="button"
                class="rounded border border-border/60 bg-surface px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
                @click="openCredentialModal"
              >
                Update credential…
              </button>
            </div>
          </div>
        </FormSection>
      </div>

      <!-- PROMPT -->
      <div
        v-show="activeTab === 'prompt'"
        role="tabpanel"
        :id="'panel-prompt'"
        :aria-labelledby="'tab-prompt'"
        tabindex="0"
        class="flex min-h-0 flex-1 flex-col gap-4"
      >
        <div
          v-if="submitAttempted && errorsByTab.prompt.length"
          class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground"
        >
          <p class="font-semibold">Fix the following to continue:</p>
          <ul class="mt-2 list-disc pl-5">
            <li v-for="e in errorsByTab.prompt" :key="e">{{ e }}</li>
          </ul>
        </div>

        <FormSection
          title="Prompt"
          helper="Edit the system prompt. Optionally apply a saved template/version."
          class="flex min-h-0 flex-1 flex-col"
        >
          <div class="flex min-h-0 flex-1 flex-col gap-3">
            <div class="grid gap-3 md:grid-cols-2">
              <div class="flex flex-col gap-1">
                <label
                  for="prompt-select"
                  class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                  >Template</label
                >
                <DropdownSelect
                  id="prompt-select"
                  v-model="promptApply.promptId"
                  :options="promptDropdownOptions"
                  class="w-full text-sm"
                  @update:modelValue="onSelectPrompt"
                />
              </div>
              <div class="flex flex-col gap-1">
                <label
                  for="prompt-version-select"
                  class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                  >Version</label
                >
                <DropdownSelect
                  id="prompt-version-select"
                  v-model="promptApply.versionId"
                  :options="versionDropdownOptions"
                  class="w-full text-sm"
                  :disabled="!promptApply.promptId || versionsLoading"
                />
              </div>
            </div>

            <div class="flex items-center justify-between gap-2">
              <p
                v-if="applyVersionError"
                class="text-xs text-danger-foreground"
              >
                {{ applyVersionError }}
              </p>
              <div class="flex items-center gap-2">
                <button
                  type="button"
                  class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border disabled:cursor-not-allowed disabled:opacity-50"
                  :disabled="!promptApply.versionId"
                  @click="applySelectedVersion"
                >
                  Apply
                </button>
              </div>
            </div>

            <div class="flex min-h-0 flex-1 flex-col">
              <label
                for="sp-system"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >System prompt</label
              >
              <div class="mt-2 flex min-h-0 flex-1 flex-col">
                <CodeEditor
                  class="flex-1"
                  id="sp-system"
                  v-model="draft.system"
                  :showToolbar="true"
                  :formatAction="null"
                  @blur="touch('system')"
                >
                  <template #left>
                    <span
                      >Used as the system instruction for this specialist.</span
                    >
                  </template>
                </CodeEditor>
              </div>
              <p
                v-if="fieldError('system')"
                class="mt-1 text-xs text-danger-foreground"
              >
                {{ fieldError("system") }}
              </p>
            </div>
          </div>
        </FormSection>
      </div>

      <!-- TOOLS -->
      <div
        v-show="activeTab === 'tools'"
        role="tabpanel"
        :id="'panel-tools'"
        :aria-labelledby="'tab-tools'"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <div
          v-if="submitAttempted && errorsByTab.tools.length"
          class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground"
        >
          <p class="font-semibold">Fix the following to continue:</p>
          <ul class="mt-2 list-disc pl-5">
            <li v-for="e in errorsByTab.tools" :key="e">{{ e }}</li>
          </ul>
        </div>

        <FormSection
          title="Tool access policy"
          helper="Choose whether this specialist may call tools."
        >
          <div class="grid grid-cols-3 gap-2">
            <label
              class="flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors"
              :class="
                draft.toolPolicy === 'none'
                  ? 'border-border/80 bg-surface-muted/60'
                  : 'border-border/50 hover:border-border'
              "
            >
              <input
                class="mt-1 h-4 w-4 shrink-0"
                type="radio"
                name="tools-policy"
                value="none"
                v-model="draft.toolPolicy"
              />
              <div>
                <p class="font-medium text-foreground">No tools</p>
                <p class="text-xs text-subtle-foreground">
                  Specialist will never call tools.
                </p>
              </div>
            </label>

            <label
              class="flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors"
              :class="
                draft.toolPolicy === 'any'
                  ? 'border-border/80 bg-surface-muted/60'
                  : 'border-border/50 hover:border-border'
              "
            >
              <input
                class="mt-1 h-4 w-4 shrink-0"
                type="radio"
                name="tools-policy"
                value="any"
                v-model="draft.toolPolicy"
              />
              <div>
                <p class="font-medium text-foreground">Allow any tool</p>
                <p class="text-xs text-subtle-foreground">
                  Every available tool can be invoked.
                </p>
              </div>
            </label>

            <label
              class="flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors"
              :class="
                draft.toolPolicy === 'allow-list'
                  ? 'border-border/80 bg-surface-muted/60'
                  : 'border-border/50 hover:border-border'
              "
            >
              <input
                class="mt-1 h-4 w-4 shrink-0"
                type="radio"
                name="tools-policy"
                value="allow-list"
                v-model="draft.toolPolicy"
              />
              <div>
                <p class="font-medium text-foreground">Allow list</p>
                <p class="text-xs text-subtle-foreground">
                  Only selected tools will be enabled.
                </p>
              </div>
            </label>
          </div>
        </FormSection>

        <FormSection
          v-if="draft.toolPolicy === 'allow-list'"
          title="Allowed tools"
          helper="Search and select which tools this specialist may invoke."
        >
          <div class="flex flex-col gap-3">
            <div class="flex items-center justify-between gap-3">
              <p class="text-sm text-muted-foreground">
                Selected:
                <span class="font-semibold text-foreground">{{
                  allowTools.length
                }}</span>
              </p>
            </div>

            <div>
              <label
                for="sp-tools-search"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Search tools</label
              >
              <input
                id="sp-tools-search"
                v-model="toolsSearch"
                type="text"
                placeholder="Type to filter by name or description"
                class="mt-1 w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm text-foreground"
              />
            </div>

            <p v-if="toolsLoading" class="text-xs text-subtle-foreground">
              Loading tools…
            </p>
            <p v-else-if="toolsError" class="text-xs text-danger-foreground">
              {{ toolsError }}
            </p>

            <div v-else class="rounded-lg border border-border/60 bg-surface">
              <div
                v-if="!filteredTools.length"
                class="px-3 py-3 text-sm text-subtle-foreground"
              >
                No tools match your search.
              </div>
              <label
                v-for="t in filteredTools"
                :key="t.name"
                class="flex cursor-pointer items-start gap-3 border-t border-border/40 px-3 py-2 transition-colors first:border-t-0 hover:bg-surface-muted/40"
              >
                <input
                  class="mt-1 h-4 w-4 shrink-0"
                  type="checkbox"
                  :checked="allowToolsSet.has(t.name)"
                  @change="
                    setToolAllowed(
                      t.name,
                      ($event.target as HTMLInputElement).checked,
                    )
                  "
                />
                <div class="min-w-0">
                  <p class="text-sm font-medium text-foreground">
                    {{ t.name }}
                  </p>
                  <p
                    v-if="t.description"
                    class="mt-0.5 text-xs text-subtle-foreground"
                  >
                    {{ t.description }}
                  </p>
                </div>
              </label>
            </div>
          </div>
        </FormSection>
      </div>

      <!-- ADVANCED -->
      <div
        v-show="activeTab === 'advanced'"
        role="tabpanel"
        :id="'panel-advanced'"
        :aria-labelledby="'tab-advanced'"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <div
          v-if="submitAttempted && errorsByTab.advanced.length"
          class="rounded-2xl border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground"
        >
          <p class="font-semibold">Fix the following to continue:</p>
          <ul class="mt-2 list-disc pl-5">
            <li v-for="e in errorsByTab.advanced" :key="e">{{ e }}</li>
          </ul>
        </div>

        <FormSection
          title="Summarization"
          helper="Override the summary context window for this specialist. Leave blank to use the global summaryContextWindowTokens fallback."
        >
          <div class="flex flex-col gap-1">
            <label
              for="sp-summary-context"
              class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
              >Summary context window (tokens)</label>
            <input
              id="sp-summary-context"
              v-model="draft.summaryContextWindowTokens"
              type="number"
              min="1"
              step="1"
              class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
              placeholder="Use global default"
              @blur="touch('summaryContextWindowTokens')"
            />
            <p
              v-if="fieldError('summaryContextWindowTokens')"
              class="text-xs text-danger-foreground"
            >
              {{ fieldError("summaryContextWindowTokens") }}
            </p>
          </div>
        </FormSection>

        <CollapsiblePanel
          v-model="headersOpen"
          title="Extra headers"
          helper="Sent with requests made by this specialist."
        >
          <KeyValueTableEditor
            v-model="extraHeadersRows"
            helper="Keys must be unique. Values are strings."
            @editJson="openJsonModal('headers')"
            @blur="touch('extraHeaders')"
          />
        </CollapsiblePanel>

        <CollapsiblePanel
          v-model="paramsOpen"
          title="Extra params"
          helper="Additional provider parameters."
        >
          <KeyValueTableEditor
            v-model="extraParamsRows"
            helper="Values are strings in the table editor. Use JSON mode for typed values."
            @editJson="openJsonModal('params')"
            @blur="touch('extraParams')"
          />
        </CollapsiblePanel>
      </div>
    </div>

    <!-- Sticky footer -->
    <div
      class="sticky bottom-0 z-10 border-t border-border/50 bg-surface/90 backdrop-blur-sm"
    >
      <div class="flex items-center justify-between gap-3 px-4 py-3">
        <div class="text-xs text-subtle-foreground">
          <span v-if="saving">Saving…</span>
          <span v-else-if="actionError">Save failed.</span>
          <span v-else-if="successMsg">{{ successMsg }}</span>
          <span v-else-if="isDirty">Changes not saved.</span>
          <span v-else>Up to date.</span>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="rounded-md border border-border/60 px-3 py-1.5 text-sm"
            @click="onCancel"
          >
            Cancel
          </button>
          <button
            type="button"
            class="rounded-md border border-border/60 bg-surface-muted px-3 py-1.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="saving"
            @click="onSave"
          >
            {{ saving ? "Saving…" : "Save" }}
          </button>
        </div>
      </div>
    </div>

    <!-- Credential modal -->
    <div
      v-if="showCredentialModal"
      class="fixed inset-0 z-50 flex items-center justify-center px-4 py-8"
      @keydown="onCredentialKeydown"
    >
      <div
        class="absolute inset-0 bg-surface/70 backdrop-blur-sm"
        @click="closeCredentialModal(false)"
      ></div>
      <div
        ref="credPanel"
        class="relative z-10 w-full max-w-lg overflow-hidden rounded-xl border border-border/70 bg-surface shadow-2xl"
      >
        <div
          class="flex items-center justify-between border-b border-border/60 px-5 py-4"
        >
          <div>
            <h3 class="text-base font-semibold text-foreground">
              Update credential
            </h3>
            <p class="text-xs text-subtle-foreground">
              Enter a new API key. It won’t be shown again.
            </p>
          </div>
          <button
            ref="credCloseBtn"
            type="button"
            class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
            @click="closeCredentialModal(false)"
          >
            Close
          </button>
        </div>
        <div class="px-5 py-4">
          <label
            for="sp-api-key"
            class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
            >API key</label
          >
          <input
            id="sp-api-key"
            ref="credInput"
            v-model="credentialDraft"
            type="password"
            class="mt-1 w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm text-foreground"
            autocomplete="new-password"
          />
          <p class="mt-2 text-xs text-subtle-foreground">
            Leave blank to keep the existing credential.
          </p>
        </div>
        <div
          class="flex items-center justify-end gap-2 border-t border-border/60 px-5 py-3"
        >
          <button
            type="button"
            class="rounded border border-border/60 bg-surface px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
            @click="closeCredentialModal(false)"
          >
            Cancel
          </button>
          <button
            type="button"
            class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
            @click="closeCredentialModal(true)"
          >
            Save
          </button>
        </div>
      </div>
    </div>

    <!-- JSON editor modal -->
    <JsonEditorModal
      v-if="showJsonModal"
      :open="showJsonModal"
      :title="jsonModalTitle"
      :subtitle="jsonModalSubtitle"
      :initialText="jsonModalText"
      @cancel="closeJsonModal"
      @apply="applyJson"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import DropdownSelect from "@/components/DropdownSelect.vue";
import FormSection from "@/components/specialists/edit/FormSection.vue";
import CollapsiblePanel from "@/components/specialists/edit/CollapsiblePanel.vue";
import CodeEditor from "@/components/specialists/edit/CodeEditor.vue";
import KeyValueTableEditor, {
  type KeyValueRow,
} from "@/components/specialists/edit/KeyValueTableEditor.vue";
import JsonEditorModal from "@/components/specialists/edit/JsonEditorModal.vue";
import {
  upsertSpecialist,
  type Specialist,
  type SpecialistProviderDefaults,
} from "@/api/client";
import {
  listPrompts,
  listPromptVersions,
  type Prompt,
  type PromptVersion,
} from "@/api/playground";
import { fetchWarppTools } from "@/api/warpp";
import type { WarppTool } from "@/types/warpp";

type TabId = "basics" | "prompt" | "tools" | "advanced";
type ToolPolicy = "none" | "any" | "allow-list";

const props = withDefaults(
  defineProps<{
    initial: Specialist;
    lockName?: boolean;
    providerDefaults?: Record<string, SpecialistProviderDefaults>;
    providerOptions: string[];
    availableGroups?: string[];
    credentialConfigured?: boolean;
  }>(),
  { lockName: false, credentialConfigured: false },
);

const emit = defineEmits<{ saved: [Specialist]; cancel: [] }>();

const qc = useQueryClient();

const activeTab = ref<TabId>("basics");
const tabs = [
  { id: "basics" as const, label: "Basics" },
  { id: "prompt" as const, label: "Prompt" },
  { id: "tools" as const, label: "Tools" },
  { id: "advanced" as const, label: "Advanced" },
];

const touched = ref(new Set<string>());
const submitAttempted = ref(false);
const saving = ref(false);

const actionError = ref<string | null>(null);
const successMsg = ref<string | null>(null);

const draft = reactive({
  name: "",
  description: "",
  provider: "",
  model: "",
  summaryContextWindowTokens: "",
  paused: false,
  useDefaultEndpoint: true,
  customBaseURL: "",
  system: "",
  toolPolicy: "none" as ToolPolicy,
});

const nameLockedAfterSave = ref(false);
const lockName = computed(() => !!props.lockName || nameLockedAfterSave.value);

const baseline = ref<Specialist | null>(null);

const extraHeadersRows = ref<KeyValueRow[]>([]);
const extraParamsRows = ref<KeyValueRow[]>([]);
const extraHeadersObj = ref<Record<string, string>>({});
const extraParamsObj = ref<Record<string, any>>({});

const headersOpen = ref(false);
const paramsOpen = ref(false);
const promptHelpOpen = ref(false);
const tools = ref<WarppTool[]>([]);
const toolsLoading = ref(false);
const toolsError = ref("");
const toolsSearch = ref("");

const groupSearch = ref("");
const selectedGroups = ref<string[]>([]);

const showCredentialModal = ref(false);
const credentialDraft = ref("");
const credPanel = ref<HTMLElement | null>(null);
const credInput = ref<HTMLInputElement | null>(null);
const credCloseBtn = ref<HTMLButtonElement | null>(null);
const credRestoreFocusEl = ref<HTMLElement | null>(null);

const showJsonModal = ref(false);
const jsonTarget = ref<"headers" | "params">("headers");

// Prompts
const availablePrompts = ref<Prompt[]>([]);
const availableVersions = ref<PromptVersion[]>([]);
const promptsLoading = ref(false);
const versionsLoading = ref(false);
const applyVersionError = ref<string | null>(null);
const promptApply = ref<{ promptId: string; versionId: string }>({
  promptId: "",
  versionId: "",
});

const providerDropdownOptions = computed(() =>
  props.providerOptions.map((opt) => ({ id: opt, label: opt, value: opt })),
);

const availableGroups = computed(() => props.availableGroups || []);
const selectedGroupsSet = computed(() => new Set(selectedGroups.value));
const filteredGroupOptions = computed(() => {
  const q = groupSearch.value.trim().toLowerCase();
  if (!q) return availableGroups.value;
  return availableGroups.value.filter((g) => g.toLowerCase().includes(q));
});

const providerDefaultsForSelected = computed(() => {
  const prov = (draft.provider || "").trim();
  return (
    (props.providerDefaults && prov && props.providerDefaults[prov]) ||
    undefined
  );
});

const defaultBaseURL = computed(
  () => providerDefaultsForSelected.value?.baseURL || "",
);

const headerTitle = computed(() =>
  baseline.value?.name ? "Edit Specialist" : "Create Specialist",
);
const headerSubtitle = computed(() =>
  baseline.value?.name ? baseline.value.name : null,
);

const allowTools = ref<string[]>([]);
const allowToolsSet = computed(() => new Set(allowTools.value));

const filteredTools = computed(() => {
  const q = toolsSearch.value.trim().toLowerCase();
  if (!q) return tools.value;
  return tools.value.filter((t) => {
    const name = (t.name || "").toLowerCase();
    const desc = (t.description || "").toLowerCase();
    return name.includes(q) || desc.includes(q);
  });
});

const credentialStatus = computed(() => {
  if (credentialDraft.value.trim()) return "Pending update";
  if (props.credentialConfigured) return "Configured";
  return "Not configured";
});

function touch(field: string) {
  touched.value.add(field);
}

function tabHasErrors(tab: TabId): boolean {
  return errorsByTab.value[tab].length > 0;
}

function fieldError(field: string): string | null {
  if (!submitAttempted.value && !touched.value.has(field)) return null;
  return fieldErrors.value[field] || null;
}

const fieldErrors = computed<Record<string, string>>(() => {
  const errs: Record<string, string> = {};

  if (!draft.name.trim()) errs.name = "Name is required.";
  if (!draft.provider.trim()) errs.provider = "Provider is required.";
  if (!draft.model.trim()) errs.model = "Model is required.";

  const computedBaseURL = draft.useDefaultEndpoint
    ? defaultBaseURL.value || ""
    : draft.customBaseURL;
  if (!computedBaseURL.trim()) {
    errs.baseURL = draft.useDefaultEndpoint
      ? "Default endpoint is unavailable. Provide a custom endpoint."
      : "Custom endpoint is required when default is disabled.";
  }

  // System prompt can be empty, but keep a basic guard when user touched it.
  if (
    touched.value.has("system") &&
    draft.system != null &&
    typeof draft.system !== "string"
  ) {
    errs.system = "System prompt must be text.";
  }

  const headerRowErrs = validateRows(extraHeadersRows.value);
  if (headerRowErrs.length) errs.extraHeaders = "Fix errors in extra headers.";

  const paramsRowErrs = validateRows(extraParamsRows.value);
  if (paramsRowErrs.length) errs.extraParams = "Fix errors in extra params.";

  const summaryOverride = String(draft.summaryContextWindowTokens || "").trim();
  if (summaryOverride) {
    const parsed = Number(summaryOverride);
    if (!Number.isFinite(parsed) || !Number.isInteger(parsed) || parsed <= 0) {
      errs.summaryContextWindowTokens =
        "Summary context window must be a positive whole number.";
    }
  }

  return errs;
});

const errorsByTab = computed(() => {
  const basics: string[] = [];
  const prompt: string[] = [];
  const toolsTab: string[] = [];
  const advanced: string[] = [];

  if (fieldErrors.value.name) basics.push(fieldErrors.value.name);
  if (fieldErrors.value.provider) basics.push(fieldErrors.value.provider);
  if (fieldErrors.value.model) basics.push(fieldErrors.value.model);
  if (fieldErrors.value.baseURL) basics.push(fieldErrors.value.baseURL);

  if (fieldErrors.value.system) prompt.push(fieldErrors.value.system);

  // No hard validation for tools selection beyond syntactic constraints.
  void toolsTab;

  if (fieldErrors.value.extraHeaders)
    advanced.push(fieldErrors.value.extraHeaders);
  if (fieldErrors.value.extraParams)
    advanced.push(fieldErrors.value.extraParams);
  if (fieldErrors.value.summaryContextWindowTokens)
    advanced.push(fieldErrors.value.summaryContextWindowTokens);

  return { basics, prompt, tools: toolsTab, advanced };
});

const isValid = computed(() => Object.keys(fieldErrors.value).length === 0);

type SpecialistComparable = Omit<Specialist, "id" | "apiKey">;

const baselinePayload = computed<SpecialistComparable>(() =>
  baseline.value
    ? normalizeComparable(baseline.value)
    : normalizeComparable(buildPayloadFromDraft()),
);

const currentPayload = computed<SpecialistComparable>(() =>
  normalizeComparable(buildPayloadFromDraft()),
);

const isDirty = computed(
  () =>
    stableStringify(baselinePayload.value) !==
    stableStringify(currentPayload.value),
);

function stableStringify(value: any): string {
  return JSON.stringify(sortKeys(value));
}

function sortKeys(value: any): any {
  if (Array.isArray(value)) return value.map(sortKeys);
  if (!value || typeof value !== "object") return value;
  const keys = Object.keys(value).sort();
  const out: any = {};
  for (const k of keys) out[k] = sortKeys(value[k]);
  return out;
}

function normalizeComparable(sp: Specialist): SpecialistComparable {
  const allowTools = Array.isArray(sp.allowTools) ? [...sp.allowTools] : [];
  allowTools.sort((a, b) =>
    a.localeCompare(b, undefined, { sensitivity: "base" }),
  );
  const groups = Array.isArray(sp.groups) ? [...sp.groups] : [];
  groups.sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }));

  return {
    name: (sp.name || "").trim(),
    description: sp.description ?? "",
    provider: sp.provider || "",
    baseURL: (sp.baseURL || "").trim(),
    model: (sp.model || "").trim(),
    summaryContextWindowTokens: sp.summaryContextWindowTokens || 0,
    enableTools: !!sp.enableTools,
    paused: !!sp.paused,
    allowTools,
    system: sp.system || "",
    extraHeaders: sp.extraHeaders || {},
    extraParams: sp.extraParams || {},
    groups,
  };
}

function normalizePayload(sp: Specialist): Specialist {
  return {
    ...sp,
    name: (sp.name || "").trim(),
    description: sp.description ?? "",
    provider: sp.provider || "",
    baseURL: (sp.baseURL || "").trim(),
    model: (sp.model || "").trim(),
    summaryContextWindowTokens: sp.summaryContextWindowTokens || 0,
    enableTools: !!sp.enableTools,
    paused: !!sp.paused,
    apiKey: sp.apiKey || undefined,
    allowTools: Array.isArray(sp.allowTools) ? sp.allowTools : [],
    system: sp.system || "",
    extraHeaders: sp.extraHeaders || {},
    extraParams: sp.extraParams || {},
    groups: Array.isArray(sp.groups) ? sp.groups : [],
  };
}

function validateRows(rows: KeyValueRow[]): string[] {
  const errors: string[] = [];
  const seen = new Set<string>();

  for (const r of rows) {
    const key = (r.key || "").trim();
    if (!key) {
      errors.push("Key is required.");
      continue;
    }
    const norm = key.toLowerCase();
    if (seen.has(norm)) {
      errors.push("Duplicate key.");
      continue;
    }
    seen.add(norm);
  }

  return errors;
}

function rowsToHeaders(rows: KeyValueRow[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const r of rows) {
    const key = r.key.trim();
    if (!key) continue;
    out[key] = r.value;
  }
  return out;
}

function rowsToParams(rows: KeyValueRow[]): Record<string, any> {
  const out: Record<string, any> = {};
  for (const r of rows) {
    const key = r.key.trim();
    if (!key) continue;
    out[key] = r.value;
  }
  return out;
}

function objectToRows(obj: Record<string, any>): KeyValueRow[] {
  return Object.entries(obj || {}).map(([k, v]) => ({
    id: crypto.randomUUID(),
    key: String(k),
    value: typeof v === "string" ? v : JSON.stringify(v),
  }));
}

function buildPayloadFromDraft(): Specialist {
  const defaults = providerDefaultsForSelected.value;
  const baseURL = draft.useDefaultEndpoint
    ? defaults?.baseURL || ""
    : draft.customBaseURL;

  const toolPolicy = draft.toolPolicy;
  const enableTools = toolPolicy !== "none";
  const allow = toolPolicy === "allow-list" ? allowTools.value : [];

  const payload: Specialist = {
    // Preserve the existing id (if any) so updates use PUT instead of POST.
    id: baseline.value?.id,
    name: draft.name.trim(),
    description: (draft.description || "").trim() || undefined,
    provider: (draft.provider || "").trim(),
    model: (draft.model || "").trim(),
    baseURL: (baseURL || "").trim(),
    summaryContextWindowTokens: 0,
    enableTools,
    paused: !!draft.paused,
    allowTools: allow,
    system: draft.system,
    extraHeaders: extraHeadersObj.value,
    extraParams: extraParamsObj.value,
    groups: selectedGroups.value,
  };

  const summaryOverride = String(draft.summaryContextWindowTokens || "").trim();
  if (summaryOverride) {
    const parsed = Number(summaryOverride);
    if (Number.isFinite(parsed) && Number.isInteger(parsed) && parsed > 0) {
      payload.summaryContextWindowTokens = parsed;
    }
  }

  const nextKey = credentialDraft.value.trim();
  if (nextKey) {
    payload.apiKey = nextKey;
  }

  return payload;
}

function focusFirstInvalid() {
  const order: Array<{ field: string; tab: TabId; el: string }> = [
    { field: "name", tab: "basics", el: "sp-name" },
    { field: "provider", tab: "basics", el: "sp-provider" },
    { field: "model", tab: "basics", el: "sp-model" },
    { field: "baseURL", tab: "basics", el: "sp-baseurl" },
  ];

  for (const item of order) {
    if (fieldErrors.value[item.field]) {
      activeTab.value = item.tab;
      nextTick(() => {
        const target = document.getElementById(item.el) as HTMLElement | null;
        target?.focus();
      });
      return;
    }
  }
}

async function onSave() {
  submitAttempted.value = true;
  actionError.value = null;
  successMsg.value = null;

  // Keep objects in sync with table editors.
  extraHeadersObj.value = rowsToHeaders(extraHeadersRows.value);
  extraParamsObj.value = rowsToParams(extraParamsRows.value);

  if (!isValid.value) {
    focusFirstInvalid();
    return;
  }

  try {
    saving.value = true;
    const saved = await upsertSpecialist(buildPayloadFromDraft());

    // Re-initialize both baseline and draft from the saved data to ensure sync.
    // This also resets error/dirty tracking; set the success message after.
    initFromInitial(saved);
    successMsg.value = "Saved.";

    await qc.invalidateQueries({ queryKey: ["specialists"] });
    await qc.invalidateQueries({ queryKey: ["agent-status"] });

    emit("saved", saved);
  } catch (e: any) {
    const msg = e?.response?.data || e?.message || "Failed to save specialist.";
    actionError.value = String(msg);
  } finally {
    saving.value = false;
  }
}

function onCancel() {
  if (isDirty.value) {
    const ok = confirm("Discard unsaved changes?");
    if (!ok) return;
  }
  emit("cancel");
}

function onTabKeydown(e: KeyboardEvent, id: TabId) {
  if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
  e.preventDefault();
  const idx = tabs.findIndex((t) => t.id === id);
  const delta = e.key === "ArrowRight" ? 1 : -1;
  const next = (idx + delta + tabs.length) % tabs.length;
  activeTab.value = tabs[next].id;
}

function onProviderChange() {
  touch("provider");
  const defaults = providerDefaultsForSelected.value;
  if (defaults) {
    if (!draft.model.trim()) draft.model = defaults.model || "";
    if (draft.useDefaultEndpoint) {
      draft.customBaseURL = "";
    }
  }
}

async function ensurePromptsLoaded() {
  if (availablePrompts.value.length > 0 || promptsLoading.value) return;
  try {
    promptsLoading.value = true;
    availablePrompts.value = await listPrompts();
  } catch (err: any) {
    applyVersionError.value = err?.message || "Failed to load prompts.";
  } finally {
    promptsLoading.value = false;
  }
}

async function onSelectPrompt() {
  promptApply.value.versionId = "";
  availableVersions.value = [];
  applyVersionError.value = null;
  if (!promptApply.value.promptId) return;
  try {
    versionsLoading.value = true;
    availableVersions.value = await listPromptVersions(
      promptApply.value.promptId,
    );
  } catch (err: any) {
    applyVersionError.value = err?.message || "Failed to load versions.";
  } finally {
    versionsLoading.value = false;
  }
}

const promptDropdownOptions = computed(() => [
  {
    id: "",
    label: promptsLoading.value ? "Loading…" : "Select prompt",
    value: "",
  },
  ...availablePrompts.value.map((p) => ({
    id: p.id,
    label: p.name,
    value: p.id,
  })),
]);

const versionDropdownOptions = computed(() => [
  {
    id: "",
    label: versionsLoading.value ? "Loading…" : "Select version",
    value: "",
  },
  ...availableVersions.value.map((v) => ({
    id: v.id,
    label: v.semver || formatDate(v.createdAt),
    value: v.id,
  })),
]);

function formatDate(value?: string) {
  if (!value) return "—";
  try {
    return new Date(value).toLocaleString();
  } catch {
    return value;
  }
}

function applySelectedVersion() {
  applyVersionError.value = null;
  const vid = promptApply.value.versionId;
  if (!vid) return;
  const v = availableVersions.value.find((x) => x.id === vid);
  if (!v) {
    applyVersionError.value = "Prompt version not found.";
    return;
  }

  if (isDirty.value && draft.system.trim()) {
    const ok = confirm(
      "Apply this version and overwrite current prompt edits?",
    );
    if (!ok) return;
  }

  draft.system = v.template || "";
  touch("system");
  if (!draft.system.trim()) {
    applyVersionError.value = "Selected prompt version has an empty template.";
  }
}

async function loadTools() {
  if (toolsLoading.value) return;
  toolsLoading.value = true;
  toolsError.value = "";
  try {
    const resp = await fetchWarppTools().catch(() => [] as WarppTool[]);
    tools.value = resp
      .filter((t) => !!t?.name)
      .sort((a, b) =>
        a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
      );
  } catch (err: any) {
    toolsError.value = err?.message ?? "Failed to load tools";
  } finally {
    toolsLoading.value = false;
  }
}

function setToolAllowed(name: string, allowed: boolean) {
  const toolName = (name || "").trim();
  if (!toolName) return;

  const existing = allowToolsSet.value;
  const next = new Set(existing);
  if (allowed) next.add(toolName);
  else next.delete(toolName);
  allowTools.value = Array.from(next).sort((a, b) =>
    a.localeCompare(b, undefined, { sensitivity: "base" }),
  );
}

function setGroupSelected(name: string, selected: boolean) {
  const groupName = (name || "").trim();
  if (!groupName) return;
  const next = new Set(selectedGroups.value);
  if (selected) next.add(groupName);
  else next.delete(groupName);
  selectedGroups.value = Array.from(next).sort((a, b) =>
    a.localeCompare(b, undefined, { sensitivity: "base" }),
  );
}

function openCredentialModal() {
  credRestoreFocusEl.value = document.activeElement as HTMLElement | null;
  credentialDraft.value = "";
  showCredentialModal.value = true;
  nextTick(() => (credInput.value || credCloseBtn.value)?.focus());
}

function closeCredentialModal(apply: boolean) {
  if (apply) {
    // credentialDraft is applied at save time; do not mutate other draft state.
    // Mark dirty indicator via credentialDraft computed.
  } else {
    credentialDraft.value = "";
  }
  showCredentialModal.value = false;
  nextTick(() => credRestoreFocusEl.value?.focus());
}

function credFocusables(): HTMLElement[] {
  const root = credPanel.value;
  if (!root) return [];
  return Array.from(
    root.querySelectorAll<HTMLElement>(
      'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => !el.hasAttribute("disabled") && el.tabIndex !== -1);
}

function onCredentialKeydown(e: KeyboardEvent) {
  if (!showCredentialModal.value) return;
  if (e.key === "Escape") {
    e.preventDefault();
    closeCredentialModal(false);
    return;
  }
  if (e.key !== "Tab") return;

  const els = credFocusables();
  if (!els.length) return;
  const first = els[0];
  const last = els[els.length - 1];
  const active = document.activeElement as HTMLElement | null;

  if (e.shiftKey) {
    if (active === first || !credContains(active)) {
      e.preventDefault();
      last.focus();
    }
  } else {
    if (active === last || !credContains(active)) {
      e.preventDefault();
      first.focus();
    }
  }
}

function credContains(el: HTMLElement | null): boolean {
  return !!(credPanel.value && el && credPanel.value.contains(el));
}

const jsonModalTitle = computed(() =>
  jsonTarget.value === "headers"
    ? "Edit extra headers as JSON"
    : "Edit extra params as JSON",
);
const jsonModalSubtitle = computed(() =>
  jsonTarget.value === "headers"
    ? "Must be a JSON object of string values."
    : "Must be a JSON object.",
);

const jsonModalText = computed(() => {
  const obj =
    jsonTarget.value === "headers"
      ? extraHeadersObj.value
      : extraParamsObj.value;
  return JSON.stringify(obj || {}, null, 2);
});

function openJsonModal(target: "headers" | "params") {
  jsonTarget.value = target;
  showJsonModal.value = true;
}

function closeJsonModal() {
  showJsonModal.value = false;
}

function applyJson(obj: any) {
  if (!obj || typeof obj !== "object" || Array.isArray(obj)) {
    actionError.value = "JSON must be an object.";
    return;
  }

  if (jsonTarget.value === "headers") {
    const headers: Record<string, string> = {};
    for (const [k, v] of Object.entries(obj)) {
      headers[String(k)] = typeof v === "string" ? v : JSON.stringify(v);
    }
    extraHeadersObj.value = headers;
    extraHeadersRows.value = objectToRows(headers);
  } else {
    extraParamsObj.value = obj;
    extraParamsRows.value = objectToRows(obj);
  }

  showJsonModal.value = false;
}

function initFromInitial(sp: Specialist) {
  const normalized = normalizePayload(sp);
  baseline.value = normalizePayload(sp);
  nameLockedAfterSave.value = !!props.lockName;

  draft.name = normalized.name || "";
  draft.description = normalized.description || "";
  draft.provider = normalized.provider || props.providerOptions[0] || "";
  draft.model = normalized.model || "";
  draft.paused = !!normalized.paused;
  draft.system = normalized.system || "";
  draft.summaryContextWindowTokens = normalized.summaryContextWindowTokens
    ? String(normalized.summaryContextWindowTokens)
    : "";

  // endpoint defaults
  const defaults = props.providerDefaults?.[draft.provider];
  const defaultUrl = defaults?.baseURL || "";
  if (
    !normalized.baseURL ||
    (defaultUrl && normalized.baseURL === defaultUrl)
  ) {
    draft.useDefaultEndpoint = true;
    draft.customBaseURL = "";
  } else {
    draft.useDefaultEndpoint = false;
    draft.customBaseURL = normalized.baseURL;
  }

  // tools policy
  if (!normalized.enableTools) {
    draft.toolPolicy = "none";
    allowTools.value = [];
  } else if (normalized.allowTools && normalized.allowTools.length) {
    draft.toolPolicy = "allow-list";
    allowTools.value = [...normalized.allowTools];
  } else {
    draft.toolPolicy = "any";
    allowTools.value = [];
  }

  selectedGroups.value = Array.isArray(normalized.groups)
    ? [...normalized.groups]
    : [];

  // advanced
  extraHeadersObj.value = normalized.extraHeaders || {};
  extraParamsObj.value = normalized.extraParams || {};
  extraHeadersRows.value = objectToRows(extraHeadersObj.value);
  extraParamsRows.value = objectToRows(extraParamsObj.value);

  // never preload secret into the draft
  credentialDraft.value = "";

  touched.value = new Set();
  submitAttempted.value = false;
  actionError.value = null;
  successMsg.value = null;
}

watch(
  () => props.initial,
  (sp) => {
    if (!sp) return;
    initFromInitial({ ...sp, apiKey: "" });
  },
  { immediate: true },
);

watch(
  () => draft.useDefaultEndpoint,
  (useDefault) => {
    if (!useDefault && !draft.customBaseURL.trim()) {
      draft.customBaseURL = defaultBaseURL.value || "";
    }
    if (useDefault) {
      draft.customBaseURL = "";
    }
  },
);

watch(
  () => extraHeadersRows.value,
  (rows) => {
    extraHeadersObj.value = rowsToHeaders(rows);
  },
  { deep: true },
);

watch(
  () => extraParamsRows.value,
  (rows) => {
    // Table editor is string-first.
    extraParamsObj.value = rowsToParams(rows);
  },
  { deep: true },
);

watch(
  () => draft.toolPolicy,
  (policy) => {
    if (policy !== "allow-list") {
      allowTools.value = [];
    }
  },
);

onMounted(() => {
  void ensurePromptsLoaded();
  void loadTools();
});
</script>
