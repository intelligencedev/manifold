Manifold User Interface Guidelines (M-UIG)

Version
- v1.0 (Aperture series)

Scope
- Canonical guidelines for Manifold’s frontend UI across all views and surfaces.
- Establishes two visual themes built from a single baseline:
  - Aperture Dark (reference baseline)
  - Aperture Light (derivative of Dark, consistent in structure and tokens, but with no drop shadows)

Core principles
- Minimize visual noise. Remove clutter and text the user does not need to see.
- Do not make the user think. Prefer obvious, self-explanatory affordances and progressive disclosure.
- Avoid borders unless absolutely necessary to emphasize an element. Prefer subtle hairline rings when emphasis is needed.
- Avoid soft drop shadows or any styling that introduces blur. Never use drop shadows to create contrast.
- Consistency across views. Components and tokens must remain consistent unless a view has a uniquely justified requirement.
- Avoid common, generic Tailwind aesthetics. Favor a distinctive, minimalist look implemented via design tokens and scoped CSS.

Design language summary
- Edges: crisp, etched, hairline rings. No fat borders.
- Surfaces: calm gradients or flat fills with restrained contrast.
- Elevation (Dark only): tight, layered shadows with short radii to add depth—not contrast. Light uses no shadows.
- Accents: narrow rails or indicators that align with component curvature; subtle opacity.
- Motion: restrained, functional; no flashy or bouncy animations.

Theming model
- Two themes share the same component structure, spacing, radii, typography, and behaviors.
- Differences are constrained to color tokens and elevation behavior.

Theme identifiers
- [data-theme="aperture-dark"]
- [data-theme="aperture-light"]

Design tokens
- Radii
  - --radius-sm: 6px
  - --radius-md: 12px
  - --radius-lg: 16px

- Spacing (4-based scale)
  - --space-1: 4px
  - --space-2: 8px
  - --space-3: 12px
  - --space-4: 16px
  - --space-6: 24px
  - --space-8: 32px

- Typography
  - Font family: System UI stack, metric-optimized; monospaced for code.
  - Sizes: 12, 14, 16 (base), 18, 20, 24.
  - Line-height: 1.45–1.6 for body, 1.3–1.4 for headings.
  - Weight: 400/500/600; avoid 700 unless essential.
  - Truncation: prefer 2–3 line clamps with explicit affordances for expansion.

- Color roles (abstract)
  - --color-bg: base background
  - --color-surface: card/panel surface
  - --color-surface-muted: subdued surface/secondary containers
  - --color-text: primary text
  - --color-text-muted: secondary text
  - --color-border: hairline ring color
  - --color-accent: brand/accent (e.g., assistant rail)
  - --color-success / --color-danger / --color-warning: semantic accents
  - --color-focus: focus ring color

- Elevation tokens
  - Light: no drop shadows allowed. Use rings, contrasts, spacing.
  - Dark: E0 (flat), E1, E2, E3 — all tight, low-blur, low-alpha shadows + optional hairline ring.
    - E1 (default card): ring + 0 2px 6px, 0 12px 18px at low alpha
    - E2: slightly stronger near-field + far-field
    - E3: used sparingly (modals, floating panes)

Elevation specification (reference)
- Aperture Dark
  - E0: box-shadow: none
  - E1: box-shadow:
      - 0 0 0 1px rgb(var(--color-border) / 0.20),
      - 0 2px 6px rgba(0, 0, 0, 0.12),
      - 0 12px 18px rgba(0, 0, 0, 0.14)
  - E2: increase near-field and far-field by ~+2px radius and +0.02 alpha
  - E3: add a third far-field layer; only for overlays

- Aperture Light
  - No drop shadows. Use a hairline ring for separation and elevation cues.
  - E1: box-shadow: 0 0 0 1px rgb(var(--color-border) / 0.20)
  - E2: ring at 0.24 alpha, optional subtle background lift
  - E3: ring at 0.28 alpha, reserved for overlays

Edges and rings
- Prefer a 1px hairline ring via box-shadow: 0 0 0 1px … to avoid adding layout thickness.
- Avoid using standard borders unless required for separators within dense controls.
- For separators, use low-contrast, 1px lines with ample breathing room.

