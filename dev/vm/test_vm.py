import subprocess
import tempfile
import unittest
from unittest.mock import patch

import vm


class FixtureOwnershipTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.environment = patch.dict(vm.os.environ, {"XDG_DATA_HOME": self.temporary.name})
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.guest = vm.Guest("freebsd", {})
        self.guest.state = {
            "uuid": "16c27c27-3e8a-4ba2-aa20-0f855016426d", "uri": "qemu:///system",
            "settings": {"disk_pool": "disk", "media_pool": "media", "own_network": False},
        }
        self.guest.save_state()

    def test_resume_uses_saved_connection_and_rejects_override(self):
        guest = vm.Guest("freebsd", {"SPECTER_VM_DISK_POOL": "different"})
        self.assertEqual(guest.settings["disk_pool"], "disk")
        with self.assertRaisesRegex(RuntimeError, "refusing URI override"):
            vm.Guest("freebsd", {"SPECTER_VM_URI": "qemu:///session"})

    def test_foreign_domain_name_is_never_adopted(self):
        def virsh(*args, **kwargs):
            output = "unrelated-uuid\n" if args[-1] == "--uuid" else self.guest.name + "\n"
            return subprocess.CompletedProcess(args, 0, output, "")
        with patch.object(self.guest, "virsh", side_effect=virsh):
            with self.assertRaisesRegex(RuntimeError, "not owned"):
                self.guest.domain()

    def test_lookup_failure_is_not_treated_as_missing_domain(self):
        with patch.object(self.guest, "virsh", side_effect=RuntimeError("connection lost")):
            with self.assertRaisesRegex(RuntimeError, "connection lost"):
                self.guest.domain()

    def test_destroy_removes_only_identity_scoped_volumes(self):
        calls = []
        own_disk = f"specter-{self.guest.uuid}-system"
        own_media = f"specter-{self.guest.uuid}-seed.iso"

        def virsh(*args, **kwargs):
            calls.append(args)
            output = ""
            if args[0] == "list":
                output = ""
            elif args[0] == "vol-list":
                output = (own_disk if args[1] == "disk" else own_media) + "  \nuser-disk\nspecter-another-fixture-system\n"
            return subprocess.CompletedProcess(args, 0, output, "")

        with patch.object(self.guest, "virsh", side_effect=virsh), patch("builtins.print"):
            self.guest.destroy()
        deleted = {args[1] for args in calls if args[0] == "vol-delete"}
        self.assertEqual(deleted, {own_disk, own_media})
        self.assertFalse(self.guest.state_file.exists())
        self.assertFalse(any(args[0].startswith("net-") for args in calls))


if __name__ == "__main__":
    unittest.main()
