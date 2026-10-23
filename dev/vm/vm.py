#!/usr/bin/env python3
"""Persistent native test guests, using the host's existing libvirt installation."""

import argparse
import contextlib
import fcntl
import hashlib
import importlib
import ipaddress
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request
import uuid
import xml.etree.ElementTree as ET


PREFIX = "SPECTER_VM_"
OPTIONS = {"URI", "DISK_POOL", "MEDIA_POOL", "NETWORK", "DNS", "CPUS", "MEMORY_MIB", "DISK_GIB"}
DEFAULTS = {"cpus": 2, "memory_mib": 4096, "disk_gib": 20}


def run(args, *, check=True, capture=True, **kwargs):
    result = subprocess.run([str(arg) for arg in args], text=True,
                            stdout=subprocess.PIPE if capture else None,
                            stderr=subprocess.PIPE if capture else None, **kwargs)
    if check and result.returncode:
        detail = (result.stderr or result.stdout or "").strip()
        raise RuntimeError(f"{shlex.join(map(str, args))} failed: {detail}")
    return result


def local_config():
    path = Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config")) / "specter/vm.env"
    values = {}
    if path.exists():
        for line in path.read_text().splitlines():
            tokens = shlex.split(line, comments=True)
            if not tokens:
                continue
            if len(tokens) != 1 or "=" not in tokens[0]:
                raise RuntimeError(f"{path}: expected NAME=value, with quoted values if needed")
            key, value = tokens[0].split("=", 1)
            if key.removeprefix(PREFIX) not in OPTIONS or not key.startswith(PREFIX):
                raise RuntimeError(f"{path}: unknown setting {key}")
            values[key] = value
    values.update({key: value for key, value in os.environ.items() if key.startswith(PREFIX)})
    return values


def parse_json_stream(text):
    decoder = json.JSONDecoder()
    while text.strip():
        value, end = decoder.raw_decode(text.lstrip())
        yield value
        text = text.lstrip()[end:]


def owned_volume(name, fixture_uuid):
    return name.startswith(f"specter-{fixture_uuid}-")


