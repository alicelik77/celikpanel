package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G32): the deploy key is made on the server, root's and 0600;
// only its public half is answered; an existing key is replaced only when
// asked; a deploy refuses what it cannot do before starting anything.
func TestGitDeployKeyStaysOnTheServer(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	savedDir, savedKeygen := gitDeployKeyDir, gitKeygen
	gitDeployKeyDir = dir
	t.Cleanup(func() { gitDeployKeyDir, gitKeygen = savedDir, savedKeygen })
	gitKeygen = func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "-l" {
			return []byte("256 SHA256:abc celikpanel-deploy@shop.example (ED25519)\n"), nil
		}
		file := args[len(args)-1]
		_ = os.WriteFile(file, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nsecret\n"), 0o600)
		_ = os.WriteFile(file+".pub", []byte("ssh-ed25519 AAAAC3Nza celikpanel-deploy@shop.example\n"), 0o644)
		return nil, nil
	}
	call := func(replace bool) transport.GitDeployKeyResponse {
		var resp transport.GitDeployKeyResponse
		_ = (&Agent{}).GitDeployKeyV1(&transport.GitDeployKeyRequest{SiteID: 7, Domain: "shop.example", Replace: replace}, &resp)
		return resp
	}
	resp := call(false)
	if resp.Error != "" || !strings.HasPrefix(resp.PublicKey, "ssh-ed25519 ") || strings.Contains(resp.PublicKey, "PRIVATE") || resp.Fingerprint != "SHA256:abc" {
		t.Fatalf("first key: %+v", resp)
	}
	if info, err := os.Stat(gitDeployKeyPath(7)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private key file: %v %v", info, err)
	}
	if again := call(false); again.Reason != "key_exists" || again.PublicKey != "" {
		t.Fatalf("second key without replace: %+v", again)
	}
	if replaced := call(true); replaced.Error != "" || replaced.PublicKey == "" {
		t.Fatalf("replace: %+v", replaced)
	}

	// A deploy of an SSH repository without git or a key never starts.
	savedGit := gitBinaryPath
	gitBinaryPath = filepath.Join(dir, "no-git")
	t.Cleanup(func() { gitBinaryPath = savedGit })
	var deploy transport.GitDeployResponse
	_ = (&Agent{}).GitDeployV1(&transport.GitDeployRequest{RequestID: "git-request-0001", SiteID: 8, RepoURL: "git@github.com:a/b.git", Branch: "main"}, &deploy)
	if deploy.Reason != "git_missing" || deploy.Phase != "" {
		t.Fatalf("no git: %+v", deploy)
	}
}
