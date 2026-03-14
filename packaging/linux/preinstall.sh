#!/bin/sh
set -eu

state_dir=/run/specter-package
stop_order="specter-dns.service specter-server.service specter-client.service"

record_and_stop_service() {
	service=$1
	marker="$state_dir/$service.restart"

	rm -f "$marker"
	if ! systemctl is-active --quiet "$service"; then
		return
	fi

	: > "$marker"
	systemctl stop "$service"
}

if ! command -v systemctl >/dev/null 2>&1; then
	exit 0
fi

mkdir -p "$state_dir"
chmod 0700 "$state_dir"

for service in $stop_order; do
	record_and_stop_service "$service"
done
