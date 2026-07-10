<template>
  <section class="flex flex-col h-full min-h-0">
    <div
      v-if="actionError"
      class="rounded-lg border border-danger/60 bg-danger/10 p-3 text-danger-foreground text-sm"
    >
      {{ actionError }}
    </div>

    <!-- list/edit layout; nested areas scroll but view itself doesn't -->
    <div class="flex min-h-0 flex-1 flex-row gap-6">
      <!-- left: card grid -->
      <div
        class="scrollbar-inset min-h-0 min-w-0 w-1/2 overflow-auto border-r border-border/60 pl-1 pr-6"
      >
        <div class="mb-4 flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold">Specialists & Teams</h2>
          <div class="flex items-center gap-2">
            <button
              @click="startCreateTeam"
              class="rounded-md border border-accent/50 px-3 py-1.5 text-xs font-semibold text-accent transition hover:bg-accent/10"
            >
              New team
            </button>
            <button
              @click="startCreate"
              class="rounded-md border border-border px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
            >
              New specialist
            </button>
          </div>
        </div>

        <div
          role="tablist"
          aria-label="Specialists and teams"
          class="mb-4 flex items-center gap-2"
        >
          <button
            role="tab"
            :aria-selected="activeListTab === 'specialists' ? 'true' : 'false'"
            :tabindex="activeListTab === 'specialists' ? 0 : -1"
            type="button"
            class="rounded-md border px-3 py-1.5 text-xs font-semibold transition"
            :class="
              activeListTab === 'specialists'
                ? 'border-border/80 bg-surface-muted/60 text-foreground'
                : 'border-border/50 text-subtle-foreground hover:border-border'
            "
            @click="activeListTab = 'specialists'"
          >
            Specialists
            <span class="ml-1 text-[11px] text-subtle-foreground">{{
              filteredSpecialists.length
            }}</span>
          </button>
          <button
            role="tab"
            :aria-selected="activeListTab === 'teams' ? 'true' : 'false'"
            :tabindex="activeListTab === 'teams' ? 0 : -1"
            type="button"
            class="rounded-md border px-3 py-1.5 text-xs font-semibold transition"
            :class="
              activeListTab === 'teams'
                ? 'border-border/80 bg-surface-muted/60 text-foreground'
                : 'border-border/50 text-subtle-foreground hover:border-border'
            "
            @click="activeListTab = 'teams'"
          >
            Teams
            <span class="ml-1 text-[11px] text-subtle-foreground">{{
              teams.length
            }}</span>
          </button>
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search…"
            class="w-40 rounded-md border border-border/60 bg-surface-muted/30 px-3 py-1.5 text-xs text-foreground placeholder:text-faint-foreground outline-none transition focus:border-accent/60 focus:ring-1 focus:ring-accent/40"
          />
        </div>

        <div v-show="activeListTab === 'teams'" role="tabpanel" class="mb-6">
          <div class="mb-2 flex items-center justify-between">
            <h3 class="text-sm font-semibold text-foreground">Teams</h3>
          </div>
          <div
            v-if="teamsLoading"
            class="rounded-lg border border-border/60 bg-surface-muted/20 p-4 text-sm text-faint-foreground"
          >
            Loading teams…
          </div>
          <div
            v-else-if="teamsError"
            class="rounded-lg border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground"
          >
            Failed to load teams.
          </div>
          <div
            v-else-if="!teams.length"
            class="rounded-lg border border-border/60 bg-surface-muted/20 p-4 text-sm text-faint-foreground"
          >
            No teams configured yet.
          </div>
          <div v-else class="flex flex-col gap-3">
            <GlassCard
              v-for="t in teams"
              :key="t.name"
              class="flex flex-col p-4 transition-all duration-200 cursor-pointer"
              :class="{
                'ring-2 ring-accent/60 ring-offset-2 ring-offset-surface':
                  isCurrentlyEditingTeam(t.name),
              }"
              interactive
              @click="editTeam(t)"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <h3 class="text-base font-semibold text-foreground">
                    {{ t.name }}
                  </h3>
                  <p
                    class="mt-1 text-[11px] font-mono uppercase tracking-[0.12em] text-faint-foreground"
                  >
                    Orchestrator
                  </p>
                  <p class="text-sm text-muted-foreground">
                    {{ teamOrchestratorLabel(t) }}
                  </p>
                </div>
                <Pill
                  :tone="t.orchestratorName ? 'accent' : 'danger'"
                  size="sm"
                >
                  {{ t.orchestratorName ? "Team" : "Needs setup" }}
                </Pill>
              </div>
              <p class="mt-3 text-sm text-subtle-foreground">
                {{ t.description || "No description provided yet." }}
              </p>
              <div
                class="mt-3 flex items-center gap-2 text-xs text-subtle-foreground"
              >
                <span
                  class="inline-flex items-center rounded-md border border-border bg-surface-muted/50 px-2 py-1 font-medium"
                  >Members · {{ t.members?.length || 0 }}</span
                >
              </div>
              <div class="mt-3 flex flex-wrap gap-2" @click.stop>
                <button
                  type="button"
                  @click="editTeam(t)"
                  class="rounded-md border border-border px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
                >
                  Edit
                </button>
                <button
                  type="button"
                  @click="removeTeam(t)"
                  class="rounded-md border border-danger/60 px-3 py-1.5 text-xs font-semibold text-danger/80 transition hover:bg-danger/10"
                >
                  Delete
                </button>
              </div>
            </GlassCard>
          </div>
        </div>

        <div v-show="activeListTab === 'specialists'" role="tabpanel">
          <div class="mb-3 flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="rounded-md border px-3 py-1 text-xs font-semibold transition"
              :class="
                teamFilter === 'all'
                  ? 'border-border/80 bg-surface-muted/60 text-foreground'
                  : 'border-border/50 text-subtle-foreground hover:border-border'
              "
              @click="teamFilter = 'all'"
            >
              All
            </button>
            <button
              type="button"
              class="rounded-md border px-3 py-1 text-xs font-semibold transition"
              :class="
                teamFilter === 'unassigned'
                  ? 'border-border/80 bg-surface-muted/60 text-foreground'
                  : 'border-border/50 text-subtle-foreground hover:border-border'
              "
              @click="teamFilter = 'unassigned'"
            >
              Unassigned
            </button>
            <button
              v-for="t in teams"
              :key="`filter-${t.name}`"
              type="button"
              class="rounded-md border px-3 py-1 text-xs font-semibold transition"
              :class="
                teamFilter === t.name
                  ? 'border-border/80 bg-surface-muted/60 text-foreground'
                  : 'border-border/50 text-subtle-foreground hover:border-border'
              "
              @click="teamFilter = t.name"
            >
              {{ t.name }}
            </button>
          </div>

          <div
            v-if="loading"
            class="rounded-lg border border-border/60 bg-surface-muted/20 p-4 text-sm text-faint-foreground"
          >
            Loading specialists…
          </div>
          <div
            v-else-if="error"
            class="rounded-lg border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground"
          >
            Failed to load specialists.
          </div>
          <div
            v-else-if="!filteredSpecialists.length"
            class="rounded-lg border border-border/60 bg-surface-muted/20 p-4 text-sm text-faint-foreground"
          >
            No specialists match this filter.
          </div>
          <div v-else class="grid grid-cols-2 gap-4">
            <GlassCard
              v-for="s in filteredSpecialists"
              :key="s.name"
              class="flex flex-col p-4 transition-all duration-200 cursor-pointer"
              :class="{
                'ring-2 ring-accent/60 ring-offset-2 ring-offset-surface':
                  isCurrentlyEditing(s.name),
              }"
              interactive
              @click="edit(s)"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <h3 class="text-base font-semibold text-foreground">
                    {{ s.name }}
                  </h3>
                  <p
                    class="mt-1 text-[11px] font-mono uppercase tracking-[0.12em] text-faint-foreground"
                  >
                    Model
                  </p>
                  <p class="text-sm text-muted-foreground">
                    {{ s.model || "—" }}
                  </p>
                </div>
                <div class="flex items-center gap-2">
                  <span
                    v-if="isCurrentlyEditing(s.name)"
                    class="rounded-md border border-accent/50 bg-accent/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-accent"
                  >
                    Editing
                  </span>
                  <Pill
                    :tone="s.paused ? 'danger' : 'success'"
                    size="sm"
                    :glow="!s.paused"
                  >
                    {{ s.paused ? "Paused" : "Active" }}
                  </Pill>
                </div>
              </div>

              <p
                class="mt-4 text-sm leading-relaxed text-subtle-foreground line-clamp-4"
              >
                {{ specialistDescription(s) }}
              </p>

              <div class="mt-3 flex flex-wrap gap-2 text-xs">
                <Pill
                  v-if="!s.teams || !s.teams.length"
                  tone="neutral"
                  size="sm"
                  >Unassigned</Pill
                >
                <Pill
                  v-for="t in s.teams || []"
                  :key="`${s.name}-${t}`"
                  tone="neutral"
                  size="sm"
                >
                  {{ t }}
                </Pill>
              </div>

              <div class="flex-grow"></div>

              <div class="mt-4 flex flex-wrap items-center gap-2 text-xs">
                <Pill :tone="s.enableTools ? 'accent' : 'neutral'" size="sm">
                  {{ s.enableTools ? "Tools enabled" : "Tools disabled" }}
                </Pill>
                <Pill v-if="s.imageGeneration" tone="accent" size="sm">
                  Image generation
                </Pill>
                <Pill v-if="s.videoGeneration" tone="accent" size="sm">
                  Video generation
                </Pill>
                <span
                  v-if="typeof s.autoDiscover === 'boolean'"
                  class="inline-flex items-center rounded-md border border-border bg-surface-muted/50 px-2 py-1 font-medium text-subtle-foreground"
                >
                  {{
                    s.autoDiscover ? "Auto-discover on" : "Auto-discover off"
                  }}
                </span>
                <span
                  v-if="Array.isArray(s.allowTools) && s.allowTools.length > 0"
                  class="inline-flex items-center rounded-md border border-border bg-surface-muted/50 px-2 py-1 font-medium text-subtle-foreground"
                >
                  Allow list · {{ s.allowTools.length }}
                </span>
              </div>

              <div class="mt-4 flex flex-wrap gap-2" @click.stop>
                <button
                  type="button"
                  @click="edit(s)"
                  class="rounded-md border border-border px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
                >
                  Edit
                </button>
                <button
                  type="button"
                  @click="cloneSpecialist(s)"
                  class="rounded-md border border-border px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
                  title="Duplicate this specialist"
                >
                  Clone
                </button>
                <button
                  type="button"
                  @click="togglePause(s)"
                  class="inline-flex items-center gap-1 rounded-md border border-border px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
                  :title="s.paused ? 'Resume specialist' : 'Pause specialist'"
                  :aria-label="
                    s.paused ? 'Resume specialist' : 'Pause specialist'
                  "
                >
                  <SolarPlay v-if="s.paused" class="h-4 w-4" />
                  <SolarPause v-else class="h-4 w-4" />
                  <span>{{ s.paused ? "Resume" : "Pause" }}</span>
                </button>
                <button
                  type="button"
                  @click="remove(s)"
                  class="rounded-md border border-danger/60 px-3 py-1.5 text-xs font-semibold text-danger/80 transition hover:bg-danger/10"
                >
                  Delete
                </button>
              </div>
            </GlassCard>
          </div>
        </div>
      </div>

      <!-- right: editor -->
      <div class="min-h-0 min-w-0 w-1/2 pl-6">
        <GlassCard
          v-if="editingType === 'specialist'"
          flat
          :padded="false"
          class="h-full min-h-0 overflow-hidden"
        >
          <EditSpecialistRoot
            :key="editorInitial?.name || 'new'"
            class="h-full"
            :initial="editorInitial!"
            :lockName="editorLockName"
            :credentialConfigured="editorCredentialConfigured"
            :providerDefaults="providerDefaultsMap"
            :providerOptions="providerOptions"
            :availableTeams="teamNames"
            @cancel="closeEditor"
            @saved="onSaved"
          />
        </GlassCard>
        <GlassCard
          v-else-if="editingType === 'team'"
          flat
          :padded="false"
          class="h-full min-h-0 overflow-hidden"
        >
          <EditTeamRoot
            :key="teamEditorInitial?.name || 'new-team'"
            class="h-full"
            :initial="teamEditorInitial!"
            :lockName="teamEditorLockName"
            :availableSpecialists="specialists"
            @cancel="closeEditor"
            @saved="onTeamSaved"
          />
        </GlassCard>
        <GlassCard
          v-else
          flat
          class="flex h-full min-h-0 items-center justify-center text-sm text-subtle-foreground"
        >
          Select a specialist or team, or click New to create one.
        </GlassCard>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import { useQuery, useQueryClient } from "@tanstack/vue-query";
