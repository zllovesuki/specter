# Specter Operator Guide

Welcome to the Specter Operator Guide. This document provides instructions for deploying, configuring, and maintaining the Specter edge server (gateway) and its associated ACME DNS solver. Whether you are running a single node for personal use or a globally distributed cluster, this guide will help you understand the core concepts and operational best practices.

## Table of Contents
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [Configuration Guide](#configuration-guide)
- [Advanced Operations](#advanced-operations)
- [Operational Tips](#operational-tips)

---

## Installation

Specter can be deployed using pre-built packages or via Docker containers. Choose the method that best fits your infrastructure.

### Packaged Installs
We currently provide native packages for Linux. These packages set up the necessary binaries, configuration templates, a dedicated `specter` service user, and systemd services for you.

**Linux (`.deb`, `.rpm`)**
- **Binaries:** `/usr/bin/specter`
- **Launchers:** `/usr/libexec/specter/{server-launch,dns-launch,client-launch}`
- **Configs:** `/etc/specter/*.env`, `/etc/specter/client.yaml`
- **Certificates:** `/etc/specter/cert`
- **Runtime RPC socket directory:** `/run/specter`
- **Services:** `specter-server.service`, `specter-dns.service`, `specter-client.service`
- **Service user:** `specter`
- **Writable paths owned by `specter`:** `/var/lib/specter`, `/var/log/specter`, `/etc/specter/cert`, `/etc/specter/client.yaml`

### Docker and Docker Compose
For containerized environments, Specter provides a `Dockerfile` and example `compose-server.yaml` and `compose-client.yaml` files.
When running via Docker, ensure that the data directory you pass via `--data-dir` and your certificate directory are mounted as persistent volumes. See the repository's `compose-server.yaml` for a working example of a server deployment.

---

## Quick Start

This section walks you through a minimal, reproducible setup of an initial Specter node (acting as the "seed") and its ACME DNS solver.

### 1. Prepare Certificates
Specter requires a private CA bundle for inter-node and client authentication. You will need a directory containing the following files: `ca.crt`, `client-ca.crt`, `client-ca.key`, `node.crt`, and `node.key`.

### 2. Launch the Initial Node
The first node in your cluster takes the role of the "seed" node. It must be started without the `--join` flag.

```sh
specter --verbose server \
  --data /var/lib/specter \
  --cert /etc/specter/cert \
  --apex example.com \
  --acme acme://ops@example.net@acme-hosted.net \
  --listen-rpc unix:///run/specter/rpc.sock
```

### 3. Start the ACME DNS Solver
The DNS solver handles Let's Encrypt (or other ACME provider) challenges. It communicates with the local Specter node via the RPC socket.

```sh
specter dns \
  --acme acme://ops@example.net@acme-hosted.net \
  --acme-ns ns1.acme-hosted.net/203.0.113.10 \
  --rpc unix:///run/specter/rpc.sock
```

### 4. Configure DNS Delegation
In your DNS provider's control panel:
- Delegate `_acme-challenge.example.com` to `managed.acme-hosted.net` using a CNAME record.
- Publish NS records for the delegated ACME zone using the names you pass via `--acme-ns` (for example `ns1.acme-hosted.net`), and publish matching A/AAAA records for those names. If those NS hostnames live inside the delegated zone, add glue records at the parent zone.

### 5. Expand the Cluster
To add more servers to your cluster, start them with the `--join <seed-advertise-addr>` flag, using the same CA bundle and `--apex` list as the initial node.

---

## Core Concepts

Understanding these core concepts will help you operate Specter more effectively.

### Gateway Behavior and Traffic
Specter is designed as an edge gateway. A single port multiplexes TLS and QUIC traffic, seamlessly handling HTTP/2, HTTP/3, and raw TCP tunnels. HTTP/3 is advertised via the `Alt-Svc` header, while HTTP/2 is always available.

The dedicated HTTP listener (default port 80) is strictly for HTTP/1.1 `CONNECT` requests and HTTPS redirects. It never serves tunnels or internal endpoints directly. 

If you place Specter behind a load balancer (LB), note that QUIC/UDP traffic cannot traverse a TCP-only balancer. Ensure your UDP listeners are directly reachable. PROXY protocol is supported for TCP listeners, but not for QUIC/UDP.

### Clustering and the Ring
Specter uses a Chord Distributed Hash Table (DHT) to form a cluster (the "ring").
- **Seed Node:** The cluster bootstrap starts with a seed node (started without `--join`). Technically, any node can be a seed node, and nodes can be started or stopped in any order. However, keep in mind that your configuration files will typically rely on a static "seed" joiner node, so that node should be highly available.
- **Identity:** A node's Chord identity is derived from its `--advertise-addr`. Changing the advertised address effectively introduces a new node and requires a rejoin.
- **Virtual Nodes:** By default, each process runs 5 virtual nodes (`--virtual`), which are separately announced in the ring to distribute the load evenly. When expanding capacity, try to match the `--virtual` count across your servers.

### Addresses and Listeners
- **Listen Address (`--listen-addr`):** The local interface and port to bind for TCP and UDP traffic. You can override these per-protocol using `--listen-tcp` and `--listen-udp`.
- **Advertise Address (`--advertise-addr`):** The public, routable address that clients and other nodes use to reach this server. It must remain stable across restarts.
- **RPC Listener (`--listen-rpc`):** Exposes internal KV and node-to-node RPC endpoints. Because it lacks authentication, **always bind this to a loopback address (`127.0.0.1`) or a secure Unix socket.**

---

## Configuration Guide

When using packaged installs, Specter is configured via environment variable files. These files allow you to pass command-line arguments to the underlying `specter` binary safely.

### Shared Configuration
You can set `SPECTER_GLOBAL_ARGS` in any environment file to pass shared flags, such as `--verbose` for detailed logging.

### Server Configuration (`server.env`)
The server environment file configures the main edge gateway. Add `--join` for joiner nodes, and specify `--acme` if you want the gateway to automatically manage TLS certificates. You can also export process environment variables here for options that should not appear in the process list, such as internal admin credentials and the Sentry DSN.

```sh
# /etc/specter/server.env
SPECTER_GLOBAL_ARGS="--verbose"
export INTERNAL_USER="admin"
export INTERNAL_PASS="change-me"
export SENTRY_DSN="https://public@example.ingest.sentry.io/123"
SPECTER_SERVER_ARGS="--data-dir /var/lib/specter \
  --cert-dir /etc/specter/cert \
  --apex example.com \
  --listen-rpc unix:///run/specter/rpc.sock"
```

### DNS Solver Configuration (`dns.env`)
The DNS solver must share the same ACME configuration as the server. Using a Unix socket for `--rpc` is highly recommended if the solver is co-located with the gateway. The packaged launcher also sources this file as a shell environment file, but the `dns` command does not currently consume `SENTRY_DSN` or internal admin credentials.

```sh
# /etc/specter/dns.env
SPECTER_GLOBAL_ARGS="--verbose"
SPECTER_DNS_ARGS="--rpc unix:///run/specter/rpc.sock \
  --acme acme://ops@example.net@acme-hosted.net \
  --acme-ns ns1.acme-hosted.net/203.0.113.10"
```

### Client Configuration (`client.env`)
The client service reads a YAML configuration file to define the tunnels it should establish. 

```sh
# /etc/specter/client.env
SPECTER_GLOBAL_ARGS="--verbose"
SPECTER_CLIENT_CONFIG="/etc/specter/client.yaml"
SPECTER_CLIENT_ARGS=""
```

*Tip: A template is provided at `/usr/share/specter/examples/client.yaml` (Linux) which you can copy and modify for your needs.*

### Enabling Services
After configuring the `.env` files, enable and start the services using systemd. The packaged units run as the unprivileged `specter` user. The server and DNS units are granted only the privilege needed to bind low ports.

**Systemd (Linux):**
```sh
systemctl daemon-reload
systemctl enable --now specter-server
systemctl enable --now specter-dns
systemctl enable --now specter-client
```

### Post-install setup for packaged server and DNS services
Edit the packaged environment files before enabling the services:

```sh
# /etc/specter/server.env
SPECTER_GLOBAL_ARGS="--verbose"
export INTERNAL_USER="admin"
export INTERNAL_PASS="change-me"
export SENTRY_DSN="https://public@example.ingest.sentry.io/123"
SPECTER_SERVER_ARGS="--data-dir /var/lib/specter \
  --cert-dir /etc/specter/cert \
  --apex example.com \
  --listen-rpc unix:///run/specter/rpc.sock"

# add --join <seed-host:443> on non-seed nodes
# add --acme acme://ops@example.net@acme-hosted.net if the gateway should issue certs
```

```sh
# /etc/specter/dns.env
SPECTER_GLOBAL_ARGS="--verbose"
SPECTER_DNS_ARGS="--rpc unix:///run/specter/rpc.sock \
  --acme acme://ops@example.net@acme-hosted.net \
  --acme-ns ns1.acme-hosted.net/203.0.113.10"
```

Then reload systemd and start the units:

```sh
systemctl daemon-reload
systemctl enable --now specter-server
systemctl enable --now specter-dns
```

---

## Advanced Operations

### ACME and Automatic Certificates
Specter integrates tightly with ACME providers (like Let's Encrypt) to automate TLS certificates for your tunnels.

1. **Apex Certificates:** For every domain listed in `--apex`, the gateway expects a `_acme-challenge.<apex>` CNAME pointing to your managed ACME zone. The co-located `specter dns` solver answers the ACME TXT queries by fetching the expected challenge values from the distributed KV store.
2. **Custom Domains:** Users can request certificates for custom domains.
   - The client runs `specter client acme <domain>` to get the exact DNS record name and content to publish.
   - The client publishes the returned `_acme-challenge.<domain>` CNAME.
   - Once the DNS record propagates, running `specter client validate <domain>` asks the gateway to verify DNS and obtain the certificate.
   - Finally, the user points their custom `<domain>` at the Specter apex, typically with a CNAME.

### Storage and Durability
The `--data-dir` is critical for Specter's operation. Each virtual node stores its key-value data in `<data-dir>/<index>`.

Specter supports different KV backends:
- **`aof` (Append-Only File):** The default backend. It uses a write-ahead log and flushes to disk approximately every 3 seconds.
- **`sqlite`:** Offers single-file durability and performs well on slower disks. It uses `<data-dir>/cache` for its native cache.
- **`memory`:** Non-persistent storage, strictly for testing.

**Important:** You must persist the entire `--data-dir`. It holds tunnel routing mappings, client tokens and hostname bindings, custom-host validation state, and ACME-related state when ACME is enabled. Using ephemeral storage will result in orphaned tunnels and force unnecessary ACME re-issuance.

### Observability and Internal Endpoints
Specter provides a suite of administrative and observability endpoints under the `/_internal` path. 

To enable these, you must configure admin credentials by setting the `INTERNAL_USER` and `INTERNAL_PASS` environment variables (or using `--auth_user` / `--auth_pass`). Without these credentials, the `/_internal` routes are disabled. When apex handling is enabled, the same apex router is also exposed locally on `127.0.0.1:9999`.

Once authenticated, you can access:
- ACME certificate inventory
- Chord DHT statistics and routing graphs
- Active tunnel views
- Profiling data (`pprof`/`expvar`)

By default, Specter logs in JSON format to standard error. Passing the `--verbose` flag switches this to human-readable, colored development logging. For error tracking, providing a `SENTRY_DSN` (or `--sentry` flag) will automatically send errors and breadcrumbs to Sentry.

---

## Operational Tips

- **Graceful Shutdown:** Specter handles `SIGINT` and `SIGTERM` signals by gracefully calling `Leave` on each virtual node in the Chord ring. Ensure your orchestrator gives the process at least 60–120 seconds to allow for ownership transfers to complete before forcefully terminating it.
- **Rolling Restarts:** When changing `--advertise-addr`, plan your deployment carefully because it changes the node identity. Add the newly configured nodes to the cluster first, allow them to stabilize, and then remove the old ones. Changes to `--apex` or CA material still need a coordinated rollout, but they do not by themselves change the Chord node ID.
- **PROXY Protocol:** Only enable `--proxy-protocol` if your upstream load balancer explicitly injects it. When enabled, any TCP connection lacking a valid PROXY header will be rejected.
- **Data Backups:** Always back up your `--data-dir` before performing migrations or changing KV providers. If using the SQLite backend, ensure the `cache/` subdirectory is included in your backup.
