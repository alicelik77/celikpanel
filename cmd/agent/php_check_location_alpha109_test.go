//go:build linux

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// alpha.109 (Frankfurt): a configuration an older release wrote has no PHP
// check location; it is added before a proof, in CelikPanel's own form only,
// keeping the listen address, roots and sockets; nginx refusing it puts the
// previous file back; a file the owner changed is refused and stays as it is.

type checkLocationRecorder struct {
	tests, reloads int
	testErr        error
	reloadErr      error
}

func checkLocationOps(rec *checkLocationRecorder) phpToolBindOps {
	return phpToolBindOps{
		snapshotConf: func(path string) (mailFileSnapshot, error) {
			data, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				return mailFileSnapshot{path: path}, nil
			}
			if err != nil {
				return mailFileSnapshot{}, err
			}
			return mailFileSnapshot{path: path, exists: true, content: data, mode: 0o600}, nil
		},
		writeConf: func(path string, content []byte) error { return os.WriteFile(path, content, 0o600) },
		restoreConf: func(snapshot mailFileSnapshot) error {
			if !snapshot.exists {
				return os.Remove(snapshot.path)
			}
			return os.WriteFile(snapshot.path, snapshot.content, snapshot.mode)
		},
		nginxTest:   func(context.Context) error { rec.tests++; return rec.testErr },
		nginxReload: func(context.Context) error { rec.reloads++; return rec.reloadErr },
	}
}

func pointPHPToolConfs(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	webmail, dbtools := phpToolWebmailConfPath, phpToolDBToolsConfPath
	t.Cleanup(func() { phpToolWebmailConfPath, phpToolDBToolsConfPath = webmail, dbtools })
	phpToolWebmailConfPath = filepath.Join(dir, "celikpanel-webmail.conf")
	phpToolDBToolsConfPath = filepath.Join(dir, "celikpanel-dbtools.conf")
	return phpToolWebmailConfPath, phpToolDBToolsConfPath
}

