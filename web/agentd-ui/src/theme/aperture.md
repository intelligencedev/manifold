# Aperture — A Distinct UI Theme for an LLM Agents Platform
A comprehensive design system and implementation guide that avoids the “typical Tailwind look” through editorial typography, warm neutrals, hairline geometry, subtle materials, and purposeful motion.

This guide covers: principles, tokens (color, type, spacing, radii, elevation), light/dark modes, accessibility, motion, component blueprints, Tailwind config, CSS variables, examples, and rollout.

---

## 0) Design Direction and Principles

- Voice: Technical, editorial, confident
- Feel: Quietly premium; precise, not flashy. Intentional whitespace with compact density options for tool-heavy screens.
- Identity: Warm neutrals, iris-violet primary, hairline borders (1.5px), etched controls (subtle inner shadow), non-standard radii (9px base), and structured layouts (strict rhythm). No gradient-chrome, no stock “blue-500 on zinc-100”.
- Practicality: Optimized for:
  - Dense textual output (chat, logs)
  - Code and JSON readability
  - Tool/agent affordances and provenance
  - High-contrast states and focus for power users

What we explicitly avoid:
- Inter + blue-500 + gray-100 + default rounded-md shadows
- Overly large paddings, default Tailwind radii, generic ring styles
- Monolithic flat surfaces with no texture or separation cues

---

## 1) Color System

We use warm neutrals and a restrained accent palette. Provide these as CSS variables (tokens) and map semantic colors to them. We define both Light and Dark mode values; semantic tokens stay the same.

### 1.1 Neutral Palette (12-step; warm, editorial)
Neutral names: Bone → Fog → Mist → Ash → Pewter → Stone → Slate → Graphite → Carbon → Soot → Pitch

- Light mode neutrals (1=lightest → 12=darkest)
  - neutral-1:  #FCFCFA (Bone)
  - neutral-2:  #F7F7F4 (Fog)
  - neutral-3:  #F2F2EF (Mist)
  - neutral-4:  #EAECE7 (Ash)
  - neutral-5:  #E0E3DD (Pewter)
  - neutral-6:  #D1D6CF (Stone)
  - neutral-7:  #BAC1B9 (Slate)
  - neutral-8:  #9DA7A0 (Graphite)
  - neutral-9:  #7D8881 (Carbon)
  - neutral-10: #5B6560 (Soot)
  - neutral-11: #3B4240 (Pitch-1)
  - neutral-12: #161A1A (Pitch-2)

- Dark mode neutrals (1=darkest surface → 12=lightest text)
  - neutral-1:  #0C0E10
  - neutral-2:  #111417
  - neutral-3:  #161A1E
  - neutral-4:  #1C2126
  - neutral-5:  #232A31
  - neutral-6:  #2D353E
  - neutral-7:  #3A454F
  - neutral-8:  #4D5A66
  - neutral-9:  #687784
  - neutral-10: #8A97A3
  - neutral-11: #B4BEC7
  - neutral-12: #E7EBEF

Rationale: Warm neutrals avoid the cold “zinc” look; they carry a more editorial tone. The dark set is tuned so text remains legible and quiet on near-black.

### 1.2 Accents and Semantics (10–12 steps)

Primary (Iris: refined violet-blue)
- iris-1:  #F8F8FF
- iris-2:  #F0EFFF
- iris-3:  #E3E3FF
- iris-4:  #D3D0FF
- iris-5:  #BDB9FF
- iris-6:  #A29EFF
- iris-7:  #8A86F6
- iris-8:  #706DE6
- iris-9:  #5A59D3  (Primary.DEFAULT)
- iris-10: #4A49B8 (Primary.hover)
- iris-11: #383795 (Primary.deep)
- iris-12: #242466 (Primary.dark)

Info (Cerulean)
- cerulean-6: #4594EA (DEFAULT)
- cerulean-8: #2266AD (hover)
- cerulean-11:#163B62 (deep)

Success (Verdigris)
- verdigris-6: #22C9A6 (DEFAULT)
- verdigris-8: #0AA384 (hover)
- verdigris-11:#0E5A4A (deep)

Warning (Citrine)
- citrine-6: #E8B109 (DEFAULT)
- citrine-8: #B88300 (hover)
- citrine-11:#533B00 (deep)

Danger (Coral)
- coral-6: #E35D4D (DEFAULT)
- coral-8: #B24334 (hover)
- coral-11:#4F1F18 (deep)

On-colors (for text/icons on colored backgrounds)
- on-primary:  #FFFFFF
- on-info:     #FFFFFF
- on-success:  #062B22 (dark-ink on light success) or #FFFFFF when used as solid fill
- on-warning:  #2B1E00 (dark-ink on light warning) or #FFFFFF when solid
- on-danger:   #FFFFFF

### 1.3 Surfaces and Structure
- surface-1 (base): light: neutral-1; dark: neutral-1
- surface-2 (raised): light: #F7F7F4 with subtle noise; dark: #111417 with subtle noise
- surface-3 (floating): light: #FFFFFF with 6–8% tint; dark: #161A1E with 4–6% tint
- strokes (hairlines):
  - stroke-soft: neutral-4 (light) / neutral-3 (dark)
  - stroke-hard: neutral-6 (light) / neutral-5 (dark)
- selection: iris-3 (light) / iris-10 (dark)
- focus-ring: iris-8 (outer 3px with 1px offset)

Material touch:
- Backdrop blur for floating layers at 8–12px
- Subtle noise texture (2–3% opacity) on raised surfaces to avoid flatness
- 1.5px control borders for a crafted, instrument-like feel

Accessibility note: Always ensure text/surface contrast ≥ 4.5:1 (prefer 7:1 for body).

---

## 2) Typography

Distinct from default Tailwind. Editorial, compact, highly legible.

- Sans: IBM Plex Sans Var, fallback: "Public Sans", system-ui, "Segoe UI", Roboto
- Mono: JetBrains Mono, fallback: "IBM Plex Mono", ui-monospace, SFMono-Regular

Variable axis (if available):
- Sans: opsz 14–22; wght 350–700
- Mono: wght 400–600

Base rhythm:
- Root font-size: 16px
- Line-height defaults:
  - Display: 1.15
  - Headings: 1.2
  - Body: 1.45
  - Dense UI microcopy: 1.35

Scale (modular 1.125; adjust for screen size):
- Display-XL: 40/46, wght 600 (H1)
- Display-L: 34/40, wght 600 (H2)
- Title: 28/34, wght 550 (H3)
- Subtitle: 24/30, wght 500 (H4)
- Body-L: 18/26, wght 400–450
- Body-M: 16/24, wght 400–450 (default)
- Body-S: 14/20, wght 400–450
- Micro: 12/16, wght 450–500, letterspacing +0.2–0.3px (labels, meta)

Uppercase labels:
- 12/16, tracking +4% to +6%, wght 500

Code and preformatted:
- 13–14px by default, 20–22px line-height
- Token contrast calm, not neon; emphasize strings/functions subtly

Max line length:
- Long-form content (chat): 66–72ch
- Tool outputs (tables/logs): allow 90–110ch grids

---

## 3) Spacing, Layout, Shape

### 3.1 Spacing Scale (non-linear; breaks the “typical Tailwind rhythm”)
- 2, 4, 6, 8, 12, 16, 20, 24, 28, 32, 40, 48, 56, 64

Usage shorthand:
- xs: 6
- sm: 8
- md: 12–16
- lg: 20–24
- xl: 32–40

Density presets:
- Cozy (default): paddings use md; rows 40px
- Compact (data-heavy): paddings use sm; rows 32–36px
- Comfortable (marketing/docs): paddings use lg

### 3.2 Shape Language
- Base radius: 9px (distinct from md/lg defaults)
- Radii:
  - r-2: 4px
  - r-3: 6px
  - r-4: 9px (base)
  - r-5: 14px
  - r-6: 20px
  - pill: 9999px
- Borders: default 1.5px for controls
- Etched controls: inner-shadow 0 1px 0 rgba(0,0,0,0.04) on light; 0 1px 0 rgba(255,255,255,0.03) on dark

