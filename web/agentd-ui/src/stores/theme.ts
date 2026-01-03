import { computed, onScopeDispose, ref, watch } from 'vue'
import { defineStore } from 'pinia'
import {
  defaultDarkTheme,
  defaultLightTheme,
  getTheme,
  isThemeId,
  resolveSystemTheme,
  themeOptions,
  type ThemeChoice,
  type ThemeDefinition,
  type ThemeId,
} from '@/theme/themes'

const STORAGE_KEY = 'agentd.ui.theme-choice'
const isClient = typeof window !== 'undefined'
const mediaQuery = isClient ? window.matchMedia('(prefers-color-scheme: dark)') : null

function systemPrefersDark(): boolean {
  return mediaQuery?.matches ?? true
}

function applyTheme(theme: ThemeDefinition) {
  if (!isClient) return
  const root = document.documentElement
  const body = document.body
  root.dataset.theme = theme.id
  body.classList.toggle('theme-obsdash', theme.id === 'obsdash-dark')
  root.style.colorScheme = theme.appearance
  Object.entries(theme.tokens).forEach(([token, value]) => {
    root.style.setProperty(`--color-${token}`, value)
  })
}

export const useThemeStore = defineStore('theme', () => {
  const selection = ref<ThemeChoice>('system')

  if (isClient) {
    const params = new URLSearchParams(window.location.search)
    const flaggedTheme = params.get('uiTheme')
    if (flaggedTheme && isThemeId(flaggedTheme)) {
      selection.value = flaggedTheme
    } else {
      const stored = window.localStorage.getItem(STORAGE_KEY)
      if (stored === 'system' || (stored && isThemeId(stored))) {
        selection.value = stored as ThemeChoice
      }
    }
  }

  const resolvedThemeId = computed<ThemeId>(() => {
    if (selection.value === 'system') {
      return resolveSystemTheme(systemPrefersDark())
    }
    return selection.value
  })

  const resolvedTheme = computed(() => getTheme(resolvedThemeId.value))

  const options = computed(() => [
    {
      id: 'system' as const,
      label: 'System',
      description: 'Use the OS preference',
      appearance: systemPrefersDark() ? 'dark' : 'light',
    },
    ...themeOptions,
  ])

  watch(
    selection,
    (value) => {
      if (!isClient) return
      window.localStorage.setItem(STORAGE_KEY, value)
    },
    { flush: 'post' },
  )

  watch(resolvedTheme, (theme) => applyTheme(theme), { immediate: true })

  const handleSystemChange = () => {
    if (selection.value === 'system') {
      applyTheme(resolvedTheme.value)
    }
  }

  mediaQuery?.addEventListener('change', handleSystemChange)
  onScopeDispose(() => {
    mediaQuery?.removeEventListener('change', handleSystemChange)
  })

  function setTheme(choice: ThemeChoice) {
    selection.value = choice
  }

  function cycleTheme() {
    const order: ThemeChoice[] = ['obsdash-dark', defaultDarkTheme]
    const currentIndex = order.indexOf(selection.value)
    const next = order[(currentIndex + 1) % order.length]
    selection.value = next
  }

  return {
    selection,
    resolvedTheme,
    resolvedThemeId,
    options,
    setTheme,
    cycleTheme,
  }
})
