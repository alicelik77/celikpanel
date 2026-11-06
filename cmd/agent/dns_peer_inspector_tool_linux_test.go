//go:build linux

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

func inspectorDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

type inspectorToolFixture struct {
	host      dnsPeerInspectorToolHost
	installed string
	current   string // this release's copy
	older     string // an earlier release tree's copy
}

const (
	inspectorNew   = "\x7fELF check tool of this release"
	inspectorOld   = "\x7fELF check tool of an earlier release"
	inspectorList  = "\x7fELF check tool known only by its listed hash"
	inspectorOwner = "\x7fELF the owner's own build"
)

// newInspectorToolFixture lays out two verified release trees (the running
// Agent is the newer one's), the installed check tool and its channel.
func newInspectorToolFixture(t *testing.T) *inspectorToolFixture {
	t.Helper()
	root := t.TempDir()
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	releases := filepath.Join(root, "releases")
	newTree := filepath.Join(releases, "0123456789ab-00112233445566778899aabb")
	oldTree := filepath.Join(releases, "ba9876543210-00112233445566778899aabb")
	write(filepath.Join(newTree, "bin", "agent"), "agent of this release", 0o755)
	write(filepath.Join(oldTree, "bin", "agent"), "agent of an earlier one", 0o755)
	write(filepath.Join(newTree, "dns-owner-tools", "bind-peer-inspect"), inspectorNew, 0o755)
	write(filepath.Join(oldTree, "dns-owner-tools", "bind-peer-inspect"), inspectorOld, 0o755)
	self := filepath.Join(root, "self-agent")
	write(self, "agent of this release", 0o755)
	libexec := filepath.Join(root, "libexec")
	write(filepath.Join(libexec, "celikpanel-bind-peer-inspect-wrapper"), "#!/bin/sh\n", 0o755)
	f := &inspectorToolFixture{
		installed: filepath.Join(libexec, "celikpanel-bind-peer-inspect"),
		current:   filepath.Join(newTree, "dns-owner-tools", "bind-peer-inspect"),
		older:     filepath.Join(oldTree, "dns-owner-tools", "bind-peer-inspect"),
	}
	f.host = dnsPeerInspectorToolHost{
		releasesRoot: releases,
		ownerTools:   filepath.Join(root, "opt-dns-owner-tools"),
		selfExe:      self,
		tools: []dnsPeerInspectorToolPaths{{engine: "bind", installed: f.installed,
			wrapper: filepath.Join(libexec, "celikpanel-bind-peer-inspect-wrapper"), released: "bind-peer-inspect"}},
		shipped:   map[string]map[string]bool{"bind": {inspectorDigest(inspectorList): true}},
		trusted:   func(os.FileInfo) bool { return true },
		chown:     func(*os.File) error { return nil },
		stateDir:  filepath.Join(root, "state"),
		writeFile: func(path string, raw []byte, mode os.FileMode) error { return os.WriteFile(path, raw, mode) },
	}
	return f
}

func (f *inspectorToolFixture) install(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(f.installed, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (f *inspectorToolFixture) installedContent(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(f.installed)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// An earlier release's check tool (the owner's backup on 6 Nov 2026) is
// replaced when the Agent starts after the update; the replaced program is
// kept and the replacement recorded.
func TestDNSCheckToolShippedVersionIsReplacedAtStartAndKept(t *testing.T) {
	for _, previous := range []string{inspectorOld, inspectorList} {
		f := newInspectorToolFixture(t)
		f.install(t, previous)
		if state := f.host.inspect(f.host.tools[0]); state.State != transport.DNSPeerInspectorToolOutdated || state.ReleasePath != f.current {
			t.Fatalf("before: %+v", state)
		}
		results := f.host.reconcile(time.Unix(1_800_000_000, 0))
		if len(results) != 1 || results[0].State != transport.DNSPeerInspectorToolCurrent {
			t.Fatalf("after: %+v", results)
		}
		info, err := os.Stat(f.installed)
		if err != nil || info.Mode().Perm() != 0o755 || f.installedContent(t) != inspectorNew {
			t.Fatalf("installed: %v %v", info, err)
		}
		kept, err := os.ReadFile(filepath.Join(f.host.stateDir, "bind-peer-inspect-"+inspectorDigest(previous)))
		if err != nil || string(kept) != previous {
			t.Fatalf("the replaced program was not kept: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(f.host.stateDir, "dns-check-tool-refresh-v1.json"))
		var record dnsPeerInspectorToolRecordV1
		if err != nil || json.Unmarshal(raw, &record) != nil || len(record.Entries) != 1 ||
			record.Entries[0].FromSHA256 != inspectorDigest(previous) || record.Entries[0].ToSHA256 != inspectorDigest(inspectorNew) {
			t.Fatalf("record: %s %v", raw, err)
		}
		// A second start changes nothing.
		if again := f.host.reconcile(time.Unix(1_800_000_100, 0)); again[0].State != transport.DNSPeerInspectorToolCurrent {
			t.Fatalf("second start: %+v", again)
		}
		if raw2, _ := os.ReadFile(filepath.Join(f.host.stateDir, "dns-check-tool-refresh-v1.json")); string(raw2) != string(raw) {
			t.Fatal("a second start recorded another replacement")
		}
		leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(f.installed), ".*update-*"))
		if len(leftovers) != 0 {
			t.Fatalf("temporary files left: %v", leftovers)
		}
	}
}

// A rollback starts the earlier release's Agent: the same rule puts that
// release's copy back (the newer copy is a shipped program too).
func TestDNSCheckToolRollbackPutsTheEarlierReleaseCopyBack(t *testing.T) {
	f := newInspectorToolFixture(t)
	f.install(t, inspectorOld)
	f.host.reconcile(time.Unix(1_800_000_000, 0))
	if f.installedContent(t) != inspectorNew {
		t.Fatal("update did not put this release's copy in place")
	}
	// Roll back: the running Agent is now the earlier release's.
	if err := os.WriteFile(f.host.selfExe, []byte("agent of an earlier one"), 0o755); err != nil {
		t.Fatal(err)
	}
	results := f.host.reconcile(time.Unix(1_800_000_200, 0))
	if len(results) != 1 || results[0].State != transport.DNSPeerInspectorToolCurrent || results[0].ReleasePath != f.older {
		t.Fatalf("after rollback: %+v", results)
	}
	if f.installedContent(t) != inspectorOld {
		t.Fatal("rollback did not put the earlier release's check tool back")
	}
}

// An owner's own file is kept; the status names the one command.
func TestDNSCheckToolChangedByTheOwnerIsKept(t *testing.T) {
	f := newInspectorToolFixture(t)
	f.install(t, inspectorOwner)
	results := f.host.reconcile(time.Unix(1_800_000_000, 0))
	want := "sudo install -o root -g root -m 0755 " + f.current + " " + f.installed
	if len(results) != 1 || results[0].State != transport.DNSPeerInspectorToolOwnerChanged || results[0].Command != want {
		t.Fatalf("owner file: %+v", results)
	}
	if f.installedContent(t) != inspectorOwner {
		t.Fatal("the owner's check tool was replaced")
	}
	if _, err := os.Stat(filepath.Join(f.host.stateDir, "dns-check-tool-refresh-v1.json")); !os.IsNotExist(err) {
		t.Fatal("a replacement was recorded for the owner's file")
	}
	if status := f.host.status(); len(status.Tools) != 1 || status.Tools[0].Command != want {
		t.Fatalf("status: %+v", status)
	}
	// A symbolic link is the owner's too.
	if err := os.Remove(f.installed); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.current, f.installed); err != nil {
		t.Fatal(err)
	}
	if results := f.host.reconcile(time.Unix(1_800_000_000, 0)); results[0].State != transport.DNSPeerInspectorToolOwnerChanged {
		t.Fatalf("symlink: %+v", results)
	}
	if target, err := os.Readlink(f.installed); err != nil || target != f.current {
		t.Fatal("the owner's symbolic link was replaced")
	}
}

// A missing check tool on a set-up channel is installed; no channel, no file.
func TestDNSCheckToolMissingIsInstalledOnlyForASetUpChannel(t *testing.T) {
	f := newInspectorToolFixture(t)
	results := f.host.reconcile(time.Unix(1_800_000_000, 0))
	if len(results) != 1 || results[0].State != transport.DNSPeerInspectorToolCurrent || f.installedContent(t) != inspectorNew {
		t.Fatalf("missing: %+v", results)
	}
	g := newInspectorToolFixture(t)
	if err := os.Remove(g.host.tools[0].wrapper); err != nil {
		t.Fatal(err)
	}
	if results := g.host.reconcile(time.Unix(1_800_000_000, 0)); len(results) != 0 {
		t.Fatalf("no channel: %+v", results)
	}
	if _, err := os.Stat(g.installed); !os.IsNotExist(err) {
		t.Fatal("a check tool was installed without a channel")
	}
}

// Without this release's copy nothing is written.
func TestDNSCheckToolWithoutAReleaseCopyChangesNothing(t *testing.T) {
	f := newInspectorToolFixture(t)
	f.install(t, inspectorOld)
	if err := os.WriteFile(f.host.selfExe, []byte("an agent of no kept release"), 0o755); err != nil {
		t.Fatal(err)
	}
	results := f.host.reconcile(time.Unix(1_800_000_000, 0))
	if len(results) != 1 || results[0].State != transport.DNSPeerInspectorToolReleaseCopyUnavailable || f.installedContent(t) != inspectorOld {
		t.Fatalf("no release copy: %+v", results)
	}
	if !strings.HasPrefix(f.installedContent(t), "\x7fELF") {
		t.Fatal("unexpected content")
	}
}
