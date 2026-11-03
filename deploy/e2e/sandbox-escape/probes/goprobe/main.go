//go:build linux

// Command goprobe is the escape-test kit's static Go probe (README.md). Run
// inside a rendered sandbox unit, it tries every escape of the matrix in
// docs/SITE-SANDBOX.md and prints one line per attempt:
//
//	SANDBOX-PROBE <item> <name> BLOCKED|ESCAPED|SKIP <detail>
//
// BLOCKED means the attempt failed, as it must. The probe then serves
// 127.0.0.1:$PROBE_PORT so run.sh can ask it to try the mail ports again
// (GET /smtp) after it removed the egress table. It is test code: it only
// tries and reports; it never keeps what an escape would give.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var out = os.Stdout

func report(item int, name string, escaped bool, detail string) {
	verdict := "BLOCKED"
	if escaped {
		verdict = "ESCAPED"
	}
	detail = strings.ReplaceAll(detail, "\n", " ")
	fmt.Fprintf(out, "SANDBOX-PROBE %d %s %s %s\n", item, name, verdict, detail)
}

func skip(item int, name, why string) {
	fmt.Fprintf(out, "SANDBOX-PROBE %d %s SKIP %s\n", item, name, why)
}

// attempt reports an operation that must fail.
func attempt(item int, name string, err error) {
	if err != nil {
		report(item, name, false, err.Error())
		return
	}
	report(item, name, true, "succeeded")
}

