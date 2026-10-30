package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A web server that is installed but cannot serve the panel's vhosts, db-tools
// or webmail is only half-installed — and "installed" must never mean that
// (operator, 24 Jul: "how can there be roundcube without nginx, nonsense").
// Debian's nginx ships an http block that already includes conf.d/*.conf and
// sites-enabled/*.conf; Arch's minimal nginx.conf includes neither and the
// directories do not even exist, so every conf the panel drops is invisible.
// EnsureNginxReady makes a freshly installed nginx panel-ready on any distro:
// the drop-in dirs exist and the http block includes them. Idempotent — on
// Debian it finds everything already in place and changes nothing.
//
// Kurulu ama panelin vhost'larını, db-araçlarını ya da webmail'ini
// sunamayan bir web sunucusu yalnız yarı kuruludur — ve "kurulu" bu asla
// demek olmamalı (operatör, 24 Tem: "nginx yokken roundcube nasıl olur,
// saçmalık"). Debian'ın nginx'i, conf.d/*.conf ve sites-enabled/*.conf'u
// zaten dahil eden bir http bloğuyla gelir; Arch'ın minimal nginx.conf'u
// hiçbirini dahil etmez ve dizinler bile yoktur, bu yüzden panelin bıraktığı
// her conf görünmez. EnsureNginxReady, yeni kurulmuş bir nginx'i her dağıtımda
// panel-hazır yapar: drop-in dizinleri var ve http bloğu onları dahil eder.
// Idempotent — Debian'da her şeyi yerinde bulur ve hiçbir şey değiştirmez.

const nginxMainConf = "/etc/nginx/nginx.conf"

type EnsureNginxReadyResponse = transport.EnsureNginxReadyResponse

type nginxReadyCommandRunner func(
	context.Context,
	string,
	...string,
) ([]byte, error)

type nginxReadyConfigWriter func(string, []byte, os.FileMode) error

var nginxReadyMu sync.Mutex

func (a *Agent) EnsureNginxReady(req *ServiceMutationRequest, resp *EnsureNginxReadyResponse) error {
	*resp = EnsureNginxReadyResponse{}
	if req == nil {
		return fmt.Errorf("nginx readiness request is required")
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepEnsureNginxReady, "nginx", "", "ready"),
	)
	if err != nil {
		*resp = EnsureNginxReadyResponse{Error: err.Error()}
		return nil
	}
	defer finishStep()
	if _, err := exec.LookPath("nginx"); err != nil {
		resp.Error = "nginx is not installed"
		return nil
	}

	// The drop-in dirs the panel writes into (site vhosts, db-tools, webmail).
	// Panelin içine yazdığı drop-in dizinleri (site vhost'ları, db-araçları, webmail).
	for _, d := range []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-available",
		"/etc/nginx/sites-enabled",
	} {
		if err := ensureNginxDropInDirectory(d); err != nil {
			resp.Error = err.Error()
			return nil
		}
	}

	changed, err := ensureNginxMainConfig(
		ctx,
		nginxMainConf,
		func(commandCtx context.Context, name string, args ...string) ([]byte, error) {
			return runServiceMutationCombinedOutput(commandCtx, name, args...)
		},
	)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}

	resp.Ready = true
	resp.Changed = changed
	return nil
}

func ensureNginxMainConfig(
	ctx context.Context,
	path string,
	run nginxReadyCommandRunner,
) (bool, error) {
	return ensureNginxMainConfigWithWriter(ctx, path, run, secureWriteConfig)
}

func ensureNginxMainConfigWithWriter(
	ctx context.Context,
	path string,
	run nginxReadyCommandRunner,
	write nginxReadyConfigWriter,
) (bool, error) {
	if run == nil {
		return false, errors.New("nginx readiness command runner is required")
	}
	if write == nil {
		return false, errors.New("nginx readiness config writer is required")
	}

	// The durable service-mutation lease already serializes normal RPC calls.
	// Keep this local guard as a second boundary for direct callers and tests.
	nginxReadyMu.Lock()
	defer nginxReadyMu.Unlock()

	data, err := secureReadConfig(path)
	if err != nil {
		return false, fmt.Errorf("read nginx.conf: %w", err)
	}
	content := string(data)
	needConfD := !strings.Contains(content, "conf.d/*.conf")
	needSites := !strings.Contains(content, "sites-enabled/")
	changed := false
	if needConfD || needSites {
		var includes strings.Builder
		includes.WriteString("\n    # Added by CelikPanel so the panel's drop-in configs are served.\n")
		if needConfD {
			includes.WriteString("    include /etc/nginx/conf.d/*.conf;\n")
		}
		if needSites {
			includes.WriteString("    include /etc/nginx/sites-enabled/*.conf;\n")
		}
		updated, err := insertIntoNginxHTTPBlock(content, includes.String())
		if err != nil {
			return false, err
		}
		if err := publishNginxMainConfigEdit(ctx, path, run, write, data, updated); err != nil {
			return false, err
		}
		changed = true
		if data, err = secureReadConfig(path); err != nil {
			return true, fmt.Errorf("read nginx.conf after adding the includes: %w", err)
		}
	}

	// alpha.113: the version nginx names in its Server header and error
	// pages. Debian 13's nginx.conf already says server_tokens off; Ubuntu's
	// has it commented out and Arch's does not have it, so nginx said
	// "nginx/1.30.6". When nginx.conf has no active server_tokens line of its
	// own, CelikPanel adds `server_tokens off;` to the http block; a line the
	// owner wrote (on or off) is kept as it is. This is a hardening step: if
	// nginx refuses it (for example another file already sets server_tokens
	// in the http block) the previous nginx.conf is put back and readiness
	// is not affected; the reason is logged.
	if !nginxServerTokensSet(string(data)) {
		updated, insertErr := insertIntoNginxHTTPBlock(string(data), nginxServerTokensBlock)
		if insertErr == nil {
			insertErr = publishNginxMainConfigEdit(ctx, path, run, write, data, updated)
		}
		if insertErr != nil {
			log.Printf("nginx readiness: server_tokens off was not added to %s: %v", path, insertErr)
		} else {
			changed = true
		}
	}
	return changed, nil
}