class Guest:
    def __init__(self, os_name, config):
        self.os_name = os_name
        self.backend = importlib.import_module(os_name)
        self.config = config
        data = Path(os.environ.get("XDG_DATA_HOME", Path.home() / ".local/share"))
        cache = Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache"))
        self.state_dir = data / "specter/vms" / os_name
        self.cache_dir = cache / "specter/vm-images"
        self.state_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.state_file = self.state_dir / "guest.json"
        self.state = json.loads(self.state_file.read_text()) if self.state_file.exists() else {}
        configured_uri = config.get(PREFIX + "URI")
        if self.state and configured_uri and configured_uri != self.state["uri"]:
            raise RuntimeError(f"fixture belongs to {self.state['uri']}; refusing URI override {configured_uri}")
        self.uri = self.state.get("uri", configured_uri or "qemu:///system")
        if self.uri not in ("qemu:///system", "qemu:///session"):
            raise RuntimeError("this runner requires a local qemu:///system or qemu:///session connection")
        self.name = f"specter-dev-{os_name}"
        self.ip = None

    @property
    def uuid(self):
        return self.state["uuid"]

    @property
    def settings(self):
        return self.state["settings"]

    @property
    def mac(self):
        return "52:54:00:" + ":".join(self.uuid.replace("-", "")[i:i + 2] for i in (0, 2, 4))

    @property
    def public_key(self):
        return (self.state_dir / "id_ed25519.pub").read_text().strip()

    @property
    def network(self):
        root = ET.fromstring(self.virsh("net-dumpxml", self.settings["network"]).stdout)
        ip = root.find("ip[@address]")
        if ip is None:
            raise RuntimeError("the selected network needs IPv4 addressing and DHCP")
        gateway = ip.attrib["address"]
        mask = ip.attrib.get("netmask") or str(ipaddress.ip_network(f"{gateway}/{ip.attrib['prefix']}", strict=False).netmask)
        return {"gateway": gateway, "dns": gateway, "netmask": mask}

    def save_state(self):
        temporary = self.state_file.with_suffix(".tmp")
        temporary.write_text(json.dumps(self.state, indent=2) + "\n")
        temporary.chmod(0o600)
        temporary.replace(self.state_file)

    def virsh(self, *args, **kwargs):
        return run(["virsh", "--connect", self.uri, *args], **kwargs)

    def xml_command(self, command, xml):
        with tempfile.NamedTemporaryFile(mode="w", suffix=".xml") as stream:
            stream.write(ET.tostring(xml, encoding="unicode"))
            stream.flush()
            return self.virsh(command, stream.name)

    def domain(self):
        if self.state and self.uuid in self.virsh("list", "--all", "--uuid").stdout.split():
            return ET.fromstring(self.virsh("dumpxml", self.uuid).stdout)
        if self.name in self.virsh("list", "--all", "--name").stdout.splitlines():
            raise RuntimeError(f"{self.name} already exists and is not owned by this fixture")
        return None

    def initialize(self):
        # Check the name before persisting an identity or creating any libvirt object.
        if self.domain() is not None:
            return
        if self.state:
            return
        settings = dict(DEFAULTS)
        for key in ("cpus", "memory_mib", "disk_gib"):
            value = self.config.get(PREFIX + key.upper())
            if value is not None:
                settings[key] = int(value)
            if settings[key] <= 0:
                raise RuntimeError(f"{key} must be positive")
        disk_pool = self.config.get(PREFIX + "DISK_POOL")
        if not disk_pool:
            raise RuntimeError("set SPECTER_VM_DISK_POOL for first creation (see dev/vm/README.md)")
        disk_xml = ET.fromstring(self.virsh("pool-dumpxml", disk_pool).stdout)
        media_pool = self.config.get(PREFIX + "MEDIA_POOL")
        if not media_pool and disk_xml.attrib["type"] == "dir":
            media_pool = disk_pool
        if not media_pool:
            raise RuntimeError("set SPECTER_VM_MEDIA_POOL to an active directory pool")
        media_xml = ET.fromstring(self.virsh("pool-dumpxml", media_pool).stdout)
        if media_xml.attrib["type"] != "dir":
            raise RuntimeError("SPECTER_VM_MEDIA_POOL must be a directory pool")
        active_pools = self.virsh("pool-list", "--name").stdout.split()
        if disk_pool not in active_pools or media_pool not in active_pools:
            raise RuntimeError("selected storage pools must already be active")
        settings.update(disk_pool=disk_pool, media_pool=media_pool,
                        network=self.config.get(PREFIX + "NETWORK") or self.name,
                        own_network=not bool(self.config.get(PREFIX + "NETWORK")))
        self.state = {"uri": self.uri, "uuid": str(uuid.uuid4()), "settings": settings, "backend": {}}
        self.save_state()

    def ensure_key(self):
        key = self.state_dir / "id_ed25519"
        if not key.exists():
            run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key])

    def ensure_network(self):
        name = self.settings["network"]
        result = self.virsh("net-dumpxml", name, check=False)
        if not self.settings["own_network"]:
            if result.returncode or name not in self.virsh("net-list", "--name").stdout.splitlines():
                raise RuntimeError(f"selected network {name} must already exist and be active")
            root = ET.fromstring(result.stdout)
            if root.find("ip/dhcp") is None:
                raise RuntimeError("selected network must provide IPv4 DHCP for SSH discovery")
            return
        if not result.returncode:
            root = ET.fromstring(result.stdout)
            if root.findtext("uuid") != self.uuid:
                raise RuntimeError(f"network {name} is not owned by this fixture")
        else:
            used = []
            for network in self.virsh("net-list", "--all", "--name").stdout.splitlines():
                if not network:
                    continue
                root = ET.fromstring(self.virsh("net-dumpxml", network).stdout)
                for item in root.findall("ip"):
                    if ":" not in item.attrib.get("address", ""):
                        used.append(ipaddress.ip_network(f"{item.attrib['address']}/{item.attrib.get('prefix', item.attrib.get('netmask', '24'))}", strict=False))
            for route in json.loads(run(["ip", "-j", "route", "show", "table", "all"]).stdout):
                if route.get("dst", "default") != "default":
                    used.append(ipaddress.ip_network(route["dst"], strict=False))
            subnet = next((ipaddress.ip_network(f"192.168.{n}.0/24") for n in range(200, 255)
                           if not any(ipaddress.ip_network(f"192.168.{n}.0/24").overlaps(item) for item in used)), None)
            if subnet is None:
                raise RuntimeError("no unused fixture subnet found; select SPECTER_VM_NETWORK")
            root = ET.Element("network")
            ET.SubElement(root, "name").text = name
            ET.SubElement(root, "uuid").text = self.uuid
            ET.SubElement(root, "description").text = "Specter native development fixture"
            ip = ET.SubElement(root, "ip", address=str(subnet[1]), netmask=str(subnet.netmask))
            dhcp = ET.SubElement(ip, "dhcp")
            ET.SubElement(dhcp, "range", start=str(subnet[10]), end=str(subnet[100]))
            ET.SubElement(dhcp, "host", mac=self.mac, name=self.name, ip=str(subnet[10]))
        dns = self.settings.get("dns", "libvirt")
        if dns not in ("libvirt", "host"):
            raise RuntimeError("SPECTER_VM_DNS must be libvirt or host")
        if dns == "host" and root.find("dns[@enable='no']") is None:
            if name in self.virsh("net-list", "--name").stdout.splitlines():
                raise RuntimeError("cannot change DNS of a running fixture network")
            ET.SubElement(root, "dns", enable="no")
        self.xml_command("net-define", root)
        if name not in self.virsh("net-list", "--name").stdout.splitlines():
            self.virsh("net-start", name)

    def fetch(self, url, checksum, algorithm="sha256"):
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        path = self.cache_dir / (checksum[:16] + "-" + url.rsplit("/", 1)[-1])
        with path.with_suffix(path.suffix + ".lock").open("w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            if path.exists():
                with path.open("rb") as stream:
                    if hashlib.file_digest(stream, algorithm).hexdigest() == checksum:
                        return path
                raise RuntimeError(f"cached image checksum mismatch: {path}")
            print(f"Downloading {url}", flush=True)
            temporary = path.with_suffix(path.suffix + ".part")
            offset = temporary.stat().st_size if temporary.exists() else 0
            request = urllib.request.Request(url, headers={"Range": f"bytes={offset}-"} if offset else {})
            with urllib.request.urlopen(request, timeout=60) as response:
                mode = "ab" if offset and response.status == 206 else "wb"
                with temporary.open(mode) as stream:
                    shutil.copyfileobj(response, stream)
            with temporary.open("rb") as stream:
                if hashlib.file_digest(stream, algorithm).hexdigest() != checksum:
                    raise RuntimeError(f"download checksum mismatch: {temporary}")
            temporary.replace(path)
        return path

    def create_volume(self, role, pool, size, source=None):
        name = f"specter-{self.uuid}-{role}"
        if self.virsh("vol-info", name, "--pool", pool, check=False).returncode:
            self.virsh("vol-create-as", pool, name, str(size), "--format", "raw")
        if source is not None:
            print(f"Uploading {role} to pool {pool}", flush=True)
            self.virsh("vol-upload", name, source, "--pool", pool, "--sparse")
        return name

    def define(self, disks):
        root = ET.Element("domain", type="kvm")
        ET.SubElement(root, "name").text = self.name
        ET.SubElement(root, "uuid").text = self.uuid
        ET.SubElement(root, "description").text = "Specter native development fixture; managed by dev/vm/vm.py"
        ET.SubElement(root, "memory", unit="MiB").text = str(self.settings["memory_mib"])
        ET.SubElement(root, "vcpu").text = str(self.settings["cpus"])
        os_xml = ET.SubElement(root, "os")
        ET.SubElement(os_xml, "type", arch="x86_64", machine="pc").text = "hvm"
        features = ET.SubElement(root, "features")
        ET.SubElement(features, "acpi")
        ET.SubElement(features, "apic")
        ET.SubElement(root, "cpu", mode="host-passthrough")
        ET.SubElement(root, "clock", offset="utc")
        for event, policy in (("poweroff", "destroy"), ("reboot", "restart"), ("crash", "preserve")):
            ET.SubElement(root, "on_" + event).text = policy
        devices = ET.SubElement(root, "devices")
        disk_index = 0
        for spec in disks:
            media = spec.get("media", False)
            source = spec.get("source")
            size = spec.get("size_gib", self.settings["disk_gib"]) * 1024 ** 3 if not media else Path(source).stat().st_size
            pool = self.settings["media_pool" if media else "disk_pool"]
            volume = self.create_volume(spec["role"], pool, size, source)
            volume_xml = ET.fromstring(self.virsh("vol-dumpxml", volume, "--pool", pool).stdout)
            disk_type = volume_xml.attrib["type"]
            if disk_type not in ("file", "block"):
                raise RuntimeError(f"unsupported volume type {disk_type}")
            # Resolve the managed volume so libvirt's AppArmor/DAC rules include
            # its actual device. Older libvirt omits LVM devices for type=volume.
            disk = ET.SubElement(devices, "disk", type=disk_type, device="cdrom" if media else "disk")
            driver = ET.SubElement(disk, "driver", name="qemu", type="raw")
            if not media:
                driver.set("cache", "none")
            ET.SubElement(disk, "source", **{"dev" if disk_type == "block" else "file": volume_xml.findtext("target/path")})
            ET.SubElement(disk, "target", dev="hdc" if media else "vd" + chr(97 + disk_index), bus="ide" if media else "virtio")
            if media:
                ET.SubElement(disk, "readonly")
            else:
                disk_index += 1
            if "boot" in spec:
                ET.SubElement(disk, "boot", order=str(spec["boot"]))
        interface = ET.SubElement(devices, "interface", type="network")
        ET.SubElement(interface, "source", network=self.settings["network"])
        ET.SubElement(interface, "mac", address=self.mac)
        ET.SubElement(interface, "model", type=getattr(self.backend, "NIC_MODEL", "e1000"))
        serial_volume = self.create_volume("serial.log", self.settings["media_pool"], 0)
        serial_path = self.virsh("vol-path", serial_volume, "--pool", self.settings["media_pool"]).stdout.strip()
        serial = ET.SubElement(devices, "serial", type="pty")
        ET.SubElement(serial, "log", file=serial_path, append="on")
        ET.SubElement(serial, "target", port="0")
        console = ET.SubElement(devices, "console", type="pty")
        ET.SubElement(console, "target", type="serial", port="0")
        self.xml_command("define", root)

    def running(self):
        return self.virsh("domstate", self.uuid).stdout.strip() in ("running", "blocked", "paused", "in shutdown")

    def address(self):
        result = self.virsh("net-dhcp-leases", self.settings["network"], "--mac", self.mac)
        for line in result.stdout.splitlines():
            for word in line.split():
                if re.fullmatch(r"\d+\.\d+\.\d+\.\d+/\d+", word):
                    return str(ipaddress.ip_interface(word).ip)
        return None

    def ssh_options(self, user=None, password=None):
        if self.ip is None:
            self.ip = self.address()
        if self.ip is None:
            raise RuntimeError("guest has no DHCP lease yet")
        options = ["-i", self.state_dir / "id_ed25519", "-o", "IdentitiesOnly=yes",
                   "-o", "ConnectTimeout=5", "-o", "ConnectionAttempts=1",
                   "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3",
                   "-o", "StrictHostKeyChecking=accept-new", "-o", f"HostKeyAlias={self.uuid}",
                   "-o", f"UserKnownHostsFile={self.state_dir / 'known_hosts'}"]
        env = os.environ.copy()
        if password is None:
            options += ["-o", "BatchMode=yes"]
        else:
            askpass = self.state_dir / "askpass"
            askpass.write_text('#!/bin/sh\nprintf "%s\\n" "$SPECTER_VM_BOOTSTRAP_PASSWORD"\n')
            askpass.chmod(0o700)
            env.update(SSH_ASKPASS=str(askpass), SSH_ASKPASS_REQUIRE="force", DISPLAY="specter-vm",
                       SPECTER_VM_BOOTSTRAP_PASSWORD=password)
            options += ["-o", "PreferredAuthentications=publickey,password",
                        "-o", "NumberOfPasswordPrompts=1"]
        return options, env, f"{user or self.backend.SSH_USER}@{self.ip}"

    def ssh(self, script, user=None, password=None, timeout=30, check=True, input=None, capture=True, stdin=None):
        options, env, destination = self.ssh_options(user, password)
        return run(["ssh", *options, destination, script], env=env, timeout=timeout, check=check,
                   input=input, stdin=stdin, capture=capture)

    def upload(self, local, remote, user=None, password=None):
        options, env, destination = self.ssh_options(user, password)
        return run(["scp", "-O", *options, local, f"{destination}:{remote}"], env=env, timeout=600)

    def wait_ssh(self, user=None, password=None, timeout=600):
        deadline = time.monotonic() + timeout
        last_error = "no DHCP lease"
        while time.monotonic() < deadline:
            self.ip = self.address()
            if self.ip:
                try:
                    result = self.ssh("true", user=user, password=password, timeout=10, check=False)
                    if not result.returncode:
                        return
                    last_error = result.stderr.strip()
                except subprocess.TimeoutExpired:
                    last_error = "SSH attempt timed out"
            if not self.running():
                raise RuntimeError("guest stopped before SSH became available")
            time.sleep(2)
        raise RuntimeError(f"SSH did not become ready within {timeout}s: {last_error}")

    def start(self):
        self.initialize()
        self.ensure_key()
        if self.domain() is None:
            self.settings["dns"] = self.config.get(PREFIX + "DNS", self.settings.get("dns", "libvirt"))
            self.save_state()
        self.ensure_network()
        if self.domain() is None:
            self.define(self.backend.prepare(self))
        if not self.running():
            print(f"Starting {self.name}", flush=True)
            self.virsh("start", self.uuid)
        deadline = time.monotonic() + 600
        while self.address() is None:
            if not self.running() or time.monotonic() >= deadline:
                raise RuntimeError("guest did not obtain an IPv4 DHCP lease within 600s")
            time.sleep(2)
        self.ip = self.address()
        self.backend.provision(self)
        print(f"{self.name} ready at {self.ip}; state: {self.state_dir}", flush=True)

    def serial_log(self):
        if not self.state:
            return
        name = f"specter-{self.uuid}-serial.log"
        path = self.state_dir / "serial.log"
        result = self.virsh("vol-download", name, path, "--pool", self.settings["media_pool"], check=False)
        if not result.returncode:
            print(f"Serial log: {path}", file=sys.stderr)

    def stop(self):
        if self.domain() is None:
            raise RuntimeError("fixture has not been created")
        if self.running():
            self.virsh("shutdown", self.uuid, "--mode", "acpi")
            deadline = time.monotonic() + 120
            while self.running() and time.monotonic() < deadline:
                time.sleep(2)
            if self.running():
                raise RuntimeError("guest did not shut down within 120s; retained for inspection")
        self.serial_log()
        print(f"Stopped {self.name}; disks retained")

    def destroy(self):
        if not self.state:
            raise RuntimeError("no fixture identity found; nothing will be removed")
        domain = self.domain()
        if domain is not None:
            if self.running():
                self.virsh("destroy", self.uuid)
            self.virsh("undefine", self.uuid)
        for pool in set((self.settings["disk_pool"], self.settings["media_pool"])):
            for volume in self.virsh("vol-list", pool, "--name").stdout.splitlines():
                volume = volume.strip()
                if owned_volume(volume, self.uuid):
                    self.virsh("vol-delete", volume, "--pool", pool)
        if self.settings["own_network"]:
            name = self.settings["network"]
            result = self.virsh("net-dumpxml", name, check=False)
            if not result.returncode:
                if ET.fromstring(result.stdout).findtext("uuid") != self.uuid:
                    raise RuntimeError("network identity changed; refusing to remove it")
                for other in self.virsh("list", "--all", "--uuid").stdout.splitlines():
                    if other and any(item.attrib.get("network") == name for item in ET.fromstring(self.virsh("dumpxml", other).stdout).findall("devices/interface/source")):
                        raise RuntimeError(f"network {name} is used by another domain; retained")
                if name in self.virsh("net-list", "--name").stdout.splitlines():
                    self.virsh("net-destroy", name)
                self.virsh("net-undefine", name)
        for path in self.state_dir.iterdir():
            if path.is_dir() and not path.is_symlink():
                shutil.rmtree(path)
            elif path.name != ".lock":
                path.unlink()
        print(f"Destroyed {self.name}; downloaded image cache retained")

    def test(self, patterns, flags):
        self.start()
        repo = Path(run(["git", "rev-parse", "--show-toplevel"]).stdout.strip())
        env = dict(os.environ, GOOS=self.backend.GOOS, GOARCH="amd64", CGO_ENABLED="0")
        packages = list(parse_json_stream(run(["go", "list", "-json", *patterns], cwd=repo, env=env).stdout))
        upload_root, guest_root = self.backend.workspace(self)
        with tempfile.TemporaryDirectory(prefix="specter-native-") as directory:
            stage = Path(directory)
            selected = []
            for package in packages:
                if not package.get("TestGoFiles") and not package.get("XTestGoFiles"):
                    continue
                relative = Path(package["Dir"]).relative_to(repo)
                destination = stage / relative
                destination.mkdir(parents=True, exist_ok=True)
                print(f"Building {package['ImportPath']} for {self.backend.GOOS}", flush=True)
                run(["go", "test", "-c", "-o", destination / "package.test", package["ImportPath"]], cwd=repo, env=env, capture=False)
                if (Path(package["Dir"]) / "testdata").exists():
                    shutil.copytree(Path(package["Dir"]) / "testdata", destination / "testdata")
                selected.append(relative)
            if not selected:
                raise RuntimeError("no test packages selected")
            for fixture in ("packaging/templates", "certs", "dev/pebble"):
                if (repo / fixture).is_dir():
                    shutil.copytree(repo / fixture, stage / fixture, dirs_exist_ok=True)
            archive = stage / "tests.tar"
            with tarfile.open(archive, "w", format=tarfile.USTAR_FORMAT) as stream:
                for path in stage.iterdir():
                    if path != archive:
                        stream.add(path, arcname=path.name)
            self.ssh(f"mkdir -p {shlex.quote(upload_root + '/.tmp')}")
            with archive.open("rb") as stream:
                self.ssh(f"cd {shlex.quote(upload_root)} && tar -xf -", stdin=stream, timeout=600)
            if hasattr(self.backend, "prepare_workspace"):
                self.backend.prepare_workspace(self)
            failed = []
            test_env = {key: os.environ[key] for key in ("GO_INTEGRATION_TUNNEL", "GO_INTEGRATION_ACME", "SPECTER_TEST_ACME_URL") if key in os.environ}
            for relative in selected:
                command = f"cd {shlex.quote(str(Path(guest_root) / relative))} && " + shlex.join([
                    "env", f"TMPDIR={guest_root}/.tmp", *[f"{key}={value}" for key, value in test_env.items()],
                    "./package.test", "-test.v", "-test.short", "-test.count=1", "-test.timeout=300s", *flags])
                print(f"Testing {relative} on {self.os_name}", flush=True)
                result = self.ssh(self.backend.command(self, command), timeout=900, check=False, capture=False)
                if result.returncode:
                    failed.append(str(relative))
            if failed:
                raise RuntimeError("native tests failed: " + ", ".join(failed))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("start", "test", "stop", "destroy"))
    parser.add_argument("os", choices=("freebsd", "smartos"))
    parser.add_argument("packages", nargs="*", help="go package patterns for test; defaults to ./...")
    argv = sys.argv[1:]
    flags = []
    if "--" in argv:
        index = argv.index("--")
        argv, flags = argv[:index], argv[index + 1:]
    args = parser.parse_args(argv)
    if args.action != "test" and (args.packages or flags):
        parser.error("package patterns and test flags apply only to test")
    guest = None
    try:
        guest = Guest(args.os, local_config())
        with (guest.state_dir / ".lock").open("w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            # A different invocation may have finished provisioning while we waited.
            guest = Guest(args.os, local_config())
            if args.action == "test":
                guest.test(args.packages or ["./..."], flags)
            else:
                getattr(guest, args.action)()
    except (RuntimeError, ValueError, OSError, subprocess.SubprocessError) as error:
        print(f"vm: {error}", file=sys.stderr)
        if guest is not None and guest.state:
            with contextlib.suppress(Exception):
                guest.serial_log()
        return 1
    except KeyboardInterrupt:
        print("vm: interrupted; fixture and partial downloads retained for the next start", file=sys.stderr)
        return 130
    return 0


if __name__ == "__main__":
    sys.exit(main())
