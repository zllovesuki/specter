# Product

## Register

product

## Users

Specter is an independently operated tunnel network, originally a Master's
capstone and now used for the owner's real tunnels. Its client and operator
interfaces are practical tools for the owner and technically capable users
configuring tunnels or investigating an unreliable connection.

## Product Purpose

The client manager helps users understand their configured tunnels and complete
hostname, publication, and custom-domain actions. The operator interface makes
local server observations available during normal operation and partial failure,
with links to detailed diagnostics. Reliability and enjoyment both matter;
improvements should remain bounded and understandable.

## Brand Personality

Compact, candid, practical. The owner explicitly prefers a compact design for
both interfaces. Familiar controls and precise language should make the tools
easy to operate without disguising the interesting distributed system beneath.

## Anti-references

Avoid marketing dashboards, decorative metric cards, oversized layouts, and
success messages unsupported by an operation result. Avoid a large rewrite or
an interface that assumes every connected peer is healthy and every registered
hostname is published.

## Design Principles

Show observed facts with their time and scope. Distinguish saved configuration,
publication, connectivity, and measured health. Preserve useful partial results
when one request fails. Put the next useful action near the relevant state.
Keep detailed diagnostics available without loading them for a simple overview.
Prioritize information density: data and actions first, terse labels, and optional
help. Avoid repeated caveats, explanatory subtitles, and instructions that restate
the visible controls. The owner explicitly rejected verbose UI and operator copy.

## Accessibility & Inclusion

Support keyboard operation, visible focus, legible contrast, narrow screens,
and text zoom. Express status in words as well as color. Keep motion restrained
and respect reduced-motion preferences. Destructive actions must explain their
actual effect and prevent duplicate submissions.
