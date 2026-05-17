<template>
  <section class="space-y-6">
    <div>
      <h1 class="text-xl font-semibold">Constitution</h1>
      <p class="mt-0.5 text-sm text-white/50">Versioned agent governance. The active version is injected into the runtime at the next enforcement checkpoint.</p>
    </div>

    <div class="grid gap-6 xl:grid-cols-[1fr_400px]">
      <!-- Editor -->
      <Card title="Draft new version" description="Constitutional guidance, operating principles, and intervention rules.">
        <template #header>
          <Badge v-if="activeVersion" variant="success" dot>v{{ activeVersion.version }} active</Badge>
          <Badge v-else variant="muted">no active version</Badge>
        </template>

        <div class="mb-3 flex items-center gap-2 text-xs text-white/40">
          <span>Markdown supported</span>
          <span>·</span>
          <span>{{ draft.length }} chars</span>
        </div>

        <textarea
          v-model="draft"
          class="min-h-[380px] w-full resize-none rounded-lg border border-border bg-black/15 p-4 font-mono text-sm text-foreground/90 placeholder:text-white/20 focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/20"
          placeholder="# Operating Principles&#10;&#10;1. Never delete files without explicit operator approval.&#10;2. Escalate irreversible actions to the interrupt inbox.&#10;3. …"
          spellcheck="false"
        />

        <div class="mt-4 flex items-center justify-between">
          <Button variant="ghost" size="sm" :disabled="!activeVersion" @click="populateFromActive">Load active version</Button>
          <div class="flex gap-2">
            <Button variant="secondary" :disabled="!draft.trim()" @click="previewDiff = true">Preview diff</Button>
            <Button variant="primary" :loading="creating" :disabled="!draft.trim()" @click="create">Create version</Button>
          </div>
        </div>

        <ErrorBanner v-if="createError" :message="createError" class="mt-3" @dismiss="createError = ''" />
      </Card>

      <!-- Version list -->
      <Card title="Versions" :description="`${store.versions.length} versions`" :no-padding="true">
        <template #header>
          <Button size="sm" variant="ghost" @click="store.refresh">↻</Button>
        </template>

        <EmptyState v-if="store.versions.length === 0" icon="📜" title="No versions yet" description="Create the first constitution version." class="py-10" />

        <div v-else class="divide-y divide-border/30">
          <div
            v-for="version in store.versions"
            :key="version.id"
            class="px-4 py-4 transition-colors hover:bg-white/3"
            :class="version.active ? 'bg-accent/5' : ''"
          >
            <div class="mb-2 flex items-center justify-between gap-2">
              <div class="flex items-center gap-2">
                <span class="text-sm font-semibold">v{{ version.version }}</span>
                <Badge v-if="version.active" variant="success" dot>active</Badge>
              </div>
              <div class="text-[10px] text-white/30">{{ reltime(version.created_at) }}</div>
            </div>
            <p class="line-clamp-4 text-xs text-white/55 leading-relaxed font-mono">{{ version.body }}</p>
            <div class="mt-3 flex items-center gap-2">
              <Button size="sm" variant="ghost" @click="diffWith(version)">Diff</Button>
              <Button
                v-if="!version.active"
                size="sm"
                variant="secondary"
                @click="confirmActivate(version)"
              >Activate</Button>
              <Badge v-else variant="success">Current</Badge>
            </div>
          </div>
        </div>
      </Card>
    </div>

    <!-- Diff preview modal -->
    <Modal :open="previewDiff && !!diffTarget" :title="diffTitle" @close="previewDiff = false; diffTarget = null">
      <div class="grid gap-3 sm:grid-cols-2 text-xs">
        <div>
          <div class="mb-1 text-[10px] uppercase text-white/30">{{ diffTarget ? `v${diffTarget.version}` : 'Active' }}</div>
          <pre class="overflow-auto rounded-lg bg-black/20 p-3 text-red-300/70 leading-relaxed" style="max-height:340px; white-space:pre-wrap;">{{ diffTarget?.body ?? activeVersion?.body ?? '(empty)' }}</pre>
        </div>
        <div>
          <div class="mb-1 text-[10px] uppercase text-white/30">Draft</div>
          <pre class="overflow-auto rounded-lg bg-black/20 p-3 text-emerald-300/70 leading-relaxed" style="max-height:340px; white-space:pre-wrap;">{{ draft }}</pre>
        </div>
      </div>
      <template #footer>
        <Button variant="ghost" @click="previewDiff = false; diffTarget = null">Close</Button>
      </template>
    </Modal>

    <!-- Activate confirm modal -->
    <Modal :open="activateModal.open" title="Activate constitution version" @close="activateModal.open = false">
      <p class="text-sm text-white/70 mb-2">This will deactivate all other versions and apply <strong class="text-white/90">v{{ activateModal.version?.version }}</strong> at the next enforcement checkpoint.</p>
      <p class="text-xs text-amber-300/70">This action cannot be undone without creating a new version.</p>
      <template #footer>
        <Button variant="ghost" @click="activateModal.open = false">Cancel</Button>
        <Button variant="primary" :loading="activating" @click="submitActivate">Activate</Button>
      </template>
    </Modal>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import Modal from "@/components/ui/Modal.vue";
import { useConstitutionStore } from "@/stores/constitution";

const store = useConstitutionStore();
const draft = ref("");
const creating = ref(false);
const activating = ref(false);
const createError = ref("");
const previewDiff = ref(false);
const diffTarget = ref<any>(null);
const diffTitle = computed(() => diffTarget.value ? `Diff: v${diffTarget.value.version} → draft` : "Diff: active → draft");
const activateModal = ref<{ open: boolean; version: any | null }>({ open: false, version: null });

const activeVersion = computed(() => store.versions.find((v: any) => v.active) ?? null);

onMounted(() => store.refresh());

async function create() {
  if (!draft.value.trim()) return;
  creating.value = true;
  createError.value = "";
  try {
    await store.create(draft.value.trim());
    draft.value = "";
  } catch (err: unknown) {
    createError.value = err instanceof Error ? err.message : "Failed to create version";
  } finally {
    creating.value = false;
  }
}

function confirmActivate(version: any) {
  activateModal.value = { open: true, version };
}

async function submitActivate() {
  if (!activateModal.value.version) return;
  activating.value = true;
  try {
    await store.activate(activateModal.value.version.id);
    activateModal.value.open = false;
  } finally {
    activating.value = false;
  }
}

function populateFromActive() {
  if (activeVersion.value) draft.value = activeVersion.value.body;
}

function diffWith(version: any) {
  diffTarget.value = version;
  previewDiff.value = true;
}

function reltime(iso: string) {
  if (!iso) return "";
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return "just now";
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}
</script>
