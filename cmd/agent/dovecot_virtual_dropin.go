package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// The virtual-mailbox drop-in (/etc/dovecot/conf.d/99-celikpanel.conf) is the
// one file that makes Dovecot read the maildirs Postfix delivers into. A
// release that changes its text must reach servers that are already running
// mail, and it must never replace a file the server administrator edited
// (D-022, D-031 style; AGENTS.md "detect owner configuration changes").
//
// Before every write the file on disk is classified:
//
//   - absent: written by the mail setup (ConfigureMailStack) only. The start
//     repair never recreates a file that is not there;
//   - CelikPanel's own file: the current form, whose first line carries the
//     SHA-256 of the rest and still matches it, or byte-for-byte one of the
//     texts earlier releases wrote (they carried no digest; the frozen texts
//     are below). It is rewritten as this release's text;
//   - anything else: the administrator's. It is kept, never overwritten; the
//     mail setup stops with the administrator's two choices named, and the
//     start repair reports that it left the file alone.
//
// How existing servers get a new text: the Panel's start runs
// Agent.WireMailFilters (main.go wireMailFiltersSynchronouslyAtStartup, and
// the deferred retry when an update holds the host). That repair already
// re-asserts Postfix's virtual maps "on upgrade"; it now also runs
// reconcileDovecotVirtualDropIn, which rewrites CelikPanel's own drop-in when
// its text differs from this release's, validates the whole configuration
// with doveconf -n (restoring the previous file when Dovecot rejects it) and
// reloads a running Dovecot. The mail setup (Agent.ConfigureMailStack) writes
// the same text whenever it runs.
//
// 99-celikpanel.conf (sanal posta kutusu eki), Dovecot'un Postfix'in teslim
// ettiği maildir'leri okumasını sağlayan dosyadır. Metni değiştiren bir sürüm,
// postası çalışan sunuculara da ulaşmalı ve yöneticinin düzenlediği dosyayı
// asla değiştirmemelidir. Her yazımdan önce dosya sınıflanır: yok (yalnız
// posta kurulumu yazar), CelikPanel'in kendi dosyası (özeti tutan güncel biçim
// ya da önceki sürümlerin bayt bayt aynı metni; yeniden yazılır) veya
// yöneticinin dosyası (korunur, adıyla söylenir). Mevcut sunucular yeni metni
// Panel başlangıcındaki Agent.WireMailFilters onarımıyla alır.

// dovecotVirtualDropInPath is the managed drop-in. A variable so component
// tests can point the start repair at a scratch file.
var dovecotVirtualDropInPath = dovecotConfDirPath + "/99-celikpanel.conf"

// dovecotInstalledForRepair reports whether a Dovecot configuration exists to
// repair; a variable so component tests can state the fact.
var dovecotInstalledForRepair = func() bool { return fileExistsAgent("/etc/dovecot") }

type dovecotVirtualDropInState string

const (
	dovecotVirtualDropInAbsent      dovecotVirtualDropInState = "absent"
	dovecotVirtualDropInManaged     dovecotVirtualDropInState = "managed"
	dovecotVirtualDropInEarlier     dovecotVirtualDropInState = "earlier_release_text"
	dovecotVirtualDropInOwnerEdited dovecotVirtualDropInState = "owner_edited"
)

// dovecotVirtualOwnerEditedReason is the D-024 text when the drop-in is the
// administrator's: what CelikPanel found, that it changed nothing, who acts
// and how the same operation resumes.
const dovecotVirtualOwnerEditedReason = "CelikPanel left /etc/dovecot/conf.d/99-celikpanel.conf unchanged: it differs from every text CelikPanel wrote there, so it holds the server administrator's settings, and Dovecot keeps running with it. CelikPanel's mail settings were not applied. The server administrator chooses one of two ways, then runs the same mail setup again: either keep the file as it is and make sure it gives the virtual users their maildir (mail_path, mail_home and mail_inbox_path under the mail root), or move it aside (for example `sudo mv /etc/dovecot/conf.d/99-celikpanel.conf /etc/dovecot/conf.d/99-celikpanel.conf.mine`) so CelikPanel writes its file; own settings belong in /etc/dovecot/local.conf."

// earlierDovecotVirtualTexts returns, byte for byte, the drop-in texts the
// releases before this one wrote for a mail root (frozen copies: from
// ef750aa84, July 2026, through v0.1.0-alpha.112 the 2.3 and 2.4 texts did
// not change). They carry no digest; equality is the only evidence they are
// CelikPanel's.
func earlierDovecotVirtualTexts(root string) []string {
	userDir := "%{user | domain}/%{user | username}"
	v23 := fmt.Sprintf(`# Managed by CelikPanel — do not edit by hand / elle düzenlemeyin
mail_location = maildir:%s/%%d/%%n
mail_uid = %s
mail_gid = %s
first_valid_uid = %s
passdb {
  driver = passwd-file
  args = scheme=CRYPT username_format=%%u %s
}
userdb {
  driver = passwd-file
  args = username_format=%%u %s
  default_fields = uid=%s gid=%s home=%s/%%d/%%n
}
`, root, vmailUID, vmailGID, vmailUID,
		dovecotUsersPath, dovecotUsersPath, vmailUID, vmailGID, root)
	v24 := fmt.Sprintf(`# Managed by CelikPanel — do not edit by hand / elle düzenlemeyin
mail_driver = maildir
mail_path = %s/%s
mail_uid = %s
mail_gid = %s
first_valid_uid = %s
passdb passwd-file {
  default_password_scheme = CRYPT
  passwd_file_path = %s
}
userdb passwd-file {
  passwd_file_path = %s
  fields {
    uid:default = %s
    gid:default = %s
    home:default = %s/%s
  }
}
`, root, userDir, vmailUID, vmailGID, vmailUID,
		dovecotUsersPath, dovecotUsersPath, vmailUID, vmailGID, root, userDir)
	return []string{v23, v24}
}

