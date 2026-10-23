package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Serving the database web tools. phpMyAdmin/phpPgAdmin are PHP files, not
// daemons: something must serve them. We give them a loopback-only nginx
// server (127.0.0.1:8306) and the panel reverse-proxies /dbtool/... to it —
// so the tools are reachable only through an authenticated admin panel
// session, never directly from the network, and no firewall port opens.
//
// Veritabanı web araçlarının sunumu. phpMyAdmin/phpPgAdmin daemon değil PHP
// dosyalarıdır: bir şey onları sunmalı. Onlara yalnız-loopback bir nginx
// sunucusu (127.0.0.1:8306) veririz ve panel /dbtool/...'u ona ters-vekiller —
// böylece araçlara yalnız kimlik doğrulamalı yönetici panel oturumu üzerinden
// ulaşılır, ağdan asla doğrudan ulaşılmaz ve güvenlik duvarında port açılmaz.

const (
	dbToolsConfPath = "/etc/nginx/conf.d/celikpanel-dbtools.conf"
	dbToolsAddr     = "127.0.0.1:8306"
)

// dbToolRoots: catalogue tool id → the docroot its distro package installs.
// dbToolRoots: katalog araç id'si → dağıtım paketinin kurduğu kök dizin.
var dbToolRoots = map[string][]string{
	"phpmyadmin": {"/usr/share/phpmyadmin", "/usr/share/webapps/phpMyAdmin"},
	"phppgadmin": {"/usr/share/phppgadmin"},
}

func firstInstalledDBToolRoot(roots []string) string {
	for _, root := range roots {
		if fileExistsAgent(root) {
			return root
		}
	}
	return ""
}

type ConfigureDBToolsResponse = transport.ConfigureDBToolsResponse

// ConfigureDBTools regenerates the loopback nginx server for whichever tools
// are installed right now; with none installed the config is removed. Called
// by the panel after a tool is installed or uninstalled — idempotent.
// ConfigureDBTools, şu an kurulu araçlar için yalnız-loopback nginx
// sunucusunu yeniden üretir; hiçbiri kurulu değilse yapılandırma kaldırılır.
// Panel bir araç kurulunca/kaldırılınca çağırır — idempotenttir.
func (a *Agent) ConfigureDBTools(req *ServiceMutationRequest, resp *ConfigureDBToolsResponse) error {
	*resp = ConfigureDBToolsResponse{}
	if req == nil {
		return fmt.Errorf("database tools configuration request is required")
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepConfigureDBTools, "dbtools", "", "configure"),
	)
	if err != nil {
		*resp = ConfigureDBToolsResponse{Error: err.Error()}
		return nil
	}
	defer finishStep()
	if _, err := exec.LookPath("nginx"); err != nil {
		resp.Error = "nginx is not installed"
		return nil
	}

	tools, installedRoots := installedDBToolRoots()

	if len(tools) == 0 {
		_ = os.Remove(dbToolsConfPath)
		if out, err := runServiceMutationCombinedOutput(ctx, "systemctl", "reload", "nginx"); err != nil {
			resp.Error = commandFailureDetail("nginx reload", out, err)
			return nil
		}
		resp.Configured = true
		return nil
	}

	// The tools are PHP. Each stays on the PHP version it is bound to (its
	// configuration line, else the Agent's record) while that version is
	// installed (alpha.104); a tool with nothing bound yet gets the
	// distribution's default PHP. The tool's PHP extensions are ensured for
	// that version. A tool whose bound version is gone keeps its line
	// unchanged and is reported: nothing moves it to another version
	// silently; its Manage window offers the explicit move.
	// Araçlar PHP'dir. Her biri bağlı olduğu PHP sürümünde, o sürüm kurulu
	// kaldıkça kalır; sessizce taşınmaz.
	sockets := make(map[string]string, len(tools))
	var problems []string
	for _, state := range phpToolStates() {
		if installedRoots[state.Tool] == "" {
			continue
		}
		socket, err := phpToolSocket(state.Tool, state.Socket)
		if err != nil {
			var gone *errPHPToolVersionGone
			if !errors.As(err, &gone) {
				resp.Error = err.Error()
				return nil
			}
			problems = append(problems, err.Error())
		} else if version := resolvedSocketVersion(socket); version != "" {
			if err := ensurePHPToolExtensions(ctx, state.Tool, version); err != nil {
				problems = append(problems, err.Error())
			}
		}
		sockets[state.Tool] = socket
	}
	if err := ensurePHPToolCheckScript(); err != nil {
		resp.Error = err.Error()
		return nil
	}
	conf := renderDBToolsConfig(tools, installedRoots, sockets)

	if err := os.WriteFile(dbToolsConfPath, []byte(conf), 0o644); err != nil {
		resp.Error = fmt.Sprintf("write nginx config: %v", err)
		return nil
	}
	// Validate before reload — a broken tools config must not take the web
	// server (and every customer site) down with it.
	// Yeniden yüklemeden önce doğrula — bozuk bir araç yapılandırması web
	// sunucusunu (ve tüm müşteri sitelerini) beraberinde düşürmemeli.
	if out, err := serviceMutationCommand(ctx, "nginx", "-t").CombinedOutput(); err != nil {
		_ = os.Remove(dbToolsConfPath)
		resp.Error = fmt.Sprintf("nginx rejected the tools config: %s", firstLine(string(out)))
		return nil
	}
	if out, err := runServiceMutationCombinedOutput(ctx, "systemctl", "reload", "nginx"); err != nil {
		resp.Error = commandFailureDetail("nginx reload", out, err)
		return nil
	}

	resp.Configured = true
	resp.Tools = tools
	for _, tool := range tools {
		if version := resolvedSocketVersion(sockets[tool]); version != "" && phpVersionInstalled(version) {
			if err := writePHPToolBinding(tool, version); err != nil {
				problems = append(problems, err.Error())
			}
		}
	}
	if len(problems) > 0 {
		resp.Error = strings.Join(problems, "; ")
	}
	return nil
}
