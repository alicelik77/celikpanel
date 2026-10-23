package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The explicit, verified move of a tool to another PHP version (alpha.104,
// owner decision of 2026-10-23).

type fakeBindHost struct {
	family       string
	installed    bool
	previous     string
	socketReady  bool
	packages     map[string]bool
	installErr   error
	cliErr       error
	testErrs     []error
	reloadErr    error
	probeErr     error
	restoreErr   error
	conf         map[string][]byte
	steps        []string
	recorded     map[string]string
	installedPkg []string
}

func newFakeBindHost() *fakeBindHost {
	return &fakeBindHost{
		family: "apt", installed: true, previous: "/run/php/php8.4-fpm.sock", socketReady: true,
		packages: map[string]bool{}, conf: map[string][]byte{"/etc/nginx/conf.d/celikpanel-dbtools.conf": []byte("old")},
		recorded: map[string]string{},
	}
}

func (h *fakeBindHost) ops() phpToolBindOps {
	return phpToolBindOps{
		family:        func() string { return h.family },
		toolInstalled: func(string) bool { return h.installed },
		state: func(tool string) phpToolState {
			return phpToolState{Tool: tool, Installed: true, Socket: h.previous, Source: "config"}
		},
		targetSocket: func(tool, version string) (string, error) {
			if version == "9.9" {
				return "", errors.New("PHP 9.9 is not installed")
			}
			return "/run/php/php" + version + "-fpm.sock", nil
		},
		socketReady:      func(string) bool { return h.socketReady },
		packageInstalled: func(pkg string) bool { return h.packages[pkg] },
		installPackages: func(_ context.Context, pkgs []string) error {
			h.steps = append(h.steps, "install:"+strings.Join(pkgs, ","))
			if h.installErr != nil {
				return h.installErr
			}
			h.installedPkg = append(h.installedPkg, pkgs...)
			return nil
		},
		reloadFPM: func(_ context.Context, version string) error {
			h.steps = append(h.steps, "reload-fpm:"+version)
			return nil
		},
		cliProbe: func(_ context.Context, tool, version string) error {
			h.steps = append(h.steps, "cli:"+version)
			return h.cliErr
		},
		render: func(tool, socket string) (string, []byte, error) {
			return "/etc/nginx/conf.d/celikpanel-dbtools.conf", []byte("new:" + socket), nil
		},
		snapshotConf: func(path string) (mailFileSnapshot, error) {
			content, ok := h.conf[path]
			return mailFileSnapshot{path: path, exists: ok, content: content, mode: 0o644}, nil
		},
		writeConf: func(path string, content []byte) error {
			h.steps = append(h.steps, "write:"+string(content))
			h.conf[path] = content
			return nil
		},
		restoreConf: func(snapshot mailFileSnapshot) error {
			if h.restoreErr != nil {
				return h.restoreErr
			}
			if !snapshot.exists {
				h.steps = append(h.steps, "remove")
				delete(h.conf, snapshot.path)
				return nil
			}
			h.steps = append(h.steps, "write:"+string(snapshot.content))
			h.conf[snapshot.path] = snapshot.content
			return nil
		},
		nginxTest: func(context.Context) error {
			h.steps = append(h.steps, "nginx-t")
			if len(h.testErrs) > 0 {
				err := h.testErrs[0]
				h.testErrs = h.testErrs[1:]
				return err
			}
			return nil
		},
		nginxReload: func(context.Context) error {
			h.steps = append(h.steps, "reload-nginx")
			return h.reloadErr
		},
		httpProbe: func(_ context.Context, tool, version string) error {
			h.steps = append(h.steps, "probe")
			return h.probeErr
		},
		writeRecord: func(tool, version string) error {
			h.recorded[tool] = version
			return nil
		},
	}
}

func TestBindPHPToolEnsuresExtensionsRewritesAndProves(t *testing.T) {
	host := newFakeBindHost()
	host.packages["php8.5-mbstring"] = true
	result, err := bindPHPToolWith(context.Background(), host.ops(), phpToolPHPMyAdmin, "8.5")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"install:php8.5-mysql,php8.5-xml", "reload-fpm:8.5", "cli:8.5",
		"write:new:/run/php/php8.5-fpm.sock", "nginx-t", "reload-nginx", "probe",
	}
	if !reflect.DeepEqual(host.steps, want) {
		t.Fatalf("steps = %v, want %v", host.steps, want)
	}
	if result.Previous != "8.4" || !result.MutationApplied || result.Restored || host.recorded[phpToolPHPMyAdmin] != "8.5" {
		t.Fatalf("result = %+v recorded=%v", result, host.recorded)
	}
}

