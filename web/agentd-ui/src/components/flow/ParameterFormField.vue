<template>
  <div class="space-y-2">
    <template v-if="isObject">
      <div v-if="fieldLabel" class="text-[11px] font-semibold text-muted-foreground">
        {{ fieldLabel }}
        <span v-if="required" class="ml-1 text-[10px] text-danger-foreground">*</span>
      </div>
      <p v-if="schema.description" class="text-[10px] text-faint-foreground">
        {{ schema.description }}
      </p>
      <div class="space-y-2 border-l border-border/60 pl-3">
        <ParameterFormField
          v-for="([key, childSchema], index) in childEntries"
          :key="`${key}-${index}`"
          :schema="childSchema"
          :label="childLabels[key]"
          :name="key"
          :required="childRequired.has(key)"
          :model-value="childValue(key)"
          @update:model-value="(value) => updateChild(key, value)"
        />
      </div>
    </template>
    <template v-else-if="isArray">
      <div v-if="fieldLabel" class="text-[11px] font-semibold text-muted-foreground">
        {{ fieldLabel }}
        <span v-if="required" class="ml-1 text-[10px] text-danger-foreground">*</span>
      </div>
      <p v-if="schema.description" class="text-[10px] text-faint-foreground">
        {{ schema.description }}
      </p>
      <div class="space-y-2 border-l border-border/60 pl-3">
        <div
          v-for="(item, index) in arrayValue"
          :key="index"
          class="flex items-start gap-2"
        >
          <div class="flex-1 min-w-0">
            <ParameterFormField
              :schema="itemSchema"
              :label="itemLabel(index)"
              :name="props.name"
              :model-value="item"
              @update:model-value="(value) => updateArrayItem(index, value)"
            />
          </div>
          <div class="flex flex-col items-center gap-1 pt-5">
            <button
              class="rounded bg-muted px-1.5 py-0.5 text-[10px] text-foreground disabled:opacity-40"
              title="Move up"
              :disabled="index === 0"
              @click="moveItem(index, -1)"
            >
              ↑
            </button>
            <button
              class="rounded bg-muted px-1.5 py-0.5 text-[10px] text-foreground disabled:opacity-40"
              title="Move down"
              :disabled="index === arrayValue.length - 1"
              @click="moveItem(index, 1)"
            >
              ↓
            </button>
            <button
              class="rounded bg-danger px-1.5 py-0.5 text-[10px] text-danger-foreground"
              title="Remove"
              @click="removeArrayItem(index)"
            >
              ✕
            </button>
          </div>
        </div>
        <div>
          <button
            class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground"
            @click="addArrayItem"
            title="Add item"
          >
            Add item
          </button>
        </div>
      </div>
    </template>
    <template v-else-if="isUnsupported">
      <div
        class="rounded border border-warning/40 bg-warning/10 px-2 py-1 text-[10px] text-warning-foreground"
      >
        Unsupported schema type for {{ fieldLabel || 'field' }}.
      </div>
    </template>
    <template v-else>
      <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
        <span class="flex items-center gap-1">
          {{ fieldLabel }}
          <span v-if="required" class="text-[10px] text-danger-foreground">*</span>
        </span>
        <select
          v-if="hasEnum"
          :value="selectValue"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          @change="onSelectChange"
        >
          <option v-if="!required" value="">(unset)</option>
          <option v-for="option in enumOptions" :key="optionKey(option)" :value="String(option)">
            {{ optionLabel(option) }}
          </option>
        </select>
        <textarea
          v-else-if="isMultilineString"
          :value="stringValue"
          :rows="multilineRows"
          :class="textareaClass"
          @input="onStringInput"
        />
        <input
          v-else-if="isNumeric"
          type="number"
          :step="numberStep"
          :value="numberInput"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          @input="onNumberInput"
        />
        <label
          v-else-if="isBoolean"
          class="flex items-center gap-2 text-[11px] text-muted-foreground"
        >
          <input
            type="checkbox"
            :checked="booleanValue"
            class="accent-accent"
            @change="onBooleanChange"
          />
          <span>{{ schema.description ?? 'Enabled' }}</span>
        </label>
        <input
          v-else
          type="text"
          :value="stringValue"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
          @input="onStringInput"
        />
      </label>
      <p v-if="schema.description && !isBoolean" class="text-[10px] text-faint-foreground">
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
  name?: string
}>()

