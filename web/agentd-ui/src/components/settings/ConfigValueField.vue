<template>
  <div class="space-y-2">
    <label
      v-if="isScalar"
      :for="fieldID"
      class="block text-sm font-medium text-foreground"
    >
      {{ formattedLabel }}
    </label>

    <label
      v-if="isBoolean"
      class="flex items-center gap-2 text-sm text-foreground"
    >
      <input
        :id="fieldID"
        :checked="model === true"
        type="checkbox"
        class="h-4 w-4"
        @change="model = ($event.target as HTMLInputElement).checked"
      />
      <span>{{ formattedLabel }}</span>
    </label>

    <select
      v-else-if="model === null"
      :id="fieldID"
      :value="'inherit'"
      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
      @change="updateNullableBoolean"
    >
      <option value="inherit">Inherit default</option>
      <option value="true">Enabled</option>
      <option value="false">Disabled</option>
    </select>

    <input
      v-else-if="typeof model === 'number'"
      :id="fieldID"
      :value="model"
      type="number"
      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
      @input="model = Number(($event.target as HTMLInputElement).value)"
    />

    <textarea
      v-else-if="isLongText"
      :id="fieldID"
      :value="displayString"
      rows="4"
      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
      @input="updateString(($event.target as HTMLTextAreaElement).value)"
    ></textarea>

    <input
      v-else-if="typeof model === 'string'"
      :id="fieldID"
      :value="displayString"
      :type="isSensitive ? 'password' : isURL ? 'url' : 'text'"
      :placeholder="isSensitive && model === '[REDACTED]' ? 'Configured' : ''"
      autocomplete="off"
      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
      @input="updateString(($event.target as HTMLInputElement).value)"
    />

    <fieldset
      v-else-if="isLLMClient"
      class="space-y-4 rounded border border-border/50 p-3"
    >
      <legend class="px-1 text-sm font-medium text-foreground">
        {{ formattedLabel }}
      </legend>
      <div class="space-y-1">
        <label
          :for="`${fieldID}-provider`"
          class="block text-sm font-medium text-foreground"
        >
          Provider
        </label>
        <select
          :id="`${fieldID}-provider`"
          data-llm-provider
          :value="selectedProvider"
          class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
          @change="updateProvider"
        >
          <option
            v-for="option in llmProviderOptions"
            :key="option.value"
            :value="option.value"
          >
            {{ option.label }}
          </option>
        </select>
      </div>
      <ConfigValueField
        :label="`${selectedProviderLabel} settings`"
        :model-value="activeProviderConfig"
        @update:model-value="updateObject(activeProviderBackend, $event)"
      />
    </fieldset>

    <fieldset
      v-else-if="isArray"
      class="space-y-2 rounded border border-border/50 p-3"
    >
      <legend class="px-1 text-sm font-medium text-foreground">
        {{ formattedLabel }}
      </legend>
      <div
        v-for="(item, index) in arrayValue"
        :key="index"
        class="flex items-start gap-2"
      >
        <ConfigValueField
          v-if="isComplex(item)"
          :label="`${formattedLabel} ${index + 1}`"
          :model-value="item"
          class="min-w-0 flex-1"
          @update:model-value="updateArray(index, $event)"
        />
        <input
          v-else
          :value="String(item ?? '')"
          type="text"
          class="min-w-0 flex-1 rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
          @input="updateArray(index, ($event.target as HTMLInputElement).value)"
        />
        <button
          type="button"
          class="rounded border border-danger/40 px-2 py-1 text-xs text-danger-foreground"
          @click="removeArrayItem(index)"
        >
          Remove
        </button>
      </div>
      <button
        type="button"
        class="rounded border border-border/60 bg-surface px-2 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
        @click="addArrayItem"
      >
        Add item
      </button>
    </fieldset>

    <fieldset v-else class="space-y-3 rounded border border-border/50 p-3">
      <legend class="px-1 text-sm font-medium text-foreground">
        {{ formattedLabel }}
      </legend>
      <ConfigValueField
        v-for="(value, key) in objectValue"
        :key="key"
        :label="String(key)"
        :model-value="value"
        @update:model-value="updateObject(String(key), $event)"
      />
      <div class="flex gap-2 border-t border-border/50 pt-3">
        <input
          v-model="newKey"
          type="text"
          placeholder="New setting name"
          class="min-w-0 flex-1 rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm"
        />
        <button
          type="button"
          class="rounded border border-border/60 bg-surface px-3 py-2 text-xs font-semibold text-subtle-foreground hover:border-border"
          @click="addObjectField"
        >
          Add
        </button>
      </div>
    </fieldset>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import {
  llmProviderBackend,
  llmProviderOptions,
} from "@/constants/llmProviders";

