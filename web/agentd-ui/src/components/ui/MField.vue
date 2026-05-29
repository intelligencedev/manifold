<template>
  <component
    :is="as"
    :value="modelValue"
    :type="as === 'input' ? type : undefined"
    :placeholder="placeholder"
    class="halo-focus w-full rounded-md border border-[rgb(var(--line-strong))] bg-surface px-[13px] py-2.5 font-sans text-sm text-foreground outline-none placeholder:text-faint-foreground"
    @input="onInput"
  />
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    modelValue?: string | number;
    as?: "input" | "textarea";
    placeholder?: string;
    type?: string;
  }>(),
  {
    modelValue: "",
    as: "input",
    type: "text",
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

function onInput(event: Event) {
  emit("update:modelValue", (event.target as HTMLInputElement).value);
}
</script>
