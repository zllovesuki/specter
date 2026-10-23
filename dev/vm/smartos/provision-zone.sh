set -eu
PATH=/usr/sbin:/usr/bin:/sbin:/bin
export PATH

zone_uuid=$1
image_uuid=$2
gateway=$3
staging=/var/tmp/specter-vm

if ! imgadm get "$image_uuid" >/dev/null 2>&1; then
    imgadm install -m "$staging/image.json" -f "$staging/image.zfs.gz"
fi
if ! vmadm get "$zone_uuid" >/dev/null 2>&1; then
    vmadm create -f "$staging/zone.json"
fi
if [ "$(vmadm list -H -o state uuid="$zone_uuid")" != running ]; then
    vmadm start "$zone_uuid"
fi

# vmadm's running state precedes SMF restoring DHCP and persistent routes.
tries=0
while [ "$(zlogin "$zone_uuid" svcs -H -o state svc:/milestone/multi-user:default 2>/dev/null)" != online ]; do
    if [ "$tries" -ge 60 ]; then
        echo "Native zone did not reach the multi-user milestone" >&2
        exit 1
    fi
    tries=$((tries + 1))
    sleep 1
done

zlogin "$zone_uuid" /bin/sh -s <<'EOF'
set -eu
PATH=/usr/sbin:/usr/bin:/sbin:/bin
export PATH
if ! getent group specter >/dev/null 2>&1; then
    groupadd specter
fi
if ! id specter >/dev/null 2>&1; then
    useradd -g specter -d /var/lib/specter-native -s /bin/sh specter
fi
mkdir -p /var/lib/specter-native/work /var/lib/specter-native/tmp
chown -R specter:specter /var/lib/specter-native
chmod 700 /var/lib/specter-native /var/lib/specter-native/tmp
EOF

# An isolated libvirt network omits the DHCP default route. Keep one for local
# source-address selection; the host still does not forward guest traffic.
zlogin "$zone_uuid" route -p add default "$gateway" >/dev/null 2>&1 ||
    zlogin "$zone_uuid" route change default "$gateway"