func TestBindPHPToolRestoresThePreviousConfigurationWhenTheProbeFails(t *testing.T) {
	host := newFakeBindHost()
	for _, s := range []string{"mysql", "mbstring", "xml"} {
		host.packages["php8.6-"+s] = true
	}
	host.probeErr = &phpToolProbeError{Kind: "http_status", Status: 502, Message: "phpMyAdmin answered HTTP 502"}
	result, err := bindPHPToolWith(context.Background(), host.ops(), phpToolPHPMyAdmin, "8.6")
	var typed *phpToolBindError
	if !errors.As(err, &typed) || typed.Code != errCodePHPToolProbeFailed || typed.Detail != "phpMyAdmin answered HTTP 502" {
		t.Fatalf("err = %v (%+v)", err, typed)
	}
	if typed.Cause.Kind != "http_status" || typed.Cause.Status != 502 || typed.Cause.Version != "8.6" {
		t.Fatalf("cause = %+v", typed.Cause)
	}
	if !strings.Contains(err.Error(), "it was put back on PHP 8.4") || !result.Restored || !result.MutationApplied {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if string(host.conf["/etc/nginx/conf.d/celikpanel-dbtools.conf"]) != "old" {
		t.Fatal("the previous configuration was not restored")
	}
	want := []string{"cli:8.6", "write:new:/run/php/php8.6-fpm.sock", "nginx-t", "reload-nginx", "probe", "write:old", "nginx-t", "reload-nginx"}
	if !reflect.DeepEqual(host.steps, want) {
		t.Fatalf("steps = %v", host.steps)
	}
	if len(host.recorded) != 0 {
		t.Fatal("a failed move was recorded")
	}
}

func TestBindPHPToolNginxRejectionKeepsThePreviousConfiguration(t *testing.T) {
	host := newFakeBindHost()
	host.packages = map[string]bool{"php8.5-pgsql": true, "php8.5-mbstring": true, "php8.5-xml": true}
	host.testErrs = []error{errors.New("nginx: [emerg] bad")}
	_, err := bindPHPToolWith(context.Background(), host.ops(), phpToolPHPPgAdmin, "8.5")
	var typed *phpToolBindError
	if !errors.As(err, &typed) || typed.Code != errCodePHPToolConfigRejected || !strings.Contains(err.Error(), "still runs on PHP 8.4") {
		t.Fatalf("err = %v", err)
	}
	if string(host.conf["/etc/nginx/conf.d/celikpanel-dbtools.conf"]) != "old" || containsStep(host.steps, "reload-nginx") {
		t.Fatalf("steps = %v", host.steps)
	}
}

func TestBindPHPToolRefusesBeforeChangingAnything(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*fakeBindHost)
		tool   string
		ver    string
		code   string
	}{
		"not apt":         {func(h *fakeBindHost) { h.family = "pacman" }, phpToolWebmail, "8.5", errCodePHPToolUnsupported},
		"tool missing":    {func(h *fakeBindHost) { h.installed = false }, phpToolPHPMyAdmin, "8.5", errCodePHPToolNotInstalled},
		"version missing": {func(*fakeBindHost) {}, phpToolPHPMyAdmin, "9.9", errCodePHPVersionNotInstalled},
		"not running":     {func(h *fakeBindHost) { h.socketReady = false }, phpToolWebmail, "8.5", errCodePHPVersionNotRunning},
		"extensions":      {func(h *fakeBindHost) { h.installErr = errors.New("apt failed") }, phpToolWebmail, "8.5", errCodePHPToolExtensionsFailed},
		"cli probe":       {func(h *fakeBindHost) { h.packages = allPackages(); h.cliErr = errors.New("PHP 8.5 lacks intl") }, phpToolWebmail, "8.5", errCodePHPToolExtensionsFailed},
	} {
		t.Run(name, func(t *testing.T) {
			host := newFakeBindHost()
			tc.mutate(host)
			result, err := bindPHPToolWith(context.Background(), host.ops(), tc.tool, tc.ver)
			var typed *phpToolBindError
			if !errors.As(err, &typed) || typed.Code != tc.code {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
			if containsStep(host.steps, "write:") || result.Restored || len(host.recorded) != 0 {
				t.Fatalf("configuration changed: steps=%v result=%+v", host.steps, result)
			}
		})
	}
}

