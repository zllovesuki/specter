# 👻 Specter

[![GoDoc](https://godoc.org/github.com/urfave/cli?status.svg)](https://pkg.go.dev/go.miragespace.co/specter)
![GitHub Workflow Status (with branch)](https://img.shields.io/github/actions/workflow/status/zllovesuki/specter/ci.yaml?branch=main)

## What is it?

Specter aims to create a distributed network of nodes that forms an overlay network, allowing users to create a reverse tunnel on any publicly exposed node, then redundantly route the traffic back to the user, without having the user's machine exposed to the Internet.

## Why another tunnel project?

<details>
<summary>Rationale</summary>
Specter is the spiritual successor of [t](https://github.com/zllovesuki/t/), another similar reverse tunnel software written by me.

(Also this is an excuse for me to play with DHT 😛)

Specter has these improvements over t:

1. Utilizes Chord DHT (with improvements) for distributed KV storage to store routing information;
   - t uses a simple periodic updates via [hashicorp/memberlist](https://github.com/hashicorp/memberlist)
   - meaning that if you have 100 nodes each with 100 clients connected, every single node has to maintain an 100\*100 list about where the client is connected to
2. Redundant connections from client to edge nodes and self-healing to maintain the tunnel;
   - t only connects to 1 public node and can encounter downtime when a node is having an outage
   - this has caused headache in production usage and causes assets to be unreachable
3. Robust testing to ensure correctness (`make full_test`).
   - t has _zero_ tests whatsoever
   - development is difficult as I have no confidence on correctness

Similar to t, specter also:

1. Uses [quic](https://github.com/lucas-clemente/quic-go) and _only_ quic for inter-nodes/node-client transport. However the transport (see `spec/transport`) is easily extensible;
2. Supports tunneling _L7_(HTTP/S)/_L4_(TCP) traffic over a single TLS port;
3. Manages Let's Encrypt certificate via dns01 challenge for gateway hostname.

specter also has some interesting features:

1. Support tunneling TCP or HTTP to Unix socket and Windows named pipe (!);
2. When connecting to the tunnel from gateway, error is propagated from the client.
</details>

## Client Configuration

### CLI

A sample barebone YAML config is needed for `specter client tunnel`:

```yaml
version: 2
apex: specter.im:443
tunnels:
  # following are shown as examples of what is valid
  - target: tcp://127.0.0.1:22
  - target: http://127.0.0.1:5173
  - target: https://example.com
  - target: unix:///run/unicorn.sock
  - target: \\.\pipe\debugger-ipc
```

On initial connection with specter gateway server, your configuration file will be updated:

```yaml
version: 2
apex: specter.im:443
certificate: |
  -----BEGIN CERTIFICATE-----
  ...
privKey: |
  -----BEGIN PRIVATE KEY-----
  ...
tunnels:
  - target: tcp://127.0.0.1:22
    hostname: overnight-graph-caboose-list-boney
  - target: http://127.0.0.1:5173
    hostname: dreamless-spirits-episode-gloomy-path
  # ...
```

### API

To manage custom hostnames, unpublish, release, or list tunnels, the `specter client tunnel` subcommand accepts an optional argument to start a local API server.

## Keyless TLS

If your specter client is running in the same local network as you, and split DNS is configured to have your custom domain pointing to the specter client's LAN address, specter will allow you to reuse the same Let's Encrypt certificate managed by the specter server, via keyless TLS. 

The private key will remain on the specter server to comply with Let's Encrypt ToS, and Keyless TLS is only enabled for custom domain validated via ACME.

Keyless TLS can be enabled on the `specter client tunnel` subcommand via option `--keyless [host]:[port]`

## Connecting to Tunnel (CLI)

### HTTP/HTTPS

For HTTP/HTTPS target, you can access the tunnel securely and directly using the assigned hostname.

### TCP/Unix/Named Pipe
 
For example, if your tunnel was assigned a hostname `overnight-graph-caboose-list-boney.specter.im`:

#### Using specter client (Recommended, Encrypted)

This command:
```
specter client connect overnight-graph-caboose-list-boney.specter.im
```
Will establish the tunnel via stdin/stdout. This can be used in combination with `ssh -o ProxyCommand` if the target is an SSH server:
```
ssh -o ProxyCommand="specter client connect overnight-graph-caboose-list-boney.specter.im" user@host
```

#### Using HTTP Connect (Unencrypted)

If the client environment does not allow you to run arbitary binary (therefore specter client is not available), HTTP Connect Proxy can be used as a fallback. For example:
```
nc -X connect -x specter.im:80 %h %p
```
Where `specter.im` is the apex of the specter server, and `80` is the http handler port (default to `80`). This can be used in combination with `ssh -o ProxyCommand` if the target is an SSH server:
```
ssh -o ProxyCommand="nc -X connect -x specter.im:80 %h %p" user@overnight-graph-caboose-list-boney.specter.im
```

**!! Note !!** The tunnel established using HTTP Connect is _unencrypted_. Therefore, the underlying protocol must support encryption (e.g. TLS, SSH, etc).

## Status

| **Component**  | Status | Description                                                                |
| -------------- | ------ | -------------------------------------------------------------------------- |
| Chord DHT      | Stable | Chord implementation is stable to be used as a dependency                  |
| Chord KV       | Stable | Key consistency is maintained during concurrent Join/Leave                 |
| Tunnel Core    | Beta   | RTT based intelligent routing is in development                            |
| Tunnel Gateway | Beta   | Unified multiplexing for TCP/TLS and QUIC with status feedback             |
| Tunnel Server  | Beta   | Server now supports persisting client tunnels                              |
| Tunnel Client  | Beta   | Client can publish multiple tunnels with redundant links with RTT tracking |

## Roadmap

Please see issues under [Roadmap](https://github.com/zllovesuki/specter/issues?q=is%3Aissue+is%3Aopen+sort%3Aupdated-desc+label%3ARoadmap).

## Development

The following should be installed on your machine:

- Docker with buildx support
- Go 1.24+
- [protoc](https://grpc.io/docs/protoc-installation)
- [protoc-gen-go](https://developers.google.com/protocol-buffers/docs/reference/go-generated)
- [protoc-gen-go-vtproto](https://github.com/planetscale/vtprotobuf#Usage)

Windows development support is limited, you may have to run `go test` manually instead of `make test`. However WSL is a great environment.

Run `make dev-server-acme` to compile binary for your architecture via buildx, bring up Let's Encrypt test server `pebble`, and a 5-node specter cluster.

Run `make dev-validate` to verify that all nodes have the same certificate (validate atomic ring maintenance).

Run `make dev-client` to start a demo nginx server as proxy target, and a specter client connected to the cluster.

For changes unrelated to KV, `make test` should be sufficient. Any changes to KV must pass `make concurrency_test`.

## References

Chord:

- Original Chord paper: [Chord: A Scalable Peer-to-peer Lookup Protocol for Internet Applications](https://pdos.csail.mit.edu/papers/ton:chord/paper-ton.pdf)
- Improvement on the original paper: [How to Make Chord Correct](https://arxiv.org/pdf/1502.06461.pdf)

Key Consistency:

- Inspiraion on join/leave KV correctness: [Atomic Data Access in Distributed Hash Tables](https://citeseerx.ist.psu.edu/viewdoc/download?doi=10.1.1.71.6111&rep=rep1&type=pdf)
- Partial atomic ring maintenance implementation: [Atomic Ring Maintenance for Distributed Hash Table](https://www.diva-portal.org/smash/get/diva2:1041775/FULLTEXT01.pdf), full dissertation is available [here](https://www.diva-portal.org/smash/get/diva2:1041220/FULLTEXT01.pdf)
  - We can see it in action in [concurrent_join.log](dev/concurrent_join.log) where the concurrent join attempt is blocked and asked to try again.
  - This is a departure from the paper where it asks for a lock queue.

KV Persistence:

- Inspired by Redis' AOF format, by appending log when mutation occurs, and replay mutation logs on start-up to restore state: [Redis persistence](https://redis.io/docs/manual/persistence/)