### 3.3 Elevation and Dividers
- Shadows (y-only; crisp):
  - shadow-0: none
  - shadow-1: 0 1px 0 rgba(0,0,0,0.05)
  - shadow-2: 0 2px 4px rgba(0,0,0,0.06)
  - shadow-3: 0 8px 24px rgba(0,0,0,0.10)
- Dividers: use neutral-4/3 with 0.75–1px (render as 1px; optional pseudo-element for 0.5px look at 2x DPR)

Surface stack (ASCII)
- Floating ── border: stroke-hard · shadow-3 · blur
- Raised ───── border: stroke-soft · shadow-2 · subtle noise
- Base ─────── background only + grid/section dividers

---

## 4) Interaction States and Motion

States
- Hover: elevate or lighten/darken by ~4–6%; never only color-shift text
- Active/Pressed: reduce elevation; darken background by ~8–10%
- Focus: 3px outer ring, iris-8, 1px offset; always visible on keyboard nav
- Disabled: reduce contrast by ~40%, remove shadow/outline
- Selected: add 1.5px inner stroke with accent tint, or subtle 2px inset ring

Motion
- Respect prefers-reduced-motion: reduce to opacity/position fades only
- Durations:
  - Micro: 90ms (buttons, toggles)
  - Standard: 140ms (menus, tabs)
  - Overlay: 220ms (modals, sheets)
- Easing:
  - out: cubic-bezier(.16,.84,.44,1)
  - in:  cubic-bezier(.3,0,.8,.15)
  - spring-like (JS): tension 280, friction 28

---

## 5) Accessibility

- Contrast: WCAG AA for body (≥ 4.5:1), AAA where feasible for dense text
- Min touch target: 40×40px
- Focus-visible: always; no outline removal without replacement
- Color-blind safety: don’t rely solely on color; pair with icons/patterns
- Text selection: visible but not overly saturated
- RTL: ensure icons and arrows flip; padding and tabs mirror
- Keyboard: logical tab order; large, persistent focus rings on interactive elements

---

## 6) Data Visualization Palette

Categorical (10 colors, color-blind–aware emphasis)
- 1 Iris:      #5A59D3
- 2 Cerulean:  #4594EA
- 3 Verdigris: #22C9A6
- 4 Citrine:   #E8B109
- 5 Coral:     #E35D4D
- 6 Teal:      #1FA2A0
- 7 Orchid:    #B05BD6
- 8 Saffron:   #F2A03A
- 9 Emerald:   #2AA657
- 10 Slate:    #6A7A8C

Use neutral-10 for axes, neutral-8 for gridlines. In dark mode, lighten gridlines slightly (neutral-7 to neutral-8).

---

## 7) Component Blueprints

All examples use the new tokens and radii; adjust for density.

### 7.1 Buttons
- Primary (solid)
  - bg: iris-9; text: on-primary; border: transparent; hover: iris-10; active: iris-11; focus ring: iris-8
- Subtle (tinted)
  - bg: iris-2; text: iris-11; border: iris-3; hover: iris-3; active: iris-4
- Ghost
  - bg: transparent; text: iris-9; border: transparent; hover: neutral-3; active: neutral-4
- Outline
  - bg: surface-1; text: iris-10; border: iris-7@30%; hover: iris-8@12%

Sizing:
- Sm: h-9 (36px) px-3 radius-9
- Md: h-10 (40px) px-4 radius-9
- Lg: h-11 (44px) px-5 radius-14

### 7.2 Inputs (Text, Textarea)
- Height: 40px default (cozy), 36px compact
- Border: 1.5px stroke-soft; radius: 9px; inner shadow subtle
- Bg: surface-1 (or 2 in dense panels)
- Placeholder: neutral-9
- Focus: 3px outer iris ring, border iris-8, light glow

Validation:
- Success: border verdigris-7; subtle icon
- Error: border coral-7; help text coral-8; never red-only

### 7.3 Segmented Control / Tabs
- Container: surface-2, 1.5px stroke-soft, radius 14px
- Item: pill radius; selected has solid iris-9 bg + on-primary text; hover uses iris-3 tint

### 7.4 Panels and Toolbars
- Panel: surface-2, border stroke-soft, radius 14px, shadow-2, noise texture
- Toolbar: 48px height; items 40px high; separators 1px neutral-4

