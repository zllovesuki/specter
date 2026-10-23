"""SmartOS boot media and a persistent native zone for illumos tests."""

import json
import os
from pathlib import Path
import secrets
import shlex
import shutil
import subprocess
import urllib.request
import uuid


GOOS = "illumos"
SSH_USER = "root"
PLATFORM = "20261023T001807Z"
ISO_URL = (
    "https://us-central.manta.mnx.io/Joyent_Dev/public/SmartOS/"
    f"{PLATFORM}/smartos-{PLATFORM}.iso"
)
ISO_SHA256 = "64c06b2ae527f323c4e0bb6cb6e97529c3e32e29038c1a9a0114d854639d25d5"
IMAGE_UUID = "b933df4b-b8f4-4bef-ad1e-236a20304496"
IMAGE_URL = f"https://images.smartos.org/images/{IMAGE_UUID}"
IMAGE_SHA1 = "e6d4bc669b25a1cfc2d8020ee8945300c752e8a0"
HERE = Path(__file__).resolve().parent
GUEST_ROOT = "/var/lib/specter-native/work"


def prepare(ctx):
    backend = ctx.state.setdefault("backend", {})
    backend.setdefault("bootstrap_password", secrets.token_urlsafe(32))
    backend.setdefault("zone_uuid", str(uuid.uuid4()))
    ctx.save_state()
    source = ctx.fetch(ISO_URL, ISO_SHA256)
    media = ctx.state_dir / "smartos-autoinstall.iso"
    if not media.exists():
        seed = ctx.state_dir / "smartos-seed"
        seed.mkdir(mode=0o700, exist_ok=True)
        answers = {
            "config_console": True,
            "skip_instructions": True,
            "simple_headers": True,
            "admin_mac": ctx.mac,
            "admin_ip": "dhcp",
            "headnode_default_gateway": "none",
            "dns_resolver1": ctx.network["dns"],
            "dns_resolver2": ctx.network["dns"],
            "dns_search": "native.invalid",
            "ntp_host": "0.smartos.pool.ntp.org",
            "skip_ntp_check": True,
            "zpool_layout": "default",
            "zpool_confirm_layout": "yes",
            "bootpool": "none",
            "install_pkgsrc": "no",
            "root_password": backend["bootstrap_password"],
            "hostname": ctx.name,
            "skip_final_confirm": True,
        }
        answer_path = seed / "answers.json"
        answer_path.write_text(json.dumps(answers) + "\n")
        answer_path.chmod(0o600)
        shutil.copyfile(HERE / "loader.conf.local", seed / "loader.conf.local")
        _remaster(source, seed, media)
    return [
        {"role": "boot", "source": media, "media": True, "boot": 1},
        {
            "role": "system",
            "source": None,
            "size_gib": ctx.settings["disk_gib"],
            "media": False,
            "boot": 2,
        },
    ]


def _remaster(source, seed, media):
    partial = media.with_suffix(".partial.iso")
    partial.unlink(missing_ok=True)
    command = shutil.which("xorriso")
    if command:
        prefix = [command]
        original, files, output = str(source), str(seed), str(partial)
    else:
        if not shutil.which("docker"):
            raise RuntimeError("SmartOS media preparation needs xorriso or Docker")
        image = "specter-smartos-media:local"
        subprocess.run(
            ["docker", "build", "--tag", image, str(HERE)], check=True
        )
        prefix = [
            "docker", "run", "--rm", "--network=none",
            "--user", f"{os.getuid()}:{os.getgid()}",
            "--volume", f"{source}:/original.iso:ro",
            "--volume", f"{seed}:/seed:ro",
            "--volume", f"{media.parent}:/output",
            image,
        ]
        original, files, output = "/original.iso", "/seed", f"/output/{partial.name}"
    subprocess.run(
        prefix + [
            "-indev", original,
            "-outdev", output,
            "-map", f"{files}/answers.json", "/private/answers.json",
            "-map", f"{files}/loader.conf.local", "/boot/loader.conf.local",
            "-boot_image", "any", "replay",
            "-commit", "-end",
        ],
        check=True,
    )
    partial.chmod(0o600)
    partial.replace(media)


