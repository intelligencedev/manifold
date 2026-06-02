<template>
  <div class="flex h-full min-h-0 flex-col overflow-hidden">
    <div class="sticky top-0 z-10 border-b border-border/50 bg-surface">
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
        aria-label="Edit Team"
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
        >
          {{ t.label }}
        </button>
      </div>
    </div>

    <div
      class="scrollbar-inset flex min-h-0 flex-1 flex-col overflow-auto px-4 pb-6 pt-4"
    >
      <div
        v-if="actionError"
        class="mb-4 rounded-lg border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground"
      >
        {{ actionError }}
      </div>
      <div
        v-if="successMsg"
        class="mb-4 rounded-lg border border-border/60 bg-surface-muted/30 p-3 text-sm text-foreground"
      >
        {{ successMsg }}
      </div>

      <div
        v-show="activeTab === 'details'"
        role="tabpanel"
        id="panel-details"
        aria-labelledby="tab-details"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <FormSection
          title="Team Identity"
          helper="Teams collect specialists and use one member as the orchestrator."
        >
          <div class="flex flex-col gap-3">
            <div class="flex flex-col gap-1">
              <label
                for="team-name"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Name</label
              >
              <input
                id="team-name"
                v-model.trim="draft.name"
                class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                :disabled="lockName"
              />
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="team-description"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Description</label
              >
              <textarea
                id="team-description"
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
        id="panel-orchestrator"
        aria-labelledby="tab-orchestrator"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <FormSection
          title="Orchestrator"
          helper="Choose one active team member specialist to coordinate the team."
        >
          <div class="flex flex-col gap-3">
            <div
              v-if="!orchestratorName"
              class="rounded border border-danger/60 bg-danger/10 px-3 py-2 text-sm text-danger-foreground"
            >
              Orchestrator required.
            </div>
            <div class="flex flex-col gap-1">
              <label
                for="team-orchestrator-name"
                class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
                >Specialist</label
              >
              <select
                id="team-orchestrator-name"
                v-model="orchestratorName"
                class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
                @change="selectOrchestrator"
              >
                <option value="" disabled>Select a specialist</option>
                <option
                  v-for="sp in orchestratorOptions"
                  :key="sp.name"
                  :value="sp.name"
                >
                  {{ sp.name }}
                </option>
              </select>
            </div>
            <div
              v-if="selectedOrchestrator"
              class="rounded border border-border/60 bg-surface-muted/20 px-3 py-2"
            >
              <p class="text-sm font-semibold text-foreground">
                {{ selectedOrchestrator.name }}
              </p>
              <p class="mt-1 text-xs text-subtle-foreground">
                {{ selectedOrchestrator.model || "No model configured" }}
              </p>
            </div>
            <div
              v-else-if="!orchestratorOptions.length"
              class="rounded border border-border/60 bg-surface-muted/20 px-3 py-2 text-sm text-subtle-foreground"
            >
              No active specialists available.
            </div>
          </div>
        </FormSection>
      </div>

      <div
        v-show="activeTab === 'members'"
        role="tabpanel"
        id="panel-members"
        aria-labelledby="tab-members"
        tabindex="0"
        class="flex flex-col gap-4"
      >
        <FormSection
          title="Members"
          helper="The orchestrator is locked as a member of this team."
        >
          <div class="flex flex-col gap-3">
            <input
              v-model="memberSearch"
              type="text"
              placeholder="Filter specialists"
              class="w-full rounded border border-border/60 bg-surface-muted/40 px-3 py-2 text-sm"
            />
            <div class="rounded-lg border border-border/60 bg-surface">
              <div
                v-if="!filteredMembers.length"
                class="px-3 py-3 text-sm text-subtle-foreground"
              >
                No specialists match your search.
              </div>
              <label
                v-for="sp in filteredMembers"
                :key="sp.name"
                class="flex cursor-pointer items-start gap-3 border-t border-border/40 px-3 py-2 transition-colors first:border-t-0 hover:bg-surface-muted/40"
                :class="{ 'cursor-not-allowed opacity-70': isOrchestratorMember(sp.name) }"
              >
                <input
                  class="mt-1 h-4 w-4 shrink-0"
                  type="checkbox"
                  :checked="selectedMembers.has(sp.name)"
                  :disabled="isOrchestratorMember(sp.name)"
                  @change="
                    toggleMember(
                      sp.name,
                      ($event.target as HTMLInputElement).checked,
                    )
                  "
                />
                <div class="min-w-0 flex-1">
                  <div class="flex min-w-0 items-center gap-2">
                    <p class="truncate text-sm font-medium text-foreground">
                      {{ sp.name }}
                    </p>
                    <span
                      v-if="isOrchestratorMember(sp.name)"
                      class="shrink-0 rounded-full border border-accent/40 bg-accent/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-accent"
                      >Orchestrator</span
                    >
                    <span
                      v-else-if="sp.paused"
                      class="shrink-0 rounded-full border border-border/60 bg-surface-muted/40 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-subtle-foreground"
                      >Paused</span
                    >
                  </div>
                  <p class="mt-0.5 text-xs text-subtle-foreground">
                    {{ sp.model || "No model configured" }}
                  </p>
                </div>
              </label>
            </div>
          </div>
        </FormSection>
      </div>
    </div>

    <div class="sticky bottom-0 z-10 border-t border-border/50 bg-surface">
      <div class="flex items-center justify-between gap-3 px-4 py-3">
        <div class="text-xs text-subtle-foreground">
          <span v-if="saving">Saving...</span>
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
            {{ saving ? "Saving..." : "Save" }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import {
  type Specialist,
  type SpecialistTeam,
  upsertTeam,
} from "@/api/client";
import FormSection from "./edit/FormSection.vue";

const props = withDefaults(
  defineProps<{
    initial: SpecialistTeam;
    lockName?: boolean;
    availableSpecialists: Specialist[];
  }>(),
  { lockName: false },
);

const emit = defineEmits<{ saved: [SpecialistTeam]; cancel: [] }>();

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
const baseline = ref<SpecialistTeam | null>(null);
const selectedMembers = ref(new Set<string>());
const orchestratorName = ref("");
const memberSearch = ref("");

const draft = reactive({
  name: "",
  description: "",
});

const headerTitle = computed(() =>
  baseline.value?.name ? "Edit Team" : "Create Team",
);

const headerSubtitle = computed(() =>
  baseline.value?.name
    ? "Update the team definition."
    : "Create a team and choose its orchestrator.",
);

const lockName = computed(() => !!props.lockName);

const availableMemberSpecialists = computed(() => {
  const seen = new Set<string>();
  return (props.availableSpecialists || [])
    .filter((sp) => {
      const name = (sp.name || "").trim();
      if (!name || name.toLowerCase() === "orchestrator") return false;
      const key = name.toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .sort((a, b) =>
      a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
    );
});

const orchestratorOptions = computed(() =>
  availableMemberSpecialists.value.filter((sp) => !sp.paused),
);

const specialistsByName = computed(() => {
  const map = new Map<string, Specialist>();
  for (const sp of availableMemberSpecialists.value) {
    const key = sp.name.trim().toLowerCase();
    if (key) map.set(key, sp);
  }
  return map;
});

const selectedOrchestrator = computed(() =>
  specialistsByName.value.get(orchestratorName.value.trim().toLowerCase()),
);

const filteredMembers = computed(() => {
  const q = memberSearch.value.trim().toLowerCase();
  if (!q) return availableMemberSpecialists.value;
  return availableMemberSpecialists.value.filter((sp) =>
    sp.name.toLowerCase().includes(q),
  );
});

const isDirty = computed(() => {
  if (!baseline.value) return true;
  if (draft.name.trim() !== (baseline.value.name || "")) return true;
  if (draft.description !== (baseline.value.description || "")) return true;
  if (
    orchestratorName.value.trim() !==
    ((baseline.value.orchestratorName || "").trim())
  )
    return true;

  const baselineMembers = new Set(baseline.value.members || []);
  if (selectedMembers.value.size !== baselineMembers.size) return true;
  for (const member of selectedMembers.value) {
    if (!baselineMembers.has(member)) return true;
  }
  return false;
});

function initFromInitial(team: SpecialistTeam) {
  baseline.value = team;
  draft.name = team.name || "";
  draft.description = team.description || "";
  orchestratorName.value = (team.orchestratorName || "").trim();
  const nextMembers = new Set(team.members || []);
  if (orchestratorName.value) {
    nextMembers.add(orchestratorName.value);
  }
  selectedMembers.value = nextMembers;
  actionError.value = null;
  successMsg.value = null;
}

function selectOrchestrator() {
  const name = orchestratorName.value.trim();
  if (!name) return;
  const next = new Set(selectedMembers.value);
  next.add(name);
  selectedMembers.value = next;
}

function isOrchestratorMember(name: string) {
  return (
    name.trim().toLowerCase() === orchestratorName.value.trim().toLowerCase()
  );
}

function toggleMember(name: string, enabled: boolean) {
  if (isOrchestratorMember(name)) return;
  const next = new Set(selectedMembers.value);
  if (enabled) next.add(name);
  else next.delete(name);
  selectedMembers.value = next;
}

function buildPayload(): SpecialistTeam {
  return {
    id: baseline.value?.id,
    name: draft.name.trim(),
    description: draft.description || "",
    orchestratorName: orchestratorName.value.trim(),
    orchestrator: baseline.value?.orchestrator || ({} as Specialist),
    members: Array.from(selectedMembers.value).sort((a, b) =>
      a.localeCompare(b, undefined, { sensitivity: "base" }),
    ),
  };
}

async function onSave() {
  actionError.value = null;
  successMsg.value = null;
  if (!draft.name.trim()) {
    actionError.value = "Team name is required.";
    return;
  }
  if (!orchestratorName.value.trim()) {
    actionError.value = "Team orchestrator is required.";
    activeTab.value = "orchestrator";
    return;
  }
  if (!selectedMembers.value.has(orchestratorName.value.trim())) {
    actionError.value = "Team orchestrator must be a member.";
    activeTab.value = "members";
    return;
  }
  try {
    saving.value = true;
    const saved = await upsertTeam(buildPayload());
    initFromInitial(saved);
    successMsg.value = "Saved.";
    emit("saved", saved);
  } catch (e: any) {
    const msg = e?.response?.data || e?.message || "Failed to save team.";
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
  (team) => {
    if (!team) return;
    initFromInitial(team);
  },
  { immediate: true },
);
</script>
