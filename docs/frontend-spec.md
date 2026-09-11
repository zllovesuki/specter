# Frontend conventions

Applies to both embedded interfaces. [PRODUCT.md](../PRODUCT.md) owns product
intent; [DESIGN.md](../DESIGN.md) owns visual tokens and interaction guidance.
Toolchain versions and runtime behavior follow the live code and lockfile.

## Structure and build

Both apps use Svelte 5 runes, strict TypeScript, Vite, and Tailwind v4's Vite
plugin. `ui/client` and `ui/operator` share components, fonts, and tokens under
`ui/shared`. One package and lockfile build separate entry points with shared
assets. See [ui/README.md](../ui/README.md) for development commands.

Vite emits HTML, JavaScript, and CSS. Go embeds and serves that output and owns
APIs, authentication, and distributed-system behavior. The UI package does not
import domain packages. Preserve existing browser URLs and diagnostic endpoints.

## Styling and components

Use Tailwind utilities for layout, spacing, responsive rules, and states. Keep
custom CSS to shared tokens, fonts, and global behavior. Reuse shared buttons,
shell, scroll regions, and input classes before adding local variants.

Use the warm dark palette and local fonts recorded in DESIGN.md. Keep assets and
dependencies small: these pages ship inside the binary. Share assets across apps
and measure size when changing the build or adding dependencies.

## Interaction and content

Prioritize data and actions. Use short labels and scoped feedback; put optional
help and long errors behind disclosure controls. Navigation should change views
or provide useful movement, not decorate a small page.

Keep draft input separate from request results. Cancel obsolete reads, guard
concurrent mutations, and preserve useful snapshots after refresh failures.
Distinguish configuration, publication, connectivity, and unknown state.

Use semantic controls, visible focus, a skip link, keyboard-accessible scroll
regions, and reduced-motion support. Narrow layouts must not overflow the page.

## Validation

Run `npm run check` for frontend code changes and build when asset output changes.
Use focused browser checks for affected interactions. Documentation-only changes
need link and syntax checks, not application test suites.