Accent rail pattern (as used in Chat View)
- Purpose: identify assistant or accent messages with a narrow left rail that respects card curvature.
- Implementation requirements
  - The parent card must clip children: overflow: hidden.
  - The rail is drawn via ::before, positioned absolutely at left: 0; top: 0; bottom: 0.
  - Rail width token: --rail-w: 3px (may vary 2–4px by density).
  - The rail has matching border-top-left-radius and border-bottom-left-radius equal to the card radius.
  - Z-order the rail below content but above the surface background.
  - Use alpha-blended accent color so it does not dominate content.
- Color guidance
  - Dark: rgb(var(--color-accent) / 0.35–0.45) depending on surface contrast
  - Light: rgb(var(--color-accent) / 0.20–0.30)

States
- Hover
  - Dark: minimal lift (+0.02 alpha on ring and small increase on near-field shadow). No bloom.
  - Light: no shadows; slightly increase ring alpha (+0.04 max) and raise surface by a subtle background change.
- Active/Pressed
  - Compress elevation by one step; do not darken text; reduce surface contrast slightly.
- Focus (keyboard/mouse)
  - Use a 2px focus ring with offset: outline: 2px solid var(--color-focus); outline-offset: 2px.
  - Do not simulate focus with shadows. Ensure clear visibility over all surfaces.
- Disabled
  - Reduce opacity of text and icons; retain layout and spacing.

Motion
- Durations: 120–160ms for small UI, 180–220ms for overlays.
- Easing: standard (0.2, 0, 0, 1) for enter, (0.4, 0, 1, 1) for exit; respect user’s reduced motion preference.
- No blur-based transitions.

Accessibility
- Contrast: text vs. surface 4.5:1 minimum (3:1 for large text); non-text (icons) 3:1.
- Focus: always visible and not color-only; use thickness and spacing.
- Target sizes: 44x44px minimum interactive targets where feasible.
- Keyboard: all interactive elements reachable in logical order; visible :focus styles.

Component library specification

1) Card / Message card (reference: Chat View article)
- Structure
  - border-radius: var(--radius-md)
  - overflow: hidden (ensures pseudo-elements and media respect curvature)
  - padding: var(--space-4) default; adjust by density as needed
  - background:
    - Dark: neutral surface with subtle vertical gradient is allowed
    - Light: flat or nearly flat surface; avoid darkening via shadows

- Elevation
  - Dark (E1 default): see Elevation specification above
  - Light (E1 default): ring only at 0.20 alpha; no shadows

- Accent rail (optional)
  - Implement as ::before; width: var(--rail-w, 3px)
  - border-top-left-radius/border-bottom-left-radius: var(--radius-md)
  - Parent overflow: hidden

- Hover
  - Dark: increase ring alpha by +0.05; near shadow radius +2px, alpha +0.02
  - Light: increase ring alpha by +0.04; slight surface lift; no shadows

2) Composer (etched control)
- Structure
  - Same radii/spacing as cards; internal gap: var(--space-3)
  - Etched feel via hairline ring and optional subtle inset highlight
  - Avoid heavy inner shadows; no blur
- Elevation
  - Dark: ring + tight near-field shadow permitted (E1)
  - Light: ring only; no shadows

3) Buttons
- Variants: Primary (filled), Secondary (tonal), Tertiary (ghost)
- Shape: border-radius: var(--radius-sm) or --radius-md depending on density
- Elevation: none; rely on fill, ring, and contrast. No shadows.
- States
  - Hover: background shift by 3–6%; ring alpha +0.04
  - Focus: 2px outline; offset 2px
  - Disabled: lower opacity; retain layout

4) Inputs (text, select, textarea)
- Use etched ring; no inner glow; no drop shadows
- Focus: 2px outline and color shift; do not increase blur
- Placeholder: muted; avoid high contrast placeholders

5) Chips/Tags
- Compact; ring-only emphasis
- No shadows; use background tint and clear focus outline

6) Toolbars / Headers
- Flat surfaces; sticky surfaces use ring at lower edge for separation
- No shadows for sticky behavior; prefer a 1px separator

7) Panels/Drawers/Modals
- Dark: E2–E3 depending on prominence. Keep shadows tight.
- Light: ring at 0.24–0.28 alpha; consider backdrop to ensure separation
- Backdrops: use opacity/blur-free dimming layers; avoid Gaussian blurs

8) Toasts/Notifications
- Compact; ring at 0.24–0.28 alpha
- No drop shadows in Light; minimal E1 in Dark

Implementation guidance

