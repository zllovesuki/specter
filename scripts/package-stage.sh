#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: $0 <os> <arch>" >&2
	exit 1
fi

target_os=$1
arch=$2
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage_root=${STAGE_ROOT:-"$repo_root/dist/stage/$target_os-$arch"}
binary_path=${BINARY_PATH:-"$repo_root/bin/specter-$target_os-$arch"}

if [ ! -f "$binary_path" ]; then
	echo "missing binary: $binary_path" >&2
	exit 1
fi

case "$target_os" in
	linux)
		sysconfdir=/etc
		bindir=/usr/bin
		libexecdir=/usr/libexec/specter
		sharedir=/usr/share/specter
		serviceroot=/lib/systemd/system
		packaging_supported=1
		;;
	freebsd)
		sysconfdir=/usr/local/etc
		bindir=/usr/local/bin
		libexecdir=/usr/local/libexec/specter
		sharedir=/usr/local/share/specter
		serviceroot=/usr/local/etc/rc.d
		packaging_supported=0
		;;
	illumos)
		sysconfdir=/opt/local/etc
		bindir=/opt/local/bin
		libexecdir=/opt/local/libexec/specter
		sharedir=/opt/local/share/specter
		serviceroot=/lib/svc/manifest/site
		packaging_supported=0
		;;
	*)
		echo "unsupported os: $target_os" >&2
		exit 1
		;;
esac

if [ "$packaging_supported" -ne 1 ]; then
	echo "packaging for $target_os is currently unsupported" >&2
	exit 1
fi

rm -rf "$stage_root"
mkdir -p \
	"$stage_root$bindir" \
	"$stage_root$libexecdir" \
	"$stage_root$sharedir/examples" \
	"$stage_root$sysconfdir/specter/cert" \
	"$stage_root/var/lib/specter" \
	"$stage_root/var/log/specter"

install -m 0755 "$binary_path" "$stage_root$bindir/specter"
install -m 0644 "$repo_root/tun/client/config.example.yaml" "$stage_root$sharedir/examples/client.yaml"
install -m 0644 "$repo_root/tun/client/config.example.yaml" "$stage_root$sysconfdir/specter/client.yaml"

cat > "$stage_root$sysconfdir/specter/server.env" <<EOF
SPECTER_GLOBAL_ARGS=
SPECTER_SERVER_ARGS="--data-dir /var/lib/specter --cert-dir $sysconfdir/specter/cert --apex example.com --listen-rpc unix:///run/specter/rpc.sock"
EOF

cat > "$stage_root$sysconfdir/specter/dns.env" <<EOF
SPECTER_GLOBAL_ARGS=
SPECTER_DNS_ARGS="--rpc unix:///run/specter/rpc.sock --acme acme://ops@example.net@acme-hosted.net --acme-ns ns1.acme-hosted.net/203.0.113.10"
EOF

cat > "$stage_root$sysconfdir/specter/client.env" <<EOF
SPECTER_GLOBAL_ARGS=
SPECTER_CLIENT_CONFIG=$sysconfdir/specter/client.yaml
SPECTER_CLIENT_ARGS=
EOF

render_template() {
	sed \
		-e "s|__BINDIR__|$bindir|g" \
		-e "s|__SYSCONFDIR__|$sysconfdir|g" \
		"$1" > "$2"
}

render_template "$repo_root/packaging/templates/server-launch.sh" "$stage_root$libexecdir/server-launch"
render_template "$repo_root/packaging/templates/dns-launch.sh" "$stage_root$libexecdir/dns-launch"
render_template "$repo_root/packaging/templates/client-launch.sh" "$stage_root$libexecdir/client-launch"
chmod 0755 \
	"$stage_root$libexecdir/server-launch" \
	"$stage_root$libexecdir/dns-launch" \
	"$stage_root$libexecdir/client-launch"

case "$target_os" in
	linux)
		mkdir -p "$stage_root$serviceroot"
		install -m 0644 "$repo_root/packaging/linux/specter-server.service" "$stage_root$serviceroot/specter-server.service"
		install -m 0644 "$repo_root/packaging/linux/specter-dns.service" "$stage_root$serviceroot/specter-dns.service"
		install -m 0644 "$repo_root/packaging/linux/specter-client.service" "$stage_root$serviceroot/specter-client.service"
		;;
	freebsd)
		mkdir -p "$stage_root$serviceroot"
		install -m 0755 "$repo_root/packaging/freebsd/specter_server" "$stage_root$serviceroot/specter_server"
		install -m 0755 "$repo_root/packaging/freebsd/specter_dns" "$stage_root$serviceroot/specter_dns"
		install -m 0755 "$repo_root/packaging/freebsd/specter_client" "$stage_root$serviceroot/specter_client"
		;;
	illumos)
		mkdir -p "$stage_root$serviceroot"
		install -m 0644 "$repo_root/packaging/illumos/specter-server.xml" "$stage_root$serviceroot/specter-server.xml"
		install -m 0644 "$repo_root/packaging/illumos/specter-dns.xml" "$stage_root$serviceroot/specter-dns.xml"
		install -m 0644 "$repo_root/packaging/illumos/specter-client.xml" "$stage_root$serviceroot/specter-client.xml"
		;;
esac

echo "$stage_root"
