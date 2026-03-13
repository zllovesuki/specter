#!/bin/sh
set -eu
ENV_FILE="${SPECTER_DNS_ENV_FILE:-__SYSCONFDIR__/specter/dns.env}"
if [ -f "$ENV_FILE" ]; then
	. "$ENV_FILE"
fi
: "${SPECTER_DNS_ARGS:?}"
exec __BINDIR__/specter ${SPECTER_GLOBAL_ARGS:-} dns ${SPECTER_DNS_ARGS}
