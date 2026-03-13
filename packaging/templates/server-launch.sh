#!/bin/sh
set -eu
ENV_FILE="${SPECTER_SERVER_ENV_FILE:-__SYSCONFDIR__/specter/server.env}"
if [ -f "$ENV_FILE" ]; then
	. "$ENV_FILE"
fi
: "${SPECTER_SERVER_ARGS:?}"
exec __BINDIR__/specter ${SPECTER_GLOBAL_ARGS:-} server ${SPECTER_SERVER_ARGS}
