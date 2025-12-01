# Specter Internal Gateway Endpoints

This document describes the administrative endpoints exposed under `/_internal` on the apex domain.

These endpoints are:

- Protected with HTTP Basic Auth (realm: `internal`).
- Intended for operators only; **do not expose publicly**.
- Available only when `INTERNAL_USER` and `INTERNAL_PASS` are configured.


## Authentication

- Configure credentials via environment variables:
  - `INTERNAL_USER`
  - `INTERNAL_PASS`
- Access pattern (example):
  - `https://<INTERNAL_USER>:<INTERNAL_PASS>@<apex-domain>/_internal/...`
- Requests without valid credentials receive `401 Unauthorized`.
- If credentials are not configured, the `/_internal` tree is disabled and requests return `404 Not Found`.


## Index

`GET /_internal`

- Returns this documentation as plain text.
- Useful as a quick reference to available internal endpoints.
- Any unmatched `/_internal/*` path (for example `/_internal/stats`) also returns this document.


## ACME Certificate Management

Mounted under `/_internal/acme` when ACME is enabled.

- `POST /_internal/acme/clean`
  - Triggers background cleanup in the ACME storage:
    - Removes old OCSP staples.
    - Deletes expired certificates (with a grace period).
  - Response: `204 No Content` on success.

- `GET /_internal/acme/certs`
  - Lists ACME issuers and the count of certificates for each.
  - Response: JSON object (`application/json`):
    - `{ "issuers": [{ "issuer": string, "count": number }, ...] }`.

- `GET /_internal/acme/certs/{issuer}`
  - Lists certificates for a specific issuer.
  - Response: JSON object (`application/json`):
    - `{ "issuer": string, "count": number, "certs": [{ "name": string, "domain": string }, ...] }`.

- `GET /_internal/acme/certs/{issuer}/{name}`
  - Returns detailed information for a single certificate:
    - Subject/issuer, validity period, SANs, key information, and optional metadata.
  - Response: JSON object (`application/json`).

- `DELETE /_internal/acme/certs/{issuer}/{name}`
  - Deletes certificate, private key, and metadata for a specific certificate.
  - Response: `204 No Content` whether or not the certificate already exists; `500 Internal Server Error` if deletion fails.


## Chord Ring Introspection

Mounted under `/_internal/chord`.

- `GET /_internal/chord/stats`
  - Returns statistics about the Chord ring from this node’s perspective.
  - Format handling via chi `URLFormat` middleware:
    - `/_internal/chord/stats` or `/_internal/chord/stats.txt` → plain text (`text/plain`).
    - `/_internal/chord/stats.html` → HTML page that wraps the same text output.
  - Optional query parameter:
    - `?key=<bytes>` → returns the raw value for a specific KV key on this node (`text/plain`), or `404` if the key is not present.

- `GET /_internal/chord/graph`
  - Returns a Graphviz/DOT representation of the ring topology:
    - Nodes in the ring.
    - Successor links.
    - Finger table edges.
    - Predecessor link from this node.
  - Response: `text/plain` DOT graph.


## Tunnel Server Introspection

Mounted under `/_internal/tun`.

- `GET /_internal/tun/`
  - HTML overview of connected tunnel clients:
    - Node identity/address.
    - Client identifier.
    - Version.
    - Number of configured tunnels per client (or error string if lookup fails).
  - Response: `text/html` page.

- `GET /_internal/tun/{id}/{address}`
  - HTML view of tunnels for a specific client node.
  - Performs an internal RPC to the client to list tunnels and cross-references with Chord state.
  - Response: `text/html` page describing hostname → target mappings.
  - Hostnames present in Chord but not reported by the client are shown with target `(unused)`.


## Configuration Migrator

Mounted under `/_internal/migrator` when enabled.

- `GET /_internal/migrator/`
  - Serves a helper HTML page explaining how to migrate legacy client configuration.

- `POST /_internal/migrator/`
  - Accepts a v1 client configuration in YAML.
  - Requires a valid `clientId` and 44-character `token`; invalid input returns `400 Bad Request`.
  - Returns a v2 configuration with a new client certificate/key and migrated tunnels.
  - Response: `application/yaml`.


## Runtime Debug / Profiling

Mounted under `/_internal/debug`.

This tree is provided by `github.com/go-chi/chi/v5/middleware.Profiler()` and includes:

- `GET /_internal/debug/pprof/`
- `GET /_internal/debug/pprof/cmdline`
- `GET /_internal/debug/pprof/profile`
- `GET /_internal/debug/pprof/symbol`
- `GET /_internal/debug/pprof/trace`
- `GET /_internal/debug/vars`

The exact set may vary with the underlying `net/http/pprof` and `expvar` handlers, but it generally mirrors the standard Go pprof endpoints.


## Internal Proxying Between Nodes

The `/_internal` tree can be accessed directly on a given node, or proxied via another node in the cluster.

- When `x-internal-proxy-node-address` is set on the request headers, the gateway will:
  - Dial the target node over the INTERNAL stream.
  - Forward the HTTP request to that node’s `/_internal` handlers.
  - If proxying fails (for example, the node is unreachable), respond with `502 Bad Gateway` and an error message.

- The gateway adds `x-internal-proxy-forwarded` to prevent loops.

This allows you to query a specific node’s internal state (ACME, Chord, tunnels, etc.) through any other node that can reach it.