import {
  listSpecialists,
  upsertSpecialist,
  deleteSpecialist,
  listSpecialistDefaults,
  listTeams,
  deleteTeam,
  type Specialist,
  type SpecialistTeam,
  type SpecialistProviderDefaults,
} from "@/api/client";
import SolarPause from "@/components/icons/SolarPause.vue";
import SolarPlay from "@/components/icons/SolarPlay.vue";
import GlassCard from "@/components/ui/GlassCard.vue";
import Pill from "@/components/ui/Pill.vue";
import EditSpecialistRoot from "@/components/specialists/EditSpecialistRoot.vue";
import EditTeamRoot from "@/components/specialists/EditTeamRoot.vue";

const qc = useQueryClient();
const {
  data,
  isLoading: loading,
  isError: error,
} = useQuery({
  queryKey: ["specialists"],
  queryFn: listSpecialists,
  staleTime: 5_000,
});
const {
  data: teamsData,
  isLoading: teamsLoading,
  isError: teamsError,
} = useQuery({ queryKey: ["teams"], queryFn: listTeams, staleTime: 5_000 });
const { data: providerDefaultsData } = useQuery<
  Record<string, SpecialistProviderDefaults>
>({
  queryKey: ["specialist-defaults"],
  queryFn: listSpecialistDefaults,
  staleTime: 30_000,
});

