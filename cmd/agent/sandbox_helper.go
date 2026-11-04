package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/sandboxbind"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The site sandbox's helper (alpha.119, docs/SITE-SANDBOX.md §1a). Every
// sandboxed unit requires its bind unit celikpanel-sandbox@<unit>, which
// runs the helper to bind the app folder by descriptor onto a root-owned
// mount point; the unit's own ExecStartPre=+ runs it to load the SMTP block
// (sitesandbox/helper.go, internal/sandboxbind).
//
// The helper is this binary started under the name "sandbox-helper". The
// Agent copies itself to /usr/lib/celikpanel-sandbox/sandbox-helper beside
// the egress table, so that the units keep starting when CelikPanel is
// removed (D-022). The copy follows the D-031 rule for a binary: an absent
// file is written, the Agent's own earlier copy is replaced, and a file the
// owner put there instead is kept and used as it is.
//
// Sandbox yardımcısı bu programın "sandbox-helper" adıyla başlatılmış
// hâlidir; Agent kendini egress tablosunun yanına kopyalar, böylece birimler
// CelikPanel kaldırılsa da başlar.

// sandboxHelperMode reports whether this process was started as the helper.
func sandboxHelperMode() bool {
	return len(os.Args) > 0 && filepath.Base(os.Args[0]) == sitesandbox.HelperName
}

// sandboxHelperSource is the running binary (the open inode, even after an
// update replaced the file); tests swap it.
var sandboxHelperSource = "/proc/self/exe"

var (
	sandboxHelperMu      sync.Mutex
	sandboxHelperSelfSum string
)

// ensureSandboxHelper puts the helper and the bind unit's template in place.
// It runs only inside a mutation (writing or starting a sandboxed unit),
// never from a read.
func ensureSandboxHelper() error {
	sandboxHelperMu.Lock()
	defer sandboxHelperMu.Unlock()
	if err := ensureSandboxHelperBinary(); err != nil {
		return err
	}
	return ensureSandboxBindTemplate()
}

