"""Offline tests of set11's pure rules: the probe files and loopback exchanges read from the two watchers, and the
owner's text edits of a vhost (CelikPanel's include line, the mail validation block, a bound listen address). No guest
is needed."""
from __future__ import annotations

import importlib.util
from pathlib import Path
import sys
import unittest

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set11_trial as s11  # noqa: E402


def guest_module():
    spec = importlib.util.spec_from_file_location("set11_guest_under_test", HERE / "guest_set11_native.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


DIR = "/var/lib/celikpanel/acme/1/2/.well-known/acme-challenge"
NAME = "celikpanel-probe-" + "a" * 32
VHOST = ("# Nginx vhost for set11-a.test\n"
         "server {\n    listen 80;\n    listen [::]:80;\n    server_name set11-a.test www.set11-a.test;\n\n"
         "    # CelikPanel's own parts of this site (certificate validation) / CelikPanel'in parçaları\n"
         "    include /etc/nginx/celikpanel-managed.d/set11-a.test/*.conf;\n"
         "    root /srv/a;\n    location / {\n        try_files $uri =404;\n    }\n}\n")


class Set11ProbeRules(unittest.TestCase):
    def test_probe_file_lifecycle(self):
        rows = [{"t": 1.0, "dir": DIR, "name": NAME, "events": ["create"], "mode": "0o640", "uid": 0, "gid": 0, "size": 0},
                {"t": 1.1, "dir": DIR, "name": NAME, "events": ["attrib"], "mode": "0o644", "uid": 0, "gid": 0, "size": 40,
                 "bytes": "celikpanel validation probe x\n"},
                {"t": 2.0, "dir": DIR, "name": NAME, "events": ["delete"], "lstat": "FileNotFoundError"},
                {"t": 1.5, "dir": DIR, "name": "token-of-certbot", "events": ["create"]},
                {"t": 1.6, "dir": "/elsewhere", "name": NAME, "events": ["create"]}]
        files = s11.probe_files(rows, DIR)
        self.assertEqual(list(files), [NAME])
        self.assertEqual(files[NAME]["final_mode"], "0o644")
        self.assertEqual((files[NAME]["created_t"], files[NAME]["deleted_t"]), (1.0, 2.0))
        self.assertEqual(files[NAME]["bytes"], "celikpanel validation probe x\n")

    def test_exchanges_pair_each_request_with_its_answer(self):
        rows = [{"t": 1.2, "request": f"GET /.well-known/acme-challenge/{NAME} HTTP/1.1", "host": "a.test", "sport": 5000},
                {"t": 1.3, "request": f"GET /.well-known/acme-challenge/{NAME} HTTP/1.1", "host": "www.a.test", "sport": 5001},
                {"t": 1.25, "answer": "HTTP/1.1 200 OK", "to_port": 5000, "body_start": "celikpanel validation probe x\n"},
                {"t": 1.35, "answer": "HTTP/1.1 404 Not Found", "to_port": 5001, "body_start": "<html>"},
                {"t": 1.4, "request": "GET /other HTTP/1.1", "host": "a.test", "sport": 5002}]
        found = s11.exchanges(rows)
        self.assertEqual([(e["host"], e["status"]) for e in found], [("a.test", 200), ("www.a.test", 404)])
        probe = {"bytes": "celikpanel validation probe x\n"}
        self.assertTrue(s11.answered_with(probe, found[0]))
        self.assertFalse(s11.answered_with(probe, found[1]))

    def test_syn_rows_after_a_time(self):
        rows = [{"t": 1.0, "syn": True, "dport": 443}, {"t": 3.0, "syn": True, "dport": 443}, {"t": 3.0, "syn": True, "dport": 80}]
        self.assertEqual(len(s11.syn_rows(rows, 443, 2.0)), 1)
        self.assertEqual(len(s11.syn_rows(rows, 80)), 1)


class Set11OwnerEdits(unittest.TestCase):
    def setUp(self):
        self.g = guest_module()

    def test_remove_and_put_back_the_managed_include(self):
        removed, what = self.g.edit_text(VHOST, "set11-a.test", "remove-managed-include", {})
        self.assertNotIn("celikpanel-managed.d/set11-a.test", removed)
        self.assertEqual(len(what["removed_lines"]), 1)
        back, _ = self.g.edit_text(removed, "set11-a.test", "add-managed-include", {})
        self.assertIn("include /etc/nginx/celikpanel-managed.d/set11-a.test/*.conf;", back)
        self.assertLess(back.index("server_name"), back.index("celikpanel-managed.d"))
        with self.assertRaises(self.g.Refused):
            self.g.edit_text(VHOST, "set11-a.test", "add-managed-include", {})

    def test_mail_block_added_then_removed(self):
        added, _ = self.g.edit_text(VHOST, "set11-a.test", "add-mail-block", {})
        self.assertIn("server_name mail.set11-a.test;", added)
        removed, what = self.g.edit_text(added, "set11-a.test", "remove-mail-block", {})
        self.assertNotIn("mail.set11-a.test", removed)
        self.assertEqual(removed.rstrip("\n"), VHOST.rstrip("\n"))
        self.assertEqual(what["removed_lines"][-1], "}")

    def test_bind_address(self):
        bound, what = self.g.edit_text(VHOST, "set11-a.test", "bind-address", {"address": "10.0.2.15"})
        self.assertIn("listen 10.0.2.15:80;", bound)
        self.assertNotIn("listen [::]:80;", bound)
        self.assertEqual(len(what["changed"]), 2)
        with self.assertRaises(self.g.Refused):
            self.g.edit_text(VHOST, "set11-a.test", "bind-address", {"address": "not-an-address"})

    def test_challenge_root_of(self):
        text = "location ^~ /.well-known/acme-challenge/ {\n    root /var/lib/celikpanel/acme/1/2;\n    try_files $uri =404;\n}\n"
        self.assertEqual(self.g.challenge_root_of(text), "/var/lib/celikpanel/acme/1/2")
        self.assertIsNone(self.g.challenge_root_of(None))

    def test_watch_label_is_plain(self):
        self.assertEqual(self.g.label_of({"tag": "a1"}), "a1")
        with self.assertRaises(self.g.Refused):
            self.g.label_of({"tag": "../x"})


if __name__ == "__main__":
    unittest.main()