const providerDefaultsMap = computed<
  Record<string, SpecialistProviderDefaults> | undefined
>(() => providerDefaultsData.value);
const teams = computed<SpecialistTeam[]>(() => teamsData.value ?? []);
const specialists = computed<Specialist[]>(() => {
  const list = data.value ?? [];
  const unique: Specialist[] = [];
  // Deduplicate by name so orchestrator-only config overlays don't render twice.
  for (const sp of list) {
    if (!sp?.name) {
      unique.push(sp);
      continue;
    }
    if (!unique.some((existing) => existing.name === sp.name)) {
      unique.push(sp);
    }
  }
  // Keep stable ordering from API but present alphabetically for UX.
  return [...unique].sort((a, b) =>
    a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
  );
});

const filteredSpecialists = computed<Specialist[]>(() => {
  let list = specialists.value;
  const query = searchQuery.value.trim().toLowerCase();
  if (query) {
    list = list.filter((sp) => sp.name.toLowerCase().includes(query));
  }
  const filter = teamFilter.value;
  if (filter === "all") return list;
  if (filter === "unassigned") {
    return list.filter((sp) => !sp.teams || sp.teams.length === 0);
  }
  return list.filter((sp) => sp.teams?.includes(filter));
});

const providerOptions = computed(() => {
  const defaults = providerDefaultsMap.value;
  if (defaults && typeof defaults === "object") {
    return Object.keys(defaults).sort();
  }
  return ["openai", "anthropic", "google", "local"];
});

