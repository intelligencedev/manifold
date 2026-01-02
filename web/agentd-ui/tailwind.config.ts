import type { Config } from 'tailwindcss'

export default {
  content: ['index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  theme: {
    borderWidth: {
      DEFAULT: '1.5px',
      0: '0',
      1: '1px',
      2: '2px',
    },
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
      borderRadius: {
        2: '4px',
        3: '6px',
        4: '9px',
        5: '14px',
        6: '20px',
      },
      boxShadow: {
        0: 'none',
        1: '0 1px 0 rgba(0,0,0,0.05)',
        2: '0 2px 4px rgba(0,0,0,0.06)',
        3: '0 8px 24px rgba(0,0,0,0.10)',
        outline: '0 0 0 3px rgb(var(--color-ring))',
      },
      transitionTimingFunction: {
        'ease-out-custom': 'cubic-bezier(.16,.84,.44,1)',
        'ease-in-custom': 'cubic-bezier(.3,0,.8,.15)',
      },
      animation: {
        'cursor-blink': 'cursorBlink 0.8s steps(2, start) infinite',
      },
      keyframes: {
        cursorBlink: {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0' },
        },
      },
    },
  },
  plugins: [
    function (api: any) {
      api.addUtilities({
        '.etched-light': { boxShadow: 'inset 0 1px 0 rgba(0,0,0,0.04)' },
        '.etched-dark': { boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.03)' },
        '.bg-grid-glow': {
          backgroundImage:
            'radial-gradient(circle at 20% 20%, rgb(var(--color-accent) / 0.18), transparent 32%),' +
            'radial-gradient(circle at 78% 8%, rgb(var(--color-info) / 0.12), transparent 30%),' +
            'radial-gradient(circle at 50% 70%, rgba(255,255,255,0.04), transparent 42%),' +
            'linear-gradient(rgba(255,255,255,0.05) 1px, transparent 1px),' +
            'linear-gradient(90deg, rgba(255,255,255,0.05) 1px, transparent 1px)',
          backgroundSize: '120% 120%, 110% 110%, 140% 140%, 72px 72px, 72px 72px',
          backgroundPosition: 'center center',
        },
        '.bg-grain': {
          backgroundImage: 'url(/assets/noise.png)',
          mixBlendMode: 'soft-light',
          opacity: '0.35',
        },
        '.glass-surface': {
          padding: '16px',
          backgroundImage:
            'linear-gradient(120deg, rgb(var(--color-surface) / 0.82), rgb(var(--color-surface-muted) / 0.76))',
          border: '1px solid rgba(255,255,255,0.08)',
          boxShadow: '0 24px 80px rgba(0,0,0,0.32), inset 0 1px 0 rgba(255,255,255,0.04)',
          backdropFilter: 'blur(18px)',
          WebkitBackdropFilter: 'blur(18px)',
        },
        '.pill-glow': {
          boxShadow:
            '0 0 0 1px rgba(var(--color-accent), 0.35), 0 10px 34px rgba(var(--color-accent), 0.18), 0 1px 0 rgba(255,255,255,0.08)',
        },
      })
    },
  ],
} satisfies Config
