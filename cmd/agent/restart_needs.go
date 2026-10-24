package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Patched is not restarted (alpha.107, D-038 §10). When a library such as
// OpenSSL is patched, the services that use it keep running the old code
// until they are restarted; a kernel update needs a reboot. This reads, with
// the operating system's own tool where it is installed (needrestart), or
// otherwise the processes' own memory maps, which services still run code an
// update replaced, and whether the server waits for a reboot. It restarts
// nothing: the owner chooses when, through the component's own restart.
//
// Yamalanmış, yeniden başlatılmış demek değildir: hangi servislerin eski
// kodu çalıştırdığı ve sunucunun yeniden başlatma bekleyip beklemediği
// salt okunur okunur; hiçbir şey kendiliğinden yeniden başlatılmaz.

var (
	rebootRequiredPath     = "/var/run/reboot-required"
	rebootRequiredPkgsPath = "/var/run/reboot-required.pkgs"
	procRoot               = "/proc"
	kernelModulesRoots     = []string{"/usr/lib/modules", "/lib/modules"}
	needrestartPath        = "/usr/sbin/needrestart"
	restartNeedsCommand    = runPackageSimulationCommand
)

const (
	restartNeedsBound       = 60 * time.Second
	restartNeedsProcessCap  = 5000
	restartNeedsMapsCap     = 2 << 20
	restartNeedsServicesCap = 100
	restartNeedsLibsCap     = 4
)

// parseNeedrestartBatch reads `needrestart -b`'s batch lines.
func parseNeedrestartBatch(out string) (services []string, current, expected string, kernelState string) {
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "NEEDRESTART-SVC":
			services = append(services, value)
		case "NEEDRESTART-KCUR":
			current = value
		case "NEEDRESTART-KEXP":
			expected = value
		case "NEEDRESTART-KSTA":
			kernelState = value
		}
	}
	return services, current, expected, kernelState
}

var (
	procPIDName  = regexp.MustCompile(`^[0-9]+$`)
	cgroupUnit   = regexp.MustCompile(`/([A-Za-z0-9@._\\-]+\.service)$`)
	deletedLib   = regexp.MustCompile(`\s(/\S+) \(deleted\)$`)
	sharedObject = regexp.MustCompile(`\.so(\.[0-9.]+)?$`)
)

// deletedLibrariesInMaps returns the replaced shared libraries one process
// still maps.
func deletedLibrariesInMaps(r io.Reader) []string {
	seen := map[string]bool{}
	var libs []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		match := deletedLib.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		path := match[1]
		if strings.HasPrefix(path, "/memfd:") || strings.HasPrefix(path, "/dev/") || strings.HasPrefix(path, "/SYSV") ||
			strings.HasPrefix(path, "/tmp/") || strings.HasPrefix(path, "/run/") || !sharedObject.MatchString(path) {
			continue
		}
		if !seen[path] {
			seen[path] = true
			libs = append(libs, path)
		}
	}
	return libs
}

func unitOfProcess(pid string) string {
	data, err := os.ReadFile(filepath.Join(procRoot, pid, "cgroup"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if match := cgroupUnit.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			return match[1]
		}
	}
	return ""
}

// servicesWithDeletedLibraries reads every process's memory map, bounded.
func servicesWithDeletedLibraries(ctx context.Context) (map[string][]string, bool) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, false
	}
	units := map[string][]string{}
	read := 0
	truncated := false
	for _, entry := range entries {
		if !procPIDName.MatchString(entry.Name()) {
			continue
		}
		if ctx.Err() != nil || read >= restartNeedsProcessCap {
			truncated = true
			break
		}
		read++
		file, err := os.Open(filepath.Join(procRoot, entry.Name(), "maps"))
		if err != nil {
			continue
		}
		libs := deletedLibrariesInMaps(io.LimitReader(file, restartNeedsMapsCap))
		file.Close()
		if len(libs) == 0 {
			continue
		}
		unit := unitOfProcess(entry.Name())
		if unit == "" {
			continue
		}
		for _, lib := range libs {
			if len(units[unit]) < restartNeedsLibsCap && !containsString(units[unit], lib) {
				units[unit] = append(units[unit], lib)
			}
		}
	}
	return units, truncated
}