// iovec is struct iovec with a plain address (the remote one is not ours).
type iovec struct {
	base uintptr
	len  uint64
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func readFile(path string) error {
	_, err := os.ReadFile(path)
	return err
}

func readDir(path string) error {
	_, err := os.ReadDir(path)
	return err
}

func dial(network, addr string) error {
	c, err := net.DialTimeout(network, addr, 3*time.Second)
	if err == nil {
		c.Close()
	}
	return err
}

func run(path string, args ...string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdin = nil
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
}

func smtp() {
	for _, port := range []string{"25", "465", "587"} {
		attempt(9, "smtp-loopback-"+port, dial("tcp", "127.0.0.1:"+port))
		attempt(9, "smtp-loopback6-"+port, dial("tcp", "[::1]:"+port))
		attempt(9, "smtp-remote-"+port, dial("tcp", net.JoinHostPort(env("PROBE_SMTP_REMOTE", "smtp.gmail.com"), port)))
	}
}

func matrix(appDir string, port int) {
	uid := os.Getuid()
	// 1. Another site's files.
	attempt(1, "other-site-file", readFile(env("PROBE_OTHER_SITE", "/var/www/celikpanel/subscriptions/990002/sites/990002/secret.txt")))
	if entries, err := os.ReadDir("/var/www/celikpanel/subscriptions"); err == nil {
		var others []string
		for _, e := range entries {
			if !strings.HasPrefix(appDir, "/var/www/celikpanel/subscriptions/"+e.Name()+"/") {
				others = append(others, e.Name())
			}
		}
		report(1, "list-subscriptions", len(others) > 0, "visible: "+strings.Join(others, ","))
	} else {
		report(1, "list-subscriptions", false, err.Error())
	}
	attempt(1, "list-home", readDir("/home"))
	// 2. Secrets of the server.
	attempt(2, "shadow", readFile("/etc/shadow"))
	attempt(2, "etc-celikpanel", readDir("/etc/celikpanel"))
	attempt(2, "var-lib-celikpanel", readDir("/var/lib/celikpanel"))
	attempt(2, "agent-private", readDir("/var/lib/celikpanel-agent-private"))
	// 3. The Agent's socket and other sites' PHP-FPM sockets.
	attempt(3, "agent-socket", dial("unix", "/run/celikpanel/agent.sock"))
	sockets, _ := filepath.Glob("/run/php/*.sock")
	more, _ := filepath.Glob("/run/php-fpm/*.sock")
	sockets = append(sockets, more...)
	report(3, "fpm-sockets-visible", len(sockets) > 0, strings.Join(sockets, ","))
	for _, s := range sockets {
		attempt(3, "fpm-socket", dial("unix", s))
	}
	// 4. Privilege: setuid, sudo, su, chmod u+s.
	attempt(4, "setuid-0", syscall.Setuid(0))
	attempt(4, "sudo", run("/usr/bin/sudo", "-n", "true"))
	attempt(4, "su", run("/bin/su", "-c", "true"))
	suid := filepath.Join(appDir, "data", "suid-test")
	_ = os.MkdirAll(filepath.Dir(suid), 0o700)
	if err := os.WriteFile(suid, []byte("x"), 0o700); err == nil {
		err = os.Chmod(suid, 0o4755)
		if err == nil {
			if info, statErr := os.Stat(suid); statErr == nil && info.Mode()&os.ModeSetuid == 0 {
				err = fmt.Errorf("the setuid bit did not stick")
			}
		}
		attempt(4, "chmod-u+s", err)
		_ = os.Remove(suid)
	}
	// 5. ptrace and process_vm_readv.
	for _, pid := range []int{1, os.Getppid()} {
		if pid <= 0 || pid == os.Getpid() {
			// With PrivatePIDs the program is PID 1 of its own namespace.
			continue
		}
		err := syscall.PtraceAttach(pid)
		if err == nil {
			_ = syscall.PtraceDetach(pid)
		}
		attempt(5, "ptrace-"+strconv.Itoa(pid), err)
		var buf [8]byte
		local := iovec{base: uintptr(unsafe.Pointer(&buf[0])), len: 8}
		remote := iovec{base: 0x400000, len: 8}
		_, _, errno := syscall.Syscall6(310, uintptr(pid), uintptr(unsafe.Pointer(&local)), 1, uintptr(unsafe.Pointer(&remote)), 1, 0) // process_vm_readv (amd64)
		if errno == 0 {
			attempt(5, "process_vm_readv-"+strconv.Itoa(pid), nil)
		} else {
			attempt(5, "process_vm_readv-"+strconv.Itoa(pid), errno)
		}
	}
	// 6. mount, unshare, pivot_root.
	attempt(6, "mount", syscall.Mount("none", filepath.Join(appDir, "data"), "tmpfs", 0, ""))
	attempt(6, "unshare-user", syscall.Unshare(syscall.CLONE_NEWUSER))
	attempt(6, "unshare-ns", syscall.Unshare(syscall.CLONE_NEWNS))
	attempt(6, "pivot_root", syscall.PivotRoot(appDir, filepath.Join(appDir, "data")))
	// 7. Other users' processes in /proc.
	var foreign []string
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, e := range entries {
			if _, err := strconv.Atoi(e.Name()); err != nil {
				continue
			}
			var st syscall.Stat_t
			if syscall.Stat("/proc/"+e.Name(), &st) == nil && int(st.Uid) != uid {
				foreign = append(foreign, e.Name())
			}
		}
	}
	report(7, "proc-other-users", len(foreign) > 0, "pids: "+strings.Join(foreign, ","))
	// 8. Raw sockets and AF_PACKET.
	if fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, 0x0300); err == nil {
		syscall.Close(fd)
		attempt(8, "af_packet", nil)
	} else {
		attempt(8, "af_packet", err)
	}
	if fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP); err == nil {
		syscall.Close(fd)
		attempt(8, "raw-icmp", nil)
	} else {
		attempt(8, "raw-icmp", err)
	}
	// 9. Mail ports and sendmail.
	smtp()
	attempt(9, "sendmail", run("/usr/sbin/sendmail", "-t", "-i"))
	// 10. Binding another port, port 0, UDP.
	for _, addr := range []string{"127.0.0.1:" + strconv.Itoa(port+1), "127.0.0.1:0"} {
		l, err := net.Listen("tcp", addr)
		if err == nil {
			l.Close()
		}
		attempt(10, "bind-"+addr, err)
	}
	if c, err := net.ListenPacket("udp", "127.0.0.1:0"); err == nil {
		c.Close()
		attempt(10, "bind-udp", nil)
	} else {
		attempt(10, "bind-udp", err)
	}
	// 12. Cloud metadata.
	attempt(12, "metadata-v4", dial("tcp", "169.254.169.254:80"))
	attempt(12, "metadata-v6", dial("tcp", "[fd00:ec2::254]:80"))
	// 13. Writes outside the folder; /tmp persistence across a restart.
	for _, p := range []string{"/etc/celik-probe", "/usr/local/celik-probe", "/var/www/celikpanel/celik-probe", filepath.Dir(appDir) + "/celik-probe"} {
		err := os.WriteFile(p, []byte("x"), 0o600)
		if err == nil {
			_ = os.Remove(p)
		}
		attempt(13, "write-"+p, err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "data", "probe-ok"), []byte("x"), 0o600); err != nil {
		report(13, "write-own-folder", true, "the program cannot write its own folder: "+err.Error())
	}
	// /tmp and /var/tmp are the unit's own (PrivateTmp) and must not
	// survive a restart.
	for _, marker := range []string{"/tmp/celik-probe-marker", "/var/tmp/celik-probe-marker"} {
		if _, err := os.Stat(marker); err == nil {
			report(13, "tmp-persistence", true, marker+" survived the restart")
		} else {
			report(13, "tmp-persistence", false, "no "+marker+" from an earlier start")
		}
		_ = os.WriteFile(marker, []byte("x"), 0o600)
	}
	// 14. exec of python3, sh, ld.so (program kind only).
	if env("PROBE_KIND", "program") == "program" {
		attempt(14, "exec-sh", run("/bin/sh", "-c", "true"))
		attempt(14, "exec-python3", run("/usr/bin/python3", "-c", "pass"))
		loaders, _ := filepath.Glob("/lib64/ld-linux*.so*")
		more, _ := filepath.Glob("/usr/lib/*/ld-linux*.so*")
		loaders = append(loaders, more...)
		for _, ld := range loaders {
			attempt(14, "ld.so-python3", run(ld, "/usr/bin/python3", "-c", "pass"))
			break
		}
	} else {
		skip(14, "exec", "the python kind runs its interpreter")
	}
	// 15. Kernel keyring.
	const sysKeyctl, keyctlGetKeyringID, keySpecUserKeyring = 250, 0, -4
	if _, _, errno := syscall.Syscall(sysKeyctl, keyctlGetKeyringID, uintptr(keySpecUserKeyring&0xffffffff), 0); errno != 0 {
		attempt(15, "keyctl-user-keyring", errno)
	} else {
		attempt(15, "keyctl-user-keyring", nil)
	}
	// 16. cgroup writes.
	attempt(16, "cgroup-root-procs", os.WriteFile("/sys/fs/cgroup/cgroup.procs", []byte("1"), 0))
	if self, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		path := "/sys/fs/cgroup" + strings.TrimSpace(strings.TrimPrefix(string(self), "0::"))
		attempt(16, "cgroup-own-memory-max", os.WriteFile(filepath.Join(path, "memory.max"), []byte("max"), 0))
	}
}