func allPackages() map[string]bool {
	out := map[string]bool{}
	for _, suffixes := range phpToolExtensionSuffixes {
		for _, s := range suffixes {
			out["php8.5-"+s] = true
		}
	}
	return out
}

func containsStep(steps []string, prefix string) bool {
	for _, step := range steps {
		if strings.HasPrefix(step, prefix) {
			return true
		}
	}
	return false
}

func TestCheckPHPToolResponseNeedsTheToolRenderedByPHP(t *testing.T) {
	for _, tc := range []struct {
		tool   string
		status int
		body   string
		ok     bool
	}{
		{phpToolPHPMyAdmin, 200, `<!DOCTYPE HTML><html lang="en"><head><title>phpMyAdmin</title></head><body><input type="text" name="pma_username" id="input_username"></body></html>`, true},
		{phpToolPHPMyAdmin, 200, `<html><title>phpMyAdmin</title><body>The mysqli extension is missing. Please check your PHP configuration.</body></html>`, false},
		{phpToolPHPMyAdmin, 500, `<title>phpMyAdmin - Error</title>`, false},
		{phpToolPHPMyAdmin, 502, `<html><head><title>502 Bad Gateway</title></head><body><center><h1>502 Bad Gateway</h1></center><hr><center>nginx</center></body></html>`, false},
		{phpToolPHPMyAdmin, 200, "<?php\ndeclare(strict_types=1);\nuse PhpMyAdmin\\Common;", false},
		{phpToolPHPPgAdmin, 200, `<title>phpPgAdmin</title><frameset>`, true},
		{phpToolPHPPgAdmin, 200, `Your PHP installation does not support PostgreSQL. You need to recompile PHP using the --with-pgsql configure option.`, false},
		{phpToolWebmail, 200, `<title>CelikPanel Webmail :: Welcome to CelikPanel Webmail</title><input name="_user" id="rcmloginuser" required size="40">`, true},
		{phpToolWebmail, 200, `<html><body>Welcome to nginx!</body></html>`, false},
	} {
		err := checkPHPToolResponse(tc.tool, tc.status, []byte(tc.body))
		if (err == nil) != tc.ok {
			t.Fatalf("%s %d %.40q: err = %v, want ok=%v", tc.tool, tc.status, tc.body, err, tc.ok)
		}
	}
}

func TestPHPToolCLIProbeCodeAsksForTheToolsExtensions(t *testing.T) {
	code := phpToolCLIProbeCode(phpToolPHPPgAdmin)
	if !strings.Contains(code, "['pgsql','mbstring','xml']") || !strings.Contains(code, "extension_loaded") {
		t.Fatalf("code = %s", code)
	}
}