### 7.5 Tables
- Header: 44px; sticky; background surface-2; bottom hairline divider
- Rows: 40px; zebra optional neutral-2; hover neutral-3
- Selection: 2px inset iris-8 tint; never heavy background only
- Dense mode: 36px rows; 12px horizontal cell padding

### 7.6 Chat Messages
- Assistant bubble: surface-2 with subtle iris-2 tint; 1.5px border stroke-soft; radius 14px; max-width 72ch
- User bubble: surface-3; slightly higher elevation
- Tool call block: inset panel inside message; subtle left accent bar (iris-8@20%)
- Streaming cursor: block caret with smooth blink (800ms)
- Metadata row: Micro type, neutral-9, spaced with 6px gap

### 7.7 Code and JSON Blocks
- Mono 13–14px, 20–22px line-height
- Background: neutral-2 light / neutral-2 dark; border stroke-soft; radius 9px
- Line numbers: neutral-9
- Selection highlight: iris-3 / iris-10
- Token colors: calm; strings slightly green-ish, keywords muted violet, numbers teal
- Copy button: ghost style top-right

### 7.8 Cards (Agents/Tools)
- Cover: surface-3; radius 14px; shadow-2
- Avatar: 36–48px, with presence ring (status color)
- Actions: ghost buttons; hover show

### 7.9 Toasts, Modals, Tooltips
- Toast: surface-3; border stroke-soft; shadow-3; left stripe by semantic color
- Modal: backdrop color with 10–14% black overlay + blur 8–12px
- Tooltip: neutral-12 bg (dark), on-surface text; radius 6px

---

## 8) Tailwind Integration

We use CSS variables for colors, semantics, and some layout tokens. We also set default border width to 1.5px and introduce custom radii/shadows.

### 8.1 CSS Variables (base.css)
```css
/* base.css */
:root {
  /* Neutrals (light) */
  --neutral-1:  #FCFCFA;
  --neutral-2:  #F7F7F4;
  --neutral-3:  #F2F2EF;
  --neutral-4:  #EAECE7;
  --neutral-5:  #E0E3DD;
  --neutral-6:  #D1D6CF;
  --neutral-7:  #BAC1B9;
  --neutral-8:  #9DA7A0;
  --neutral-9:  #7D8881;
  --neutral-10: #5B6560;
  --neutral-11: #3B4240;
  --neutral-12: #161A1A;

  /* Accents */
  --iris-1:#F8F8FF; --iris-2:#F0EFFF; --iris-3:#E3E3FF; --iris-4:#D3D0FF;
  --iris-5:#BDB9FF; --iris-6:#A29EFF; --iris-7:#8A86F6; --iris-8:#706DE6;
  --iris-9:#5A59D3; --iris-10:#4A49B8; --iris-11:#383795; --iris-12:#242466;
  --cerulean-6:#4594EA; --cerulean-8:#2266AD; --cerulean-11:#163B62;
  --verdigris-6:#22C9A6; --verdigris-8:#0AA384; --verdigris-11:#0E5A4A;
  --citrine-6:#E8B109; --citrine-8:#B88300; --citrine-11:#533B00;
  --coral-6:#E35D4D; --coral-8:#B24334; --coral-11:#4F1F18;

  /* Semantic surface tokens */
  --bg: var(--neutral-1);
  --surface-1: var(--neutral-1);
  --surface-2: var(--neutral-2);
  --surface-3: #FFFFFF;
  --text: var(--neutral-12);
  --text-subtle: var(--neutral-10);
  --stroke-soft: var(--neutral-4);
  --stroke-hard: var(--neutral-6);

  /* Semantics */
  --primary: var(--iris-9);
  --primary-hover: var(--iris-10);
  --on-primary: #FFFFFF;

  --info: var(--cerulean-6);
  --success: var(--verdigris-6);
  --warning: var(--citrine-6);
  --danger: var(--coral-6);

  --focus-ring: var(--iris-8);

  /* Spacing tokens (optional naming helpers) */
  --space-2: 2px; --space-4: 4px; --space-6: 6px; --space-8: 8px;
  --space-12: 12px; --space-16: 16px; --space-20: 20px; --space-24: 24px;
  --space-28: 28px; --space-32: 32px; --space-40: 40px; --space-48: 48px;
  --space-56: 56px; --space-64: 64px;
}

.theme-dark {
  /* Neutrals (dark) */
  --neutral-1:  #0C0E10;
  --neutral-2:  #111417;
  --neutral-3:  #161A1E;
  --neutral-4:  #1C2126;
  --neutral-5:  #232A31;
  --neutral-6:  #2D353E;
  --neutral-7:  #3A454F;
  --neutral-8:  #4D5A66;
  --neutral-9:  #687784;
  --neutral-10: #8A97A3;
  --neutral-11: #B4BEC7;
  --neutral-12: #E7EBEF;

  /* Surfaces (dark) */
  --bg: var(--neutral-1);
  --surface-1: var(--neutral-1);
  --surface-2: var(--neutral-2);
  --surface-3: var(--neutral-3);
  --text: var(--neutral-12);
  --text-subtle: var(--neutral-10);
  --stroke-soft: var(--neutral-3);
  --stroke-hard: var(--neutral-5);

  /* Accents remain; on-colors still valid */
}

@media (prefers-color-scheme: dark) {
  :root:not(.theme-light) { color-scheme: dark; }
}

/* Base text + smoothing */
@layer base {
  html { font-feature-settings: "cv02","cv03","cv04","cv11"; }
  body {
    background: var(--bg);
    color: var(--text);
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
    font-family: "IBM Plex Sans", "Public Sans", system-ui, "Segoe UI", Roboto, Arial, sans-serif;
    text-rendering: optimizeLegibility;
  }
  ::selection { background: var(--iris-3); color: var(--neutral-12); }
  .theme-dark ::selection { background: var(--iris-10); color: #fff; }
}

/* Focus-visible ring (consistent across components) */
.focus-ring:focus-visible {
  outline: 3px solid var(--focus-ring);
  outline-offset: 2px;
}

/* Subtle noise for raised surfaces */
.surface-noise {
  background-image:
    radial-gradient(rgba(0,0,0,0.03) 1px, transparent 1px);
  background-size: 3px 3px;
  background-blend-mode: multiply;
}
```

