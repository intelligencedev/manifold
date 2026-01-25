<template>
  <div class="flex h-full min-h-0 flex-col overflow-hidden">
    <div class="sticky top-0 z-10 border-b border-border/50 bg-surface/90 backdrop-blur-sm">
      <div class="flex items-start justify-between gap-3 px-4 pb-3 pt-4">
        <div class="min-w-0">
          <h2 class="text-base font-semibold text-foreground">
            {{ headerTitle }}
          </h2>
          <p v-if="headerSubtitle" class="mt-0.5 text-xs text-subtle-foreground">
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

      <div role="tablist" aria-label="Edit Group" class="flex flex-wrap gap-2 px-4 pb-3">
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
        >
          {{ t.label }}
        </button>
      </div>
    </div>

    <div class="flex flex-1 min-h-0 flex-col overflow-auto px-4 pb-6 pt-4 scrollbar-inset">
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

      <div
        v-show="activeTab === 'details'"
        role="tabpanel"
        :id="'panel-details'"
        :aria-labelledby="'tab-details'"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <FormSection
          title="Group Identity"
          helper="Teams collect specialists and have a dedicated orchestrator instance."
        >
          <div class="flex flex-col gap-3">
            <div class="flex flex-col gap-1">
              <label
                for="group-name"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Name</label
              >
              <input
                id="group-name"
                v-model.trim="draft.name"
                class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                :disabled="lockName"
              />
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="group-description"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Description</label
              >
              <textarea
                id="group-description"
                v-model="draft.description"
                rows="4"
                class="w-full resize-y overflow-auto rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
              ></textarea>
            </div>
          </div>
        </FormSection>
      </div>

      <div
        v-show="activeTab === 'orchestrator'"
        role="tabpanel"
        :id="'panel-orchestrator'"
        :aria-labelledby="'tab-orchestrator'"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <FormSection
          title="Orchestrator"
          helper="Each group has a dedicated orchestrator configuration."
        >
          <div class="flex flex-col gap-3">
            <div class="rounded border border-border/60 bg-surface-muted/20 px-3 py-2 text-sm text-subtle-foreground">
              Orchestrator name: <span class="font-semibold text-foreground">{{ orchestratorName }}</span>
            </div>
            <div class="grid gap-3 md:grid-cols-2">
              <div class="flex flex-col gap-1">
                <label
                  for="group-orch-provider"
                  class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                  >Provider</label
                >
                <DropdownSelect
                  id="group-orch-provider"
                  v-model="orchestratorDraft.provider"
                  :options="providerDropdownOptions"
                  class="w-full text-sm"
                  @update:modelValue="applyProviderDefaults"
                />
              </div>
              <div class="flex flex-col gap-1">
                <label
                  for="group-orch-model"
                  class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                  >Model</label
                >
                <input
                  id="group-orch-model"
                  v-model.trim="orchestratorDraft.model"
                  class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                />
              </div>
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="group-orch-baseurl"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Base URL</label
              >
              <input
                id="group-orch-baseurl"
                v-model.trim="orchestratorDraft.baseURL"
                class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                placeholder="https://…"
              />
            </div>
            <label class="inline-flex items-center justify-between gap-3 rounded border border-border/60 bg-surface-muted/20 px-3 py-2">
              <span class="text-sm text-foreground">Enable tools</span>
              <input v-model="orchestratorDraft.enableTools" type="checkbox" class="h-4 w-4" />
            </label>
            <div class="flex flex-col gap-1">
              <label
                for="group-orch-allow"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Allowed tools (comma separated)</label
              >
              <input
                id="group-orch-allow"
                v-model.trim="orchestratorDraft.allowToolsText"
                class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                placeholder="web.search, files.read, ..."
              />
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="group-orch-system"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >System prompt</label
              >
              <textarea
                id="group-orch-system"
                v-model="orchestratorDraft.system"
                rows="6"
                class="w-full resize-y overflow-auto rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
              ></textarea>
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="group-orch-apikey"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >API Key (optional)</label
              >
              <input
                id="group-orch-apikey"
                v-model.trim="orchestratorDraft.apiKey"
                type="password"
                autocomplete="off"
                class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                placeholder="Override provider API key"
              />
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="group-orch-summary-ctx"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Summary context window (tokens)</label
              >
              <input
                id="group-orch-summary-ctx"
                v-model.number="orchestratorDraft.summaryContextWindowTokens"
                type="number"
                min="1"
                step="1"
                class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                placeholder="Use global default"
              />
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="group-orch-extra-headers"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Extra headers (JSON)</label
              >
              <textarea
                id="group-orch-extra-headers"
                v-model="orchestratorDraft.extraHeadersJson"
                rows="3"
                class="w-full resize-y overflow-auto rounded border border-border/60 bg-surface-muted/40 px-3 py-2 font-mono text-sm"
                placeholder='{}'
              ></textarea>
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="group-orch-extra-params"
                class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
                >Extra params (JSON)</label
              >
              <textarea
                id="group-orch-extra-params"
                v-model="orchestratorDraft.extraParamsJson"
                rows="3"
                class="w-full resize-y overflow-auto rounded border border-border/60 bg-surface-muted/40 px-3 py-2 font-mono text-sm"
                placeholder='{}'
              ></textarea>
            </div>
          </div>
        </FormSection>
      </div>

      <div
        v-show="activeTab === 'members'"
        role="tabpanel"
        :id="'panel-members'"
        :aria-labelledby="'tab-members'"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <FormSection
          title="Members"
          helper="Specialists can belong to multiple groups."
        >
          <div class="flex flex-col gap-3">
            <input
              v-model="memberSearch"
              type="text"
              placeholder="Filter specialists"
              class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
            />
            <div class="rounded-lg border border-border/60 bg-surface">
              <div v-if="!filteredMembers.length" class="px-3 py-3 text-sm text-subtle-foreground">
                No specialists match your search.
              </div>
              <label
                v-for="name in filteredMembers"
                :key="name"
                class="flex cursor-pointer items-start gap-3 border-t border-border/40 px-3 py-2 transition-colors first:border-t-0 hover:bg-surface-muted/40"
              >
                <input
                  class="mt-1 h-4 w-4 shrink-0"
                  type="checkbox"
                  :checked="selectedMembers.has(name)"
                  @change="toggleMember(name, ($event.target as HTMLInputElement).checked)"
                />
                <div class="min-w-0">
                  <p class="text-sm font-medium text-foreground">{{ name }}</p>
                </div>
              </label>
            </div>
          </div>
        </FormSection>
      </div>
    </div>

    <div class="sticky bottom-0 z-10 border-t border-border/50 bg-surface/90 backdrop-blur-sm">
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
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import FormSection from "@/components/specialists/edit/FormSection.vue";
import {
  type Specialist,
  type SpecialistGroup,
  type SpecialistProviderDefaults,
  upsertGroup,
} from "@/api/client";

