# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` entrypoints for CLI and helpers; `main.go` wires server binaries.
- `tun/` tunnel client/server logic and UI (see `tun/client/ui`), `gateway/` for edge handling, `overlay/` for routing glue.
- `chord/` and `kv/` implement the DHT and persistence; `spec/` holds protobuf definitions and generated RPC code.
- `integrations/` and package-level `*_test.go` files cover functional and unit tests; `dev/` contains local TLS/ACME assets and docker configs; `assets/` static files.

## Build, Test, and Development Commands
- `make test` — short and race-tested Go suite for most changes.
- `make full_test` — extended, long, and concurrency suites (required when touching DHT/KV/routing).
- `make dev-server-acme` / `make dev-server` / `make dev-client` — bring up compose-based clusters and a demo client.
- `make proto` — regenerate protobuf/Twirp artifacts under `spec/` (needs protoc + vtproto + twirp installed).
- `make ui` — build the tunnel client UI assets.
- Direct Go builds: `go build ./...`; per-package testing: `go test -run TestName ./path/...`.

## Coding Style & Naming Conventions
- Go formatting: `gofmt`/`goimports` (tabs, 2-space alignment only when needed); keep package names short and domain-specific (`chord`, `kv`, `tun`).
- Favor explicit contexts and deadlines for network calls; avoid global state.
- Regenerated files live near their sources (e.g., `spec/protocol/*.twirp.go`); do not hand-edit generated code.

## Testing Guidelines
- Use Go’s testing package; name tests `TestFeature`, benchmarks `BenchmarkX`, and table-driven where practical.
- Run `make test` before commits; changes in `chord/` or `kv/` must also pass `make concurrency_test` (or `make full_test`).
- Integration flows rely on TLS/ACME fixtures under `dev/`; use `make dev-validate` for ring consistency checks when modifying certificate flows.

## Commit & Pull Request Guidelines
- Commit messages follow a short scope prefix and imperative summary (e.g., `tun/server: add route cache tests`, `rpc: use bounded receive`).
- Keep commits focused; include regenerated code in the same commit as the change.
- PRs should describe scope, risks, and test evidence (`make test`, `make full_test` for KV/DHT changes). Link issues and add screenshots for UI adjustments.

## Security & Configuration Notes
- Do not commit real certificates or secrets; use `dev/` fixtures and `certs/` generated via `make certs`.
- Validate transport changes with race flags enabled and prefer bounded reads/writes (see recent RPC changes) to limit attack surface.
