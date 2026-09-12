# Lightweight tunnel compatibility

Lightweight tunnels reuse the existing Chord/KV protocol and routing for
registered hostnames. Full clients do not need the new RPCs.

| Operation | Required server support |
| --- | --- |
| Mint, list, or revoke tokens | Delegation methods on `TunnelService` |
| Discover servers | `GetNodes` on `TunnelService`, with a verified client certificate |
| Connect with `client expose` or `client serve` | Open methods on `TunnelService`; delegated Open publishes one assigned route slot |
| Reach a token hostname through a gateway | Existing tunnel routing and forwarding |
| Reach an ephemeral URL through a gateway | Ephemeral hostname resolution |

Both lightweight clients bootstrap through their apex. Token clients discover
peers; ephemeral clients reconnect to the server that assigned their URL.
Unsupported peers do not replace working token connections. No token or URL is
returned when the operation is unsupported.
For ephemeral URLs, route wildcard DNS to gateways that support ephemeral
resolution before using `client expose`.

## Verify compatibility

[TestIntegrationCompat](../integrations/compat_test.go) compares the current code
with the fixed baseline revision `5d469e8f8eb1f9c9f92d70b76c61634c02e71ca9`.
It checks full-client interoperability, unsupported RPC responses, and token
traffic through a baseline gateway. It builds the baseline binary automatically
and requires its worktree to remain clean.

Use the Go and Node versions required by the checkouts, npm, and OpenSSL. From
the main checkout root, build the embedded UI and generate development
certificates if `certs/` is absent:

```sh
make ui
if [ ! -d certs ]; then make certs; fi
```

Create the baseline worktree once. The revision must be available in local Git
history; fetch the full history first if using a shallow clone. Its UI must also
be built because the Go binary embeds those assets.

```sh
git worktree add --detach .worktrees/lightweight-compat-old 5d469e8f8eb1f9c9f92d70b76c61634c02e71ca9
make -C .worktrees/lightweight-compat-old ui
```

Run from the main checkout root, with the test's loopback ports available:

```sh
GO_INTEGRATION_COMPAT=1 \
SPECTER_COMPAT_OLD_WORKTREE="$PWD/.worktrees/lightweight-compat-old" \
go test -timeout 900s -v -count=1 -run '^TestIntegrationCompat$' ./integrations
```

[TestIntegrationLightweight](../integrations/lightweight_test.go) covers ephemeral
routing and the token lifecycle on supporting servers. Run it with the same UI
and certificate prerequisites:

```sh
GO_INTEGRATION_TUNNEL=1 go test -race -timeout 600s -count=1 -run '^TestIntegrationLightweight$' ./integrations
```