### 8.2 Tailwind Config (tailwind.config.js)
```js
// tailwind.config.js
module.exports = {
  darkMode: ['class', '[data-theme="dark"]'],
  content: ['./src/**/*.{js,ts,jsx,tsx,mdx,html}'],
  theme: {
    borderWidth: {
      DEFAULT: '1.5px',
      0: '0',
      1: '1px',
      2: '2px',
    },
    extend: {
      colors: {
        bg: 'var(--bg)',
        text: 'var(--text)',
        'text-subtle': 'var(--text-subtle)',
        primary: {
          DEFAULT: 'var(--primary)',
          hover: 'var(--primary-hover)',
          fg: 'var(--on-primary)',
        },
        info: 'var(--info)',
        success: 'var(--success)',
        warning: 'var(--warning)',
        danger: 'var(--danger)',
        surface: {
          1: 'var(--surface-1)',
          2: 'var(--surface-2)',
          3: 'var(--surface-3)',
        },
        stroke: {
          soft: 'var(--stroke-soft)',
          hard: 'var(--stroke-hard)',
        },
        neutral: {
          1: 'var(--neutral-1)',
          2: 'var(--neutral-2)',
          3: 'var(--neutral-3)',
          4: 'var(--neutral-4)',
          5: 'var(--neutral-5)',
          6: 'var(--neutral-6)',
          7: 'var(--neutral-7)',
          8: 'var(--neutral-8)',
          9: 'var(--neutral-9)',
          10: 'var(--neutral-10)',
          11: 'var(--neutral-11)',
          12: 'var(--neutral-12)',
        },
      },
      fontFamily: {
        sans: ['IBM Plex Sans', 'Public Sans', 'system-ui', 'Segoe UI', 'Roboto', 'Arial', 'sans-serif'],
        mono: ['JetBrains Mono', 'IBM Plex Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
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
        outline: '0 0 0 3px var(--focus-ring)',
      },
      spacing: {
        1.5: '6px',
        2: '8px',
        3: '12px',
        4: '16px',
        5: '20px',
        6: '24px',
        7: '28px',
        8: '32px',
        10: '40px',
        12: '48px',
        14: '56px',
        16: '64px',
      },
      backdropBlur: {
        xs: '2px',
        sm: '6px',
        md: '10px',
        lg: '14px',
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
          '0%, 49%': { opacity: '1' },
          '50%, 100%': { opacity: '0' },
        },
      },
    },
  },
  plugins: [
    // Example plugin: utility for etched control inner-shadow
    function({ addUtilities }) {
      addUtilities({
        '.etched-light': { boxShadow: 'inset 0 1px 0 rgba(0,0,0,0.04)' },
        '.etched-dark':  { boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.03)' },
      });
    },
  ],
};
```

