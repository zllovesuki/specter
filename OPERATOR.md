# Specter Operator Guide

Field notes for running the Specter server (edge/gateway) and the ACME DNS solver.

## Quick start (minimal, reproducible)
- Prepare a private CA bundle containing `ca.crt`, `client-ca.crt`, `client-ca.key`, `node.crt`, `node.key`.
- Launch the seed (no `--join`):\
  `specter --verbose server --data /var/lib/specter --cert /etc/specter/cert --apex example.com --acme acme://ops@example.net@acme-hosted.net --listen-rpc unix:///var/run/specter-rpc.sock`
- Start the ACME DNS solver pointing at the seed RPC endpoint:\
  `specter dns --acme acme://ops@example.net@acme-hosted.net --acme-ns ns1.acme-hosted.net/203.0.113.10 --rpc unix:///var/run/specter-rpc.sock`
- In DNS, delegate `_acme-challenge.example.com` → `managed.acme-hosted.net` (CNAME) and publish NS glue for `acme-hosted.net` to the solver addresses above.
- Bring up more servers with `--join <seed-advertise-addr>` using the same CA bundle and `--apex` list.

## Addresses, listeners, and identity
- `--listen-addr` (repeatable or comma-separated env `LISTEN_ADDR`) supplies the shared TCP+UDP binds; override per-protocol with repeatable `--listen-tcp` / `--listen-udp` (env: `LISTEN_TCP`, `LISTEN_UDP`). QUIC needs UDP reachability on the advertised port.
- `--advertise-addr` is the routable address clients dial and is hashed into the node identity; changing it effectively introduces a new node. Keep it stable across restarts.
- HTTP/1.1 CONNECT + HTTPS redirect (`--listen-http`, default 80) only starts when the advertised port is 443 **or** `--listen-http` is explicitly set. Hostnames reuse the TCP listen hosts.
- PROXY protocol is honored only on the TCP listeners when `--proxy-protocol` is set. QUIC/UDP has no PROXY support; your load balancer must pass the real UDP source or accept that the gateway sees the LB address.
- `--listen-rpc` exposes KV and vnode RPC (default `tcp://127.0.0.1:11180`; accepts `unix:///path.sock`). There is no authentication—bind it to loopback or a Unix socket.

## Certificates, PKI, and certificate loading
- Inter-node and client auth rely on the private CA bundle. Provide it via `--cert-dir` (alias `--cert`) or base64 envs with `--cert-env` (`CERT_CA`, `CERT_NODE`, `CERT_NODE_KEY`, `CERT_CLIENT_CA`, `CERT_CLIENT_CA_KEY`).
- ACME is enabled with `--acme acme://EMAIL@ACME_ZONE`. Self-signed certs are used when `--acme` is absent; the first `--apex` value becomes the default SNI for self-signed.
- Client certificates are minted by the server-side PKI and enforced on overlay (inter-node) and tunnel links. Gateway PKI endpoints are mounted when admin credentials are configured.

## ACME and DNS flow
- Apex certificates: for each `--apex`, publish `_acme-challenge.<apex>` CNAME `managed.<ACME_ZONE>`; the solver answers TXT from the Chord KV.
- ACME DNS solver: `specter dns` must be given the same ACME URI and the authoritative NS/Glue via `--acme-ns` (`ns1.zone/IPv4[,IPv6]`, repeatable). It speaks both TCP and UDP on its own `--listen-*` addresses (defaults to 53).
- Custom domains (beyond the managed apex list) are allowed once ownership is proven:
  1. Client runs `specter client acme <domain>` to get `_acme-challenge.<domain>` → `<token>.<ACME_ZONE>` (CNAME).
  2. After the record is live, client runs `specter client validate <domain>`; on success the gateway will obtain and renew a cert for that hostname.
  3. Point `<domain>` (CNAME) to the Specter apex (value returned by `validate`, usually your `--apex`).
- On-demand issuance uses the stored validation token in KV, not just the apex list—so keep the KV durable.

## Storage and durability
- `--data-dir` is required. Each virtual node stores its KV data in `<data-dir>/<index>`; SQLite uses `<data-dir>/cache` for its native cache.
- KV backends: `aof` (default, append-only WAL, flushes ~3s), `sqlite` (single-file durability, good on slower disks), `memory` (non-persistent; tests only).
- Persist the entire `--data-dir` to keep ACME state, tunnel mappings, RTT hints, and custom-host validation tokens. Ephemeral disks will orphan tunnels and force ACME re-issuance.

## Cluster bootstrap and lifecycle
- Seed: start without `--join`; keep it **first up, last down**. All nodes require the same CA trust roots.
- Joins: `--join <seed-advertise-addr>`; default 5 virtual nodes per process (`--virtual`). Each virtual node is separately announced in the ring.
- Shutdown: process traps SIGINT/SIGTERM and calls `Leave` on each vnode; give at least 60–120s so ownership transfers complete (compose files set `stop_grace_period: 120s`).
- Changing `--advertise-addr`, `--apex`, or the CA bundle changes the node identity and will require a rejoin; plan rolling changes by adding new nodes before removing old ones.

## Gateway behavior and traffic
- One port multiplexes TLS/QUIC for HTTP/2, HTTP/3, and raw TCP tunnels. Alt-Svc is advertised for h3; HTTP/2 is always available.
- HTTP listener (port 80 by default) only handles CONNECT and redirects; it never serves tunnels or internal endpoints.
- Buffer tuning: `--transport-buffer` and `--proxy-buffer` (default `16KiB`) control read/write sizes when proxying HTTP to clients.
- Behind TCP LBs that split UDP, be sure the UDP listeners are reachable directly; QUIC sessions cannot traverse TCP-only balancers.

## Observability and admin endpoints
- Set `INTERNAL_USER` and `INTERNAL_PASS` (or `--auth_user` / `--auth_pass`) to enable `/_internal` on apex hostnames **and** on the local loopback helper (`127.0.0.1:9999`). Without credentials, the entire `/_internal` tree is disabled.
- Internal routes include ACME cert inventory, Chord stats/graph, tunnel views, migrator, pprof/expvar (see `gateway/endpoints.md`).
- Logging: JSON on stderr by default; `--verbose` switches to colored development logging. `SENTRY_DSN` / `--sentry` sends errors to Sentry with breadcrumbs at info level.
- Validation helpers: `make dev-validate` checks CA consistency in the dev ring; `/_internal/chord/stats?key=<kvkey>` can confirm ownership placement.

## Operational tips
- Prefer Unix sockets for `--listen-rpc` when co-locating `specter dns`.
- Keep PROXY protocol off unless your LB injects it; when enabled, TCP connections without a PROXY header are rejected.
- When expanding capacity, match `--virtual` counts across nodes to keep ring balance predictable.
- Back up `--data-dir` before changing KV providers or migrating storage; include the `cache/` subdir when using SQLite.
