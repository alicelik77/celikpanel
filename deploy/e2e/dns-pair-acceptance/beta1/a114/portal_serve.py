#!/usr/bin/env python3
"""labnet: the lab stand-in for https://celikpanel.net at 192.0.2.61:443 (alpha.114 update topology only).
Serves /root/a114-portal/site read-only with a lab CA certificate for the name celikpanel.net, logs every
request path to /root/a114-portal/requests.log. Only guests that were given the lab CA and a hosts line
reach it. Lab only."""
import http.server, os, ssl, sys, time
from pathlib import Path

ROOT = Path("/root/a114-portal/site")
LOG = Path("/root/a114-portal/requests.log")


class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        p = (ROOT / self.path.lstrip("/").split("?")[0]).resolve()
        with LOG.open("a") as f:
            f.write(f"{time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())} {self.client_address[0]} {self.path}\n")
        if ROOT not in p.parents or not p.is_file() or self.headers.get("Host", "").split(":")[0] != "celikpanel.net":
            self.send_error(404); return
        raw = p.read_bytes()
        self.send_response(200)
        self.send_header("Content-Type", "application/octet-stream")
        self.send_header("Content-Length", str(len(raw)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, *a):
        pass


srv = http.server.ThreadingHTTPServer(("192.0.2.61", 443), H)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain("/root/a114-portal/tls.pem", "/root/a114-portal/tls-key.pem")
srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
srv.serve_forever()
