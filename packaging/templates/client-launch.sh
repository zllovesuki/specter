#!/bin/sh
set -eu
ENV_FILE="${SPECTER_CLIENT_ENV_FILE:-__SYSCONFDIR__/specter/client.env}"
if [ -f "$ENV_FILE" ]; then
	. "$ENV_FILE"
fi
: "${SPECTER_CLIENT_CONFIG:=__SYSCONFDIR__/specter/client.yaml}"
if [ ! -f "$SPECTER_CLIENT_CONFIG" ]; then
	echo "missing client config: $SPECTER_CLIENT_CONFIG" >&2
	exit 1
fi
exec __BINDIR__/specter ${SPECTER_GLOBAL_ARGS:-} client tunnel --config "$SPECTER_CLIENT_CONFIG" ${SPECTER_CLIENT_ARGS:-}