const props = withDefaults(
  defineProps<{
    initial: SpecialistGroup;
    lockName?: boolean;
    providerDefaults?: Record<string, SpecialistProviderDefaults>;
    providerOptions: string[];
    availableSpecialists: string[];
  }>(),
  { lockName: false },
);

const emit = defineEmits<{ saved: [SpecialistGroup]; cancel: [] }>();

type TabId = "details" | "orchestrator" | "members";

const tabs = [
  { id: "details" as const, label: "Details" },
  { id: "orchestrator" as const, label: "Orchestrator" },
  { id: "members" as const, label: "Members" },
];

const activeTab = ref<TabId>("details");
const saving = ref(false);
const actionError = ref<string | null>(null);
const successMsg = ref<string | null>(null);

const draft = reactive({
  name: "",
  description: "",
});

const orchestratorDraft = reactive({
  provider: "",
  model: "",
  baseURL: "",
  apiKey: "",
  enableTools: false,
  allowToolsText: "",
  system: "",
  summaryContextWindowTokens: null as number | null,
  extraHeadersJson: "{}",
  extraParamsJson: "{}",
});

const baseline = ref<SpecialistGroup | null>(null);

const selectedMembers = ref(new Set<string>());
const memberSearch = ref("");

const providerDropdownOptions = computed(() =>
  props.providerOptions.map((opt) => ({ id: opt, label: opt, value: opt })),
);

const defaultProvider = computed(() => props.providerOptions[0] || "openai");

const headerTitle = computed(() =>
  baseline.value?.name ? "Edit Group" : "Create Group",
);

const headerSubtitle = computed(() =>
  baseline.value?.name
    ? "Update the group definition and orchestrator."
    : "Create a new team and configure its orchestrator.",
);

const lockName = computed(() => !!props.lockName);

const orchestratorName = computed(() => `${draft.name || "group"}-orchestrator`);

const filteredMembers = computed(() => {
  const q = memberSearch.value.trim().toLowerCase();
  const list = props.availableSpecialists || [];
  if (!q) return list;
  return list.filter((name) => name.toLowerCase().includes(q));
});

function normalizeAllowTools(value: string): string[] {
  return value
    .split(",")
    .map((v) => v.trim())
    .filter((v) => v);
}

function parseJsonSafe<T>(json: string, fallback: T): T {
  try {
    return JSON.parse(json) || fallback;
  } catch {
    return fallback;
  }
}

