export type ThemeId = "aperture-dark" | "aperture-light" | "obsdash-dark";

export type ThemeTokenName =
  | "background"
  | "surface"
  | "surface-muted"
  | "border"
  | "input"
  | "ring"
  | "foreground"
  | "muted-foreground"
  | "subtle-foreground"
  | "faint-foreground"
  | "muted"
  | "accent"
  | "accent-foreground"
  | "destructive"
  | "destructive-foreground"
  | "success"
  | "success-foreground"
  | "info"
  | "info-foreground"
  | "warning"
  | "warning-foreground"
  | "danger"
  | "danger-foreground";

export type ThemeTokens = Record<ThemeTokenName, string>;

export type ThemeDefinition = {
  id: ThemeId;
  label: string;
  description: string;
  appearance: "light" | "dark";
  tokens: ThemeTokens;
};

export const defaultDarkTheme: ThemeId = "aperture-dark";
export const defaultLightTheme: ThemeId = "aperture-light";

export const themes: ThemeDefinition[] = [
  // Observability — Dark Glass
  {
    id: "obsdash-dark",
    label: "Observability (Dark)",
    description:
      "Graphite telemetry UI with reduced contrast and signal-blue highlights.",
    appearance: "dark",
    tokens: {
      background: "8 11 14",
      surface: "18 22 27",
      "surface-muted": "24 29 35",
      border: "66 77 91",
      input: "23 28 34",
      ring: "127 176 255",
      foreground: "238 242 248",
      "muted-foreground": "168 177 190",
      "subtle-foreground": "132 141 154",
      "faint-foreground": "95 104 117",
      muted: "18 22 27",
      accent: "102 163 255",
      "accent-foreground": "9 16 28",
      destructive: "232 99 92",
      "destructive-foreground": "255 255 255",
      success: "78 210 160",
      "success-foreground": "11 31 24",
      info: "102 163 255",
      "info-foreground": "9 16 28",
      warning: "238 184 71",
      "warning-foreground": "43 30 0",
      danger: "232 99 92",
      "danger-foreground": "255 255 255",
    },
  },
  // Aperture — Dark
  {
    id: "aperture-dark",
    label: "Aperture (Dark)",
    description:
      "Reduced graphite palette with precise borders and signal-blue accents.",
    appearance: "dark",
    tokens: {
      background: "15 16 19",
      surface: "24 26 31",
      "surface-muted": "33 35 41",
      border: "70 74 84",
      input: "29 31 37",
      ring: "127 176 255",
      foreground: "238 241 246",
      "muted-foreground": "171 179 191",
      "subtle-foreground": "132 141 155",
      "faint-foreground": "100 108 120",
      muted: "24 26 31",
      accent: "102 163 255",
      "accent-foreground": "255 255 255",
      destructive: "232 99 92",
      "destructive-foreground": "255 255 255",
      success: "78 210 160",
      "success-foreground": "10 31 24",
      info: "102 163 255",
      "info-foreground": "11 19 33",
      warning: "238 184 71",
      "warning-foreground": "43 30 0",
      danger: "232 99 92",
      "danger-foreground": "255 255 255",
    },
  },
  // Aperture — Light
  {
    id: "aperture-light",
    label: "Aperture (Light)",
    description:
      "Cool editorial light UI with clear structure and restrained contrast.",
    appearance: "light",
    tokens: {
      background: "245 247 250",
      surface: "255 255 255",
      "surface-muted": "238 241 246",
      border: "205 212 223",
      input: "240 243 248",
      ring: "71 107 219",
      foreground: "18 24 33",
      "muted-foreground": "82 94 108",
      "subtle-foreground": "106 118 132",
      "faint-foreground": "135 145 158",
      muted: "255 255 255",
      accent: "71 107 219",
      "accent-foreground": "255 255 255",
      destructive: "213 71 66",
      "destructive-foreground": "255 255 255",
      success: "35 168 119",
      "success-foreground": "255 255 255",
      info: "71 107 219",
      "info-foreground": "255 255 255",
      warning: "202 138 4",
      "warning-foreground": "43 30 0",
      danger: "213 71 66",
      "danger-foreground": "255 255 255",
    },
  },
];

const themeRegistry = new Map<ThemeId, ThemeDefinition>(
  themes.map((theme) => [theme.id, theme]),
);

export function getTheme(id: ThemeId): ThemeDefinition {
  return themeRegistry.get(id) ?? themeRegistry.get(defaultDarkTheme)!;
}

export type ThemeChoice = ThemeId | "system";

export function isThemeId(value: string): value is ThemeId {
  return themeRegistry.has(value as ThemeId);
}

export function resolveSystemTheme(isDark: boolean): ThemeId {
  return isDark ? defaultDarkTheme : defaultLightTheme;
}

export const themeOptions = themes.map((theme) => ({
  id: theme.id,
  label: theme.label,
  description: theme.description,
  appearance: theme.appearance,
}));

// Notes:
// - The Aperture themes follow the design system in src/theme/aperture.md.
// - Tokens map to CSS custom properties consumed by Tailwind (see tailwind.config.ts).
// - Theme application is handled in the theme store by writing --color-* variables to :root.