const teamNames = computed(() =>
  teams.value
    .map((team) => team.name)
    .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" })),
);
const specialistsByName = computed(() => {
  const map = new Map<string, Specialist>();
  for (const sp of specialists.value) {
    const key = (sp.name || "").trim().toLowerCase();
    if (key) map.set(key, sp);
  }
  return map;
});

const activeListTab = ref<"specialists" | "teams">("specialists");
const teamFilter = ref<"all" | "unassigned" | string>("all");
const searchQuery = ref("");

const editingType = ref<"specialist" | "team" | null>(null);
const editorInitial = ref<Specialist | null>(null);
const editorLockName = ref(false);
const editorCredentialConfigured = ref(false);
const teamEditorInitial = ref<SpecialistTeam | null>(null);
const teamEditorLockName = ref(false);
const actionError = ref<string | null>(null);

// Track which specialist/team is currently being edited for visual feedback
const currentEditingName = computed(() =>
  editingType.value === "specialist" ? editorInitial.value?.name || null : null,
);
const currentEditingTeamName = computed(() =>
  editingType.value === "team" ? teamEditorInitial.value?.name || null : null,
);

function isCurrentlyEditing(name: string): boolean {
  return (
    editingType.value === "specialist" && editorInitial.value?.name === name
  );
}

function isCurrentlyEditingTeam(name: string): boolean {
  return editingType.value === "team" && teamEditorInitial.value?.name === name;
}