// nginxServerTokensBlock is what CelikPanel adds to an http block that has
// no server_tokens line.
const nginxServerTokensBlock = "\n    # Added by CelikPanel: nginx does not name its version in the Server header or on error pages.\n" +
	"    server_tokens off;\n"

var nginxActiveServerTokens = regexp.MustCompile(`(?m)^[ \t]*server_tokens[ \t]`)

// nginxServerTokensSet says whether nginx.conf has an active (uncommented)
// server_tokens line, whatever its value.
func nginxServerTokensSet(content string) bool {
	return nginxActiveServerTokens.MatchString(content)
}

// insertIntoNginxHTTPBlock adds text right after the opening of the http
// block.
func insertIntoNginxHTTPBlock(content, text string) (string, error) {
	marker := "http {"
	idx := strings.Index(content, marker)
	if idx < 0 {
		return "", errors.New("nginx.conf has no http block to extend")
	}
	cut := idx + len(marker)
	return content[:cut] + text + content[cut:], nil
}

// publishNginxMainConfigEdit replaces nginx.conf with updated, validates and
// reloads nginx; on any failure the original bytes are put back, validated
// and reloaded, and the answer says how far that got.
func publishNginxMainConfigEdit(
	ctx context.Context,
	path string,
	run nginxReadyCommandRunner,
	write nginxReadyConfigWriter,
	data []byte,
	updated string,
) error {
	rollback := func(reason string) error {
		// Rollback is a safety operation, not ordinary request work. Once the
		// replacement may have been published it must not be cancelled merely
		// because the initiating RPC disconnected.
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		restoreErr := write(path, data, 0o644)
		restored, readErr := secureReadConfig(path)
		if readErr != nil {
			return fmt.Errorf("%s; rollback could not be verified: %w", reason, readErr)
		}
		if !bytes.Equal(restored, data) {
			return fmt.Errorf("%s; rollback failed: restored nginx.conf differs from the original", reason)
		}
		if out, commandErr := run(rollbackCtx, "nginx", "-t"); commandErr != nil {
			return fmt.Errorf(
				"%s; original nginx.conf bytes were restored but validation failed: %s",
				reason,
				firstLine(string(out)),
			)
		}
		if out, commandErr := run(rollbackCtx, "systemctl", "reload", "nginx"); commandErr != nil {
			return fmt.Errorf(
				"%s; original nginx.conf was restored and validated but reload failed: %s",
				reason,
				commandFailureDetail("nginx rollback reload failed", out, commandErr),
			)
		}
		if restoreErr != nil {
			return fmt.Errorf(
				"%s; original nginx.conf was restored, validated and reloaded, but directory durability could not be confirmed: %w",
				reason,
				restoreErr,
			)
		}
		return errors.New(reason + "; original nginx.conf restored")
	}

	if writeErr := write(path, []byte(updated), 0o644); writeErr != nil {
		// rename(2) may already have published the new inode before a parent
		// directory fsync fails. Re-read the authoritative path instead of
		// assuming an error means that no bytes changed.
		current, readErr := secureReadConfig(path)
		if readErr == nil && bytes.Equal(current, data) {
			return fmt.Errorf("write nginx.conf failed before publication: %w", writeErr)
		}
		reason := fmt.Sprintf("write nginx.conf may have published replacement bytes: %v", writeErr)
		if readErr != nil {
			reason = fmt.Sprintf("%s; current nginx.conf could not be verified: %v", reason, readErr)
		}
		return rollback(reason)
	}

	if out, commandErr := run(ctx, "nginx", "-t"); commandErr != nil {
		reason := fmt.Sprintf(
			"nginx rejected the updated config: %s",
			firstLine(string(out)),
		)
		return rollback(reason)
	}
	if out, commandErr := run(ctx, "systemctl", "reload", "nginx"); commandErr != nil {
		reason := commandFailureDetail("nginx reload failed", out, commandErr)
		return rollback(reason)
	}
	return nil
}
