<template>
  <div class="space-y-3">
    <div
      v-for="field in fields"
      :key="field.name"
      class="rounded border border-border/50 bg-surface-muted/40 p-2"
    >
      <div class="flex items-center justify-between gap-2">
        <div>
          <div class="text-[11px] font-semibold text-foreground">
            {{ field.label }}
          </div>
          <div v-if="field.description" class="text-[10px] text-faint-foreground">
            {{ field.description }}
          </div>
        </div>
        <div class="inline-flex overflow-hidden rounded border border-border/60 text-[10px]">
          <button
            type="button"
            class="px-2 py-1 transition"
            :class="modeFor(field.name) === 'literal' ? 'bg-accent text-accent-foreground' : 'text-subtle-foreground hover:text-foreground'"
            @click="updateMode(field.name, 'literal')"
          >
            Literal
          </button>
          <button
            type="button"
            class="border-l border-border/60 px-2 py-1 transition"
            :class="modeFor(field.name) === 'expression' ? 'bg-accent text-accent-foreground' : 'text-subtle-foreground hover:text-foreground'"
            @click="updateMode(field.name, 'expression')"
          >
            Binding
          </button>
        </div>
      </div>

      <div class="mt-2">
        <textarea
          v-if="modeFor(field.name) === 'expression'"
          :value="expressionValueFor(field.name)"
          rows="2"
          class="w-full rounded border border-border/60 bg-surface px-2 py-1 text-[11px] text-foreground resize-y"
          placeholder="={{$run.input.query}}"
          @input="updateExpression(field.name, ($event.target as HTMLTextAreaElement).value)"
        />
        <textarea
          v-else
          :value="literalValueFor(field.name)"
          rows="field.multiline ? 3 : 2"
          class="w-full rounded border border-border/60 bg-surface px-2 py-1 text-[11px] text-foreground resize-y"
          :placeholder="field.literalPlaceholder"
          @input="updateLiteral(field.name, ($event.target as HTMLTextAreaElement).value)"
        />
      </div>

      <p class="mt-1 text-[10px] text-faint-foreground">
        <template v-if="modeFor(field.name) === 'expression'">
          Use Flow expressions like <code>={{$run.input.query}}</code> or <code>={{$node.fetch.output.body}}</code>.
        </template>
        <template v-else>
          Literal values are saved directly. JSON objects and arrays are supported.
        </template>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

type FieldSpec = {
  name: string;
  label: string;
  description?: string;
  multiline?: boolean;
  literalPlaceholder?: string;
};

const props = defineProps<{
  schema?: Record<string, any> | null;
  modelValue: Record<string, unknown>;
}>();

const emit = defineEmits<{
  "update:modelValue": [Record<string, unknown>];
}>();

const fields = computed<FieldSpec[]>(() => {
  const properties = props.schema?.properties;
  if (!properties || typeof properties !== "object") return [];
  return Object.entries(properties).map(([name, spec]) => {
    const property = spec as Record<string, unknown>;
    const type = typeof property.type === "string" ? property.type : undefined;
    return {
      name,
      label: String(property.title ?? name),
      description:
        typeof property.description === "string" ? property.description : undefined,
      multiline: type === "object" || type === "array" || type === "string",
      literalPlaceholder: type === "object" || type === "array" ? "{}" : "Literal value",
    };
  });
});

function modeFor(name: string): "literal" | "expression" {
  const value = props.modelValue?.[name];
  return typeof value === "string" && looksLikeExpression(value)
    ? "expression"
    : "literal";
}

function expressionValueFor(name: string): string {
  const value = props.modelValue?.[name];
  return typeof value === "string" ? value : "={{$run.input.query}}";
}

function literalValueFor(name: string): string {
  const value = props.modelValue?.[name];
  if (value === undefined || value === null) return "";
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function updateMode(name: string, mode: "literal" | "expression") {
  if (mode === modeFor(name)) return;
  const next = { ...(props.modelValue ?? {}) };
  if (mode === "expression") {
    next[name] = "={{$run.input.query}}";
  } else {
    next[name] = "";
  }
  emit("update:modelValue", next);
}

function updateExpression(name: string, value: string) {
  const next = { ...(props.modelValue ?? {}) };
  next[name] = value;
  emit("update:modelValue", next);
}

function updateLiteral(name: string, raw: string) {
  const next = { ...(props.modelValue ?? {}) };
  next[name] = parseLiteral(raw);
  emit("update:modelValue", next);
}

function parseLiteral(raw: string): unknown {
  const trimmed = raw.trim();
  if (!trimmed) return "";
  if (trimmed === "true") return true;
  if (trimmed === "false") return false;
  if (trimmed === "null") return null;
  if (/^-?\d+(\.\d+)?$/.test(trimmed)) return Number(trimmed);
  if (
    (trimmed.startsWith("{") && trimmed.endsWith("}")) ||
    (trimmed.startsWith("[") && trimmed.endsWith("]"))
  ) {
    try {
      return JSON.parse(trimmed);
    } catch {
      return raw;
    }
  }
  return raw;
}

function looksLikeExpression(value: string): boolean {
  const trimmed = value.trim();
  return (
    trimmed.startsWith("=") ||
    (trimmed.startsWith("{{") && trimmed.endsWith("}}")) ||
    trimmed.startsWith("$run.") ||
    trimmed.startsWith("$node.")
  );
}
</script>