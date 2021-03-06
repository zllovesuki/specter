# 👻 Specter
[![GoDoc](https://godoc.org/github.com/urfave/cli?status.svg)](https://pkg.go.dev/kon.nect.sh/specter)
![badge](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/zllovesuki/49efb3a7978bf0df7d91bfad39da7092/raw/specter.json?style=flat)

## What is it?

Specter aims to create a distributed network of nodes that forms an overlay network, allowing users to create a reverse tunnel on any publicly exposed node, then redundantly route the traffic back to the user, without having the user's machine exposed to the Internet.

## Why another tunnel project?

Specter is the spiritual successor of [t](https://github.com/zllovesuki/t/), another similar reverse tunnel software written by me.

Specter has these improvements over t:
1. Built-in Chord DHT for distributed KV storage for routing information;
    - t uses a simple periodic updates via [hashicorp/memberlist](https://github.com/hashicorp/memberlist)
    - meaning that if you have 100 nodes each with 100 clients connected, every single node has to maintain an 100*100 list about where the client is connected to
2. Redundant connections from client to edge nodes and self-healing to maintain the tunnel;
    - t only connects to 1 public node and can encounter downtime when a node is having an outage
    - this has caused headache in production usage and causes assets to be unreachable
3. Robust testing to ensure correctness (`make full_test`).
    - t has _zero_ tests whatsoever
    - development is difficult as I have no confidence on correctness

Similar to t, specter also:
1. Uses [quic](https://github.com/lucas-clemente/quic-go) and _only_ quic for inter-nodes/node-client transport. However the transport (see `spec/transport`) is easily extensible;
2. Supports tunneling _L7_(HTTP/S)/_L4_(TCP) traffic over a single TLS port;
3. Manages Let's Encrypt certificate via dns01 challange for gateway hostname.

## Status

| **Component**  | Status | Description                                               |
|----------------|--------|-----------------------------------------------------------|
| Chord DHT      | Stable | Chord implementation is stable to be used as a dependency |
| Chord KV       | Beta*  | Key consistency is maintained when independent Join/Leave is happening while concurrent KV operations are inprogress. However, concurrent Join/Leave behavior is currently undefined               |
| Tunnel Core    | Beta   | Storage format is subjected to change                     |
| Tunnel Gateway | Alpha  |                                                           |
| Server         | Beta   |                                                           |
| Client         | Alpha  |                                                           |

## Roadmap

Please see issues under [Roadmap](https://github.com/zllovesuki/specter/issues?q=is%3Aissue+is%3Aopen+sort%3Aupdated-desc+label%3ARoadmap).

## Development

The following should be installed on your machine:
- Docker with buildx support
- Go 1.18+
- [protoc](https://grpc.io/docs/protoc-installation)
- [protoc-gen-go](https://developers.google.com/protocol-buffers/docs/reference/go-generated)
- [protoc-gen-go-vtproto](https://github.com/planetscale/vtprotobuf#Usage)

Run `make dev` to compile binary for your architecture via buildx, bring up Let's Encrypt test server `pebble`, and a 5-node specter cluster.

For changes unrelated to KV, `make test` should be sufficient. Any changes to KV must pass `make concurrency_test`.

## References

Chord:
- Original Chord paper: [Chord: A Scalable Peer-to-peer Lookup Protocol for Internet Applications](https://pdos.csail.mit.edu/papers/ton:chord/paper-ton.pdf)
- Improvement on the original paper: [How to Make Chord Correct](https://arxiv.org/pdf/1502.06461.pdf)

Key Consistency:
- Inspiraion on join/leave KV correctness: [Atomic Data Access in Distributed Hash Tables](https://citeseerx.ist.psu.edu/viewdoc/download?doi=10.1.1.71.6111&rep=rep1&type=pdf)