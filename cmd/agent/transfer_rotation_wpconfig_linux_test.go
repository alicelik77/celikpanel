//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func installWordPressConfigRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	docRoot := filepath.Join(root, "7", "sites", "9", "public_html")
	if err := os.MkdirAll(docRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "7", "sites"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldRoot, oldOwner, oldUID := wordpressConfigRoot, wordpressConfigRootOwner, wordpressConfigOwnerUID
	wordpressConfigRoot = func() string { return root }
	wordpressConfigRootOwner = uint32(os.Geteuid())
	wordpressConfigOwnerUID = func(string) (int, error) { return os.Geteuid(), nil }
	t.Cleanup(func() {
		wordpressConfigRoot, wordpressConfigRootOwner, wordpressConfigOwnerUID = oldRoot, oldOwner, oldUID
	})
	return docRoot
}

func wordpressApp(required bool) *transport.DatabaseUserApp {
	return &transport.DatabaseUserApp{Kind: "wordpress", SubscriptionID: 7, DomainID: 9, SiteUsername: "shop_test", DBName: "shop_wp", Required: required}
}

func TestRotateDatabaseUserPasswordWritesWordPressConfig(t *testing.T) {
	docRoot := installWordPressConfigRoot(t)
	path := filepath.Join(docRoot, "wp-config.php")
	if err := os.WriteFile(path, []byte(installedWPConfig), 0o640); err != nil {
		t.Fatal(err)
	}
	engine := &rotationEngine{present: true}
	installRotationEngine(t, "mysql", engine)
	secret := "Zq7xN2kLp9Vt4RmW"
	response := rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{
		User: transport.SuspensionDatabaseUser{Engine: "mysql", Username: "shop_user"}, NewPassword: secret, App: wordpressApp(true),
	})
	if !response.Applied || !response.AppUpdated {
		t.Fatalf("response %+v", response)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "define( 'DB_PASSWORD', '"+secret+"' );") || !strings.Contains(string(content), "$table_prefix = 'wp_';") {
		t.Fatalf("file:\n%s", content)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", info.Mode())
	}
	entries, _ := os.ReadDir(docRoot)
	if len(entries) != 1 {
		t.Fatalf("a temporary file stayed: %v", entries)
	}
}

func TestRotateDatabaseUserPasswordKeepsASiteWhoseConfigChanged(t *testing.T) {
	docRoot := installWordPressConfigRoot(t)
	path := filepath.Join(docRoot, "wp-config.php")
	owned := strings.Replace(installedWPConfig, "'shop_user'", "'someone_else'", 1)
	if err := os.WriteFile(path, []byte(owned), 0o640); err != nil {
		t.Fatal(err)
	}
	engine := &rotationEngine{present: true}
	installRotationEngine(t, "mysql", engine)
	user := transport.SuspensionDatabaseUser{Engine: "mysql", Username: "shop_user"}
	// Required (a rotation): nothing changes, the site keeps working.
	response := rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{User: user, NewPassword: "Zq7xN2kLp9Vt4RmW", App: wordpressApp(true)})
	if response.Applied || !response.AppConflict || response.PasswordChanged || len(engine.statements) != 0 {
		t.Fatalf("required: %+v %q", response, engine.statements)
	}
	// Not required (the owner sets a password): the engine changes, the file stays.
	response = rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{User: user, NewPassword: "Zq7xN2kLp9Vt4RmW", App: wordpressApp(false)})
	if !response.Applied || response.AppUpdated || response.AppDetail == "" {
		t.Fatalf("not required: %+v", response)
	}
	if content, _ := os.ReadFile(path); string(content) != owned {
		t.Fatal("the owner's file was changed")
	}
	// A symbolic link is never followed.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere.php")
	if err := os.WriteFile(target, []byte(installedWPConfig), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	engine.statements = nil
	response = rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{User: user, NewPassword: "Zq7xN2kLp9Vt4RmW", App: wordpressApp(true)})
	if !response.AppConflict || len(engine.statements) != 0 {
		t.Fatalf("symlink: %+v", response)
	}
	if content, _ := os.ReadFile(target); string(content) != installedWPConfig {
		t.Fatal("the link target was changed")
	}
}
