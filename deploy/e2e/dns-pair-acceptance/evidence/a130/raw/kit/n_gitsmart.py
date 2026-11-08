# labnet: smart HTTP for the lab Git server (git http-backend behind a tiny TLS handler), git.lab.test 192.0.2.62:443. Lab only.
import http.server, ssl, subprocess, os, time
class H(http.server.BaseHTTPRequestHandler):
    def run(self):
        path, _, query = self.path.partition("?")
        body = self.rfile.read(int(self.headers.get("Content-Length") or 0)) if self.command == "POST" else b""
        env = dict(os.environ, GIT_PROJECT_ROOT="/srv/git", GIT_HTTP_EXPORT_ALL="1", PATH_INFO=path, QUERY_STRING=query,
                   REQUEST_METHOD=self.command, CONTENT_TYPE=self.headers.get("Content-Type", ""), CONTENT_LENGTH=str(len(body)),
                   REMOTE_ADDR=self.client_address[0], GIT_CONFIG_COUNT="1", GIT_CONFIG_KEY_0="safe.directory", GIT_CONFIG_VALUE_0="*")
        if self.headers.get("Content-Encoding"): env["HTTP_CONTENT_ENCODING"] = self.headers["Content-Encoding"]
        if self.headers.get("Git-Protocol"): env["GIT_PROTOCOL"] = self.headers["Git-Protocol"]
        out = subprocess.run(["git", "http-backend"], input=body, env=env, capture_output=True).stdout
        head, _, rest = out.partition(b"\r\n\r\n")
        if not _: head, _, rest = out.partition(b"\n\n")
        status = 200; hdrs = []
        for line in head.decode("latin-1").splitlines():
            k, _, v = line.partition(":")
            if k.lower() == "status": status = int(v.strip().split()[0])
            elif k: hdrs.append((k, v.strip()))
        self.send_response(status)
        for k, v in hdrs: self.send_header(k, v)
        self.send_header("Content-Length", str(len(rest))); self.end_headers(); self.wfile.write(rest)
        open("/root/git-requests.log", "a").write(time.strftime("%H:%M:%S ") + self.client_address[0] + " smart " + self.command + " " + self.path + " " + str(status) + "\n")
    do_GET = run; do_POST = run
    def log_message(self, *a): pass
srv = http.server.ThreadingHTTPServer(("192.0.2.62", 443), H)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER); ctx.load_cert_chain("/root/git-tls.pem", "/root/git-tls-key.pem")
srv.socket = ctx.wrap_socket(srv.socket, server_side=True); srv.serve_forever()
