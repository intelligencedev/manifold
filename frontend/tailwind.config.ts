import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{vue,ts,tsx}"],
  theme: {
    extend: {
      colors: {
        background:  "rgb(var(--color-background) / <alpha-value>)",
        surface:     "rgb(var(--color-surface) / <alpha-value>)",
        border:      "rgb(var(--color-border) / <alpha-value>)",
        foreground:  "rgb(var(--color-foreground) / <alpha-value>)",
        accent:      "rgb(var(--color-accent) / <alpha-value>)",
        muted:       "rgb(var(--color-muted) / <alpha-value>)",
        success:     "rgb(var(--color-success) / <alpha-value>)",
        warning:     "rgb(var(--color-warning) / <alpha-value>)",
        danger:      "rgb(var(--color-danger) / <alpha-value>)",
      },
      fontFamily: {
        sans: ["Inter", "ui-sans-serif", "system-ui", "-apple-system", "sans-serif"],
        mono: ["JetBrains Mono", "ui-monospace", "SFMono-Regular", "monospace"],
      },
      animation: {
        "pulse-slow": "pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite",
      },
    },
  },
  plugins: [],
} satisfies Config;