def provision(ctx):
    backend = ctx.state["backend"]
    # OpenSSH tries the persistent key first, then the first-boot password.
    # This also recovers an interrupted key install or a guest still rebooting.
    password = backend["bootstrap_password"]
    ctx.wait_ssh(password=password, timeout=900)
    ctx.ssh(
        (HERE / "configure-ssh.sh").read_text(),
        password=password,
        input=ctx.public_key.strip() + "\n",
    )
    ctx.wait_ssh(timeout=60)

    # The isolated bridge intentionally does not forward traffic, but Specter's
    # outbound-address selection still needs a local default route.
    gateway = ctx.network["gateway"]
    ctx.ssh(
        "set -eu; umask 077; sed "
        + shlex.quote(f"s/^headnode_default_gateway=.*/headnode_default_gateway={gateway}/")
        + " /usbkey/config > /usbkey/config.new; "
        "mv /usbkey/config.new /usbkey/config; "
        f"route -n add default {shlex.quote(gateway)} >/dev/null 2>&1 || "
        f"route -n change default {shlex.quote(gateway)}"
    )

    staging = "/var/tmp/specter-vm"
    ctx.ssh(f"umask 077; mkdir -p {staging}")
    if ctx.ssh(f"imgadm get {IMAGE_UUID} >/dev/null 2>&1", check=False).returncode:
        with urllib.request.urlopen(IMAGE_URL, timeout=60) as response:
            manifest = json.load(response)
        if (
            manifest.get("uuid") != IMAGE_UUID
            or manifest.get("name") != "base-64-lts"
            or manifest.get("version") != "25.4.0"
            or manifest.get("files", [{}])[0].get("sha1") != IMAGE_SHA1
        ):
            raise RuntimeError("SmartOS zone manifest differs from the pinned image")
        manifest_path = ctx.state_dir / "smartos-image.json"
        manifest_path.write_text(json.dumps(manifest) + "\n")
        archive = ctx.fetch(IMAGE_URL + "/file", IMAGE_SHA1, algorithm="sha1")
        ctx.upload(manifest_path, f"{staging}/image.json")
        ctx.upload(archive, f"{staging}/image.zfs.gz")

    zone = {
        "uuid": backend["zone_uuid"],
        "brand": "joyent",
        "image_uuid": IMAGE_UUID,
        "alias": "specter-native",
        "hostname": "specter-native",
        "autoboot": True,
        "max_physical_memory": 2048,
        "quota": 8,
        "nics": [{"nic_tag": "admin", "ips": ["dhcp"], "primary": True}],
    }
    zone_path = ctx.state_dir / "smartos-zone.json"
    zone_path.write_text(json.dumps(zone) + "\n")
    ctx.upload(zone_path, f"{staging}/zone.json")
    script = (HERE / "provision-zone.sh").read_text()
    ctx.ssh(
        f"/bin/sh -c {shlex.quote(script)} provision-zone "
        f"{shlex.quote(backend['zone_uuid'])} {IMAGE_UUID} {shlex.quote(gateway)}",
        timeout=900,
    )


def workspace(ctx):
    return f"/zones/{ctx.state['backend']['zone_uuid']}/root{GUEST_ROOT}", GUEST_ROOT


def prepare_workspace(ctx):
    zone_uuid = ctx.state["backend"]["zone_uuid"]
    ctx.ssh(f"zlogin {shlex.quote(zone_uuid)} chown -R specter:specter {GUEST_ROOT}")


def command(ctx, script):
    zone_uuid = ctx.state["backend"]["zone_uuid"]
    script = "export TMPDIR=/var/lib/specter-native/tmp; " + script
    # zlogin reconstructs its command through a login shell. Pass the script on
    # stdin so that an extra shell does not consume quotes or regex anchors.
    return (
        f"printf '%s\\n' {shlex.quote(script)} | "
        f"zlogin -l specter {shlex.quote(zone_uuid)} /bin/sh -s"
    )
