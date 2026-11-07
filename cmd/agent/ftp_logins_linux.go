//go:build linux

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alicelik/celikpanel/internal/fileaccess"
)

// killFTPLoginSessions ends every open session of a login. A login shares
// the site's uid, so its processes are found by their root: sshd's
// internal-sftp and vsftpd's session run chrooted in the login's jail. The
// site's own processes (PHP, Node) are never touched.
func killFTPLoginSessions(login string) error {
	if err := fileaccess.ValidateLoginName(login); err != nil {
		return err
	}
	jail := fileaccess.LoginJail(login)
	for round := 0; round < 5; round++ {
		pids, err := processesRootedAt(jail)
		if err != nil {
			return err
		}
		if len(pids) == 0 {
			return nil
		}
		for _, pid := range pids {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("sessions of %s are still running", login)
}

func processesRootedAt(root string) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 || pid == os.Getpid() {
			continue
		}
		target, err := os.Readlink("/proc/" + entry.Name() + "/root")
		if err != nil {
			continue
		}
		if target == root || strings.HasPrefix(target, root+"/") {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// ftpLoginMountPresent reports whether the login's folder is mounted on its
// jail (the host's mount table).
func ftpLoginMountPresent(login string) bool {
	content, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	want := fileaccess.LoginFilesDir(login)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 4 && fields[4] == want {
			return true
		}
	}
	return false
}