// sandboxDaemonReload makes systemd read a changed template; tests swap it.
var sandboxDaemonReload = func() error {
	if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureSandboxBindTemplate writes celikpanel-sandbox@.service under the
// D-031 rule (an owner's change is kept, CelikPanel's text goes beside it)
// and reloads systemd when it changed.
func ensureSandboxBindTemplate() error {
	dir, err := sandboxUnitDir()
	if err != nil {
		return err
	}
	file := sandboxNativeFile{Kind: "sandbox_bind_template", Path: filepath.Join(dir, sitesandbox.BindTemplate), Body: sitesandbox.BindTemplateText()}
	ledger := readSandboxLedger()
	result, err := services.ApplyManagedSiteFile(file.managed(ledger[file.Path].RenderSHA256), nil, nil)
	if err != nil {
		return fmt.Errorf("%s: %v", file.Path, err)
	}
	switch result.Outcome {
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeRecreated, transport.SiteFileOutcomeTaken:
		ledger[file.Path] = sandboxLedgerEntry{Kind: file.Kind, RenderSHA256: result.RenderSHA256, WrittenAt: time.Now().UTC().Format(time.RFC3339)}
		if err := writeSandboxLedger(ledger); err != nil {
			log.Printf("site sandbox: record the bind unit: %v", err)
		}
		if err := sandboxDaemonReload(); err != nil {
			return err
		}
	case transport.SiteFileOutcomeUnchanged:
		if ledger[file.Path].RenderSHA256 != result.RenderSHA256 {
			ledger[file.Path] = sandboxLedgerEntry{Kind: file.Kind, RenderSHA256: result.RenderSHA256, WrittenAt: time.Now().UTC().Format(time.RFC3339)}
			if err := writeSandboxLedger(ledger); err != nil {
				log.Printf("site sandbox: record the bind unit: %v", err)
			}
		}
	case transport.SiteFileOutcomeKept:
		log.Printf("site sandbox: %s was changed on this server and is kept (%s); CelikPanel's text is in %s", file.Path, result.State, result.PendingPath)
	default:
		return fmt.Errorf("%s could not be written (%s %s): %s", file.Path, result.Outcome, result.Reason, result.Detail)
	}
	return nil
}

// ensureSandboxHelperBinary copies the running Agent to the helper's place.
func ensureSandboxHelperBinary() error {
	if sandboxHelperSelfSum == "" {
		sum, err := fileSHA256(sandboxHelperSource)
		if err != nil {
			return fmt.Errorf("read the Agent's own program: %v", err)
		}
		sandboxHelperSelfSum = sum
	}
	if err := os.MkdirAll(sandboxEgressDir, 0o755); err != nil {
		return fmt.Errorf("the sandbox folder %s: %v", sandboxEgressDir, err)
	}
	dst := filepath.Join(sandboxEgressDir, sitesandbox.HelperName)
	ledger := readSandboxLedger()
	fi, err := os.Lstat(dst)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("%s: %v", dst, err)
	case !fi.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file; remove it, then try again", dst)
	default:
		current, err := fileSHA256(dst)
		if err != nil {
			return fmt.Errorf("%s: %v", dst, err)
		}
		if current == sandboxHelperSelfSum {
			if fi.Mode().Perm() != 0o755 {
				if err := os.Chmod(dst, 0o755); err != nil {
					return err
				}
			}
			if ledger[dst].RenderSHA256 != current {
				ledger[dst] = sandboxLedgerEntry{Kind: "sandbox_helper", RenderSHA256: current, WrittenAt: time.Now().UTC().Format(time.RFC3339)}
				if err := writeSandboxLedger(ledger); err != nil {
					log.Printf("site sandbox: record the helper: %v", err)
				}
			}
			return nil
		}
		if recorded := ledger[dst].RenderSHA256; recorded == "" || recorded != current {
			// Not the Agent's earlier copy: the owner's, kept and used.
			log.Printf("site sandbox: %s is not CelikPanel's copy and is kept as it is; the units run it before every start", dst)
			return nil
		}
	}
	if err := copySandboxHelper(dst); err != nil {
		return fmt.Errorf("install %s: %v", dst, err)
	}
	ledger[dst] = sandboxLedgerEntry{Kind: "sandbox_helper", RenderSHA256: sandboxHelperSelfSum, WrittenAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeSandboxLedger(ledger); err != nil {
		log.Printf("site sandbox: record the helper: %v", err)
	}
	return nil
}

// copySandboxHelper writes the running binary to dst atomically and checks
// that what was written is the binary.
func copySandboxHelper(dst string) error {
	src, err := os.Open(sandboxHelperSource)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp := dst + ".partial"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), src)
	if err == nil {
		err = out.Sync()
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err == nil && hex.EncodeToString(h.Sum(nil)) != sandboxHelperSelfSum {
		err = errors.New("the copy differs from the running program")
	}
	if err == nil {
		err = os.Chmod(tmp, 0o755)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// sandboxFolderRefusal checks an app folder as the helper will before the
// next start: "" when it may start, a code and the reason otherwise
// (sandboxbind.CodeNotReal, CodeNotOwned, CodeUnreadable).
var sandboxFolderRefusal = func(appDir string, uid int) (string, string) {
	if uid < 0 {
		return sandboxbind.CodeNotOwned, "the site's account is unknown"
	}
	err := sandboxbind.CheckAppFolder(appDir, uint32(uid))
	if err == nil {
		return "", ""
	}
	if r, ok := sandboxbind.AsRefusal(err); ok {
		return r.Code, sandboxFolderAdvice(r)
	}
	return sandboxbind.CodeUnreadable, err.Error()
}

func sandboxFolderAdvice(r *sandboxbind.Refusal) string {
	switch r.Code {
	case sandboxbind.CodeNotReal:
		return r.Error() + ". The app folder must be a real folder of this site, with no link on its way; put the real folder back, then try again"
	case sandboxbind.CodeNotOwned:
		return r.Error() + ". The administrator gives the folder back to the site's account (or removes the others' write permission), then try again"
	}
	return r.Error()
}
