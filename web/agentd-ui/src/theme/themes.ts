export type ThemeId = "halo-dark" | "halo-light" | "halo-sodium";

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

export const defaultDarkTheme: ThemeId = "halo-dark";
export const defaultLightTheme: ThemeId = "halo-light";

const haloDarkTokens: ThemeTokens = {
  background: "8 9 12",
  surface: "14 17 22",
  "surface-muted": "20 24 33",
  border: "30 35 44",
  input: "26 31 42",
  ring: "124 134 255",
  foreground: "236 239 243",
  "muted-foreground": "154 164 176",
  "subtle-foreground": "154 164 176",
  "faint-foreground": "92 102 114",
  muted: "14 17 22",
  accent: "124 134 255",
  "accent-foreground": "10 11 18",
  destructive: "240 112 95",
  "destructive-foreground": "10 11 18",
  success: "70 211 154",
  "success-foreground": "10 11 18",
  info: "79 214 192",
  "info-foreground": "10 11 18",
  warning: "232 177 74",
  "warning-foreground": "10 11 18",
  danger: "240 112 95",
  "danger-foreground": "10 11 18",
};

export const themes: ThemeDefinition[] = [
  {
    id: "halo-dark",
    label: "Halo (Dark)",
    description:
      "Operator console — deep cool ink, structure from strokes, one periwinkle accent.",
    appearance: "dark",
    tokens: haloDarkTokens,
  },
  {
    id: "halo-light",
    label: "Halo (Light)",
    description: "Daytime editorial mode.",
    appearance: "light",
    tokens: {
      background: "247 247 244",
      surface: "255 255 255",
      "surface-muted": "240 241 244",
      border: "214 218 224",
      input: "241 243 246",
      ring: "90 94 220",
      foreground: "18 21 27",
      "muted-foreground": "74 84 96",
      "subtle-foreground": "92 102 114",
      "faint-foreground": "124 135 148",
      muted: "250 250 252",
      accent: "90 94 220",
      "accent-foreground": "255 255 255",
      destructive: "240 112 95",
      "destructive-foreground": "255 255 255",
      success: "70 211 154",
      "success-foreground": "255 255 255",
      info: "79 214 192",
      "info-foreground": "6 43 34",
      warning: "232 177 74",
      "warning-foreground": "43 30 0",
      danger: "240 112 95",
      "danger-foreground": "255 255 255",
    },
  },
  {
    id: "halo-sodium",
    label: "Halo (Sodium)",
    description: "Warm amber alternate for night operation.",
    appearance: "dark",
    tokens: {
      ...haloDarkTokens,
      ring: "232 177 74",
      accent: "232 177 74",
      "accent-foreground": "10 11 18",
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
// - Halo themes use stroke-based structure with one interaction accent.
// - Tokens map to CSS custom properties consumed by Tailwind (see tailwind.config.ts).
// - Theme application is handled in the theme store by writing --color-* variables to :root.
