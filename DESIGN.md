---
name: specter
description: Compact interfaces for tunnel management and server operations
colors:
  canvas: "#221f21"
  chrome: "#1b181a"
  surface: "#2a2729"
  line: "#423f42"
  foreground: "#f5f4f4"
  muted: "#a3a1a8"
  zinc-500: "#747178"
  accent-300: "#7cd1dc"
  accent-400: "#57bfcd"
  accent-500: "#3bafbe"
  success: "#9bd7a7"
  warning: "#e9c581"
  danger: "#f9a2a2"
typography:
  body:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, -apple-system, sans-serif"
    fontSize: "15px"
    fontWeight: 450
    lineHeight: 1.5
  control:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 600
    lineHeight: "20px"
  mono:
    fontFamily: "JetBrains Mono, SF Mono, monospace"
rounded:
  control: "6px"
spacing:
  compact: "8px"
  control: "12px"
  mobile: "16px"
  desktop: "24px"
components:
  button-primary:
    backgroundColor: "{colors.accent-500}"
    textColor: "{colors.chrome}"
    typography: "{typography.control}"
    rounded: "{rounded.control}"
    padding: "6px 12px"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.control}"
  input:
    backgroundColor: "{colors.chrome}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.control}"
    padding: "8px 12px"
---

# Specter design system

## Overview

Compact operating tools: data and actions first, optional help, concise feedback.
[PRODUCT.md](PRODUCT.md) defines the audience and tone;
[frontend conventions](docs/frontend-spec.md) define implementation boundaries.

## Colors

Warm dark neutrals with a muted cyan accent for primary actions and focus. Use
semantic colors with text labels. Tokens are implemented in
[theme.css](ui/shared/theme.css); update this snapshot when those values change.

## Typography

Locally bundled Hanken Grotesk carries the UI; JetBrains Mono identifies hosts,
addresses, and code. Use 22–24px page headings and 14px controls. Keep help short
and prose within 72 characters per line.

## Elevation

Use tonal surfaces and thin borders. Avoid decorative shadows, gradients, and
nested cards. Compact tables carry the primary information.

## Components

Reuse [Button](ui/shared/Button.svelte), [Shell](ui/shared/Shell.svelte),
[ScrollRegion](ui/shared/ScrollRegion.svelte), and [input styles](ui/shared/styles.ts).
Impeccable previews are recorded in [.impeccable/design.json](.impeccable/design.json).
Buttons are at least 36px high (small) or 40px (medium); inputs are at least 40px.
The shell is at most 72rem wide, with 16px horizontal padding or 24px from `sm`.

Hover color changes are instant; press transforms use 75ms transitions. Keyboard
focus uses a 2px accent outline with a 3px offset. Respect reduced motion. Tables
scroll inside their region without widening the page. Confirm destructive actions
inline, with their effect stated briefly.

## Do's and Don'ts

Keep loading, empty, failed, and stale states distinct. Preserve useful data on
refresh failure. Show saved state and publication separately. Avoid the marketing
dashboards, decorative metric cards, oversized layouts, and unsupported success
messages identified in [PRODUCT.md](PRODUCT.md). Do not add jump navigation for
sections already visible on a short page.
