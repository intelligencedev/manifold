export type ThemeId =
  | 'midnight'
  | 'dawn'
  | 'aurora'
  | 'minimalist-future'
  | 'aperture-dark'
  | 'aperture-light'

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
  // Aperture — Dark
  {
    id: 'aperture-dark',
    label: 'Aperture (Dark)',
    description: 'Warm near-black neutrals with Iris primary, hairline strokes, and quiet contrast.',
    appearance: 'dark',
    tokens: {
      // Neutrals (dark)
      background: '7 9 12', // deepened base for stronger figure-ground contrast
      surface: '20 24 30', // lifted cards for clearer separation
      'surface-muted': '30 36 44', // elevated muted surface tone
      border: '48 56 66', // brighter stroke for visible delineation
      input: '40 48 60', // etched controls stand out against surface
      ring: '138 134 255', // higher energy iris ring
      foreground: '242 245 248', // brighter foreground copy
      'muted-foreground': '176 187 200', // clearer secondary text
      'subtle-foreground': '140 152 167', // more legible tertiary text
      'faint-foreground': '108 120 134', // improved icon/meta contrast
      muted: '20 24 30', // aligns with raised surfaces
      // Primary/semantics
      accent: '90 89 211', // iris-9 #5A59D3
      'accent-foreground': '255 255 255',
      destructive: '227 93 77', // coral-6 #E35D4D
      'destructive-foreground': '24 24 27',
      success: '34 201 166', // verdigris-6 #22C9A6
      'success-foreground': '15 23 42',
      info: '69 148 234', // cerulean-6 #4594EA
      'info-foreground': '15 23 42',
      warning: '232 177 9', // citrine-6 #E8B109
      'warning-foreground': '24 24 27',
      danger: '227 93 77', // coral-6 #E35D4D
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
      // Neutrals (light)
      background: '254 254 252', // brighter canvas for sharper figure-ground contrast
      surface: '244 244 240', // darker raised surfaces to create separation
      'surface-muted': '234 235 228', // stronger muted panel tone
      border: '208 212 204', // deeper stroke for clear edges
      input: '238 238 233', // etched control bed
      ring: '128 124 250', // slightly brighter iris ring
      foreground: '15 19 22', // inkier body text
      'muted-foreground': '56 65 62', // bolder secondary copy
      'subtle-foreground': '90 100 97', // improved tertiary contrast
      'faint-foreground': '126 136 132', // clearer metadata/icons
      muted: '244 244 240', // aligns with revised surface tone
      // Primary/semantics
      accent: '90 89 211', // iris-9 #5A59D3
      'accent-foreground': '255 255 255',
      destructive: '227 93 77', // coral-6 #E35D4D
      'destructive-foreground': '255 255 255',
      success: '34 201 166', // verdigris-6 #22C9A6
      'success-foreground': '6 43 34', // dark ink on light success
      info: '69 148 234', // cerulean-6 #4594EA
      'info-foreground': '255 255 255',
      warning: '232 177 9', // citrine-6 #E8B109
      'warning-foreground': '43 30 0',
      danger: '227 93 77', // coral-6
      'danger-foreground': '255 255 255',
    },
  },
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
    id: 'minimalist-future',
    label: 'Minimalist Future',
    description:
      'Ultra-clean light theme: airy neutrals, near-invisible chrome, and an electric jade accent for timeless focus.',
    appearance: 'light',
    tokens: {
      // Foundation — bone white with a fresh mint undertone (not blue like Dawn)
      background: '252 255 252', // bone-mint white
      surface: '255 255 255',
      'surface-muted': '238 250 244', // whisper mint
      border: '186 219 206', // hairline mint-gray
      input: '228 244 238',
      ring: '0 224 189', // neon jade ring

      // Typography — graphite on bone for ultra readability
      foreground: '10 17 24',
      'muted-foreground': '35 48 56',
      'subtle-foreground': '64 84 92',
      'faint-foreground': '98 120 128',

      // Surfaces & accents — singular, futuristic accent
      muted: '241 250 246',
      accent: '0 224 189', // neon jade
      'accent-foreground': '2 10 16',

      // Semantic colors — modern but accessible
      destructive: '255 99 132',
      'destructive-foreground': '255 255 255',
      success: '6 214 160',
      'success-foreground': '2 10 16',
      info: '0 184 255',
      'info-foreground': '2 10 16',
      warning: '255 191 0',
      'warning-foreground': '23 23 23',
      danger: '255 99 132',
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

// Notes:
// - The Aperture themes follow the design system in src/theme/aperture.md.
// - Tokens map to CSS custom properties consumed by Tailwind (see tailwind.config.ts).
// - Theme application is handled in the theme store by writing --color-* variables to :root.