function statusBadgeClass(paused: boolean): string {
  return paused
    ? "inline-flex items-center rounded-md border border-border/60 bg-border/20 px-2 py-1 text-xs font-semibold text-subtle-foreground"
    : "inline-flex items-center rounded-md border border-success/40 bg-success/10 px-2 py-1 text-xs font-semibold text-success";
}

function toolsBadgeClass(enabled: boolean): string {
  return enabled
    ? "inline-flex items-center rounded-md border border-success/40 bg-success/10 px-2 py-1 font-medium text-success"
    : "inline-flex items-center rounded-md border border-border/50 bg-surface-muted/30 px-2 py-1 font-medium text-subtle-foreground";
}

function teamOrchestratorLabel(team: SpecialistTeam): string {
  const name = (team.orchestratorName || "").trim();
  if (!name) return "Required";
  const spec = specialistsByName.value.get(name.toLowerCase());
  const model = (spec?.model || "").trim();
  return model ? `${name} · ${model}` : name;
}

function specialistDescription(s: Specialist): string {
  const primary = (s.description ?? "").trim();
  if (primary) {
    return primary;
  }
  const systemSnippet = (s.system || "").trim();
  if (!systemSnippet) {
    return "No description provided yet.";
  }
  const condensed = systemSnippet.replace(/\s+/g, " ");
  return condensed.length > 180 ? `${condensed.slice(0, 177)}…` : condensed;
}

function setErr(e: unknown, fallback: string) {
  actionError.value = null;
  const anyErr = e as any;
  const msg = anyErr?.response?.data || anyErr?.message || fallback;
  actionError.value = String(msg);
}

// Bootstrap tool allow-list applied to newly created specialists. Keep in sync
// with config.DefaultAgentToolAllowList() in the Go backend.
const DEFAULT_AGENT_ALLOW_TOOLS = [
  "run_cli",
  "web_fetch",
  "transit_create",
  "transit_get",
  "transit_update",
  "transit_delete",
  "transit_search",
  "transit_discover",
  "transit_list_keys",
  "transit_list_recent",
];

function startCreate() {
  activeListTab.value = "specialists";
  const defaultProvider = providerOptions.value[0] || "openai";
  editorInitial.value = {
    name: "",
    description: "",
    provider: defaultProvider,
    model: "",
    baseURL: "",
    enableTools: true,
    requestInfoEnabled: true,
    imageGeneration: false,
    videoGeneration: false,
    autoDiscover: true,
    paused: false,
    system: "",
    allowTools: [...DEFAULT_AGENT_ALLOW_TOOLS],
    extraHeaders: {},
    extraParams: {},
  };
  editorLockName.value = false;
  editorCredentialConfigured.value = false;
  editingType.value = "specialist";
  teamEditorInitial.value = null;
  teamEditorLockName.value = false;
}
function edit(s: Specialist) {
  activeListTab.value = "specialists";
  // If already editing the same specialist, do nothing
  if (
    editingType.value === "specialist" &&
    editorInitial.value?.name === s.name
  ) {
    return;
  }
  editorInitial.value = {
    ...s,
    provider: s.provider || providerOptions.value[0] || "openai",
    description: s.description ?? "",
    apiKey: "",
  };
  editorLockName.value = true;
  editorCredentialConfigured.value = !!s.apiKey;
  editingType.value = "specialist";
  teamEditorInitial.value = null;
  teamEditorLockName.value = false;
}
function cloneSpecialist(s: Specialist) {
  activeListTab.value = "specialists";
  const baseName = `${s.name || "specialist"} copy`;
  const uniqueName = generateUniqueName(baseName);
  const clonedHeaders = { ...(s.extraHeaders || {}) };
  const clonedParams = s.extraParams
    ? JSON.parse(JSON.stringify(s.extraParams))
    : {};
  const clonedAllowTools = Array.isArray(s.allowTools)
    ? [...s.allowTools]
    : s.allowTools;
  editorInitial.value = {
    ...s,
    name: uniqueName,
    paused: true,
    description: s.description ?? "",
    apiKey: "",
    autoDiscover: s.autoDiscover === true,
    requestInfoEnabled: s.requestInfoEnabled !== false,
    imageGeneration: !!s.imageGeneration,
    videoGeneration: !!s.videoGeneration,
    allowTools: clonedAllowTools,
    extraHeaders: clonedHeaders,
    extraParams: clonedParams,
  };
  if (!editorInitial.value.provider) {
    editorInitial.value.provider = providerOptions.value[0] || "openai";
  }
  editorLockName.value = false;
  editorCredentialConfigured.value = false;
  editingType.value = "specialist";
  teamEditorInitial.value = null;
  teamEditorLockName.value = false;
}
function generateUniqueName(base: string) {
  const names = new Set((data.value ?? []).map((sp) => sp.name));
  if (!names.has(base)) {
    return base;
  }
  let suffix = 2;
  let candidate = `${base} ${suffix}`;
  while (names.has(candidate)) {
    suffix += 1;
    candidate = `${base} ${suffix}`;
  }
  return candidate;
}
function closeEditor() {
  editingType.value = null;
  editorInitial.value = null;
  editorLockName.value = false;
  editorCredentialConfigured.value = false;
  teamEditorInitial.value = null;
  teamEditorLockName.value = false;
}

