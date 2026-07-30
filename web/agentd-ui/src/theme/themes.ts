export type ThemeId =
  | "desert-night"
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
  | "live"
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

export const defaultDarkTheme: ThemeId = "desert-night";
export const defaultLightTheme: ThemeId = "halo-light";

const desertNight: ThemeTokens = {
  background: "11 12 14",
  surface: "20 22 26",
  "surface-muted": "26 29 34",
  border: "58 52 40",
  input: "26 29 34",
  muted: "16 18 20",
  foreground: "242 236 226",
  "muted-foreground": "184 175 160",
  "subtle-foreground": "140 131 114",
  "faint-foreground": "104 98 86",
  accent: "230 160 32",
  "accent-foreground": "11 12 14",
  ring: "240 190 90",
  live: "62 207 207",
  success: "108 188 120",
  "success-foreground": "11 12 14",
  info: "106 158 196",
  "info-foreground": "11 12 14",
  warning: "212 162 74",
  "warning-foreground": "11 12 14",
  destructive: "208 112 112",
  "destructive-foreground": "11 12 14",
  danger: "208 112 112",
  "danger-foreground": "11 12 14",
};

const haloDark: ThemeTokens = {
  ...desertNight,
  background: "10 12 18",
  surface: "18 21 30",
  "surface-muted": "27 31 44",
  border: "45 52 69",
  input: "32 37 52",
  muted: "15 18 26",
  foreground: "243 246 251",
  "muted-foreground": "172 181 194",
  "subtle-foreground": "139 150 166",
  "faint-foreground": "94 105 122",
  accent: "151 164 255",
  "accent-foreground": "10 11 18",
  ring: "151 164 255",
  live: "79 214 192",
};

const haloLight: ThemeTokens = {
  background: "244 246 251",
  surface: "255 255 255",
  "surface-muted": "238 242 249",
  border: "213 218 228",
  input: "236 240 247",
  muted: "250 251 253",
  foreground: "17 23 34",
  "muted-foreground": "72 84 102",
  "subtle-foreground": "91 104 123",
  "faint-foreground": "126 137 153",
  accent: "91 102 230",
  "accent-foreground": "255 255 255",
  ring: "91 102 230",
  live: "20 146 122",
  destructive: "197 63 75",
  "destructive-foreground": "255 255 255",
  success: "46 125 91",
  "success-foreground": "255 255 255",
  info: "49 126 168",
  "info-foreground": "255 255 255",
  warning: "154 107 19",
  "warning-foreground": "255 255 255",
  danger: "197 63 75",
  "danger-foreground": "255 255 255",
};

export const themes: ThemeDefinition[] = [
  {
    id: "desert-night",
    label: "Desert Night",
    description:
      "Warm graphite, sand-toned type, amber controls, and cyan live state.",
    appearance: "dark",
    tokens: desertNight,
  },
  {
    id: "halo-dark",
    label: "Halo Dark",
    description: "Cool graphite with a violet command accent.",
    appearance: "dark",
    tokens: haloDark,
  },
  {
    id: "halo-light",
    label: "Halo Light",
    description: "A clean daylight workspace.",
    appearance: "light",
    tokens: haloLight,
  },
  {
    id: "halo-sodium",
    label: "Sodium Dark",
    description: "Legacy dark theme retained for saved preferences.",
    appearance: "dark",
    tokens: {
      ...haloDark,
      accent: "232 177 74",
      ring: "232 177 74",
      "accent-foreground": "10 11 18",
    },
  },
  {
    id: "halo-sodium-light",
    label: "Sodium Light",
    description: "Legacy light theme retained for saved preferences.",
    appearance: "light",
    tokens: {
      ...haloLight,
      accent: "196 130 28",
      ring: "196 130 28",
    },
  },
];

const themeRegistry = new Map(themes.map((theme) => [theme.id, theme]));

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