// dovecotVirtualManagedIntact reports whether content is the current form of
// CelikPanel's drop-in with a body that still matches its digest.
func dovecotVirtualManagedIntact(content []byte) bool {
	text := string(content)
	newline := strings.IndexByte(text, '\n')
	if newline < 0 || !strings.HasPrefix(text, dovecotVirtualHeaderStart) {
		return false
	}
	want := strings.TrimPrefix(text[:newline], dovecotVirtualHeaderStart)
	sum := sha256.Sum256([]byte(text[newline+1:]))
	return want == hex.EncodeToString(sum[:])
}

// classifyDovecotVirtualDropIn decides whose the drop-in is. roots are the
// mail roots an earlier release may have written (the resolved root and the
// literal product root).
func classifyDovecotVirtualDropIn(content []byte, exists bool, roots []string) dovecotVirtualDropInState {
	if !exists {
		return dovecotVirtualDropInAbsent
	}
	if dovecotVirtualManagedIntact(content) {
		return dovecotVirtualDropInManaged
	}
	for _, root := range roots {
		for _, earlier := range earlierDovecotVirtualTexts(root) {
			if bytes.Equal(content, []byte(earlier)) {
				return dovecotVirtualDropInEarlier
			}
		}
	}
	return dovecotVirtualDropInOwnerEdited
}

// dovecotVirtualEarlierRoots is the set of roots an earlier text may name.
func dovecotVirtualEarlierRoots(resolved string) []string {
	if resolved == mailRootDir {
		return []string{resolved}
	}
	return []string{resolved, mailRootDir}
}

// Outcomes of the start repair, for the Agent's one-line detail.
const (
	dovecotVirtualRepairUpdated = "dovecot: CelikPanel's virtual mailbox settings (99-celikpanel.conf) were updated to this release's text and Dovecot was reloaded"
	dovecotVirtualRepairWritten = "dovecot: CelikPanel's virtual mailbox settings (99-celikpanel.conf) were updated to this release's text; Dovecot is stopped and reads them when it starts"
)

// reconcileDovecotVirtualDropIn brings CelikPanel's own drop-in to this
// release's text at the Panel's start (see the file comment). It returns one
// sentence for the Agent's detail, or "" when there was nothing to do. It
// never fails the caller: the milter wiring it runs beside is a separate
// change, and every outcome here is reported in words.
func reconcileDovecotVirtualDropIn(ctx context.Context) string {
	if !dovecotInstalledForRepair() {
		return ""
	}
	path := dovecotVirtualDropInPath
	snapshot, err := snapshotMailFile(path)
	if err != nil {
		return fmt.Sprintf("dovecot: CelikPanel could not read %s, so it changed nothing there (%v); the mail setup reports the same when it runs", path, err)
	}
	if !snapshot.exists {
		// Mail was never configured here, or the administrator removed the
		// file; neither is the start repair's to undo.
		return ""
	}
	modern, err := dovecotIs24()
	if err != nil {
		return "dovecot: the installed Dovecot version could not be read, so CelikPanel did not check its virtual mailbox settings and changed nothing; it checks again at the next Panel start"
	}
	root, err := managedMailRootPath()
	if err != nil {
		return fmt.Sprintf("dovecot: the mail root could not be resolved (%v), so CelikPanel changed nothing in %s", err, path)
	}
	want := buildDovecotVirtualConfAt(modern, root)
	if bytes.Equal(snapshot.content, []byte(want)) {
		return ""
	}
	switch classifyDovecotVirtualDropIn(snapshot.content, true, dovecotVirtualEarlierRoots(root)) {
	case dovecotVirtualDropInManaged, dovecotVirtualDropInEarlier:
	default:
		return "dovecot: " + dovecotVirtualOwnerEditedReason
	}
	if err := applyDovecotConf(path, want); err != nil {
		// applyDovecotConf put the previous file back when Dovecot refused.
		return fmt.Sprintf("dovecot: CelikPanel's virtual mailbox settings were not updated: %v; the previous file is in place and Dovecot keeps running with it", err)
	}
	if err := secureSetMailFileMetadata(path, snapshot.mode, snapshot.uid, snapshot.gid); err != nil {
		return fmt.Sprintf("dovecot: CelikPanel's virtual mailbox settings were updated, but the file's previous owner and mode could not be restored: %v", err)
	}
	state, err := applyDovecotVerified(mailServiceLeaseRunner(ctx), mailServiceReload)
	if err != nil {
		var serviceErr *mailServiceError
		unknown := errors.As(err, &serviceErr) && serviceErr.unknown
		if unknown {
			return fmt.Sprintf("dovecot: CelikPanel's virtual mailbox settings were updated and validated, but whether Dovecot reloaded them is unknown (%v); the server administrator runs `sudo systemctl reload dovecot`", err)
		}
		return fmt.Sprintf("dovecot: CelikPanel's virtual mailbox settings were updated and validated, but Dovecot did not reload them (%v); the server administrator runs `sudo systemctl reload dovecot` and reads `journalctl -u dovecot`", err)
	}
	if state == mailServiceNotRunning {
		return dovecotVirtualRepairWritten
	}
	return dovecotVirtualRepairUpdated
}
