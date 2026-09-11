# Embedded interfaces

Svelte 5 + Vite + Tailwind v4. Vite builds both HTML pages and their shared
JavaScript, CSS, and fonts. Go embeds and serves the output; APIs and operator
authentication stay in Go.

```sh
npm ci --ignore-scripts
npm run check
npm run build
npm run dev
npm run preview
```

Development pages: `http://localhost:5173/` and
`http://localhost:5173/_internal/`. API proxies default to the client on port
1180 and operator on port 11180. Set `SPECTER_CLIENT_ORIGIN` or
`SPECTER_OPERATOR_ORIGIN` to override them. Preview uses the same paths on port
4173. Run `make ui` before building the Go binary.

## Fonts

Unmodified Google Fonts Latin WOFF2 subsets, downloaded 2026-09-11. Other
characters use system fallbacks. SIL OFL 1.1 licenses ship in `public/fonts/`.

| Asset | Bytes | Upstream |
| --- | ---: | --- |
| Hanken Grotesk, weight 400–700 | 34,704 | [Google Fonts](https://fonts.gstatic.com/s/hankengrotesk/v12/ieVn2YZDLWuGJpnzaiwFXS9tYtpd59A.woff2) |
| JetBrains Mono, weight 400–500 | 31,432 | [Google Fonts](https://fonts.gstatic.com/s/jetbrainsmono/v24/tDbv2o-flEEny0FZhsfKu5WU4zr3E_BX0PnT8RD8yKwBNntkaToggR7BYRbKPxDcwg.woff2) |