func TestRenderedDBToolsConfigNamesEachToolsOwnSocket(t *testing.T) {
	conf := renderDBToolsConfig(
		[]string{phpToolPHPMyAdmin, phpToolPHPPgAdmin},
		map[string]string{phpToolPHPMyAdmin: "/usr/share/phpmyadmin", phpToolPHPPgAdmin: "/usr/share/phppgadmin"},
		map[string]string{phpToolPHPMyAdmin: "/run/php/php8.5-fpm.sock", phpToolPHPPgAdmin: "/run/php/php8.4-fpm.sock"},
	)
	got := parseDBToolsFPMSockets(conf)
	want := map[string]string{phpToolPHPMyAdmin: "/run/php/php8.5-fpm.sock", phpToolPHPPgAdmin: "/run/php/php8.4-fpm.sock"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed = %v", got)
	}
}

func TestPHPToolRecordRemembersAToolWithoutAConfigurationLine(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	host.installVersion(t, "8.6", true)
	if err := writePHPToolBinding(phpToolPHPMyAdmin, "8.4"); err != nil {
		t.Fatal(err)
	}
	state := phpToolStateFor(phpToolPHPMyAdmin)
	if state.Source != "record" || state.version() != "8.4" {
		t.Fatalf("state = %+v", state)
	}
	// The configuration line wins (an owner's edit is respected).
	host.writeFrankfurtConfs(t)
	if state := phpToolStateFor(phpToolPHPPgAdmin); state.Source != "config" || state.version() != "8.5" {
		t.Fatalf("pgadmin state = %+v", state)
	}
	info, err := os.Stat(phpToolBindingRecordPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("record = %v %v", info, err)
	}
	if err := writePHPToolBinding("wordpress", "8.4"); err == nil {
		t.Fatal("an unknown tool was recorded")
	}
}

func TestPHPToolsBoundToVersionCountsOnlyInstalledTools(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	host.installVersion(t, "8.5", true)
	host.writeFrankfurtConfs(t)
	// Frankfurt after the incident: phpMyAdmin is gone, its line remains.
	phpToolInstalledCheck = func(tool string) bool { return tool != phpToolPHPMyAdmin }
	if got := phpToolsBoundToVersion("8.4"); !reflect.DeepEqual(got, []string{"webmail"}) {
		t.Fatalf("bound to 8.4 = %v", got)
	}
}

func TestFirstBindingPrefersTheMetaPackageDefaultThenTheNewestWithExtensions(t *testing.T) {
	host := newPHPToolHost(t)
	for _, version := range []string{"8.4", "8.5", "8.6"} {
		host.installVersion(t, version, true)
	}
	phpDefaultVersionFromMetas = func() string { return "8.4" }
	if got, _ := phpToolSocket(phpToolPHPMyAdmin, ""); got != filepath.Join(host.run, "php8.4-fpm.sock") {
		t.Fatalf("default = %q", got)
	}
	phpDefaultVersionFromMetas = func() string { return "" }
	phpToolPackageInstalled = func(pkg string) bool { return strings.HasPrefix(pkg, "php8.5-") }
	if got, _ := phpToolSocket(phpToolPHPMyAdmin, ""); got != filepath.Join(host.run, "php8.5-fpm.sock") {
		t.Fatalf("newest with extensions = %q", got)
	}
}

func TestPHPToolBindingsViewNamesStaleBindings(t *testing.T) {
	host := newPHPToolHost(t)
	host.installVersion(t, "8.4", true)
	host.installVersion(t, "8.5", true)
	host.writeFrankfurtConfs(t)
	host.removeVersion(t, "8.5")
	view := phpToolBindingsView()
	if !reflect.DeepEqual(view.Versions, []string{"8.4"}) {
		t.Fatalf("versions = %v", view.Versions)
	}
	for _, binding := range view.Tools {
		if binding.Tool == phpToolPHPPgAdmin && (binding.Version != "8.5" || binding.SocketReady || binding.VersionInstalled) {
			t.Fatalf("pgadmin = %+v", binding)
		}
		if binding.Tool == phpToolWebmail && (binding.Version != "8.4" || !binding.SocketReady || !binding.VersionInstalled) {
			t.Fatalf("webmail = %+v", binding)
		}
	}
}

