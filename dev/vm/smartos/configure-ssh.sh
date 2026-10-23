set -eu
umask 077

# /etc/ssh is backed by /usbkey/ssh; /root is recreated on every boot.
cat > /usbkey/ssh/specter_authorized_keys
config=/usbkey/ssh/sshd_config
sed \
    -e '/^[[:space:]]*AuthorizedKeysFile[[:space:]]/d' \
    -e '/^[[:space:]]*PasswordAuthentication[[:space:]]/d' \
    -e '/^[[:space:]]*ChallengeResponseAuthentication[[:space:]]/d' \
    "$config" > "$config.new"
cat >> "$config.new" <<'EOF'
AuthorizedKeysFile /etc/ssh/specter_authorized_keys
PasswordAuthentication no
ChallengeResponseAuthentication no
EOF
/usr/lib/ssh/sshd -t -f "$config.new"
mv "$config.new" "$config"
svcadm refresh svc:/network/ssh:default