### 8.3 Example Component Classes

Button (Primary)
```html
<button
  class="inline-flex items-center justify-center gap-2 h-10 px-4 rounded-4 border border-transparent
         bg-primary text-primary-fg shadow-2
         hover:bg-primary-hover active:shadow-1
         focus-visible:outline-none focus-visible:shadow-outline transition
         ease-out-custom duration-150">
  Run agent
</button>
```

Subtle Button
```html
<button
  class="inline-flex items-center justify-center gap-2 h-10 px-4 rounded-4 border border-[color:var(--iris-3)]
         bg-[color:var(--iris-2)] text-[color:var(--iris-11)] shadow-0
         hover:bg-[color:var(--iris-3)] active:bg-[color:var(--iris-4)]
         focus-visible:outline-none focus-visible:shadow-outline transition ease-out-custom duration-150">
  New tool
</button>
```

Input
```html
<input
  class="h-10 w-full px-4 rounded-4 border border-stroke-soft bg-surface-1 text-text
         placeholder:text-[color:var(--neutral-9)]
         etched-light dark:etched-dark
         focus-visible:outline-none focus-visible:shadow-outline
         transition ease-out-custom duration-140" placeholder="Agent name" />
```

Panel
```html
<div class="rounded-5 border border-stroke-soft bg-surface-2 shadow-2 surface-noise p-6">
  <div class="text-[14px] text-text-subtle uppercase tracking-wider mb-3">Tool Run</div>
  <div class="text-[15px] leading-6">…content…</div>
</div>
```

Chat Message (Assistant)
```html
<article class="max-w-[72ch] rounded-5 border border-stroke-soft bg-surface-2 shadow-1 p-5">
  <div class="prose prose-neutral dark:prose-invert">
    <!-- Markdown-rendered assistant content -->
  </div>
  <div class="mt-3 text-[12px] leading-4 text-text-subtle">Model: gpt-5 | 2m ago</div>
</article>
```

Code Block
```html
<pre class="rounded-4 border border-stroke-soft bg-neutral-2 dark:bg-neutral-3 p-4 overflow-auto">
  <code class="font-mono text-[13px] leading-[20px]">...</code>
</pre>
```

Table
```html
<div class="rounded-5 border border-stroke-soft bg-surface-2 shadow-1 overflow-hidden">
  <table class="w-full border-collapse">
    <thead class="bg-surface-2 sticky top-0">
      <tr class="h-11 border-b border-stroke-soft text-left text-[12px] uppercase tracking-wider text-text-subtle">
        <th class="px-4">Tool</th>
        <th class="px-4">Latency</th>
        <th class="px-4">Status</th>
      </tr>
    </thead>
    <tbody class="[&_tr:hover]:bg-neutral-3">
      <tr class="h-10 border-b border-stroke-soft">
        <td class="px-4">Web Search</td>
        <td class="px-4">1.2s</td>
        <td class="px-4"><span class="inline-flex items-center h-6 px-2 rounded-full bg-[color:var(--verdigris-6)] text-white text-[12px]">OK</span></td>
      </tr>
    </tbody>
  </table>
</div>
```

---

## 9) Usage Do’s and Don’ts

Do
- Use radius 9px (rounded-4) for most controls and 14px (rounded-5) for panels/cards
- Keep focus rings visible and consistent (shadow-outline)
- Use subtle noise for raised surfaces; keep shadows y-only and minimal
- Cap line length to 72ch for chat content
- Prefer subtle buttons and ghost actions; reserve solids for primaries

