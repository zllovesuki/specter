# 👻 Specter
[![GoDoc](https://godoc.org/github.com/urfave/cli?status.svg)](https://pkg.go.dev/kon.nect.sh/specter)
![badge](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/zllovesuki/49efb3a7978bf0df7d91bfad39da7092/raw/specter.json?style=flat)

## What is it?

Specter aims to create a distributed network of nodes that forms an overlay network, allowing users to create a reverse tunnel on any publicly exposed node, then redundantly route the traffic back to the user, without having the user's machine exposed to the Internet.

Specter is the spiritual successor of [t](https://github.com/zllovesuki/t/), another similar reverse tunnel software written by me.

Specter has these improvements over t:
1. Built-in Chord DHT for distributed KV storage for routing information;
    - t uses a simple periodic updates via [hashicorp/memberlist](https://github.com/hashicorp/memberlist)
    - meaning that if you have 100 nodes each with 100 clients connected, every single node has to maintain an 100*100 list about where the client is connected to
2. Redundant connections from client to edge nodes and self-healing to maintain the tunnel;
    - t only connects to 1 public node and can encounter downtime when a node is having an outage
    - this has caused headache in production usage and causes assets to be unreachable
3. Robust testing to ensure correctness (`make extended_test` and `make long_test`).
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
| Chord KV       | Beta   | Key transfers require more in depth testing               |
| Tunnel Core    | Beta   | Storage format is subjected to change                     |
| Tunnel Gateway | Alpha  |                                                           |
| Server         | Beta   |                                                           |
| Client         | Alpha  |                                                           |

## Roadmap

*Chord DHT*
- [ ] Implement a Dynamo-like replication scheme
- [ ] Expose KV functionalities via HTTP/SDK

*Tunnel Core*
- [ ] Support multiple tunnels to a single client endpoint
- [ ] Persistent tunnel hostnames
- [ ] Support UDP forwarding from edge to client

*Tunnel Gateway*
- [ ] Uses HTTP/3 for handling eyeball traffic
- [ ] Support custom hostname and Let's Encrypt cert issurance

## Contributing

The following should be installed on your machine:
- Go 1.18+
- [protoc](https://grpc.io/docs/protoc-installation)
- [proto-gen-go](https://developers.google.com/protocol-buffers/docs/reference/go-generated)
- [protoc-gen-go-vtproto](https://github.com/planetscale/vtprotobuf#Usage)

See `Makefile` for more details.

## References

Chord:
- Original Chord paper: [Chord: A Scalable Peer-to-peer Lookup Protocol for Internet Applications](https://pdos.csail.mit.edu/papers/ton:chord/paper-ton.pdf)
- Improvement on the original paper: [How to Make Chord Correct](https://arxiv.org/pdf/1502.06461.pdf)