// load modes (item 11): run.sh starts the probe again with PROBE_MODE set
// and checks the limit from outside (systemd's Result, the journal).
func load(mode string) {
	switch mode {
	case "fork":
		n := 0
		for ; n < 100000; n++ {
			_, err := syscall.ForkExec("/proc/self/exe", []string{"goprobe"}, &syscall.ProcAttr{Env: []string{"PROBE_MODE=sleep"}})
			if err != nil {
				report(11, "fork-bomb", false, fmt.Sprintf("refused after %d processes: %v", n, err))
				return
			}
		}
		report(11, "fork-bomb", true, "100000 processes started")
	case "memory":
		var hold [][]byte
		for i := 0; i < 64; i++ {
			chunk := make([]byte, 64<<20)
			for j := range chunk {
				chunk[j] = byte(j)
			}
			hold = append(hold, chunk)
			fmt.Fprintf(out, "SANDBOX-PROBE-LOAD memory %d MB\n", (i+1)*64)
		}
		report(11, "memory-balloon", true, "4 GB allocated")
		_ = hold
	case "cpu":
		end := time.Now().Add(20 * time.Second)
		for time.Now().Before(end) {
		}
		fmt.Fprintln(out, "SANDBOX-PROBE-LOAD cpu done")
	case "logflood":
		for i := 0; i < 50000; i++ {
			fmt.Fprintf(out, "SANDBOX-PROBE-FLOOD %d\n", i)
		}
		fmt.Fprintln(out, "SANDBOX-PROBE-LOAD logflood done")
	case "sleep":
		time.Sleep(time.Hour)
	case "bind-any":
		l, err := net.Listen("tcp", "0.0.0.0:"+env("PROBE_PORT", "0"))
		if err != nil {
			report(10, "bind-any", false, err.Error())
			return
		}
		fmt.Fprintln(out, "SANDBOX-PROBE-LOAD bind-any listening")
		for {
			c, err := l.Accept()
			if err == nil {
				c.Close()
			}
		}
	}
}

func main() {
	mode := env("PROBE_MODE", "matrix")
	if mode != "matrix" {
		load(mode)
		fmt.Fprintln(out, "SANDBOX-PROBE DONE")
		select {}
	}
	appDir := env("PROBE_APPDIR", "")
	port, _ := strconv.Atoi(env("PROBE_PORT", "0"))
	if appDir == "" || port == 0 {
		fmt.Fprintln(os.Stderr, "PROBE_APPDIR and PROBE_PORT are required")
		os.Exit(2)
	}
	matrix(appDir, port)
	fmt.Fprintln(out, "SANDBOX-PROBE DONE")
	http.HandleFunc("/smtp", func(w http.ResponseWriter, r *http.Request) {
		out = os.Stdout
		smtp()
		fmt.Fprintln(out, "SANDBOX-PROBE SMTP-AGAIN DONE")
		fmt.Fprintln(w, "ok")
	})
	if err := http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port), nil); err != nil {
		fmt.Fprintln(out, "SANDBOX-PROBE LISTEN-FAILED", err)
		os.Exit(1)
	}
}