function onSaved(saved: Specialist) {
  // Clear any previous errors
  actionError.value = null;
  activeListTab.value = "specialists";

  // Update the editor state to reflect the saved specialist
  // This keeps the editor showing the saved specialist with updated state
  editorInitial.value = {
    ...saved,
    provider: saved.provider || providerOptions.value[0] || "openai",
    description: saved.description ?? "",
    apiKey: "", // Never keep the secret in memory
  };
  editorLockName.value = true;
  editorCredentialConfigured.value = !!saved.apiKey;
  editingType.value = "specialist";
  void qc.invalidateQueries({ queryKey: ["teams"] });
}

function startCreateTeam() {
  activeListTab.value = "teams";
  teamEditorInitial.value = {
    name: "",
    description: "",
    orchestratorName: "",
    members: [],
    orchestrator: {} as Specialist,
  };
  teamEditorLockName.value = false;
  editingType.value = "team";
  editorInitial.value = null;
  editorLockName.value = false;
  editorCredentialConfigured.value = false;
}

function editTeam(team: SpecialistTeam) {
  activeListTab.value = "teams";
  if (
    editingType.value === "team" &&
    teamEditorInitial.value?.name === team.name
  ) {
    return;
  }
  teamEditorInitial.value = {
    ...team,
    description: team.description ?? "",
    orchestratorName: team.orchestratorName || "",
    members: team.members || [],
    orchestrator: team.orchestrator,
  };
  teamEditorLockName.value = true;
  editingType.value = "team";
  editorInitial.value = null;
  editorLockName.value = false;
  editorCredentialConfigured.value = false;
}

function onTeamSaved(saved: SpecialistTeam) {
  actionError.value = null;
  activeListTab.value = "teams";
  teamEditorInitial.value = {
    ...saved,
    description: saved.description ?? "",
    orchestratorName: saved.orchestratorName || "",
    members: saved.members || [],
    orchestrator: saved.orchestrator,
  };
  teamEditorLockName.value = true;
  editingType.value = "team";
  void qc.invalidateQueries({ queryKey: ["teams"] });
  void qc.invalidateQueries({ queryKey: ["specialists"] });
}
async function togglePause(s: Specialist) {
  try {
    const { apiKey: _apiKey, ...rest } = s;
    await upsertSpecialist({
      ...rest,
      paused: !s.paused,
    });
    actionError.value = null;
    await qc.invalidateQueries({ queryKey: ["specialists"] });
    await qc.invalidateQueries({ queryKey: ["agent-status"] });
  } catch (e) {
    setErr(e, "Failed to update pause state.");
  }
}
async function remove(s: Specialist) {
  if (!confirm(`Delete specialist ${s.name}?`)) return;
  try {
    await deleteSpecialist(s.name);
    actionError.value = null;
    await qc.invalidateQueries({ queryKey: ["specialists"] });
    await qc.invalidateQueries({ queryKey: ["teams"] });
    await qc.invalidateQueries({ queryKey: ["agent-status"] });
  } catch (e) {
    setErr(e, "Failed to delete specialist.");
  }
}

async function removeTeam(t: SpecialistTeam) {
  if (!confirm(`Delete team ${t.name}?`)) return;
  try {
    await deleteTeam(t.name);
    actionError.value = null;
    await qc.invalidateQueries({ queryKey: ["teams"] });
    await qc.invalidateQueries({ queryKey: ["specialists"] });
  } catch (e) {
    setErr(e, "Failed to delete team.");
  }
}
</script>
