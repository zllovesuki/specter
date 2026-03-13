#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
package_version=${PACKAGE_VERSION:-0.0.0}
package_release=${PACKAGE_RELEASE:-edge.$(git -C "$repo_root" rev-parse --short HEAD)}
arches=${LINUX_PACKAGE_ARCHES:-"amd64 arm64 arm"}
out_root=${OUT_ROOT:-"$repo_root/dist/pkg/linux"}
container_repo_root=/work

mkdir -p "$out_root"

if command -v nfpm >/dev/null 2>&1; then
	nfpm_mode=native
elif command -v docker >/dev/null 2>&1; then
	nfpm_mode=docker
else
	echo "nfpm or docker is required" >&2
	exit 1
fi

run_nfpm() {
	config_path=$1
	target_path=$2
	packager=$3
	case "$nfpm_mode" in
		native)
			nfpm package --config "$config_path" --target "$target_path" --packager "$packager"
			;;
		docker)
			docker run --rm -v "$repo_root:/work" -w /work goreleaser/nfpm:latest \
				package --config "$config_path" --target "$target_path" --packager "$packager"
			;;
	esac
}

for arch in $arches; do
	stage_root=$(STAGE_ROOT="$repo_root/dist/stage/linux-$arch" sh "$repo_root/scripts/package-stage.sh" linux "$arch")
	work_root="$repo_root/dist/pkg/linux/$arch"
	case "$nfpm_mode" in
		native)
			runtime_repo_root=$repo_root
			runtime_stage_root=$stage_root
			runtime_work_root=$work_root
			;;
		docker)
			runtime_repo_root=$container_repo_root
			runtime_stage_root="$container_repo_root/dist/stage/linux-$arch"
			runtime_work_root="$container_repo_root/dist/pkg/linux/$arch"
			;;
	esac
	mkdir -p "$work_root"
	config_path="$work_root/nfpm.yaml"
	cat > "$config_path" <<EOF
name: specter
arch: $arch
platform: linux
version: $package_version
release: $package_release
section: net
priority: optional
maintainer: miragespace <opensource@miragespace.co>
description: Specter edge server, ACME DNS solver, and client tunnel service
vendor: miragespace
homepage: https://github.com/zllovesuki/specter
license: MIT
contents:
  - src: $runtime_stage_root/usr/bin/specter
    dst: /usr/bin/specter
    file_info:
      mode: 0755
  - src: $runtime_stage_root/usr/libexec/specter/server-launch
    dst: /usr/libexec/specter/server-launch
    file_info:
      mode: 0755
  - src: $runtime_stage_root/usr/libexec/specter/dns-launch
    dst: /usr/libexec/specter/dns-launch
    file_info:
      mode: 0755
  - src: $runtime_stage_root/usr/libexec/specter/client-launch
    dst: /usr/libexec/specter/client-launch
    file_info:
      mode: 0755
  - src: $runtime_stage_root/usr/share/specter/examples/client.yaml
    dst: /usr/share/specter/examples/client.yaml
    type: config|noreplace
  - src: $runtime_stage_root/etc/specter/server.env
    dst: /etc/specter/server.env
    type: config|noreplace
  - src: $runtime_stage_root/etc/specter/dns.env
    dst: /etc/specter/dns.env
    type: config|noreplace
  - src: $runtime_stage_root/etc/specter/client.env
    dst: /etc/specter/client.env
    type: config|noreplace
  - src: $runtime_stage_root/etc/specter/client.yaml
    dst: /etc/specter/client.yaml
    type: config|noreplace
  - src: $runtime_stage_root/lib/systemd/system/specter-server.service
    dst: /lib/systemd/system/specter-server.service
  - src: $runtime_stage_root/lib/systemd/system/specter-dns.service
    dst: /lib/systemd/system/specter-dns.service
  - src: $runtime_stage_root/lib/systemd/system/specter-client.service
    dst: /lib/systemd/system/specter-client.service
  - dst: /etc/specter/cert
    type: dir
  - dst: /var/lib/specter
    type: dir
  - dst: /var/log/specter
    type: dir
scripts:
  postinstall: $runtime_repo_root/packaging/linux/postinstall.sh
  postremove: $runtime_repo_root/packaging/linux/postremove.sh
EOF
	case "$nfpm_mode" in
		native)
			runtime_config_path=$config_path
			;;
		docker)
			runtime_config_path="$container_repo_root/dist/pkg/linux/$arch/nfpm.yaml"
			;;
	esac
	run_nfpm "$runtime_config_path" "$runtime_work_root" deb
	run_nfpm "$runtime_config_path" "$runtime_work_root" rpm
done