// restartInterruption says what restarting a unit interrupts.
func restartInterruption(unit string, svc *core.ManagedService) string {
	name := strings.TrimSuffix(unit, ".service")
	switch {
	case strings.HasPrefix(name, "celikpanel"):
		return "panel"
	case name == "ssh" || name == "sshd":
		return "ssh"
	case svc == nil:
		return "other"
	case svc.ID == "php-fpm":
		return "php"
	}
	switch svc.Category {
	case "web":
		return "web"
	case "database", "cache":
		return "database"
	case "email":
		return "mail"
	case "dns":
		return "dns"
	}
	return "other"
}

func restartNeedsService(unit string, libs []string) transport.RestartNeedsService {
	svc := core.ServiceForUnit(strings.TrimSuffix(unit, ".service"))
	item := transport.RestartNeedsService{Unit: unit, Interruption: restartInterruption(unit, svc), Libraries: libs}
	if svc != nil {
		item.Component = svc.ID
	}
	return item
}

func restartKernelRelease() string {
	data, err := os.ReadFile(filepath.Join(procRoot, "sys", "kernel", "osrelease"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func kernelModulesPresent(release string) bool {
	for _, root := range kernelModulesRoots {
		if info, err := os.Stat(filepath.Join(root, release)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// restartNeeds is the read, through its seams.
func restartNeeds(ctx context.Context) transport.RestartNeedsResponse {
	resp := transport.RestartNeedsResponse{Services: []transport.RestartNeedsService{}}
	resp.RunningKernel = restartKernelRelease()
	if _, err := os.Stat(rebootRequiredPath); err == nil {
		resp.RebootKnown, resp.RebootRequired, resp.RebootReason = true, true, "reboot_required_file"
		if data, err := os.ReadFile(rebootRequiredPkgsPath); err == nil && len(data) < 64<<10 {
			for _, line := range strings.Split(string(data), "\n") {
				if name := strings.TrimSpace(line); name != "" && !containsString(resp.RebootPackages, name) {
					resp.RebootPackages = append(resp.RebootPackages, name)
				}
			}
		}
	} else if os.IsNotExist(err) && pathExists(filepath.Dir(rebootRequiredPath)) {
		resp.RebootKnown = true
	}
	if resp.RunningKernel != "" && !resp.RebootRequired {
		if !kernelModulesPresent(resp.RunningKernel) {
			// The running kernel's modules were removed by an upgrade: the
			// installed kernel is another one (Arch replaces it in place).
			resp.RebootKnown, resp.RebootRequired, resp.RebootReason = true, true, "kernel_modules_gone"
		} else {
			resp.RebootKnown = true
		}
	}
	if pathExists(needrestartPath) {
		out, err := restartNeedsCommand(ctx, packageCommandEnv(), needrestartPath, "-b", "-r", "l")
		if err == nil {
			units, current, expected, kernelState := parseNeedrestartBatch(string(out))
			resp.Source, resp.ServicesKnown = "needrestart", true
			for _, unit := range units {
				if len(resp.Services) < restartNeedsServicesCap {
					resp.Services = append(resp.Services, restartNeedsService(unit, nil))
				} else {
					resp.Truncated = true
				}
			}
			if kernelState == "2" || kernelState == "3" {
				resp.RebootKnown, resp.RebootRequired = true, true
				if resp.RebootReason == "" {
					resp.RebootReason = "needrestart_kernel"
				}
				resp.ExpectedKernel = expected
			}
			if current != "" {
				resp.RunningKernel = current
			}
		}
	}
	if !resp.ServicesKnown {
		units, truncated := servicesWithDeletedLibraries(ctx)
		if units != nil {
			resp.Source, resp.ServicesKnown, resp.Truncated = "maps", true, truncated
			names := make([]string, 0, len(units))
			for unit := range units {
				names = append(names, unit)
			}
			sort.Strings(names)
			for _, unit := range names {
				if len(resp.Services) < restartNeedsServicesCap {
					resp.Services = append(resp.Services, restartNeedsService(unit, units[unit]))
				} else {
					resp.Truncated = true
				}
			}
		}
	}
	resp.NeedrestartAutoRestart = needrestartRestartsAutomatically()
	return resp
}

// RestartNeedsV1 answers the restarts and reboot card (alpha.107). Read
// only and bounded.
func (a *Agent) RestartNeedsV1(_ *transport.Empty, resp *transport.RestartNeedsResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), restartNeedsBound)
	defer cancel()
	*resp = restartNeeds(ctx)
	return nil
}