func TestBindPHPToolStepNeedsItsOwnJob(t *testing.T) {
	allowed := mutationPolicyJob("php_tool_bind", "phpmyadmin", "8.5")
	if err := authorizeServiceMutationStep(allowed, mutationPolicyClaim(serviceMutationStepBindPHPTool, "phpmyadmin", "8.5", "bind")); err != nil {
		t.Fatalf("own job refused: %v", err)
	}
	for name, tc := range map[string]struct {
		job   *ServiceMutationJob
		claim serviceMutationStepClaim
	}{
		"other version": {allowed, mutationPolicyClaim(serviceMutationStepBindPHPTool, "phpmyadmin", "8.6", "bind")},
		"other tool":    {allowed, mutationPolicyClaim(serviceMutationStepBindPHPTool, "phppgadmin", "8.5", "bind")},
		"uninstall job": {mutationPolicyJob("service_uninstall", "php-fpm", "php8.4-fpm"), mutationPolicyClaim(serviceMutationStepBindPHPTool, "phpmyadmin", "8.5", "bind")},
		"unknown tool":  {mutationPolicyJob("php_tool_bind", "wordpress", "8.5"), mutationPolicyClaim(serviceMutationStepBindPHPTool, "wordpress", "8.5", "bind")},
		"no version":    {mutationPolicyJob("php_tool_bind", "webmail", ""), mutationPolicyClaim(serviceMutationStepBindPHPTool, "webmail", "", "bind")},
		"wrong action":  {mutationPolicyJob("php_tool_bind", "webmail", "8.5"), mutationPolicyClaim(serviceMutationStepBindPHPTool, "webmail", "8.5", "configure")},
	} {
		if err := authorizeServiceMutationStep(tc.job, tc.claim); err == nil {
			t.Fatalf("%s: authorized", name)
		}
	}
}

func TestBindPHPToolSaysWhichRestoreStepFailed(t *testing.T) {
	host := newFakeBindHost()
	host.packages = allPackagesFor("8.6")
	host.probeErr = &phpToolProbeError{Kind: "missing_extensions", Extensions: []string{"mysqli"}, Message: "PHP-FPM 8.6 lacks mysqli for phpMyAdmin"}
	host.testErrs = []error{nil, errors.New("nginx: [emerg] owner include")}
	_, err := bindPHPToolWith(context.Background(), host.ops(), phpToolPHPMyAdmin, "8.6")
	var typed *phpToolBindError
	if !errors.As(err, &typed) || !strings.Contains(err.Error(), "the previous configuration was put back, but nginx rejected it") {
		t.Fatalf("err = %v", err)
	}
	if typed.Cause.Kind != "missing_extensions" || len(typed.Cause.Extensions) != 1 || typed.Cause.Extensions[0] != "mysqli" {
		t.Fatalf("cause = %+v", typed.Cause)
	}
	host = newFakeBindHost()
	host.packages = allPackagesFor("8.6")
	host.probeErr = errors.New("down")
	host.restoreErr = errors.New("read-only file system")
	_, err = bindPHPToolWith(context.Background(), host.ops(), phpToolPHPMyAdmin, "8.6")
	if err == nil || !strings.Contains(err.Error(), "the previous configuration could not be put back (read-only file system)") {
		t.Fatalf("err = %v", err)
	}
}

func allPackagesFor(version string) map[string]bool {
	out := map[string]bool{}
	for _, suffixes := range phpToolExtensionSuffixes {
		for _, s := range suffixes {
			out["php"+version+"-"+s] = true
		}
	}
	return out
}
