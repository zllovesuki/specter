#!/bin/sh
set -eu

create_group() {
	if getent group specter >/dev/null 2>&1; then
		return
	fi
	if command -v groupadd >/dev/null 2>&1; then
		groupadd -r specter >/dev/null 2>&1 || groupadd --system specter >/dev/null 2>&1 || groupadd specter
		return
	fi
	if command -v addgroup >/dev/null 2>&1; then
		addgroup --system specter >/dev/null 2>&1 || addgroup specter
		return
	fi
	echo "unable to create specter group" >&2
	exit 1
}

create_user() {
	if getent passwd specter >/dev/null 2>&1; then
		return
	fi
	nologin=/usr/sbin/nologin
	if [ ! -x "$nologin" ]; then
		nologin=/sbin/nologin
	fi
	if [ ! -x "$nologin" ]; then
		nologin=/bin/false
	fi
	if command -v useradd >/dev/null 2>&1; then
		useradd --system --gid specter --home-dir /var/lib/specter --no-create-home --shell "$nologin" specter >/dev/null 2>&1 || \
			useradd -r -g specter -d /var/lib/specter -M -s "$nologin" specter >/dev/null 2>&1 || \
			useradd -g specter -d /var/lib/specter -M -s "$nologin" specter
		return
	fi
	if command -v adduser >/dev/null 2>&1; then
		adduser --system --ingroup specter --home /var/lib/specter --no-create-home --shell "$nologin" specter >/dev/null 2>&1 || \
			adduser -S -G specter -h /var/lib/specter -s "$nologin" specter >/dev/null 2>&1 || \
			adduser -D -H -G specter -h /var/lib/specter -s "$nologin" specter
		return
	fi
	echo "unable to create specter user" >&2
	exit 1
}

create_group
create_user

mkdir -p /etc/specter/cert /var/lib/specter /var/log/specter

chown root:specter /etc/specter
chmod 0750 /etc/specter

chown -R specter:specter /etc/specter/cert /var/lib/specter /var/log/specter
chmod 0750 /etc/specter/cert /var/lib/specter /var/log/specter

for env_file in /etc/specter/server.env /etc/specter/dns.env /etc/specter/client.env; do
	if [ -f "$env_file" ]; then
		chown root:specter "$env_file"
		chmod 0640 "$env_file"
	fi
done

if [ -f /etc/specter/client.yaml ]; then
	chown specter:specter /etc/specter/client.yaml
	chmod 0640 /etc/specter/client.yaml
fi

if command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi
