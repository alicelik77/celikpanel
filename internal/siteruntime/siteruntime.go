// Package siteruntime is the site runtime engine (alpha.119,
// docs/RUNTIME-APPS.md): a customer's own program that runs as a sandboxed
// site service behind the site's HTTPS. This release implements the
// "program" kind (an uploaded Go, Rust or any other executable); the "python"
// kind is reserved here and built on the same engine by its own workstream.
//
// Every program is arbitrary code. The boundary is the site sandbox
// (internal/sitesandbox), never the absence of an interpreter: an uploaded
// binary may start Python. So nothing here runs a shell, nothing runs as the
// web server's user, and a host that enforces less than the full sandbox is
// refused unless the administrator accepted the named gaps.
//
// The package renders only native text (the systemd unit, the environment
// file); it never touches the server.
//
// Paket siteruntime, müşterinin kendi programını sitenin HTTPS'i arkasında,
// korumalı bir site hizmeti olarak çalıştırır. Her program rastgele kod
// sayılır; sınır site korumasıdır. Kabuk yok, web sunucusunun kullanıcısı yok;
// tam korumayı sağlamayan sunucu, yönetici adı konmuş açığı kabul etmedikçe
// reddedilir.
package siteruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// Kind is what a runtime app is.
type Kind string

const (
	// KindProgram: an uploaded executable (ELF, or a "#!" script whose
	// interpreter is inside the app folder).
	KindProgram Kind = "program"
	// KindPython: a Python web app (reserved; built by the Python
	// workstream on this engine, migration 064).
	KindPython Kind = "python"
)

// Kinds lists every kind the schema knows, implemented or not.
var Kinds = []Kind{KindProgram, KindPython}

// Known reports whether k is a kind the schema knows.
func Known(k Kind) bool { return k == KindProgram || k == KindPython }

// Implemented reports whether this release can create and deploy apps of k.
// The Python kind's workstream turns its own kind on here.
func Implemented(k Kind) bool { return k == KindProgram }

// Permission is the D-042 permission id of a kind.
func Permission(k Kind) string {
	switch k {
	case KindProgram:
		return "program_apps"
	case KindPython:
		return "python_apps"
	}
	return ""
}

// OfferingProgram is the offering id of uploaded programs.
const OfferingProgram = "runtime:program"

// OfferingID is the offering a kind is gated by. The Python kind is offered
// per distribution Python (component:python:<minor>), which its workstream
// supplies; "" here.
func OfferingID(k Kind) string {
	if k == KindProgram {
		return OfferingProgram
	}
	return ""
}

// KindOfOffering is the kind an offering id names.
func KindOfOffering(id string) (Kind, bool) {
	switch {
	case id == OfferingProgram:
		return KindProgram, true
	case strings.HasPrefix(id, "component:python:"):
		return KindPython, true
	}
	return "", false
}

// SandboxKind is the sandbox kind a runtime kind runs as.
func SandboxKind(k Kind) sitesandbox.Kind {
	if k == KindPython {
		return sitesandbox.KindPython
	}
	return sitesandbox.KindProgram
}

// Layout is where a site's runtime app lives: inside the site's own home,
// beside (not inside) public_html, so its files are the site's, they count
// toward the subscription's disk and nginx never serves them. The customer
// uploads into bin/ with the file manager or SFTP.
//
//	<site home>/app/
//	  bin/    the uploaded program (the entry and anything it needs)
//	  data/   the program's own data (its HOME)
//	  config/ the program's own configuration files
//
// The environment file is not in this folder: systemd reads it as root before
// the sandbox starts, and a file the site's account controls could be a link
// to a root-only file. It is EnvDir/<unit>.env, root's, 0600.
type Layout struct {
	Root string
}

// LayoutFor derives the layout from database identities only.
func LayoutFor(subscriptionID, domainID int) (Layout, error) {
	home, err := hostingpath.SiteHome(subscriptionID, domainID)
	if err != nil {
		return Layout{}, err
	}
	return Layout{Root: path.Join(home, "app")}, nil
}

func (l Layout) BinDir() string    { return path.Join(l.Root, "bin") }
func (l Layout) DataDir() string   { return path.Join(l.Root, "data") }
func (l Layout) ConfigDir() string { return path.Join(l.Root, "config") }

// EnvDir holds the runtime apps' environment files (root, 0700; each file
// 0600). It is native configuration and stays without CelikPanel.
const EnvDir = "/etc/celikpanel-runtime"

// EnvFile is the environment file of an app's unit.
func EnvFile(appID int) string { return path.Join(EnvDir, UnitName(appID)+".env") }

// UnitName is an app's unit (with ".service").
func UnitName(appID int) string { return fmt.Sprintf("celikrt-%d.service", appID) }

// Ports: every runtime app listens on 127.0.0.1 only, on one port the Panel
// allocates from this range. The service apps use 21000-29999, so the two
// never meet; Node.js sites may choose any port and are checked.
const (
	PortMin = 30000
	PortMax = 34999
)

// AllocatePort takes the lowest port of the range nobody holds.
func AllocatePort(taken map[int]bool) (int, error) {
	for port := PortMin; port <= PortMax; port++ {
		if !taken[port] {
			return port, nil
		}
	}
	return 0, errors.New("no free loopback port is left for runtime apps")
}

// ValidPort reports whether port is in the runtime range.
func ValidPort(port int) bool { return port >= PortMin && port <= PortMax }

// Limits are the administrator's limits of an offering.
type Limits struct {
	MemoryMB   int `json:"memory_mb"`
	CPUPercent int `json:"cpu_percent"`
	Tasks      int `json:"tasks_max"`
}

// DefaultLimits are offered until the administrator chooses others.
func DefaultLimits(k Kind) Limits {
	return Limits{MemoryMB: 512, CPUPercent: 100, Tasks: 256}
}

// Validate checks limits against the sandbox's range.
func (l Limits) Validate() error {
	if l.MemoryMB < 64 || l.MemoryMB > 65536 || l.CPUPercent < 10 || l.CPUPercent > 1600 || l.Tasks < 16 || l.Tasks > 8192 {
		return errors.New("memory must be 64 to 65536 MB, CPU 10 to 1600 percent, processes 16 to 8192")
	}
	return nil
}

// Risk is the risk summary the administrator reads before offering a kind
// and the customer sees before deploying. Each entry is a code the screen
// words in English and Turkish (runtime.risk.<code>).
type Risk struct {
	CanDo   []string `json:"can_do"`
	Blocked []string `json:"blocked"`
	Network []string `json:"network"`
	Remains []string `json:"remains"`
}

// RiskOf is the risk summary of a kind.
func RiskOf(k Kind) Risk {
	return Risk{
		CanDo:   []string{"any_code", "own_folder", "loopback_port", "own_outbound"},
		Blocked: []string{"other_sites", "panel_files", "privileges", "mail_ports", "metadata", "other_ports", "system_interpreters"},
		Network: []string{"https_only", "outbound_open"},
		Remains: []string{"outbound_abuse", "mail_apis", "cpu_memory_share", "reduced_level", "customer_code"},
	}
}

// RiskDigest is the digest of what the administrator agrees to when
// offering: the kind, the risk summary, the default limits and the sandbox
// generation. A release that changes any of them asks again.
func RiskDigest(k Kind) string {
	r := RiskOf(k)
	l := DefaultLimits(k)
	parts := []string{string(k), fmt.Sprint(sitesandbox.Generation), fmt.Sprint(l.MemoryMB, l.CPUPercent, l.Tasks),
		strings.Join(r.CanDo, ","), strings.Join(r.Blocked, ","), strings.Join(r.Network, ","), strings.Join(r.Remains, ",")}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}

// GapsCovered reports whether every gap the host has now was accepted.
func GapsCovered(current []sitesandbox.Gap, accepted []string) bool {
	ok := map[string]bool{}
	for _, g := range accepted {
		ok[g] = true
	}
	for _, g := range current {
		if !ok[string(g)] {
			return false
		}
	}
	return true
}

// NormalizeGaps sorts and de-duplicates gap names and refuses an unknown one.
func NormalizeGaps(gaps []string) ([]string, error) {
	known := map[string]bool{
		string(sitesandbox.GapSystemdOld): true, string(sitesandbox.GapSystemdUnknown): true,
		string(sitesandbox.GapBPF): true, string(sitesandbox.GapCgroupV2): true, string(sitesandbox.GapNFTCgroup): true,
	}
	seen := map[string]bool{}
	out := []string{}
	for _, g := range gaps {
		if !known[g] {
			return nil, fmt.Errorf("unknown sandbox gap %q", g)
		}
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	sort.Strings(out)
	return out, nil
}
