package nodesite

import (
	"strings"
	"testing"
)

// alpha.128 (PLESK-GAP G32): Git deploy addresses, steps and the unit.

func TestGitURLsAreHTTPSOrSSHWithoutSecrets(t *testing.T) {
	good := map[string]bool{
		"https://github.com/acme/site.git":         false,
		"https://git.example.org:8443/team/app":    false,
		"git@github.com:acme/site.git":             true,
		"ssh://git@git.example.org:2222/acme/site": true,
	}
	for raw, wantSSH := range good {
		ssh, err := ParseGitURL(raw)
		if err != nil || ssh != wantSSH {
			t.Errorf("%s: ssh=%v err=%v", raw, ssh, err)
		}
	}
	for _, raw := range []string{
		"", "http://github.com/acme/site.git", "file:///etc", "/srv/repo", "ext::sh -c id",
		"https://user:token@github.com/acme/site.git", "https://github.com/acme/site.git?x=1",
		"-uhttps://github.com/acme/site.git", "https://github.com/../etc", "git@github.com:../x",
		"ssh://git:pw@host/x", "https://github.com/acme/si te.git", "https://github.com/acme/$(id).git",
	} {
		if _, err := ParseGitURL(raw); err == nil {
			t.Errorf("%q accepted", raw)
		}
	}
	for branch, ok := range map[string]bool{"main": true, "release/1.2": true, "-x": false, "a..b": false, "x.lock": false, "a b": false, "": false} {
		if ValidGitBranch(branch) != ok {
			t.Errorf("branch %q", branch)
		}
	}
}

func TestGitDeployUnitIsSandboxedAndHandsTheKeyAsACredential(t *testing.T) {
	caps := systems[3].caps // debian-13, systemd 257
	p := GitDeployParams{SiteID: 12, Domain: "shop.example.com", User: "shop_example_com", Group: "shop_example_com", AppDir: docRoot,
		RepoURL: "git@github.com:acme/site.git", Branch: "main", KeyFile: "/etc/celikpanel-runtime/gitkey-12", Caps: caps}
	args, err := GitDeployRunArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\n")
	for _, want := range []string{
		"--unit=celikgit-12.service", "--uid=shop_example_com", "--setenv=GIT_ALLOW_PROTOCOL=https:ssh",
		"--property=LoadCredential=deploykey:/etc/celikpanel-runtime/gitkey-12",
		"-i /run/credentials/celikgit-12.service/deploykey",
		"--property=ExecStartPost=/usr/bin/git -C " + docRoot + " fetch -q --depth 1 -- git@github.com:acme/site.git main",
		"--property=ExecStartPost=/usr/bin/git -C " + docRoot + " reset -q --hard FETCH_HEAD",
		"--property=ProtectSystem=strict",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	if args[len(args)-6] != "--" || args[len(args)-5] != GitBinary || args[len(args)-1] != "-q" {
		t.Fatalf("first step: %v", args[len(args)-6:])
	}
	// An https repository hands no key over.
	p.RepoURL, p.KeyFile = "https://github.com/acme/site.git", ""
	args, _ = GitDeployRunArgs(p)
	if strings.Contains(strings.Join(args, " "), "LoadCredential") {
		t.Fatal("an https deploy loads a key")
	}
	// An SSH repository without a key, or on a systemd that cannot hand it over, is refused.
	p.RepoURL = "git@github.com:acme/site.git"
	if _, err := GitDeployRunArgs(p); err == nil {
		t.Fatal("an SSH deploy without a key was accepted")
	}
	p.KeyFile, p.Caps = "/etc/celikpanel-runtime/gitkey-12", systems[0].caps // ubuntu 22.04, systemd 249
	if _, err := GitDeployRunArgs(p); err == nil || !strings.Contains(err.Error(), "https://") {
		t.Fatalf("systemd 249: %v", err)
	}
	// A Node.js build follows the files.
	p.RepoURL, p.KeyFile, p.Caps, p.Build, p.NodeVersion = "https://github.com/acme/site.git", "", caps, true, "22.11.0"
	steps, err := GitDeploySteps(p)
	if err != nil || len(steps) != 5 || steps[4][len(steps[4])-2] != "build" {
		t.Fatalf("build steps: %v %v", steps, err)
	}
}
