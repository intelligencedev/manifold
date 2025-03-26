<template>
  <button type="button" class="toggle-password" @click="toggle">
    <span v-if="modelValue">👁️</span>
    <span v-else>🙈</span>
  </button>
</template>

<script setup>
const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['update:modelValue'])

function toggle() {
  emit('update:modelValue', !props.modelValue)
}
</script>

<style scoped>
.toggle-password {
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  color: var(--icon-color, #eee);
}
</style>