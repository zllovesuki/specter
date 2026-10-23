# Native development VMs

Run tests on FreeBSD or SmartOS, and keep the VM available while you work.
The same guest can be reused from any checkout or Git worktree.

The host needs Python 3.11+, a local libvirt/QEMU/KVM installation, `virsh`,
`qemu-img`, OpenSSH, `genisoimage`, and Go. SmartOS also needs `xorriso` or
Docker to prepare its boot ISO.

## Create a guest

Choose existing, active storage pools with `virsh pool-list --all`. Replace the
example names below with your disk and media pools. Media needs a directory
pool; disks can use a directory or LVM pool.

```sh
export SPECTER_VM_DISK_POOL=vm-disks
export SPECTER_VM_MEDIA_POOL=vm-media
python3 dev/vm/vm.py start freebsd
```

Use `smartos` for a SmartOS guest. The first start downloads pinned official
images and provisions SSH. Later starts reuse the existing installation.

## Run tests

From the checkout you want to test:

```sh
python3 dev/vm/vm.py test freebsd ./config ./kv/sqlite3
python3 dev/vm/vm.py test smartos ./config -- -test.run TestStore
```

The runner builds native test binaries on the host, copies them and their
fixtures, and runs them as an unprivileged guest user. No guest Go installation
is needed. Without package arguments it tests `./...`; arguments after `--` are
Go test-binary flags. Short tests and a five-minute package timeout are defaults.
Failures return a nonzero status and leave the guest running for inspection.
These `CGO_ENABLED=0` tests supplement the usual host race and integration checks.

Run the cluster, DNS, and tunnel integration tests inside the guest with:

```sh
GO_INTEGRATION_TUNNEL=1 python3 dev/vm/vm.py test freebsd ./integrations -- -test.run '^TestIntegration'
```

ACME integration can use an already running Pebble server in the guest:

```sh
GO_INTEGRATION_ACME=1 SPECTER_TEST_ACME_URL=https://127.0.0.1:14000/dir \
  python3 dev/vm/vm.py test freebsd ./acme -- -test.run '^TestIntegrationACME$'
```

The URL must be reachable from the guest and present a TLS certificate trusted
by `dev/pebble/certs/cert.pem`. Run Pebble with `PEBBLE_VA_NOSLEEP=1`,
`PEBBLE_VA_ALWAYS_VALID=1`, and `PEBBLE_WFE_NONCEREJECT=0`, matching the Docker
fixture. Without `SPECTER_TEST_ACME_URL`, the test starts its Docker container.

```sh
python3 dev/vm/vm.py stop freebsd     # shut down; retain the installation
python3 dev/vm/vm.py start freebsd    # resume work
python3 dev/vm/vm.py destroy freebsd # delete this fixture when finished
```

## Configuration

Settings can also go in `${XDG_CONFIG_HOME:-~/.config}/specter/vm.env`, as
literal `NAME=value` lines. Environment variables take precedence.

| Setting | Purpose |
| --- | --- |
| `SPECTER_VM_URI` | Local libvirt connection; default `qemu:///system`. |
| `SPECTER_VM_DISK_POOL` | Pool for guest disks; required on first creation. |
| `SPECTER_VM_MEDIA_POOL` | Directory pool for boot media; defaults to the disk pool if suitable. |
| `SPECTER_VM_NETWORK` | Existing active IPv4 DHCP network; otherwise create an isolated fixture network. |
| `SPECTER_VM_DNS` | `libvirt` by default; use `host` if an existing host DNS service listens on every interface. |
| `SPECTER_VM_CPUS`, `SPECTER_VM_MEMORY_MIB`, `SPECTER_VM_DISK_GIB` | Override initial guest resources. |

Both guests default to 2 CPUs, 4 GiB RAM and a 20 GiB disk. Creation settings do
not move or resize an existing guest. Commands retain its saved connection and
identity.

The default network connects guests to the host without Internet forwarding.
Downloads happen on the host. Select a suitable existing network when tests
need external integration services.

SmartOS boots from its pinned ISO and keeps configuration and a native `joyent`
zone on its ZFS disk. Tests run as `specter` in that zone through `zlogin`, with
their workspace and temporary files on ZFS. SSH settings persist under
`/usbkey` across reboots.

Domains are named `specter-dev-freebsd` and `specter-dev-smartos`. Private SSH
state and boot-failure logs live in `${XDG_DATA_HOME:-~/.local/share}/specter/vms/`;
downloaded images use `${XDG_CACHE_HOME:-~/.cache}/specter/vm-images/`. Libvirt
owns the disks. Removing a worktree or running `make clean` leaves guests intact.

For a live boot console, use `virsh console specter-dev-freebsd` (or the SmartOS
domain). `destroy` removes only the selected fixture's owned resources and keeps
the download cache.

Runner checks: `python3 -m unittest discover -s dev/vm -p 'test_*.py'`.
