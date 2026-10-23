"""FreeBSD's official nuageinit image needs only an SSH seed disk."""

import lzma
import shlex
import shutil
import subprocess

GOOS = "freebsd"
SSH_USER = "freebsd"
NIC_MODEL = "virtio"
IMAGE = "https://download.freebsd.org/releases/VM-IMAGES/15.1-RELEASE/amd64/Latest/FreeBSD-15.1-RELEASE-amd64-BASIC-CLOUDINIT-zfs.qcow2.xz"
SHA256 = "4159e137d4a78f46b62d3523edd9a4dc79fd0cdcf17e34e531342f52333f4131"


def prepare(ctx):
    archive = ctx.fetch(IMAGE, SHA256)
    qcow = ctx.state_dir / "freebsd.qcow2"
    raw = ctx.state_dir / "freebsd.raw"
    with lzma.open(archive) as source, qcow.open("wb") as destination:
        shutil.copyfileobj(source, destination)
    subprocess.run(["qemu-img", "convert", "-f", "qcow2", "-O", "raw", qcow, raw], check=True)
    qcow.unlink()
    seed = ctx.state_dir / "seed"
    seed.mkdir(exist_ok=True)
    (seed / "meta-data").write_text(f"instance-id: {ctx.uuid}\nlocal-hostname: {ctx.name}\n")
    (seed / "user-data").write_text(f"""#cloud-config
ssh_authorized_keys:
  - {ctx.public_key}
ssh_pwauth: false
write_files:
  - path: /etc/rc.conf.d/firstboot_pkg_upgrade
    permissions: "0644"
    content: |
      firstboot_pkg_upgrade_enable="NO"
  - path: /etc/rc.conf.d/routing
    permissions: "0644"
    content: |
      defaultrouter="{ctx.network['gateway']}"
""")
    iso = ctx.state_dir / "seed.iso"
    subprocess.run(["genisoimage", "-quiet", "-output", iso, "-volid", "cidata", "-joliet", "-rock", seed], check=True)
    return [{"role": "system", "source": raw, "boot": 1},
            {"role": "seed.iso", "source": iso, "media": True}]


def provision(ctx):
    ctx.wait_ssh()
    ctx.ssh("mkdir -p /home/freebsd/specter-native/.tmp")
    # The libvirt volume owns the installed disk; this staging copy is disposable.
    (ctx.state_dir / "freebsd.raw").unlink(missing_ok=True)


def workspace(ctx):
    root = "/home/freebsd/specter-native"
    return root, root


def command(ctx, script):
    return "/bin/sh -c " + shlex.quote(script)