defineOptions({ name: "ConfigValueField" });

const props = defineProps<{ label: string; modelValue: unknown }>();
const emit = defineEmits<{ "update:modelValue": [value: unknown] }>();

const newKey = ref("");
const model = computed({
  get: () => props.modelValue,
  set: (value: unknown) => emit("update:modelValue", value),
});
const fieldID = computed(
  () => `config-${props.label.replaceAll(/[^a-zA-Z0-9]+/g, "-").toLowerCase()}`,
);
const formattedLabel = computed(() =>
  props.label
    .replaceAll(/([a-z])([A-Z])/g, "$1 $2")
    .replaceAll(/[_-]/g, " ")
    .replace(/^./, (letter) => letter.toUpperCase()),
);
const isBoolean = computed(() => typeof model.value === "boolean");
const isArray = computed(() => Array.isArray(model.value));
const isLLMClient = computed(
  () =>
    isObject(model.value) &&
    typeof model.value.provider === "string" &&
    ["openai", "anthropic", "google"].some((key) => key in model.value),
);
const isScalar = computed(
  () => model.value === null || typeof model.value !== "object",
);
const isSensitive = computed(() =>
  /key|secret|token|password|authorization/i.test(props.label),
);
const isURL = computed(() => /url|endpoint|dsn/i.test(props.label));
const isLongText = computed(
  () =>
    typeof model.value === "string" &&
    /prompt|instruction|system|justification|description/i.test(props.label),
);
const displayString = computed(() =>
  isSensitive.value && model.value === "[REDACTED]"
    ? ""
    : String(model.value ?? ""),
);
const arrayValue = computed(() =>
  Array.isArray(model.value) ? model.value : [],
);
const objectValue = computed<Record<string, unknown>>(() =>
  isObject(model.value) ? model.value : {},
);
const selectedProvider = computed(() => {
  const provider = String(objectValue.value.provider ?? "openai");
  return llmProviderOptions.some((option) => option.value === provider)
    ? provider
    : "openai";
});
const selectedProviderLabel = computed(
  () =>
    llmProviderOptions.find((option) => option.value === selectedProvider.value)
      ?.label ?? "OpenAI / compatible",
);
const activeProviderBackend = computed(() =>
  llmProviderBackend(selectedProvider.value),
);
const activeProviderConfig = computed(
  () => objectValue.value[activeProviderBackend.value] ?? {},
);

function updateString(value: string) {
  if (isSensitive.value && value === "" && model.value === "[REDACTED]") return;
  model.value = value;
}

function updateNullableBoolean(event: Event) {
  const value = (event.target as HTMLSelectElement).value;
  model.value = value === "inherit" ? null : value === "true";
}

function updateProvider(event: Event) {
  updateObject("provider", (event.target as HTMLSelectElement).value);
}

function updateArray(index: number, value: unknown) {
  const next = [...arrayValue.value];
  next[index] = value;
  model.value = next;
}

function removeArrayItem(index: number) {
  model.value = arrayValue.value.filter((_, candidate) => candidate !== index);
}

function addArrayItem() {
  model.value = [...arrayValue.value, emptyLike(arrayValue.value[0])];
}

function updateObject(key: string, value: unknown) {
  model.value = { ...objectValue.value, [key]: value };
}

function addObjectField() {
  const key = newKey.value.trim();
  if (!key || key in objectValue.value) return;
  updateObject(key, "");
  newKey.value = "";
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isComplex(value: unknown): boolean {
  return typeof value === "object" && value !== null;
}

function emptyLike(value: unknown): unknown {
  if (Array.isArray(value)) return [];
  if (isObject(value))
    return Object.fromEntries(
      Object.entries(value).map(([key, child]) => [key, emptyLike(child)]),
    );
  if (typeof value === "boolean") return false;
  if (typeof value === "number") return 0;
  return "";
}
</script>
