"""set18b offline check of the H2 fix (no guest): a local TLS server with two certificates (one for SNI "localhost",
another for a client that sends none, as the Panel after the handover, set18 O2). The set18b transport pinned to the
"localhost" leaf must succeed and send SNI "localhost" with Host 127.0.0.1:port; the set18 transport (negative
control) must fail with PinMismatch because it sends no SNI.  usage: snitest.py HARNESS_DIR"""
import hashlib, http.server, json, os, ssl, subprocess, sys, tempfile, threading
from pathlib import Path

harness = Path(sys.argv[1])
sys.path.insert(0, str(harness))
import set18b_trial as t  # noqa: E402

api = t.t18.out.pair_modules()["panel_api"]
work = Path(tempfile.mkdtemp(prefix="set18b-snitest-"))
for name in ("named", "default"):
    subprocess.run(["openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes", "-days", "1",
                    "-subj", f"/CN=set18b-{name}", "-keyout", str(work / f"{name}.key"), "-out", str(work / f"{name}.pem")],
                   check=True, capture_output=True)
der = lambda name: ssl.PEM_cert_to_DER_cert((work / f"{name}.pem").read_text())
named_leaf = hashlib.sha256(der("named")).hexdigest()
seen = {"sni": [], "host": []}
default_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER); default_ctx.load_cert_chain(work / "default.pem", work / "default.key")
named_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER); named_ctx.load_cert_chain(work / "named.pem", work / "named.key")


def pick(sock, server_name, _ctx):
    seen["sni"].append(server_name)
    if server_name:
        sock.context = named_ctx


default_ctx.sni_callback = pick


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        seen["host"].append(self.headers.get("Host"))
        body = b'{"ok":true}'
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)

    def log_message(self, *args):
        pass


server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
server.socket = default_ctx.wrap_socket(server.socket, server_side=True)
port = server.server_address[1]
threading.Thread(target=server.serve_forever, daemon=True).start()
result = {"port": port, "named_leaf": named_leaf}
fixed = t.sni_transport_class(api)("127.0.0.1", port, named_leaf)
r = fixed("GET", "/x", {}, None, 10)
result["set18b_transport"] = {"status": r.status, "sni_sent": seen["sni"][-1], "host": seen["host"][-1]}
try:
    api.PinnedHTTPSTransport("127.0.0.1", port, named_leaf)("GET", "/x", {}, None, 10)
    result["set18_transport"] = "succeeded (unexpected)"
except api.PinMismatch as exc:
    result["set18_transport"] = f"PinMismatch (expected negative control; SNI sent: {seen['sni'][-1]!r})"
server.shutdown()
ok = result["set18b_transport"] == {"status": 200, "sni_sent": "localhost", "host": f"127.0.0.1:{port}"} and result["set18_transport"].startswith("PinMismatch")
result["verdict"] = "passed" if ok else "failed"
for f in work.iterdir():
    f.unlink()
work.rmdir()
print(json.dumps(result, sort_keys=True))
sys.exit(0 if ok else 1)
