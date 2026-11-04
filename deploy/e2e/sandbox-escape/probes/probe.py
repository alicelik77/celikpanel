"""Escape-test kit: the Python probe (README.md).

Run inside a rendered sandbox unit of the python kind, it tries the escape
matrix of docs/SITE-SANDBOX.md with Python's own means (ctypes for the
system calls) and prints one line per attempt:

    SANDBOX-PROBE <item> <name> BLOCKED|ESCAPED|SKIP <detail>

It then serves 127.0.0.1:$PROBE_PORT; GET /smtp tries the mail ports again.
Test code only: it tries and reports, and keeps nothing an escape would give.
"""

import ctypes
import ctypes.util
import glob
import http.server
import os
import socket
import subprocess
import sys

libc = ctypes.CDLL(ctypes.util.find_library("c"), use_errno=True)


def say(item, name, verdict, detail=""):
    detail = str(detail).replace("\n", " ")
    print(f"SANDBOX-PROBE {item} {name} {verdict} {detail}", flush=True)


def attempt(item, name, fn):
    """fn must raise (or return a non-zero errno) for the attempt to be blocked."""
    try:
        result = fn()
    except Exception as exc:  # the expected path
        say(item, name, "BLOCKED", f"{type(exc).__name__}: {exc}")
        return
    if isinstance(result, int) and result != 0:
        say(item, name, "BLOCKED", f"errno {ctypes.get_errno()}")
        return
    say(item, name, "ESCAPED", "succeeded")


def call(rc):
    return 0 if rc == 0 else -1


def dial(family, addr):
    s = socket.socket(family, socket.SOCK_STREAM)
    s.settimeout(3)
    try:
        s.connect(addr)
    finally:
        s.close()


def run(*argv):
    done = subprocess.run(argv, stdin=subprocess.DEVNULL, capture_output=True, timeout=20)
    if done.returncode != 0:
        raise RuntimeError(f"exit {done.returncode}: {done.stderr.decode(errors='replace').strip()[:120]}")


def smtp():
    remote = os.environ.get("PROBE_SMTP_REMOTE", "smtp.gmail.com")
    for port in (25, 465, 587):
        attempt(9, f"smtp-loopback-{port}", lambda: dial(socket.AF_INET, ("127.0.0.1", port)))
        attempt(9, f"smtp-loopback6-{port}", lambda: dial(socket.AF_INET6, ("::1", port)))
        attempt(9, f"smtp-remote-{port}", lambda: socket.create_connection((remote, port), timeout=3).close())


