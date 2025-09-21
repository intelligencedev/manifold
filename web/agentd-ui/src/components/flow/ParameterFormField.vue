<template>
  <div class="space-y-2">
    <template v-if="isObject">
      <div v-if="fieldLabel" class="text-[11px] font-semibold text-slate-200">
        {{ fieldLabel }}
        <span v-if="required" class="ml-1 text-[10px] text-red-400">*</span>
      </div>
      <p v-if="schema.description" class="text-[10px] text-slate-500">{{ schema.description }}</p>
      <div class="space-y-2 border-l border-slate-800 pl-3">
        <ParameterFormField
          v-for="([key, childSchema], index) in childEntries"
          :key="`${key}-${index}`"
          :schema="childSchema"
          :label="childLabels[key]"
          :required="childRequired.has(key)"
          :model-value="childValue(key)"
          @update:modelValue="(value) => updateChild(key, value)"
        />
      </div>
    </template>
    <template v-else-if="isArray">
      <div class="rounded border border-amber-500/30 bg-amber-500/10 px-2 py-1 text-[10px] text-amber-200">
        Array parameters are not yet supported in the visual editor.
      </div>
    </template>
    <template v-else-if="isUnsupported">
      <div class="rounded border border-amber-500/30 bg-amber-500/10 px-2 py-1 text-[10px] text-amber-200">
        Unsupported schema type for {{ fieldLabel || 'field' }}.
      </div>
    </template>
    <template v-else>
      <label class="flex flex-col gap-1 text-[11px] text-slate-300">
        <span class="flex items-center gap-1">
          {{ fieldLabel }}
          <span v-if="required" class="text-[10px] text-red-400">*</span>
        </span>
        <select
          v-if="hasEnum"
          :value="selectValue"
          class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-[11px] text-white"
          @change="onSelectChange"
        >
          <option value="" v-if="!required">(unset)</option>
          <option v-for="option in enumOptions" :key="optionKey(option)" :value="String(option)">
            {{ optionLabel(option) }}
          </option>
        </select>
        <input
          v-else-if="isNumeric"
          type="number"
          :step="numberStep"
          :value="numberInput"
          class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-[11px] text-white"
          @input="onNumberInput"
        />
        <label v-else-if="isBoolean" class="flex items-center gap-2 text-[11px] text-slate-300">
          <input type="checkbox" :checked="booleanValue" @change="onBooleanChange" />
          <span>{{ schema.description ?? 'Enabled' }}</span>
        </label>
        <input
          v-else
          type="text"
          :value="stringValue"
          class="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-[11px] text-white"
          @input="onStringInput"
        />
      </label>
      <p v-if="schema.description && !isBoolean" class="text-[10px] text-slate-500">
        {{ schema.description }}
      </p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

defineOptions({ name: 'ParameterFormField' })

const props = defineProps<{
  schema: Record<string, any>
  modelValue: unknown
  label?: string
  required?: boolean
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: unknown): void
}>()

function schemaType(schema: Record<string, any> | undefined): string | undefined {
  if (!schema) return undefined
  if (schema.type) {
    if (Array.isArray(schema.type)) {
      return schema.type[0]
    }
    return schema.type
  }
  if (schema.properties) {
    return 'object'
  }
  if (schema.enum) {
    return 'string'
  }
  return undefined
}

const type = computed(() => schemaType(props.schema))

const fieldLabel = computed(() => props.label ?? props.schema.title ?? 'Field')

const isObject = computed(() => type.value === 'object')
const isArray = computed(() => type.value === 'array')
const isBoolean = computed(() => type.value === 'boolean')
const isNumeric = computed(() => type.value === 'number' || type.value === 'integer')
const isUnsupported = computed(() => !isObject.value && !isArray.value && !isBoolean.value && !isNumeric.value && !hasEnum.value && type.value !== 'string')

const enumOptions = computed(() => (Array.isArray(props.schema.enum) ? props.schema.enum : []))
const hasEnum = computed(() => enumOptions.value.length > 0)

const childRequired = computed(() => new Set<string>(Array.isArray(props.schema.required) ? props.schema.required : []))
const childEntries = computed(() => Object.entries(props.schema.properties ?? {}))
const childLabels = computed(() => {
  const out: Record<string, string> = {}
  childEntries.value.forEach(([key, schema]) => {
    out[key] = schema?.title ?? key
  })
  return out
})

function childValue(key: string) {
  if (props.modelValue && typeof props.modelValue === 'object' && !Array.isArray(props.modelValue)) {
    return (props.modelValue as Record<string, unknown>)[key]
  }
  return undefined
}

function updateChild(key: string, value: unknown) {
  const base: Record<string, unknown> =
    props.modelValue && typeof props.modelValue === 'object' && !Array.isArray(props.modelValue)
      ? { ...(props.modelValue as Record<string, unknown>) }
      : {}

  if (value === undefined || (value === '' && !childRequired.value.has(key))) {
    delete base[key]
  } else {
    base[key] = value
  }
  emit('update:modelValue', base)
}

const stringValue = computed(() => (typeof props.modelValue === 'string' ? props.modelValue : ''))

function onStringInput(event: Event) {
  const target = event.target as HTMLInputElement
  const value = target.value
  emit('update:modelValue', value === '' && !props.required ? undefined : value)
}

const numberInput = computed(() => {
  if (typeof props.modelValue === 'number') {
    return Number.isFinite(props.modelValue) ? String(props.modelValue) : ''
  }
  if (typeof props.modelValue === 'string' && props.modelValue.trim() !== '') {
    return props.modelValue
  }
  return ''
})

const numberStep = computed(() => (type.value === 'integer' ? 1 : 'any'))

function onNumberInput(event: Event) {
  const target = event.target as HTMLInputElement
  const raw = target.value
  if (raw === '') {
    emit('update:modelValue', undefined)
    return
  }
  const parsed = type.value === 'integer' ? parseInt(raw, 10) : parseFloat(raw)
  if (Number.isNaN(parsed)) {
    return
  }
  emit('update:modelValue', parsed)
}

const booleanValue = computed(() => Boolean(props.modelValue))

function onBooleanChange(event: Event) {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.checked)
}

const selectValue = computed(() => {
  if (props.modelValue === undefined || props.modelValue === null) {
    return ''
  }
  return String(props.modelValue)
})

function onSelectChange(event: Event) {
  const target = event.target as HTMLSelectElement
  const raw = target.value
  if (raw === '') {
    emit('update:modelValue', undefined)
    return
  }
  if (type.value === 'number' || type.value === 'integer') {
    const parsed = type.value === 'integer' ? parseInt(raw, 10) : parseFloat(raw)
    emit('update:modelValue', Number.isNaN(parsed) ? undefined : parsed)
    return
  }
  emit('update:modelValue', raw)
}

function optionKey(option: unknown) {
  if (typeof option === 'object') {
    return JSON.stringify(option)
  }
  return String(option)
}

function optionLabel(option: unknown) {
  if (typeof option === 'object') {
    return JSON.stringify(option)
  }
  return String(option)
}
</script>
