"""Offline tests of set10's pure rules: the render header as the classifier reads it, the lab's stale text, the
journal times and lines, and the verdict of a cell. No guest is needed."""
from __future__ import annotations

import hashlib
import importlib.util
from pathlib import Path
import sys
import unittest

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set10_trial as s10  # noqa: E402


def guest_module():
    spec = importlib.util.spec_from_file_location("set10_guest_under_test", HERE / "guest_set10_native.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


BODY = b"# Nginx vhost for set10-a.test\nserver {\n    listen 80;\n    server_name set10-a.test www.set10-a.test;\n}\n"


class Set10Rules(unittest.TestCase):
    def setUp(self):
        self.g = guest_module()

    def test_sealed_text_has_a_valid_header(self):
        sealed = self.g.seal(BODY)
        head = self.g.header_of(sealed)
        self.assertTrue(head["present"] and head["valid"])
        self.assertEqual(head["declared"], hashlib.sha256(BODY).hexdigest())
        self.assertEqual(sealed.split(b"\n", 1)[1], BODY)

    def test_an_edited_body_keeps_the_header_but_not_its_digest(self):
        edited = self.g.seal(BODY).replace(b"listen 80;", b"listen 8080;")
        head = self.g.header_of(edited)
        self.assertTrue(head["present"])
        self.assertFalse(head["valid"])

    def test_crlf_header_line_is_still_the_header(self):
        sealed = self.g.seal(BODY)
        first, rest = sealed.split(b"\n", 1)
        self.assertTrue(self.g.header_of(first + b"\r\n" + rest)["present"])

    def test_no_header_and_malformed_header(self):
        self.assertFalse(self.g.header_of(BODY)["present"])
        self.assertFalse(self.g.header_of(b"# celikpanel-render v2 sha256=XYZ\n" + BODY)["present"])
        self.assertFalse(self.g.header_of(None)["present"])

    def test_stale_text_is_header_valid_and_differs(self):
        sealed = self.g.seal(BODY)
        stale = self.g.seal(self.g.stale_body(sealed))
        head = self.g.header_of(stale)
        self.assertTrue(head["valid"])
        self.assertNotEqual(stale, sealed)
        with self.assertRaises(self.g.Refused):
            self.g.stale_body(BODY)

    def test_owner_include_texts(self):
        self.assertIn("location /owner-dir/", self.g.owner_include_text("location", "set10-a.test"))
        self.assertIn("root ", self.g.owner_include_text("duplicate-root", "set10-a.test"))
        self.assertTrue(self.g.OWNER_FILE_RE.fullmatch("set10-dup-root.conf"))
        self.assertIsNone(self.g.OWNER_FILE_RE.fullmatch("../x.conf"))
        self.assertIsNone(self.g.OWNER_FILE_RE.fullmatch("owner.conf"))

    def test_start_line_wait_includes_the_d031_line(self):
        self.assertIn("site configuration files at start:", self.g.g8.RECONCILE_LINES)


class Set10Driver(unittest.TestCase):
    def test_journal_lines(self):
        lines = ["2026-10-10T19:00:00.000000+00:00 host nginx[1]: Reloaded nginx.service",
                 "2026-10-10T19:00:10.000000+00:00 host systemd[1]: Reloaded nginx.service - x",
                 "2026-10-10T19:00:05.000000+00:00 host panel[2]: site configuration set10-a.test (startup): unchanged"]
        epoch = s10.journal_epoch(lines[0])
        self.assertIsNotNone(epoch)
        self.assertEqual(len(s10.lines_after(lines, epoch + 1, "Reloaded")), 1)
        self.assertEqual(s10.site_lines(lines, "set10-a.test"), [lines[2]])
        self.assertEqual(s10.site_lines(lines, "set10-a.tes"), [])
        self.assertIsNone(s10.journal_epoch("not a time"))

    def test_verdicts(self):
        self.assertEqual(s10.verdict_of([("x", True, None)]), "PASS")
        self.assertEqual(s10.verdict_of([("x", True, None), ("y", None, None)]), "NOT-MEASURED")
        self.assertEqual(s10.verdict_of([("x", False, None), ("y", None, None)]), "FAIL")

    def test_runs_and_states(self):
        self.assertEqual(len(s10.RUNS), 12)
        self.assertEqual({r[3] for r in s10.RUNS}, {"start", "restart", "save"})
        self.assertTrue(all(r[4] == (r[3] == "start") for r in s10.RUNS))
        self.assertEqual(s10.kept_word("owner_edited"), "kept (owner-edited)")
        self.assertEqual(s10.KEPT_STATE["replace"], "foreign")
        self.assertEqual(s10.s8.DOMAIN_A, "set10-a.test")

    def test_safe_name(self):
        self.assertEqual(s10.safe_name("a1 after"), "a1-after")
        self.assertEqual(s10.safe_name("after the update set10-r1.test"), "after-the-update-set10-r1.test")

    def test_body_of(self):
        self.assertEqual(s10.body_of("# h\nbody\n"), "body\n")
        self.assertIsNone(s10.body_of(None))


POOL_BODY = (b"; The PHP-FPM pool of site1, written by CelikPanel. A line you add or change here is kept:\n"
             b"; CelikPanel then shows the difference and asks before writing this file again.\n"
             b"; Buraya ekledi\xc4\x9finiz ya da de\xc4\x9fi\xc5\x9ftirdi\xc4\x9finiz sat\xc4\xb1r korunur.\n"
             b"[site1]\nuser = celik_set10_a_test\ngroup = celik_set10_a_test\nlisten = /var/run/php/php8.4-fpm-site1.sock\n"
             b"listen.owner = www-data\nlisten.group = www-data\nlisten.mode = 0660\npm = dynamic\npm.max_children = 5\n"
             b"pm.start_servers = 2\npm.min_spare_servers = 1\npm.max_spare_servers = 3\npm.max_requests = 500\nchdir = /\n")
UNIT_BODY = (b"# The application unit of this site, written by CelikPanel.\n"
             b"# Your own settings go in /etc/systemd/system/celikapp-2.service.d/*.conf (systemd drop-ins); CelikPanel never writes or removes them.\n"
             b"# An edit to this file is kept: CelikPanel then shows the difference and asks before writing it again.\n"
             b"# Kendi ayarlar\xc4\xb1n\xc4\xb1z ... korunur.\n"
             b"[Unit]\nDescription=CelikPanel app: set10-b.test\nAfter=network.target\n\n[Service]\nUser=www-data\n"
             b"WorkingDirectory=/srv/b\nEnvironment=PORT=3001\nEnvironment=NODE_ENV=production\nEnvironment=PATH=/usr/bin\n"
             b"ExecStart=/bin/sh -lc 'node app.js'\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=multi-user.target\n")


class Set10SecondStep(unittest.TestCase):
    """D-031 second step (2026-10-11): the pool's INI header, the alpha.82 forms the lab writes, the new sections."""

    def setUp(self):
        self.g = guest_module()

    def test_the_pool_header_is_an_ini_comment_and_not_the_hash_form(self):
        sealed = self.g.seal(POOL_BODY, self.g.INI_HEADER_PREFIX)
        self.assertTrue(sealed.startswith(b"; celikpanel-render v2 sha256="))
        head = self.g.header_of(sealed, self.g.INI_HEADER_PREFIX)
        self.assertTrue(head["present"] and head["valid"])
        self.assertFalse(self.g.header_of(sealed)["present"])
        edited = sealed + self.g.OWNER_POOL_LINES
        self.assertFalse(self.g.header_of(edited, self.g.INI_HEADER_PREFIX)["valid"])

    def test_the_legacy_pool_is_the_alpha82_form(self):
        legacy = self.g.legacy_pool_text(self.g.seal(POOL_BODY, self.g.INI_HEADER_PREFIX))
        self.assertTrue(legacy.startswith(b"[site1]\nuser = "))
        self.assertTrue(legacy.endswith(b"chdir = /\n"))
        self.assertEqual(legacy.count(b"\n"), 14)
        self.assertNotIn(b";", legacy)
        with self.assertRaises(self.g.Refused):
            self.g.legacy_pool_text(POOL_BODY)

    def test_the_legacy_unit_is_the_alpha82_form(self):
        legacy = self.g.legacy_unit_text(self.g.seal(UNIT_BODY))
        self.assertTrue(legacy.startswith("# Managed by CelikPanel — do not edit by hand.\n".encode()))
        self.assertEqual(legacy.split(b"\n")[2], b"[Unit]")
        self.assertNotIn(b"drop-ins", legacy)
        with self.assertRaises(self.g.Refused):
            self.g.legacy_unit_text(self.g.seal(UNIT_BODY, self.g.INI_HEADER_PREFIX))

    def test_owner_texts_and_units(self):
        self.assertIn(b"php_admin_value[memory_limit]", self.g.OWNER_POOL_LINES)
        self.assertIn(b"[Service]", self.g.OWNER_DROPIN_TEXT)
        self.assertEqual(self.g.php_unit_of("/etc/php/8.4/fpm/pool.d/site1.conf"), "php8.4-fpm")
        self.assertEqual(self.g.php_unit_of("/etc/php/php-fpm.d/site1.conf"), "php-fpm")
        self.assertEqual(self.g.unit_path(7), "/etc/systemd/system/celikapp-7.service")
        for mode in ("read-files10", "owner-pool10", "owner-unit10", "lab-legacy10"):
            self.assertIn(mode, self.g.MODES)

    def test_the_new_sections_are_planned_last(self):
        keys = [k for k, _ in s10.SECTIONS]
        self.assertEqual(keys[-3:], ["m-pool-directive", "n-unit-dropin", "o-legacy-pool"])
        for name in ("m_pool", "n_unit", "o_legacy_pool", "files10", "other_file", "node_version", "hosting_node"):
            self.assertTrue(callable(getattr(s10.Set10Trial, name)), name)


if __name__ == "__main__":
    unittest.main()
