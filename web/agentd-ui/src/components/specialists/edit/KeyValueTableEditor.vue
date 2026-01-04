<template>
  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between gap-2">
      <p v-if="helper" class="text-xs text-subtle-foreground">{{ helper }}</p>
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
          @click="$emit('editJson')"
        >
          Edit as JSON
        </button>
        <button
          type="button"
          class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
          @click="addRow"
        >
          Add row
        </button>
      </div>
    </div>

    <div class="overflow-x-auto rounded border border-border/60">
      <table class="w-full text-sm">
        <thead class="bg-surface-muted/30">
          <tr class="text-left text-xs font-semibold uppercase tracking-wide text-subtle-foreground">
            <th class="px-3 py-2">Key</th>
            <th class="px-3 py-2">Value</th>
            <th class="w-[1%] px-3 py-2"></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/40">
          <tr v-for="(row, idx) in rows" :key="row.id" class="align-top">
            <td class="px-3 py-2">
              <input
                v-model="row.key"
                type="text"
                class="w-full rounded border border-border/60 bg-surface px-2 py-1.5 text-sm"
                @blur="$emit('blur')"
              />
              <p v-if="rowErrors[idx]?.key" class="mt-1 text-xs text-danger-foreground">{{ rowErrors[idx]?.key }}</p>
            </td>
            <td class="px-3 py-2">
              <input
                v-model="row.value"
                type="text"
                class="w-full rounded border border-border/60 bg-surface px-2 py-1.5 text-sm font-mono"
                @blur="$emit('blur')"
              />
              <p v-if="rowErrors[idx]?.value" class="mt-1 text-xs text-danger-foreground">{{ rowErrors[idx]?.value }}</p>
            </td>
            <td class="px-3 py-2">
              <button
                type="button"
                class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
                @click="removeRow(idx)"
                aria-label="Remove row"
                title="Remove row"
              >
                Remove
              </button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="!rows.length" class="px-3 py-3 text-sm text-subtle-foreground">No entries.</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

export type KeyValueRow = { id: string; key: string; value: string }

const props = defineProps<{ helper?: string }>()

defineEmits<{ blur: []; editJson: [] }>()

const rows = defineModel<KeyValueRow[]>({ required: true })

function addRow() {
  rows.value = [...rows.value, { id: crypto.randomUUID(), key: '', value: '' }]
}

function removeRow(idx: number) {
  const next = [...rows.value]
  next.splice(idx, 1)
  rows.value = next
}

const rowErrors = computed(() => {
  const errors: Array<{ key?: string; value?: string }> = []
  const seen = new Map<string, number>()

  rows.value.forEach((row, idx) => {
    const e: { key?: string; value?: string } = {}
    if (!row.key.trim()) {
      e.key = 'Key is required.'
    }
    const normalized = row.key.trim().toLowerCase()
    if (normalized) {
      const prior = seen.get(normalized)
      if (prior != null) {
        e.key = 'Duplicate key.'
        errors[prior] = { ...(errors[prior] || {}), key: 'Duplicate key.' }
      } else {
        seen.set(normalized, idx)
      }
    }
    errors[idx] = { ...(errors[idx] || {}), ...e }
  })

  return errors
})
</script>
