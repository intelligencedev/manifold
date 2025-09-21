import type { Config } from 'tailwindcss'

export default {
  content: ['index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        background: 'rgb(var(--color-background) / <alpha-value>)',
        surface: 'rgb(var(--color-surface) / <alpha-value>)',
        'surface-muted': 'rgb(var(--color-surface-muted) / <alpha-value>)',
        border: 'rgb(var(--color-border) / <alpha-value>)',
        input: 'rgb(var(--color-input) / <alpha-value>)',
        ring: 'rgb(var(--color-ring) / <alpha-value>)',
        foreground: 'rgb(var(--color-foreground) / <alpha-value>)',
        'muted-foreground': 'rgb(var(--color-muted-foreground) / <alpha-value>)',
        'subtle-foreground': 'rgb(var(--color-subtle-foreground) / <alpha-value>)',
        'faint-foreground': 'rgb(var(--color-faint-foreground) / <alpha-value>)',
        muted: 'rgb(var(--color-muted) / <alpha-value>)',
        accent: 'rgb(var(--color-accent) / <alpha-value>)',
        'accent-foreground': 'rgb(var(--color-accent-foreground) / <alpha-value>)',
        destructive: 'rgb(var(--color-destructive) / <alpha-value>)',
        'destructive-foreground': 'rgb(var(--color-destructive-foreground) / <alpha-value>)',
        success: 'rgb(var(--color-success) / <alpha-value>)',
        'success-foreground': 'rgb(var(--color-success-foreground) / <alpha-value>)',
        info: 'rgb(var(--color-info) / <alpha-value>)',
        'info-foreground': 'rgb(var(--color-info-foreground) / <alpha-value>)',
        warning: 'rgb(var(--color-warning) / <alpha-value>)',
        'warning-foreground': 'rgb(var(--color-warning-foreground) / <alpha-value>)',
        danger: 'rgb(var(--color-danger) / <alpha-value>)',
        'danger-foreground': 'rgb(var(--color-danger-foreground) / <alpha-value>)',
      },
      borderColor: {
        DEFAULT: 'rgb(var(--color-border) / <alpha-value>)',
      },
      ringColor: {
        DEFAULT: 'rgb(var(--color-ring) / <alpha-value>)',
      },
    },
  },
  plugins: [],
} satisfies Config