Don’t
- Don’t use Tailwind defaults like rounded-md/lg, ring-blue-500, or zinc-100 backgrounds
- Don’t stack heavy shadows on flat surfaces
- Don’t rely only on color for state; pair with icons/labels
- Don’t exceed 24px vertical padding on tool-heavy screens (maintain density)
- Don’t center long-form content edge-to-edge; enforce readable widths

---

## 10) Rationale: Why These Choices Work for LLM Platforms

- Editorial typography and warm neutrals reduce fatigue for long reading sessions
- Hairline borders and etched surfaces provide structure without heaviness
- Non-standard radius (9px) and 1.5px borders give a crafted, custom look
- Distinct primary (Iris) avoids “stock Tailwind blue”
- Motion is restrained and responsive to user preferences (power-user friendly)
- Code and data layouts emphasize clarity, with calm token colors and line numbers
- Component recipes fit conversations, tool runs, provenance, and logs

---

## 11) Red-Team Completeness Checks

- Contrast: token pairs designed to exceed AA; verify in UI with automated tooling
- Keyboard/Screen Reader: focus ring, aria labels for tool outputs and code copy buttons
- RTL: tabs, breadcrumb, and arrows flip; padding mirrored
- Reduced Motion: animations gracefully degrade
- Color-blind: semantic statuses have shape/icon in addition to color
- Print styles (optional): neutral background, text black, links underlined

---

## 12) Rollout Checklist

- Implement base.css with CSS variables and preflight
- Wire theme toggling via .theme-dark class on <html> or root node
- Extend Tailwind config (border DEFAULT 1.5px, custom radii/shadows)
- Replace all ring and rounded utilities with the new tokens/classes
- Migrate buttons/inputs/panels/tables to the new recipes
- Verify AA contrast for all text and interactions
- QA compact density across tool-heavy views
- Add Storybook with light/dark snapshots for core components
- Validate RTL and keyboard navigation
- Ship a “Design Kitchen Sink” page to demo all primitives

---

## 13) Quick Reference Tables

Colors (Semantic)
| Token          | Light Value           | Dark Value            | Text (on-*) |
| -------------- | --------------------- | --------------------- | ----------- |
| primary        | var(--iris-9)         | var(--iris-9)         | #FFFFFF     |
| info           | var(--cerulean-6)     | var(--cerulean-6)     | #FFFFFF     |
| success        | var(--verdigris-6)    | var(--verdigris-6)    | #062B22/#FFF|
| warning        | var(--citrine-6)      | var(--citrine-6)      | #2B1E00/#FFF|
| danger         | var(--coral-6)        | var(--coral-6)        | #FFFFFF     |
| surface-1      | var(--neutral-1)      | var(--neutral-1)      | var(--text) |
| surface-2      | var(--neutral-2)      | var(--neutral-2)      | var(--text) |
| surface-3      | #FFFFFF               | var(--neutral-3)      | var(--text) |
| stroke-soft    | var(--neutral-4)      | var(--neutral-3)      | —           |
| stroke-hard    | var(--neutral-6)      | var(--neutral-5)      | —           |

Typography Scale
| Role       | Size / Line | Weight | Notes                       |
| ---------- | ----------- | ------ | --------------------------- |
| H1         | 40 / 46     | 600    | Editorial display           |
| H2         | 34 / 40     | 600    |
| H3         | 28 / 34     | 550    |
| H4         | 24 / 30     | 500    |
| Body L     | 18 / 26     | 400–450|
| Body M     | 16 / 24     | 400–450|
| Body S     | 14 / 20     | 400–450|
| Micro      | 12 / 16     | 450–500| Uppercase labels + tracking |

Radii
- 4: 9px (base), 5: 14px (panels/cards), 6: 20px (modals), pill: 9999px

Shadows
- 1: 0 1px 0 rgba(0,0,0,0.05)
- 2: 0 2px 4 rgba(0,0,0,0.06)
- 3: 0 8px 24 rgba(0,0,0,0.10)
- outline: 0 0 0 3px var(--focus-ring)
