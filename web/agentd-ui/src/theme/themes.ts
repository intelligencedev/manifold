export type ThemeId =
  | "halo-dark"
  | "halo-light"
  | "halo-sodium"
  | "halo-sodium-light";

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

export const defaultDarkTheme: ThemeId = "halo-sodium";
export const defaultLightTheme: ThemeId = "halo-light";

const darkStatusTokens = {
  destructive: "240 112 95",
  "destructive-foreground": "10 11 18",
  success: "70 211 154",
  "success-foreground": "10 11 18",
  info: "79 214 192",
  warning: "232 177 74",
  danger: "240 112 95",
} satisfies Pick<
  ThemeTokens,
  | "destructive"
  | "destructive-foreground"
  | "success"
  | "success-foreground"
  | "info"
  | "warning"
  | "danger"
>;

const lightStatusTokens = {
  destructive: "240 112 95",
  "destructive-foreground": "255 255 255",
  success: "70 211 154",
  "success-foreground": "255 255 255",
  info: "79 214 192",
  warning: "232 177 74",
  danger: "240 112 95",
  "danger-foreground": "255 255 255",
} satisfies Pick<
  ThemeTokens,
  | "destructive"
  | "destructive-foreground"
  | "success"
  | "success-foreground"
  | "info"
  | "warning"
  | "danger"
  | "danger-foreground"
>;

const haloDarkTokens: ThemeTokens = {
  background: "10 12 18",
  surface: "18 21 30",
  "surface-muted": "27 31 44",
  border: "45 52 69",
  input: "32 37 52",
  ring: "151 164 255",
  foreground: "243 246 251",
  "muted-foreground": "172 181 194",
  "subtle-foreground": "139 150 166",
  "faint-foreground": "94 105 122",
  muted: "15 18 26",
  accent: "151 164 255",
  "accent-foreground": "10 11 18",
  ...darkStatusTokens,
  "info-foreground": "10 11 18",
  "warning-foreground": "10 11 18",
  "danger-foreground": "10 11 18",
};

const haloLightTokens: ThemeTokens = {
  accent: "91 102 230",
  "accent-foreground": "255 255 255",
  border: "213 218 228",
  background: "244 246 251",
  "faint-foreground": "126 137 153",
  foreground: "17 23 34",
  input: "236 240 247",
  muted: "250 251 253",
  "muted-foreground": "72 84 102",
  ring: "91 102 230",
  "subtle-foreground": "91 104 123",
  surface: "255 255 255",
  "surface-muted": "238 242 249",
  ...lightStatusTokens,
  "info-foreground": "6 43 34",
  "warning-foreground": "43 30 0",
};

/** Shared sodium-vapor amber used as the interaction accent. */
const sodiumAccent = "232 177 74";
const sodiumAccentForegroundDark = "10 11 18";
/** Slightly denser amber for light surfaces so nails remain readable. */
const sodiumAccentLight = "196 130 28";
const sodiumAccentForegroundLight = "255 255 255";

export const themes: ThemeDefinition[] = [
  {
    id: "halo-dark",
    label: "Halo (Dark)",
    description:
      "Layered noir control room — deep ink, rounded panels, and a violet command accent.",
    appearance: "dark",
    tokens: haloDarkTokens,
  },
  {
    id: "halo-light",
    label: "Halo (Light)",
    description: "Daytime editorial mode.",
    appearance: "light",
    tokens: haloLightTokens,
  },
  {
    id: "halo-sodium",
    label: "Halo (Sodium)",
    description: "Warm amber alternate for night operation.",
    appearance: "dark",
    tokens: {
      ...haloDarkTokens,
      ring: sodiumAccent,
      accent: sodiumAccent,
      "accent-foreground": sodiumAccentForegroundDark,
    },
  },
  {
    id: "halo-sodium-light",
    label: "Halo (Sodium Light)",
    description: "Warm amber daytime mode — sodium vapor on paper.",
    appearance: "light",
    tokens: {
      ...haloLightTokens,
      ring: sodiumAccentLight,
      accent: sodiumAccentLight,
      "accent-foreground": sodiumAccentForegroundLight,
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
// - Sodium variants swap only the accent/ring family; surfaces match their Halo counterpart.