func TestAnOlderWebmailConfigurationGetsThePHPCheckLocationBeforeTheProof(t *testing.T) {
	webmailPath, _ := pointPHPToolConfs(t)
	const socket, root, fpm = "/run/celikpanel/webmail.sock", "/var/lib/celikpanel-webmail/public_html", "/run/php/php8.4-fpm.sock"
	legacy := legacyRenderWebmailNginxConfig(socket, root, fpm)
	if err := os.WriteFile(webmailPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &checkLocationRecorder{}
	changed, err := ensurePHPToolCheckLocationWith(context.Background(), checkLocationOps(rec), phpToolWebmail)
	if err != nil || !changed || rec.tests != 1 || rec.reloads != 1 {
		t.Fatalf("changed=%v err=%v tests=%d reloads=%d", changed, err, rec.tests, rec.reloads)
	}
	got, _ := os.ReadFile(webmailPath)
	if string(got) != renderWebmailNginxConfig(socket, root, fpm) || !phpToolHasCheckLocation(string(got), phpToolWebmail) {
		t.Fatalf("rewritten configuration:\n%s", got)
	}
	// Already in today's form: nothing is written or reloaded.
	changed, err = ensurePHPToolCheckLocationWith(context.Background(), checkLocationOps(rec), phpToolWebmail)
	if err != nil || changed || rec.tests != 1 || rec.reloads != 1 {
		t.Fatalf("second pass changed=%v err=%v tests=%d", changed, err, rec.tests)
	}
}

func TestAnOlderDatabaseToolsConfigurationKeepsItsSocketsAndGetsTheCheck(t *testing.T) {
	_, dbPath := pointPHPToolConfs(t)
	tools := []string{"phpmyadmin", "phppgadmin"}
	roots := map[string]string{"phpmyadmin": "/usr/share/phpmyadmin", "phppgadmin": "/usr/share/phppgadmin"}
	sockets := map[string]string{"phpmyadmin": "/run/php/php8.3-fpm.sock", "phppgadmin": "/run/php/php8.4-fpm.sock"}
	if err := os.WriteFile(dbPath, []byte(legacyRenderDBToolsConfig(tools, roots, sockets)), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &checkLocationRecorder{}
	changed, err := ensurePHPToolCheckLocationWith(context.Background(), checkLocationOps(rec), "phppgadmin")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got, _ := os.ReadFile(dbPath)
	if string(got) != renderDBToolsConfig(tools, roots, sockets) {
		t.Fatalf("rewritten configuration:\n%s", got)
	}
}

func TestNginxRefusingTheCheckLocationPutsThePreviousFileBack(t *testing.T) {
	webmailPath, _ := pointPHPToolConfs(t)
	legacy := legacyRenderWebmailNginxConfig("/run/celikpanel/webmail.sock", "/var/lib/celikpanel-webmail/public_html", "/run/php/php8.4-fpm.sock")
	os.WriteFile(webmailPath, []byte(legacy), 0o600)
	rec := &checkLocationRecorder{testErr: errors.New("nginx: [emerg] unexpected }")}
	changed, err := ensurePHPToolCheckLocationWith(context.Background(), checkLocationOps(rec), phpToolWebmail)
	if err == nil || changed || rec.reloads != 0 || !strings.Contains(err.Error(), "put back") {
		t.Fatalf("changed=%v err=%v reloads=%d", changed, err, rec.reloads)
	}
	if got, _ := os.ReadFile(webmailPath); string(got) != legacy {
		t.Fatalf("the previous file is not back:\n%s", got)
	}
}

func TestAnOwnerEditedWebmailConfigurationIsRefusedAndTheUpdateChangesNothing(t *testing.T) {
	webmailPath, _ := pointPHPToolConfs(t)
	edited := legacyRenderWebmailNginxConfig("/run/celikpanel/webmail.sock", "/var/lib/celikpanel-webmail/public_html", "/run/php/php8.4-fpm.sock") + "# the owner's line\n"
	os.WriteFile(webmailPath, []byte(edited), 0o600)
	rec := &checkLocationRecorder{}
	_, err := ensurePHPToolCheckLocationWith(context.Background(), checkLocationOps(rec), phpToolWebmail)
	var owner *errPHPToolConfigOwnerEdited
	if !errors.As(err, &owner) || rec.tests != 0 {
		t.Fatalf("err=%v tests=%d", err, rec.tests)
	}
	if got, _ := os.ReadFile(webmailPath); string(got) != edited {
		t.Fatal("an owner-edited file was changed")
	}

	live, ops, proofs := roundcubeUpdateFixture(t, func(int) error { return nil })
	ops.prepareProof = func(ctx context.Context) error {
		_, err := ensurePHPToolCheckLocationWith(ctx, checkLocationOps(rec), phpToolWebmail)
		return err
	}
	resp := updateRoundcubeTo(context.Background(), "1.6.20", ops)
	if resp.ErrorCode != "CONFIG_OWNER_EDITED" || resp.RolledBack || *proofs != 0 || roundcubeTreeVersion(live) != "1.6.15" {
		t.Fatalf("resp=%+v proofs=%d", resp, *proofs)
	}
	if !strings.Contains(resp.Error, "Roundcube was not changed") {
		t.Fatalf("error: %s", resp.Error)
	}
}

// alpha.109 (Frankfurt, unstyled webmail): the proof also fetches the
// stylesheet the login page names and requires it as text/css.
func TestWebmailProofRequiresItsStylesheetAsCSS(t *testing.T) {
	page := []byte(`<html><head><link rel="shortcut icon" href="skins/elastic/images/favicon.ico">` +
		`<link rel="stylesheet" href="skins/elastic/deps/bootstrap.min.css?s=1">` +
		`<link rel="stylesheet" href="skins/elastic/styles/styles.min.css?s=1743252480&amp;x=1"></head></html>`)
	contentType := "text/css; charset=utf-8"
	var asked string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RequestURI()
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte("body{}"))
	}))
	defer server.Close()
	if err := probeWebmailStylesheet(context.Background(), server.Client(), server.URL, page); err != nil {
		t.Fatal(err)
	}
	if asked != "/webmail/skins/elastic/styles/styles.min.css?s=1743252480&x=1" {
		t.Fatalf("asked %q", asked)
	}
	contentType = "text/plain"
	err := probeWebmailStylesheet(context.Background(), server.Client(), server.URL, page)
	var probe *phpToolProbeError
	if !errors.As(err, &probe) || probe.Kind != "asset_unreadable" || !strings.Contains(err.Error(), `"text/plain"`) {
		t.Fatalf("text/plain stylesheet: %v", err)
	}
	if err := probeWebmailStylesheet(context.Background(), server.Client(), server.URL, []byte("<html></html>")); !errors.As(err, &probe) {
		t.Fatalf("no stylesheet: %v", err)
	}
	if _, err := webmailStylesheetURL(server.URL, []byte(`<link rel="stylesheet" href="https://elsewhere.example/x.css">`)); err == nil {
		t.Fatal("a stylesheet elsewhere was accepted")
	}
}
