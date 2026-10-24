# Shared configuration

Shared configuration lets you change DNS and web protection settings once,
through the operator page. Each Specter server keeps a local copy and
synchronizes with its peers. Settings take effect without a restart.

DNS records and [web protection](web-protection.md) settings are shared. Listener
addresses, certificates, join peers, and the ACME email and zone still belong
in your deployment configuration. Installations without Specter DNS need no
initial configuration.

## First setup or upgrade

Initialize one server, then let the others synchronize from it.

Create `initial-config.json` using your existing ACME zone and nameservers.
When upgrading, these are the values previously supplied through `ACME_URI`
and `ACME_NS`:

```json
[
  {
    "name": "dns",
    "schema": 1,
    "value": {
      "zone": "acme.example.com",
      "nameservers": [
        {"host": "ns1.acme.example.com", "addresses": ["192.0.2.10"]},
        {"host": "ns2.acme.example.com", "addresses": ["192.0.2.20", "2001:db8::20"]}
      ]
    }
  }
]
```

1. Install the new version on the first server. Run the initializer as the
   server's service user, using its actual data directory:

   ```sh
   specter config init --data-dir /var/lib/specter --file initial-config.json
   ```

   The server may already be running. Initialization refuses to replace any
   existing documents.

2. Start the server and DNS services if they are not already running. DNS waits
   for usable configuration automatically. Query DNS to check its records before
   proceeding to the next server.

3. Upgrade the remaining servers one at a time and start their services normally.
   Check DNS before moving to the next server. Run the initializer only on the
   first server. Older servers cannot relay shared configuration, so upgraded
   servers must be able to reach one another.

Keep DNS's existing `--acme`, `--rpc`, and listener arguments. The updated DNS
process reads its records from the server; `--acme-ns` and `ACME_NS` are accepted
for compatibility but ignored. A new or restarted DNS process waits until
usable configuration is available.

## Edit DNS settings

Open **ACME DNS** in the operator page at `/_internal/config/dns`. The page uses the
existing operator credentials, configured through `INTERNAL_USER` and
`INTERNAL_PASS`.

Save nameserver and address changes there. The zone must match the existing
ACME zone and cannot be changed through this editor. Records at your registrar
or parent DNS provider must still be updated separately when needed.
Use **Refresh** to check for newer saved values; the editor preserves unsaved edits.

A successful save is durable on the server named in the response and starts
propagation to its peers. Servers also check their local configuration and peer
connections at startup and every 15 seconds. They exchange configuration when
it changes or a peer needs to catch up, retrying failed exchanges automatically.
A change may pass through several peers before reaching every server. DNS
checks for changes every five seconds and keeps its last working records if
its server is unavailable or an update cannot be used.
If a save's result is uncertain, retry it or refresh to review saved values.
The editor preserves your draft and the original version guard; a committed
save may make a retry report a conflict. A read shows the recovered state
without confirming that an uncertain save is durable.

The local RPC listener exposes `ConfigService.Get` to read a document by name.
It does not expose peer `Exchange` or configuration writes. Keep it on loopback
or a protected Unix socket. Authenticated operator API calls use `GET` and
`PUT /_internal/api/config/dns`.

## Restarts and backups

For routine upgrades, preserve the data directory and restart normally. Do
not rerun initialization. A replacement server can obtain configuration from
surviving peers after joining the cluster.

Configuration lives in `<data-dir>/config/db`. Stop the server before copying
its `config` directory for backup. Replication also copies mistaken edits, so
keep a separate backup. Restore with the server stopped; an older restored copy
will catch up if newer copies survive elsewhere.

Use a separate data directory for each server process. Shared configuration
supports up to 4,096 documents, with values up to 256 KiB each and an 8 MiB
encoded snapshot. The combined documents across peers must fit these limits;
an oversized merge is rejected without changing the local copy.

Upgrade consumers before enabling settings that require a newer document
format. An older running DNS process can retain its previous records, but an
older binary starting from scratch cannot use an unsupported document.

## Local development

Initialize the Compose seed volume once, then start the cluster:

```sh
make dev-init-config
make dev-server-acme
```

The initializer uses the example in [dev/shared-config.json](../dev/shared-config.json).
Keep the volumes between runs. Initialize again only after deliberately
removing all of the cluster's configuration data.