def matrix(app_dir, port):
    uid = os.getuid()
    other = os.environ.get("PROBE_OTHER_SITE", "/var/www/celikpanel/subscriptions/990002/sites/990002/secret.txt")
    attempt(1, "other-site-file", lambda: open(other).read())
    try:
        visible = [e for e in os.listdir("/var/www/celikpanel/subscriptions") if not app_dir.startswith(f"/var/www/celikpanel/subscriptions/{e}/")]
        say(1, "list-subscriptions", "ESCAPED" if visible else "BLOCKED", "visible: " + ",".join(visible))
    except OSError as exc:
        say(1, "list-subscriptions", "BLOCKED", exc)
    attempt(1, "list-home", lambda: os.listdir("/home"))
    attempt(2, "shadow", lambda: open("/etc/shadow").read())
    attempt(2, "etc-celikpanel", lambda: os.listdir("/etc/celikpanel"))
    attempt(2, "var-lib-celikpanel", lambda: os.listdir("/var/lib/celikpanel"))
    attempt(3, "agent-socket", lambda: dial(socket.AF_UNIX, "/run/celikpanel/agent.sock"))
    sockets = glob.glob("/run/php/*.sock") + glob.glob("/run/php-fpm/*.sock")
    say(3, "fpm-sockets-visible", "ESCAPED" if sockets else "BLOCKED", ",".join(sockets))
    for s in sockets:
        attempt(3, "fpm-socket", lambda s=s: dial(socket.AF_UNIX, s))
    attempt(4, "setuid-0", lambda: os.setuid(0))
    attempt(4, "sudo", lambda: run("/usr/bin/sudo", "-n", "true"))
    attempt(4, "su", lambda: run("/bin/su", "-c", "true"))
    suid = os.path.join(app_dir, "data", "suid-test-py")
    os.makedirs(os.path.dirname(suid), exist_ok=True)
    with open(suid, "w") as f:
        f.write("x")

    def set_suid():
        os.chmod(suid, 0o4755)
        if not os.stat(suid).st_mode & 0o4000:
            raise RuntimeError("the setuid bit did not stick")

    attempt(4, "chmod-u+s", set_suid)
    os.remove(suid)
    PTRACE_ATTACH, PTRACE_DETACH = 16, 17
    for pid in (1, os.getppid()):
        def ptrace(pid=pid):
            rc = libc.ptrace(PTRACE_ATTACH, pid, None, None)
            if rc == 0:
                libc.ptrace(PTRACE_DETACH, pid, None, None)
                return 0
            raise OSError(ctypes.get_errno(), os.strerror(ctypes.get_errno()))
        attempt(5, f"ptrace-{pid}", ptrace)
    CLONE_NEWNS, CLONE_NEWUSER = 0x00020000, 0x10000000

    def errno_call(rc):
        if rc != 0:
            raise OSError(ctypes.get_errno(), os.strerror(ctypes.get_errno()))

    attempt(6, "mount", lambda: errno_call(libc.mount(b"none", os.path.join(app_dir, "data").encode(), b"tmpfs", 0, None)))
    attempt(6, "unshare-user", lambda: errno_call(libc.unshare(CLONE_NEWUSER)))
    attempt(6, "unshare-ns", lambda: errno_call(libc.unshare(CLONE_NEWNS)))
    foreign = [p for p in os.listdir("/proc") if p.isdigit() and os.stat(f"/proc/{p}").st_uid != uid]
    say(7, "proc-other-users", "ESCAPED" if foreign else "BLOCKED", "pids: " + ",".join(foreign))
    attempt(8, "af_packet", lambda: socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(3)).close())
    attempt(8, "raw-icmp", lambda: socket.socket(socket.AF_INET, socket.SOCK_RAW, socket.IPPROTO_ICMP).close())
    smtp()
    attempt(9, "sendmail", lambda: run("/usr/sbin/sendmail", "-t", "-i"))
    for addr in (("127.0.0.1", port + 1), ("127.0.0.1", 0)):
        def bind(addr=addr):
            s = socket.socket()
            try:
                s.bind(addr)
            finally:
                s.close()
        attempt(10, f"bind-{addr[0]}:{addr[1]}", bind)

    def bind_udp():
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        try:
            s.bind(("127.0.0.1", 0))
        finally:
            s.close()

    attempt(10, "bind-udp", bind_udp)
    attempt(12, "metadata-v4", lambda: dial(socket.AF_INET, ("169.254.169.254", 80)))
    attempt(12, "metadata-v6", lambda: dial(socket.AF_INET6, ("fd00:ec2::254", 80)))
    for path in ("/etc/celik-probe", "/usr/local/celik-probe", "/var/www/celikpanel/celik-probe", os.path.dirname(app_dir) + "/celik-probe"):
        def write(path=path):
            with open(path, "w") as f:
                f.write("x")
            os.remove(path)
        attempt(13, f"write-{path}", write)
    for marker in ("/tmp/celik-probe-marker", "/var/tmp/celik-probe-marker"):
        found = os.path.exists(marker)
        say(13, "tmp-persistence", "ESCAPED" if found else "BLOCKED", f"{marker} from an earlier start" if found else f"no {marker} from an earlier start")
        with open(marker, "w") as f:
            f.write("x")
    say(14, "exec", "SKIP", "the python kind runs its interpreter")
    SYS_KEYCTL = 250  # x86_64
    attempt(15, "keyctl-user-keyring", lambda: errno_call(0 if libc.syscall(SYS_KEYCTL, 0, ctypes.c_int(-4), 0) >= 0 else -1))
    attempt(16, "cgroup-root-procs", lambda: open("/sys/fs/cgroup/cgroup.procs", "w").write("1"))
    own = open("/proc/self/cgroup").read().strip().split("::", 1)[-1]
    attempt(16, "cgroup-own-memory-max", lambda: open(f"/sys/fs/cgroup{own}/memory.max", "w").write("max"))


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/smtp":
            smtp()
            print("SANDBOX-PROBE SMTP-AGAIN DONE", flush=True)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok\n")

    def log_message(self, *args):
        pass


def main():
    app_dir, port = os.environ.get("PROBE_APPDIR", ""), int(os.environ.get("PROBE_PORT", "0"))
    if not app_dir or not port:
        print("PROBE_APPDIR and PROBE_PORT are required", file=sys.stderr)
        sys.exit(2)
    matrix(app_dir, port)
    print("SANDBOX-PROBE DONE", flush=True)
    http.server.HTTPServer(("127.0.0.1", port), Handler).serve_forever()


if __name__ == "__main__":
    main()
