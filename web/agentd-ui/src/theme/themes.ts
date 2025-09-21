export type ThemeId = 'midnight' | 'dawn' | 'aurora'

export type ThemeTokenName =
  | 'background'
  | 'surface'
  | 'surface-muted'
  | 'border'
  | 'input'
  | 'ring'
  | 'foreground'
  | 'muted-foreground'
  | 'subtle-foreground'
  | 'faint-foreground'
  | 'muted'
  | 'accent'
  | 'accent-foreground'
  | 'destructive'
  | 'destructive-foreground'
  | 'success'
  | 'success-foreground'
  | 'info'
  | 'info-foreground'
  | 'warning'
  | 'warning-foreground'
  | 'danger'
  | 'danger-foreground'

export type ThemeTokens = Record<ThemeTokenName, string>

export type ThemeDefinition = {
  id: ThemeId
  label: string
  description: string
  appearance: 'light' | 'dark'
  tokens: ThemeTokens
}

export const defaultDarkTheme: ThemeId = 'midnight'
export const defaultLightTheme: ThemeId = 'dawn'

export const themes: ThemeDefinition[] = [
  {
    id: 'midnight',
    label: 'Midnight',
    description: 'High contrast dark palette inspired by the original styling.',
    appearance: 'dark',
    tokens: {
      background: '2 6 23',
      surface: '15 23 42',
      'surface-muted': '30 41 59',
      border: '51 65 85',
      input: '30 41 59',
      ring: '16 185 129',
      foreground: '226 232 240',
      'muted-foreground': '203 213 225',
      'subtle-foreground': '148 163 184',
      'faint-foreground': '100 116 139',
      muted: '15 23 42',
      accent: '52 211 153',
      'accent-foreground': '15 23 42',
      destructive: '248 113 113',
      'destructive-foreground': '24 24 27',
      success: '52 211 153',
      'success-foreground': '15 23 42',
      info: '56 189 248',
      'info-foreground': '15 23 42',
      warning: '251 191 36',
      'warning-foreground': '24 24 27',
      danger: '248 113 113',
      'danger-foreground': '24 24 27',
    },
  },
  {
    id: 'dawn',
    label: 'Dawn',
    description: 'Light theme with soft gray neutrals and blue accents.',
    appearance: 'light',
    tokens: {
      background: '248 250 252',
      surface: '255 255 255',
      'surface-muted': '241 245 249',
      border: '203 213 225',
      input: '226 232 240',
      ring: '59 130 246',
      foreground: '15 23 42',
      'muted-foreground': '30 41 59',
      'subtle-foreground': '71 85 105',
      'faint-foreground': '100 116 139',
      muted: '241 245 249',
      accent: '59 130 246',
      'accent-foreground': '255 255 255',
      destructive: '220 38 38',
      'destructive-foreground': '255 255 255',
      success: '34 197 94',
      'success-foreground': '255 255 255',
      info: '14 165 233',
      'info-foreground': '255 255 255',
      warning: '234 179 8',
      'warning-foreground': '23 23 23',
      danger: '239 68 68',
      'danger-foreground': '255 255 255',
    },
  },
  {
    id: 'aurora',
    label: 'Aurora',
    description: 'Dark teal and violet blend for dashboards that need a little flair.',
    appearance: 'dark',
    tokens: {
      background: '9 12 24',
      surface: '18 24 40',
      'surface-muted': '32 44 67',
      border: '62 80 109',
      input: '32 44 67',
      ring: '129 140 248',
      foreground: '224 231 255',
      'muted-foreground': '196 203 233',
      'subtle-foreground': '165 180 203',
      'faint-foreground': '148 163 184',
      muted: '18 24 40',
      accent: '129 140 248',
      'accent-foreground': '15 23 42',
      destructive: '248 113 113',
      'destructive-foreground': '24 24 27',
      success: '45 212 191',
      'success-foreground': '15 23 42',
      info: '96 165 250',
      'info-foreground': '15 23 42',
      warning: '250 204 21',
      'warning-foreground': '24 24 27',
      danger: '252 165 165',
      'danger-foreground': '24 24 27',
    },
  },
]

const themeRegistry = new Map<ThemeId, ThemeDefinition>(themes.map((theme) => [theme.id, theme]))

export function getTheme(id: ThemeId): ThemeDefinition {
  return themeRegistry.get(id) ?? themeRegistry.get(defaultDarkTheme)!
}

export type ThemeChoice = ThemeId | 'system'

export function isThemeId(value: string): value is ThemeId {
  return themeRegistry.has(value as ThemeId)
}

export function resolveSystemTheme(isDark: boolean): ThemeId {
  return isDark ? defaultDarkTheme : defaultLightTheme
}

export const themeOptions = themes.map((theme) => ({
  id: theme.id,
  label: theme.label,
  description: theme.description,
  appearance: theme.appearance,
}))
