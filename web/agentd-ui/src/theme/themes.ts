export type ThemeId = 'aperture-dark' | 'aperture-light' | 'obsdash-dark'

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

export const defaultDarkTheme: ThemeId = 'aperture-dark'
export const defaultLightTheme: ThemeId = 'aperture-light'

export const themes: ThemeDefinition[] = [
  // Observability — Dark Glass
  {
    id: 'obsdash-dark',
    label: 'Observability (Dark)',
    description: 'Glass dashboard aesthetic with grid glow, crisp strokes, and dual accents.',
    appearance: 'dark',
    tokens: {
      background: '6 8 12',
      surface: '14 18 26',
      'surface-muted': '18 22 32',
      border: '52 60 76',
      input: '28 32 44',
      ring: '118 182 255',
      foreground: '232 238 247',
      'muted-foreground': '166 176 196',
      'subtle-foreground': '128 138 158',
      'faint-foreground': '94 104 124',
      muted: '14 18 26',
      accent: '108 127 255',
      'accent-foreground': '14 16 22',
      destructive: '235 104 96',
      'destructive-foreground': '14 16 22',
      success: '72 214 172',
      'success-foreground': '14 16 22',
      info: '118 182 255',
      'info-foreground': '14 16 22',
      warning: '244 188 110',
      'warning-foreground': '14 16 22',
      danger: '235 104 96',
      'danger-foreground': '14 16 22',
    },
  },
  // Aperture — Dark
  {
    id: 'aperture-dark',
    label: 'Aperture (Dark)',
    description: 'Warm near-black neutrals with Iris primary, hairline strokes, and quiet contrast.',
    appearance: 'dark',
    tokens: {
      background: '7 9 12',
      surface: '20 24 30',
      'surface-muted': '30 36 44',
      border: '48 56 66',
      input: '40 48 60',
      ring: '138 134 255',
      foreground: '242 245 248',
      'muted-foreground': '176 187 200',
      'subtle-foreground': '140 152 167',
      'faint-foreground': '108 120 134',
      muted: '20 24 30',
      accent: '90 89 211',
      'accent-foreground': '255 255 255',
      destructive: '227 93 77',
      'destructive-foreground': '24 24 27',
      success: '34 201 166',
      'success-foreground': '15 23 42',
      info: '69 148 234',
      'info-foreground': '15 23 42',
      warning: '232 177 9',
      'warning-foreground': '24 24 27',
      danger: '227 93 77',
      'danger-foreground': '24 24 27',
    },
  },
  // Aperture — Light
  {
    id: 'aperture-light',
    label: 'Aperture (Light)',
    description: 'Editorial warm neutrals, Iris primary, 1.5px hairlines, and etched materials.',
    appearance: 'light',
    tokens: {
      background: '254 254 252',
      surface: '244 244 240',
      'surface-muted': '234 235 228',
      border: '208 212 204',
      input: '238 238 233',
      ring: '128 124 250',
      foreground: '15 19 22',
      'muted-foreground': '56 65 62',
      'subtle-foreground': '90 100 97',
      'faint-foreground': '126 136 132',
      muted: '244 244 240',
      accent: '90 89 211',
      'accent-foreground': '255 255 255',
      destructive: '227 93 77',
      'destructive-foreground': '255 255 255',
      success: '34 201 166',
      'success-foreground': '6 43 34',
      info: '69 148 234',
      'info-foreground': '255 255 255',
      warning: '232 177 9',
      'warning-foreground': '43 30 0',
      danger: '227 93 77',
      'danger-foreground': '255 255 255',
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

// Notes:
// - The Aperture themes follow the design system in src/theme/aperture.md.
// - Tokens map to CSS custom properties consumed by Tailwind (see tailwind.config.ts).
// - Theme application is handled in the theme store by writing --color-* variables to :root.
