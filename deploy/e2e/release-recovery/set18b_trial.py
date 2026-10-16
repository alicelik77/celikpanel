#!/usr/bin/env python3
"""set18b: the twenty-second native record. set18_trial (unchanged on disk) with the source of the set18 D1/O1/O3 fix
(HEAD 6b363ac46 of feat/alpha87-second-server plus set18's policy-only fixture commit labelled v0.1.0-alpha.87 / 87),
installed fresh on a disposable Debian 13 guest with set18's lab DNS and ACME (guest_set18_lab_dns.py, unchanged),
the owner's setup and its extension followed in a real headless Chrome (web/tools/browser-inspect/cold-load-set18b.mjs).

One harness change against set18 (its run note H2): the driver's pinned API client sends the TLS server name
``localhost``. set18's client connected to 127.0.0.1 without a server name, while the pin is read on the guest with
the server name ``localhost`` (guest_upd1_workload.py ``tls-leaf``). After the certificate handover the Panel serves
its lab-CA certificate for any server name but its first self-signed certificate to a client that sends none (set18
O2), so every driver read after the handover failed with ``PinMismatch``. ``SNIPinnedHTTPSTransport`` reads and pins
the same certificate the guest reports; the pin itself is unchanged (the exact leaf digest).

  set18b_trial.py plan --cell set18-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X [--dry-run]
  set18b_trial.py run  --cell set18-debian13 --artifacts A.json --work-root /var/tmp/cp-release-drill-X --execute
"""
from __future__ import annotations

import hashlib
import http.client
from pathlib import Path
import ssl
import sys
import time

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))
import set18b_fixture as fx18b  # noqa: E402  (set18's profile; the source named is 6b363ac46)
import set18_trial as t18  # noqa: E402

base = t18.base
SNI_NAME = "localhost"   # the name guest_upd1_workload.tls_leaf sends when it reads the pin on the guest


class _SNIConnection(http.client.HTTPSConnection):
    """An HTTPS connection to the loopback forward that sends SNI_NAME (the Host header stays 127.0.0.1:port)."""

    def connect(self) -> None:
        http.client.HTTPConnection.connect(self)
        self.sock = self._context.wrap_socket(self.sock, server_hostname=SNI_NAME)


def sni_transport_class(panel_api):
    class SNIPinnedHTTPSTransport(panel_api.PinnedHTTPSTransport):
        """panel_api.PinnedHTTPSTransport with the server name SNI_NAME; the exact leaf pin is unchanged."""

        sni = SNI_NAME

        def __call__(self, method, path, headers, body, timeout):
            context = ssl.create_default_context()
            context.check_hostname = False
            context.verify_mode = ssl.CERT_NONE  # replaced by the exact leaf pin below
            connection = _SNIConnection(self.host, self.port, timeout=timeout, context=context)
            started = time.monotonic()
            try:
                connection.connect()
                der = connection.sock.getpeercert(binary_form=True)
                if hashlib.sha256(der or b"").hexdigest() != self.leaf_sha256:
                    raise panel_api.PinMismatch("panel TLS leaf differs from the guest-reported certificate")
                connection.request(method, path, body=body, headers=headers)
                response = connection.getresponse()
                payload = response.read(8 << 20)
                return panel_api.Response(response.status, list(response.getheaders()), payload, time.monotonic() - started)
            finally:
                connection.close()
    return SNIPinnedHTTPSTransport


class Set18bTrial(t18.Set18Trial):
    def refresh_pin(self) -> str:
        """owner_update_trial.refresh_pin with set18b's transport (H2); a changed leaf is recorded as before."""
        leaf = self.workload("tls-leaf")["leaf_sha256"]
        if self.transport is None:
            self.transport = sni_transport_class(self.p["panel_api"])("127.0.0.1", self.local_port, leaf)
        elif self.transport.leaf_sha256 != leaf:
            self.state.setdefault("pin_changes", []).append({"at": base.utc_now(), "from": self.transport.leaf_sha256,
                                                             "to": leaf, "sni": SNI_NAME})
            self.transport.leaf_sha256 = leaf
        return leaf


# set18's main() builds the trial and names the cell kind from these module globals (read at call time).
t18.Set18Trial = Set18bTrial
t18.CELL_KIND = "set18b-fresh-setup-to-ready-extension"
main = t18.main

if __name__ == "__main__":
    raise SystemExit(main())
