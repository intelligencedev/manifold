<template>
  <div class="space-y-2">
    <ParameterFormField
      v-for="field in fields"
      :key="field.name"
      :schema="field.schema"
      :model-value="modelValue?.[field.name]"
      :label="field.label"
      :name="field.name"
      @update:model-value="update(field.name, $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import ParameterFormField from "@/components/flow/ParameterFormField.vue";

defineOptions({ name: "FlowInputBindingsEditor" });

const props = defineProps<{
  schema?: Record<string, any> | null;
  modelValue: Record<string, unknown>;
}>();

const emit = defineEmits<{
  "update:modelValue": [Record<string, unknown>];
}>();

const fields = computed(() => {
  const properties = props.schema?.properties;
  if (!properties || typeof properties !== "object") return [];
  return Object.entries(properties).map(([name, spec]) => ({
    name,
    label: String((spec as any).title ?? name),
    schema: spec as Record<string, any>,
  }));
});

function update(name: string, value: unknown) {
  emit("update:modelValue", { ...(props.modelValue ?? {}), [name]: value });
}
</script>