const isDirty = computed(() => {
  if (!baseline.value) return true;

  // Compare group fields
  if (draft.name.trim() !== (baseline.value.name || "")) return true;
  if (draft.description !== (baseline.value.description || "")) return true;

  // Compare members
  const baselineMembers = new Set(baseline.value.members || []);
  if (selectedMembers.value.size !== baselineMembers.size) return true;
  for (const m of selectedMembers.value) {
    if (!baselineMembers.has(m)) return true;
  }

  // Compare orchestrator fields
  const orch = baseline.value.orchestrator || ({} as Specialist);
  if ((orchestratorDraft.provider || defaultProvider.value) !== (orch.provider || defaultProvider.value)) return true;
  if ((orchestratorDraft.model || "") !== (orch.model || "")) return true;
  if ((orchestratorDraft.baseURL || "") !== (orch.baseURL || "")) return true;
  if ((orchestratorDraft.apiKey || "") !== (orch.apiKey || "")) return true;
  if (orchestratorDraft.enableTools !== !!orch.enableTools) return true;
  if ((orchestratorDraft.system || "") !== (orch.system || "")) return true;
  if ((orchestratorDraft.summaryContextWindowTokens ?? null) !== (orch.summaryContextWindowTokens ?? null)) return true;

  // Compare allowTools
  const currentAllowTools = normalizeAllowTools(orchestratorDraft.allowToolsText).sort();
  const baselineAllowTools = (orch.allowTools || []).slice().sort();
  if (JSON.stringify(currentAllowTools) !== JSON.stringify(baselineAllowTools)) return true;

  // Compare extraHeaders and extraParams
  const currentHeaders = parseJsonSafe(orchestratorDraft.extraHeadersJson, {});
  const baselineHeaders = orch.extraHeaders || {};
  if (JSON.stringify(currentHeaders) !== JSON.stringify(baselineHeaders)) return true;

  const currentParams = parseJsonSafe(orchestratorDraft.extraParamsJson, {});
  const baselineParams = orch.extraParams || {};
  if (JSON.stringify(currentParams) !== JSON.stringify(baselineParams)) return true;

  return false;
});

function initFromInitial(group: SpecialistGroup) {
  baseline.value = group;
  draft.name = group.name || "";
  draft.description = group.description || "";

  const orch = group.orchestrator || ({} as Specialist);
  orchestratorDraft.provider = orch.provider || defaultProvider.value;
  orchestratorDraft.model = orch.model || "";
  orchestratorDraft.baseURL = orch.baseURL || "";
  orchestratorDraft.apiKey = orch.apiKey || "";
  orchestratorDraft.enableTools = !!orch.enableTools;
  orchestratorDraft.allowToolsText = (orch.allowTools || []).join(", ");
  orchestratorDraft.system = orch.system || "";
  orchestratorDraft.summaryContextWindowTokens = orch.summaryContextWindowTokens ?? null;
  orchestratorDraft.extraHeadersJson = JSON.stringify(orch.extraHeaders || {}, null, 2);
  orchestratorDraft.extraParamsJson = JSON.stringify(orch.extraParams || {}, null, 2);

  applyProviderDefaults();

  selectedMembers.value = new Set(group.members || []);
  actionError.value = null;
  successMsg.value = null;
}

function applyProviderDefaults() {
  const defaults = props.providerDefaults?.[orchestratorDraft.provider];
  if (!defaults) return;
  if (!orchestratorDraft.model) orchestratorDraft.model = defaults.model || "";
  if (!orchestratorDraft.baseURL) orchestratorDraft.baseURL = defaults.baseURL || "";
}

function toggleMember(name: string, enabled: boolean) {
  const next = new Set(selectedMembers.value);
  if (enabled) next.add(name);
  else next.delete(name);
  selectedMembers.value = next;
}

function buildPayload(): SpecialistGroup {
  const baseOrch = baseline.value?.orchestrator;
  const orchestrator: Specialist = {
    // Preserve existing id from backend
    ...(baseOrch?.id ? { id: baseOrch.id } : {}),
    name: orchestratorName.value,
    provider: orchestratorDraft.provider || defaultProvider.value,
    model: orchestratorDraft.model || "",
    baseURL: orchestratorDraft.baseURL || "",
    apiKey: orchestratorDraft.apiKey || "",
    enableTools: orchestratorDraft.enableTools,
    paused: false,
    allowTools: normalizeAllowTools(orchestratorDraft.allowToolsText),
    system: orchestratorDraft.system || "",
    description: `Group orchestrator for ${draft.name || "group"}`,
    summaryContextWindowTokens: orchestratorDraft.summaryContextWindowTokens ?? undefined,
    extraHeaders: parseJsonSafe(orchestratorDraft.extraHeadersJson, {}),
    extraParams: parseJsonSafe(orchestratorDraft.extraParamsJson, {}),
  };

  return {
    id: baseline.value?.id,
    name: draft.name.trim(),
    description: draft.description || "",
    orchestrator,
    members: Array.from(selectedMembers.value).sort((a, b) =>
      a.localeCompare(b, undefined, { sensitivity: "base" }),
    ),
  };
}

async function onSave() {
  actionError.value = null;
  successMsg.value = null;
  if (!draft.name.trim()) {
    actionError.value = "Group name is required.";
    return;
  }
  try {
    saving.value = true;
    const payload = buildPayload();
    const saved = await upsertGroup(payload);
    initFromInitial(saved);
    successMsg.value = "Saved.";
    emit("saved", saved);
  } catch (e: any) {
    const msg = e?.response?.data || e?.message || "Failed to save group.";
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

watch(
  () => props.initial,
  (g) => {
    if (!g) return;
    initFromInitial(g);
  },
  { immediate: true },
);
</script>
