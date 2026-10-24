# Web protection

Specter uses Coraza and OWASP Core Rule Set (CRS) to inspect public HTTP requests.
Protection runs on the servers. Existing clients benefit without changing
their tunnel configuration.

## Enable protection

Upgrade the gateways, then open **Web protection** in the operator page at
`/_internal/config/waf`. Protection starts Off. Enable it and save once; the
settings propagate to the other servers without an initialization command.

**Block** rejects requests that trigger the rules. **Monitor** logs the tunnel
hostname and matched rule IDs without blocking. Disabling protection in the
operator page overrides every hostname's settings.

Choose **LTS** for targeted security and maintenance updates, or **Current
stable** for newer detection improvements. Both receive automatic rule updates.
The page shows the selected server's active rules version and update errors.
Keep Specter itself updated for engine fixes and newer supported rule releases.

## Resolve a mistaken block

The denial response includes a request ID and the rules that matched. Operators
can search server logs for the same `request_id` to find the matching event.
The gateway assigns the ID, replacing any visitor-supplied `X-Request-ID`.
It returns the ID in the response header and forwards it to the application
for allowed requests, including in Monitor and Off modes. The client's debug
access logs and forwarding-error logs include it; applications can log the
header to correlate their own events.

Open **Web protection** in the client UI and select the registered hostname.
Add a rule exception using the ID from the denial. Specify an exact URL path to
limit the exception, or leave the path empty to apply it to the whole hostname.
Paths use their URL-escaped form without a query string; `/api/save` matches
that path regardless of its query. Choose **Enable again** to remove an exception.

Exceptions apply to detection rules present in the active release. Request
safeguards are not eligible. If a request is still blocked, check its response
for other matching rules.

Owners can also inherit the operator default, choose Block or Monitor, or turn
protection Off for their hostname. Unpublishing preserves these settings;
releasing the hostname removes them. Delegated clients inherit the owner's
settings. Temporary hostnames use the operator default.

## Inspection coverage

Requests for a destination known to be missing return 404 before WAF inspection.
This follows the existing route cache; it does not check whether the application
is healthy.

The WAF checks request URLs, query parameters, and headers. It also inspects up
to 16 KiB of supported form, JSON, XML, and multipart bodies. Larger bodies
are forwarded intact when allowed. Content beyond the limit is uninspected;
structured fields cut off at that point may also be unavailable to the rules.

Binary and recognized streaming request formats receive URL and header checks.
Response bodies and WebSocket frames are not inspected. Response streaming and
WebSocket upgrades remain available. Raw TCP, CONNECT tunnels, and direct LAN
keyless access are outside this WAF.

## Updates and recovery

Rule updates are signed. Failed updates leave working protection active and
appear on the operator page. Cached rules under `<data-dir>/waf` and the bundled
baseline allow servers to start offline.

Under **Advanced**, choose **Use previous version** and save to pin the cluster
to the version shown. This pauses automatic updates. Choose **Resume automatic
updates** and save to follow the selected channel again. Changing channels also
removes a pin. Working rules stay active until the requested version is ready.

Preserve the data directory during normal upgrades. A fresh replacement starts
unconfigured until shared settings arrive; check its active state before routing
traffic to it when uninterrupted protection is required.
