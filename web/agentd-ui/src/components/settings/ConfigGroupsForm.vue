<template>
  <div class="space-y-5">
    <section
      v-for="group in groups"
      :key="group.key"
      class="rounded-md border border-border/60 bg-surface-muted/30"
    >
      <header
        class="flex items-start justify-between gap-4 border-b border-border/50 px-4 py-3"
      >
        <div>
          <h3 class="text-sm font-semibold text-foreground">
            {{ group.title }}
          </h3>
          <p class="mt-1 text-xs text-subtle-foreground">
            {{ group.description }}
          </p>
        </div>
        <button
          type="button"
          class="shrink-0 rounded bg-accent px-3 py-2 text-xs font-semibold text-accent-foreground hover:bg-accent/90 disabled:opacity-60"
          :disabled="saving"
          @click="$emit('save', group.key, drafts[group.key])"
        >
          Save {{ group.title }}
        </button>
      </header>
      <div class="space-y-4 p-4">
        <ConfigValueField
          :label="group.title"
          :model-value="drafts[group.key]"
          @update:model-value="drafts[group.key] = $event"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import ConfigValueField from "@/components/settings/ConfigValueField.vue";

export type ConfigGroup = {
  key: string;
  title: string;
  description: string;
  rootKeys?: string[];
};

const props = defineProps<{
  config: Record<string, unknown>;
  groups: ConfigGroup[];
  saving?: boolean;
}>();

defineEmits<{ save: [group: string, value: unknown] }>();

const drafts = ref<Record<string, unknown>>({});

watch(
  () => [props.config, props.groups] as const,
  () => {
    drafts.value = Object.fromEntries(
      props.groups.map((group) => [group.key, clone(sourceValue(group))]),
    );
  },
  { immediate: true },
);

function sourceValue(group: ConfigGroup): unknown {
  if (group.rootKeys) {
    return Object.fromEntries(
      group.rootKeys.map((key) => [key, valueFor(key)]),
    );
  }
  return valueFor(group.key);
}

function valueFor(key: string): unknown {
  if (props.config[key] !== undefined) return props.config[key];
  const normalized = normalizeKey(key);
  const match = Object.keys(props.config).find(
    (candidate) => normalizeKey(candidate) === normalized,
  );
  return match ? props.config[match] : {};
}

function normalizeKey(key: string): string {
  return key.toLowerCase().replaceAll(/[_-]/g, "");
}

function clone(value: unknown): unknown {
  return JSON.parse(JSON.stringify(value ?? {}));
}
</script>
