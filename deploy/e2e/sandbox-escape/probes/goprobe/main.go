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
//
// With -serve it runs as a real CelikPanel runtime app (panel-runtime.sh,
// docs/RUNTIME-APPS.md): it takes the port from -port (the "{port}"
// argument) or $PORT, the app folder from -appdir or its working folder,
// runs the matrix once, then answers 200 on / (the deploy's health step)
// and /smtp on 127.0.0.1:<port>.
package main

import (
	"flag"
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

// tracePeers is item 5: ptrace and process_vm_readv of another process.
// With PrivatePIDs (systemd 257+) the probe is PID 1 of its own namespace
// and its parent PID is 0, so PID 1 and the parent leave nothing to try
// (alpha.119 lab round 1: item 5 had no result). The probe therefore tries:
//
//   - a child of its own (the same account, inside the namespace): only the
//     system-call filter (~@debug, process_vm_readv) stands in the way;
//   - PROBE_TARGET_PID, a host PID outside its namespace that the runner
//     passes (a PHP-FPM worker, journald): in the probe's namespace the
//     number names no process (ESRCH) or the attempt is refused (EPERM).
//
// Every refusal is BLOCKED; a success is ESCAPED.
func tracePeers() {
	self := os.Getpid()
	type target struct {
		pid  int
		name string
	}
	var targets []target
	for _, pid := range []int{1, os.Getppid()} {
		if pid > 0 && pid != self {
			targets = append(targets, target{pid, strconv.Itoa(pid)})
		}
	}
	child := 0
	if pid, err := syscall.ForkExec("/proc/self/exe", []string{"goprobe"}, &syscall.ProcAttr{Env: []string{"PROBE_MODE=sleep"}}); err == nil {
		child = pid
		targets = append(targets, target{pid, "own-child"})
	} else {
		skip(5, "own-child", "no child could be started: "+err.Error())
	}
	if v := os.Getenv("PROBE_TARGET_PID"); v != "" {
		pid, err := strconv.Atoi(v)
		switch {
		case err != nil || pid <= 0:
			skip(5, "outside", "PROBE_TARGET_PID is not a PID: "+v)
		case pid == self || pid == child:
			skip(5, "outside", "host PID "+v+" is this probe's own number in its namespace; pass another")
		default:
			targets = append(targets, target{pid, "outside-" + v})
		}
	} else if child == 0 {
		skip(5, "outside", "no PROBE_TARGET_PID and no child: nothing to try")
	}
	for _, t := range targets {
		err := syscall.PtraceAttach(t.pid)
		if err == nil {
			_ = syscall.PtraceDetach(t.pid)
		}
		attempt(5, "ptrace-"+t.name, err)
		var buf [8]byte
		local := iovec{base: uintptr(unsafe.Pointer(&buf[0])), len: 8}
		remote := iovec{base: 0x400000, len: 8}
		_, _, errno := syscall.Syscall6(310, uintptr(t.pid), uintptr(unsafe.Pointer(&local)), 1, uintptr(unsafe.Pointer(&remote)), 1, 0) // process_vm_readv (amd64)
		if errno == 0 {
			attempt(5, "process_vm_readv-"+t.name, nil)
		} else {
			attempt(5, "process_vm_readv-"+t.name, errno)
		}
	}
	if child > 0 {
		_ = syscall.Kill(child, syscall.SIGKILL)
		var status syscall.WaitStatus
		_, _ = syscall.Wait4(child, &status, 0, nil)
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
	tracePeers()
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

// serveApp is the -serve mode: the probe as an uploaded program deployed
// through the panel. Every line goes to stdout, so the panel's log view
// (GET .../runtime-apps/logs) carries the SANDBOX-PROBE lines.
func serveApp(port int, appDir string) {
	if port == 0 {
		port, _ = strconv.Atoi(os.Getenv("PORT"))
	}
	if appDir == "" {
		appDir, _ = os.Getwd()
	}
	if port == 0 || appDir == "" || appDir == "/" {
		fmt.Fprintln(out, "SANDBOX-PROBE SERVE-FAILED the port (-port or $PORT) and the app folder (-appdir or the working folder) are required")
		os.Exit(2)
	}
	// The run marker lets panel-runtime.sh read only this start's lines.
	fmt.Fprintf(out, "SANDBOX-PROBE-RUN %s\n", env("PROBE_RUN", "none"))
	fmt.Fprintf(out, "SANDBOX-PROBE-ENV port=%d env_port=%s host=%s home=%s path=%s appdir=%s uid=%d\n",
		port, os.Getenv("PORT"), os.Getenv("HOST"), os.Getenv("HOME"), os.Getenv("PATH"), appDir, os.Getuid())
	// A secret environment value, printed on purpose: the panel must hide
	// it in the log it shows (panel-runtime.sh checks that).
	if secret := os.Getenv("PROBE_SECRET"); secret != "" {
		fmt.Fprintf(out, "SANDBOX-PROBE-ENVECHO %s\n", secret)
	}
	matrix(appDir, port)
	fmt.Fprintln(out, "SANDBOX-PROBE DONE")
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "CelikPanel sandbox probe: lab only")
	})
	mux.HandleFunc("/smtp", func(w http.ResponseWriter, r *http.Request) {
		smtp()
		fmt.Fprintln(out, "SANDBOX-PROBE SMTP-AGAIN DONE")
		fmt.Fprintln(w, "ok")
	})
	// Loopback only, whatever HOST says: the deploy proof would stop any
	// other listener anyway.
	if err := http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port), mux); err != nil {
		fmt.Fprintln(out, "SANDBOX-PROBE LISTEN-FAILED", err)
		os.Exit(1)
	}
}

func main() {
	serve := flag.Bool("serve", false, "run as a CelikPanel runtime app (panel-runtime.sh)")
	servePort := flag.Int("port", 0, "with -serve: the loopback port (default $PORT)")
	serveDir := flag.String("appdir", "", "with -serve: the app folder (default: the working folder)")
	flag.Parse()
	if *serve {
		serveApp(*servePort, *serveDir)
		return
	}
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
