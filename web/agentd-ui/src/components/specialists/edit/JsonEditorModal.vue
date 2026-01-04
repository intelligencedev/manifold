<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center px-4 py-8" @keydown="onKeydown">
    <div class="absolute inset-0 bg-surface/70 backdrop-blur-sm" @click="emitCancel"></div>

    <div ref="panel" class="relative z-10 flex w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-border/70 bg-surface shadow-2xl">
      <div class="flex items-center justify-between border-b border-border/60 px-5 py-4">
        <div>
          <h3 class="text-base font-semibold text-foreground">{{ title }}</h3>
          <p v-if="subtitle" class="text-xs text-subtle-foreground">{{ subtitle }}</p>
        </div>
        <button
          ref="closeBtn"
          type="button"
          class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
          @click="emitCancel"
        >
          Close
        </button>
      </div>

      <div class="flex min-h-0 flex-1 flex-col gap-3 px-5 py-4">
        <div v-if="parseError" class="rounded border border-danger/60 bg-danger/10 p-3 text-sm text-danger-foreground">
          {{ parseError }}
        </div>

        <CodeEditor
          v-model="text"
          :showToolbar="true"
          :formatAction="formatJson"
          :defaultWrap="true"
          @blur="validate"
        />
      </div>

      <div class="flex items-center justify-between border-t border-border/60 px-5 py-3">
        <button
          type="button"
          class="rounded border border-border/60 bg-surface px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border"
          @click="emitCancel"
        >
          Cancel
        </button>
        <button
          type="button"
          class="rounded border border-border/60 bg-surface-muted px-3 py-1 text-xs font-semibold text-subtle-foreground hover:border-border disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="!!parseError"
          @click="emitApply"
        >
          Apply
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import CodeEditor from './CodeEditor.vue'

const props = defineProps<{ open: boolean; title: string; subtitle?: string; initialText: string }>()
const emit = defineEmits<{ apply: [any]; cancel: [] }>()

const text = ref('')
const parseError = ref<string | null>(null)

const panel = ref<HTMLElement | null>(null)
const closeBtn = ref<HTMLButtonElement | null>(null)
const restoreFocusEl = ref<HTMLElement | null>(null)

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    restoreFocusEl.value = document.activeElement as HTMLElement | null
    text.value = props.initialText || ''
    parseError.value = null
    await nextTick()
    closeBtn.value?.focus()
    validate()
  },
)

function validate() {
  const raw = text.value.trim()
  if (!raw) {
    parseError.value = 'JSON is required.'
    return null
  }
  try {
    JSON.parse(raw)
    parseError.value = null
    return true
  } catch (e: any) {
    parseError.value = formatJsonError(raw, e)
    return null
  }
}

function formatJson() {
  try {
    const obj = JSON.parse(text.value)
    text.value = JSON.stringify(obj, null, 2)
    parseError.value = null
  } catch (e: any) {
    parseError.value = formatJsonError(text.value, e)
  }
}

function formatJsonError(source: string, err: any): string {
  const msg = String(err?.message || 'Invalid JSON.')
  const m = msg.match(/position\s+(\d+)/i)
  if (!m) return msg

  const pos = Number(m[1])
  if (!Number.isFinite(pos) || pos < 0) return msg

  const prefix = source.slice(0, pos)
  const lines = prefix.split('\n')
  const line = lines.length
  const col = lines[lines.length - 1].length + 1
  return `${msg} (line ${line}, column ${col})`
}

function emitCancel() {
  emit('cancel')
  nextTick(() => restoreFocusEl.value?.focus())
}

function emitApply() {
  if (!validate()) return
  const obj = JSON.parse(text.value)
  emit('apply', obj)
  nextTick(() => restoreFocusEl.value?.focus())
}

function focusables(): HTMLElement[] {
  const root = panel.value
  if (!root) return []
  return Array.from(
    root.querySelectorAll<HTMLElement>(
      'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => !el.hasAttribute('disabled') && el.tabIndex !== -1)
}

function onKeydown(e: KeyboardEvent) {
  if (!props.open) return
  if (e.key === 'Escape') {
    e.preventDefault()
    emitCancel()
    return
  }
  if (e.key !== 'Tab') return
  const els = focusables()
  if (!els.length) return
  const first = els[0]
  const last = els[els.length - 1]
  const active = document.activeElement as HTMLElement | null

  if (e.shiftKey) {
    if (active === first || !rootContains(active)) {
      e.preventDefault()
      last.focus()
    }
  } else {
    if (active === last || !rootContains(active)) {
      e.preventDefault()
      first.focus()
    }
  }
}

function rootContains(el: HTMLElement | null): boolean {
  return !!(panel.value && el && panel.value.contains(el))
}
</script>