Tokens and scoping
- Define CSS variables at the theme root and scope with [data-theme="aperture-dark"] and [data-theme="aperture-light"].
- Avoid Tailwind shadow-* and border classes for core components. Prefer component classes that consume tokens.
- Utilities are acceptable for spacing/layout, but avoid generic visual styles that undermine uniqueness.

Reference implementation (Chat View)
- Assistant message card with accent rail (Dark)

  [data-theme="aperture-dark"] .chat-modern .chat-pane article[class*='bg-accent/5'] {
    border-radius: var(--radius-md);
    overflow: hidden; /* clip rail and media to curvature */
    background: var(--color-surface);
    box-shadow:
      0 0 0 1px rgb(var(--color-border) / 0.20),
      0 2px 6px rgba(0, 0, 0, 0.12),
      0 12px 18px rgba(0, 0, 0, 0.14);
  }

  [data-theme="aperture-dark"] .chat-modern .chat-pane article[class*='bg-accent/5']::before {
    content: "";
    position: absolute;
    inset: 0 auto 0 0;
    width: var(--rail-w, 3px);
    background: rgb(var(--color-accent) / 0.40);
    border-top-left-radius: var(--radius-md);
    border-bottom-left-radius: var(--radius-md);
  }

- Hover (Dark)

  [data-theme="aperture-dark"] .chat-modern .chat-pane article[class*='bg-accent/5']:hover {
    box-shadow:
      0 0 0 1px rgb(var(--color-border) / 0.25),
      0 4px 8px rgba(0, 0, 0, 0.14),
      0 16px 24px rgba(0, 0, 0, 0.16);
  }

- Assistant message card with accent rail (Light)

  [data-theme="aperture-light"] .chat-modern .chat-pane article[class*='bg-accent/5'] {
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--color-surface);
    box-shadow: 0 0 0 1px rgb(var(--color-border) / 0.20); /* ring only */
  }

  [data-theme="aperture-light"] .chat-modern .chat-pane article[class*='bg-accent/5']::before {
    content: "";
    position: absolute;
    inset: 0 auto 0 0;
    width: var(--rail-w, 3px);
    background: rgb(var(--color-accent) / 0.24);
    border-top-left-radius: var(--radius-md);
    border-bottom-left-radius: var(--radius-md);
  }

- Hover (Light)

  [data-theme="aperture-light"] .chat-modern .chat-pane article[class*='bg-accent/5']:hover {
    box-shadow: 0 0 0 1px rgb(var(--color-border) / 0.24); /* subtle ring emphasis */
  }

Do and Don’t (key rules)
- Do
  - Use 1px hairline rings for separation and emphasis
  - Clip children to container radii when using pseudo-elements or media
  - Keep hover/active states restrained and purposeful
  - Maintain consistency across views; reuse tokens and component patterns
- Don’t
  - Don’t use soft, blurry shadows anywhere
  - Don’t use drop shadows for contrast
  - Don’t add borders where a ring or spacing would suffice
  - Don’t rely on generic Tailwind aesthetic shortcuts (shadow-*, ring-*, bright borders) for core components

Quality checklist (per view)
- [ ] All cards/panels follow the Card spec with correct radii and rings
- [ ] No soft drop shadows; Light theme has zero shadows
- [ ] Accent rails, if present, curve with the card and are clipped
- [ ] Focus is visible and consistent; no shadow-based focus
- [ ] Text contrast meets WCAG thresholds
- [ ] Hover and active states are perceptible but not distracting
- [ ] No redundant labels or cluttering text; progressive disclosure used
- [ ] Tailwind utilities do not override core tokens/styles for visuals

Performance considerations
- Avoid CSS filters and blurs; they are GPU-expensive and reduce crispness
- Prefer CSS transforms for motion; keep paint areas small
- Minimize overdraw by avoiding stacked translucent layers

Extending the system
- New components must declare:
  - Which elevation token (E0–E3) they use in Dark and the Light equivalent ring level
  - Which radii and spacing tokens they use
  - Color role usage and alpha ranges
  - Hover, focus, active behaviors
  - Accessibility guarantees

Appendix: mapping to Chat View changes
- Crisp rings replace borders to define edges.
- Assistant rail is implemented with ::before, curved caps, and parent overflow: hidden.
- Dark theme uses a tight, layered shadow stack for subtle depth (E1 by default).
- Light theme mirrors geometry and spacing, but removes shadows and relies on a ring-only elevation cue.

This document is the single source of truth for visual consistency across Manifold. When in doubt, remove, simplify, and clarify.