const emit = defineEmits<{
  (event: 'update:model-value', value: unknown): void
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
  if (schema.items) {
    return 'array'
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
const isUnsupported = computed(
  () =>
    !isObject.value &&
    !isArray.value &&
    !isBoolean.value &&
    !isNumeric.value &&
    !hasEnum.value &&
    type.value !== 'string',
)

const enumOptions = computed(() => (Array.isArray(props.schema.enum) ? props.schema.enum : []))
const hasEnum = computed(() => enumOptions.value.length > 0)

const childRequired = computed(
  () => new Set<string>(Array.isArray(props.schema.required) ? props.schema.required : []),
)
const childEntries = computed<[string, Record<string, any>][]>(
  () => Object.entries(props.schema.properties ?? {}) as [string, Record<string, any>][]
)
const childLabels = computed(() => {
  const out: Record<string, string> = {}
  childEntries.value.forEach(([key, schema]) => {
    out[key] = (schema as any)?.title ?? key
  })
  return out
})

function childValue(key: string) {
  if (
    props.modelValue &&
    typeof props.modelValue === 'object' &&
    !Array.isArray(props.modelValue)
  ) {
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
  emit('update:model-value', base)
}

const stringValue = computed(() => (typeof props.modelValue === 'string' ? props.modelValue : ''))

function onStringInput(event: Event) {
  const target = event.target as HTMLInputElement | HTMLTextAreaElement
  const value = target.value
  emit('update:model-value', value === '' && !props.required ? undefined : value)
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
    emit('update:model-value', undefined)
    return
  }
  const parsed = type.value === 'integer' ? parseInt(raw, 10) : parseFloat(raw)
  if (Number.isNaN(parsed)) {
    return
  }
  emit('update:model-value', parsed)
}

const booleanValue = computed(() => Boolean(props.modelValue))

function onBooleanChange(event: Event) {
  const target = event.target as HTMLInputElement
  emit('update:model-value', target.checked)
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
    emit('update:model-value', undefined)
    return
  }
  if (type.value === 'number' || type.value === 'integer') {
    const parsed = type.value === 'integer' ? parseInt(raw, 10) : parseFloat(raw)
    emit('update:model-value', Number.isNaN(parsed) ? undefined : parsed)
    return
  }
  emit('update:model-value', raw)
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

// Multiline string heuristics and styles
const isString = computed(() => type.value === 'string' && !hasEnum.value && !isBoolean.value && !isNumeric.value)

function includesWord(hay: string | undefined, re: RegExp): boolean {
  if (!hay) return false
  return re.test(hay.toLowerCase())
}

const isMultilineString = computed(() => {
  if (!isString.value) return false
  const s = props.schema || {}
  // Explicit schema hints
  if (s.format && typeof s.format === 'string' && s.format.toLowerCase() === 'textarea') return true
  if (s['x-ui'] && String(s['x-ui']).toLowerCase() === 'textarea') return true
  if ((s as any)['x-multiline'] === true) return true
  if (typeof (s as any).contentMediaType === 'string' && (s as any).contentMediaType.startsWith('text/')) return true

  // Heuristics based on field name/label
  const name = (props.name || (props.label ?? (s as any).title) || '').toString().toLowerCase()
  if (includesWord(name, /(patch|diff|body|content|script|message|notes|description|text)/)) return true

  return false
})

const isMonospace = computed(() => {
  const s = props.schema || {}
  if ((s as any)['x-monospace'] === true) return true
  if (typeof (s as any).format === 'string' && /^(code|diff|textarea-code)$/.test((s as any).format.toLowerCase())) return true
  const name = (props.name || (props.label ?? (s as any).title) || '').toString().toLowerCase()
  return includesWord(name, /(patch|diff|code|script|json|yaml)/)
})

const multilineRows = computed(() => (includesWord((props.name || props.label || '').toString(), /(patch|diff)/i) ? 8 : 4))

const textareaClass = computed(() => [
  'rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground overflow-auto',
  isMonospace.value ? 'font-mono' : '',
].join(' '))

// Array handling
const itemSchema = computed(() => {
  const it = (props.schema as any)?.items
  if (Array.isArray(it)) return it[0] ?? { type: 'string' }
  return it ?? { type: 'string' }
})

const arrayValue = computed<unknown[]>(() => (Array.isArray(props.modelValue) ? props.modelValue : []))

function defaultForSchema(s: any): unknown {
  const t = schemaType(s)
  switch (t) {
    case 'object':
      return {}
    case 'number':
    case 'integer':
      return 0
    case 'boolean':
      return false
    case 'array':
      return []
    case 'string':
    default:
      return ''
  }
}

function addArrayItem() {
  const next = arrayValue.value.slice()
  next.push(defaultForSchema(itemSchema.value))
  emit('update:model-value', next)
}

function updateArrayItem(index: number, value: unknown) {
  const next = arrayValue.value.slice()
  if (value === undefined) {
    next.splice(index, 1)
  } else {
    next[index] = value
  }
  emit('update:model-value', next)
}

function removeArrayItem(index: number) {
  const next = arrayValue.value.slice()
  next.splice(index, 1)
  emit('update:model-value', next.length ? next : undefined)
}

function moveItem(index: number, delta: number) {
  const next = arrayValue.value.slice()
  const newIndex = index + delta
  if (newIndex < 0 || newIndex >= next.length) return
  const [item] = next.splice(index, 1)
  next.splice(newIndex, 0, item)
  emit('update:model-value', next)
}

function itemLabel(index: number) {
  return `${props.schema?.items?.title || 'Item'} #${index + 1}`
}
</script>
