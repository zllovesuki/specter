# Repository Guidelines

## Rules To Remember

"Can we do more with less code?"

"Do the hard things now, or be forced to do harder things in the future."

These rules are ordered, not contradictory.

Start with the smallest correct implementation that preserves correctness,
performance, security, and clarity. Prefer existing code and a local fix when
the problem is local.

If that fix creates known fragility, blurred ownership, or an architectural
dead end, pay the structural cost now instead of layering another shortcut on
top. A larger abstraction must solve a current problem; otherwise present it
as an option rather than making it the default.

Keep ownership explicit: startup code selects and validates providers; each
provider owns its storage; Chord owns membership and key ownership; transports
own connection lifecycles. UIs consume API results rather than reimplementing
domain decisions.

Keep tests tied to concrete behavior and changed boundaries. Complete required
checks, then stop unless a new change or failure justifies more verification.
Keep documentation proportional to what a reader needs to understand or do.

## Project Structure & Module Organization
- `main.go` launches the `specter` CLI; `cmd/specter` wires top-level commands, and `cmd/{server,client,dns,...}` holds command entrypoints/helpers.
- `tun/` tunnel client/server logic, `ui/` embedded client and operator frontends, `gateway/` edge handling, `overlay/` routing glue.
- `chord/` and `kv/` implement the DHT and persistence; `spec/` holds protobuf definitions and generated RPC code.
- `integrations/` and package-level `*_test.go` files cover functional and unit tests; `dev/` contains local TLS/ACME assets and docker configs; `assets/` static files.

## Build, Test, and Development Commands
- `make test` — short and race-tested Go suite for most changes.
- `make full_test` — extended, long, and concurrency suites (required when touching DHT/KV/routing).
- `make dev-server-acme` / `make dev-server` / `make dev-client` — bring up compose-based clusters and a demo client.
- `make proto` — regenerates protobuf/Twirp artifacts and applies repo VT/Twirp patches; this target bootstraps tools via `make dep` and requires network access.
- `make ui` — check and build both embedded frontends.
- Toolchain: Go `1.27.x`, Node.js `22.12+` + npm (for `make ui`), and Docker with buildx for local compose/dev workflows.
- Direct Go builds: `go build ./...`; per-package testing: `go test -run TestName ./path/...`.

## Coding Style & Naming Conventions
- Go formatting: `gofmt`/`goimports` (tabs, 2-space alignment only when needed); keep package names short and domain-specific (`chord`, `kv`, `tun`).
- Favor explicit contexts and deadlines for network calls; avoid global state.
- Regenerated files live near their sources (e.g., `*.pb.go`, `*_vtproto.pb.go`, `*.twirp.go`, `*_string.go`); do not hand-edit generated code.

## Frontend Guidance

Use Impeccable with [PRODUCT.md](PRODUCT.md) for product intent,
[DESIGN.md](DESIGN.md) for visual design, and
[docs/frontend-spec.md](docs/frontend-spec.md) for frontend conventions.
Keep these references repository-local; live code and package configuration
remain authoritative for architecture and versions.

## Testing Guidelines
- Use Go’s testing package; name tests `TestFeature`, benchmarks `BenchmarkX`, and table-driven where practical.
- Run `make test` before commits. For ring-concurrency changes in `chord/`, also run `make concurrency_test`; for `kv/`, DHT/routing, or other distributed-path changes, run `make full_test`.
- Run `make integration_test` when touching gateway/tunnel/certificate or ACME integration flows.
- Integration flows rely on TLS/ACME fixtures under `dev/`; use `make dev-validate` for ring consistency checks when modifying certificate flows.

## Commit & Pull Request Guidelines
- Commit messages follow a short scope prefix and imperative summary (e.g., `tun/server: add route cache tests`, `rpc: use bounded receive`).
- Keep commits focused; include regenerated code in the same commit as the change.
- PRs should describe scope, risks, and test evidence (`make test`, `make full_test` for KV/DHT changes). Link issues and add screenshots for UI adjustments.

## Security & Configuration Notes
- Do not commit real certificates or secrets; use `dev/` fixtures and `certs/` generated via `make certs`.
- Validate transport changes with race flags enabled and prefer bounded reads/writes (see recent RPC changes) to limit attack surface.
