# Lightweight tunnels

Expose one upstream without a client configuration. Lightweight clients keep a
fresh key and certificate in memory and print a URL when the server accepts the
tunnel. This does not indicate whether the upstream is healthy. Supported targets
are HTTP, HTTPS, TCP, Unix sockets, and Windows named pipes.

## Temporary URL

```sh
specter client expose --apex tunnels.example.com http://127.0.0.1:8080
```

The process connects through the apex initially. After receiving a URL, it
retries the server that assigned it and keeps the same URL. Exiting ends access;
starting a new process creates a new URL.

## Registered hostname

Register a generated hostname or custom domain with a full client first. Use
**Unpublish** in that client's UI to remove its target while keeping the hostname.
Run only one client serving that hostname at a time; concurrent clients replace
each other's routes.

With the owner client stopped, mint a token using its configuration:

```sh
specter client token mint --config owner.yaml app.example.com
```

Mint prints JSON. Save the `token` value in `tunnel.token`; it cannot be retrieved
later. Grants have no expiry by default; add `--expires-at` with a future RFC3339
timestamp to set one.

```sh
specter client serve --apex tunnels.example.com --token-file tunnel.token http://127.0.0.1:8080
```

Alternatively, set `SPECTER_TUNNEL_TOKEN` and omit `--token-file`. The token is read
at startup, so restart the process after changing it. The owner can stay offline.
Token clients discover peers from the apex and maintain up to three server
connections. Discovery uses the same bounded successor view as the full client
and may return fewer peers. The URL is printed after the first connection is
ready; additional connections are established afterward. Connection logs include
the grant ID and expiry (`none` for a grant without expiry), never the bearer token.

Manage grants with the owner stopped:

```sh
specter client token list --config owner.yaml
specter client token revoke --config owner.yaml GRANT_ID
```

While the owner runs, use **Domain tokens** in its UI or the local API:

| Method | Path | Request or result |
| --- | --- | --- |
| GET | `/api/tokens` | Grant metadata, without bearer tokens |
| POST | `/api/tokens` | `{"hostname":"app.example.com"}`; optional `expiresAt` in RFC3339; returns grant and token |
| POST | `/api/tokens/<id>/revoke` | Returns `revoked` and optional `indexError` |

If mint has an ambiguous outcome, list grants before retrying. Revoke incomplete
or unwanted entries. If revoke returns `revoked: true` with `indexError`, access
was revoked; repeat the command to finish cleanup.

## Connections and limits

`--apex host[:port]` selects the TLS authority, bootstrap server, and generated
URL port for both commands. The server supplies the URL's managed apex. Custom
domains use their own HTTPS authority.

Each token connection publishes a different route. Losing one interrupts its
existing streams while other connections continue serving. The client attempts
repair immediately, then uses normal 30-second maintenance while any connection
survives. A total outage uses jittered retries with backoff up to 30 seconds.

Token sessions check grants and ownership every 30 seconds. Temporary storage
failures allow at most two minutes since the last successful check, capped by
grant and certificate expiry. Revocation or expiry closes active streams;
reconnecting requires fresh validation. Lightweight clients exit when their
certificate expires; restart them to obtain a new one.

An owner can have 128 grants, including expired and incomplete entries. Revoke
unused grants to free capacity. Each server admits 1,024 lightweight sessions,
including unfinished setup and cleanup.

Revoke grants before releasing their hostname: reclaiming it can make an
unrevoked grant usable again. After restoring older server data, revoke unwanted
grants again.

See [compatibility](lightweight-compatibility.md) for server requirements and tests.
