package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Outgoing mail through another server (alpha.130, smarthost;
// docs/MAIL-RELAY.md; transport/mail_relay_contracts.go).
//
// What CelikPanel writes, and nothing else:
//
//   - /etc/postfix/celikpanel-relay-sasl (root, 0600) with its index from
//     postmap (root, 0600): "[host]:port user:password";
//   - master.cf service celikpanel-relay/unix: Postfix's smtp client with its
//     own -o options (SASL login, mandatory TLS, the map above), so the
//     distribution's smtp_* settings are not touched;
//   - main.cf default_transport = celikpanel-relay and relayhost = [host]:port.
//
// They are written in that order and removed in the opposite order, so an
// interrupted run leaves either nothing of the relay in main.cf or
// default_transport naming CelikPanel's service: a relayhost without
// default_transport = celikpanel-relay is always the owner's. A relayhost,
// default_transport or sender-dependent relay the owner set is never changed;
// CelikPanel's own service or map changed on the server is never overwritten.
// Postfix reads all of it natively and keeps relaying without the panel
// (D-022). Turning the relay off removes exactly these (no leftovers).
//
// Posta başka bir sunucu üzerinden: CelikPanel yalnız bu dosya, bu hizmet ve bu
// iki main.cf satırını yazar; kapatınca yalnız onları kaldırır. Sahibin relay
// ayarına dokunulmaz.

const (
	mailRelayService      = "celikpanel-relay"
	mailRelayServiceEntry = mailRelayService + "/unix"
	mailRelayServiceLine  = mailRelayService + " unix - - n - - smtp"
	mailRelayMapHeader    = "# CelikPanel: the sign-in of the relay server for Postfix (root only). Turn the relay off in CelikPanel to remove this file."
	mailRelayCommandWait  = 30 * time.Second
)

var (
	mailRelayMapPath = "/etc/postfix/celikpanel-relay-sasl"
	// The main.cf settings that make another relay win over CelikPanel's
	// (or are the owner's relay): any of them set by the owner stops the apply.
	mailRelayOwnerParams = []string{"sender_dependent_relayhost_maps", "sender_dependent_default_transport_maps"}
)

// Swapped by tests.
var (
	mailRelayRun = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, mailRelayCommandWait)
		defer cancel()
		return executeMailTLSCommand(ctx, name, args...)
	}
	mailRelayLookPath = exec.LookPath
	mailRelayRead     = func(path string) ([]byte, bool, error) {
		raw, err := secureReadConfig(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		return raw, true, nil
	}
	mailRelayWriteSecret = func(path string, content []byte) error {
		return secureWriteConfig(path, content, 0o600)
	}
	mailRelayChmodSecret = func(path string) error {
		exists, err := secureMailFileExists(path)
		if err != nil || !exists {
			return err
		}
		return secureChmodMailFile(path, 0o600)
	}
	mailRelayRemove = func(path string) error {
		err := secureRemoveConfig(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	mailRelaySnapshot = snapshotMailFile
	mailRelayRestore  = restoreMailFile
	mailRelayMapType  = postfixMapTypeContext
	mailRelayReload   = func() error {
		_, err := applyPostfixVerified(runMailTLSCommand, mailServiceReload)
		return err
	}
	mailRelayGlob = filepath.Glob
)

var mailRelayMu sync.Mutex

// mailRelayPostfixFiles are the files a relay apply may change and puts back
// when Postfix does not take the change.
var mailRelayPostfixFiles = []string{"/etc/postfix/main.cf", "/etc/postfix/master.cf"}

// ---- Reading the server -----------------------------------------------------

type mailRelayFacts struct {
	relayhost        string
	defaultTransport string
	ownerParams      []string
	servicePresent   bool
	serviceParams    map[string]string
	serviceFields    map[string]string
	mapPresent       bool
	mapOurs          bool
}

func (f mailRelayFacts) state() string {
	dtOurs := f.defaultTransport == mailRelayService
	switch {
	case len(f.ownerParams) > 0,
		f.defaultTransport != "" && !dtOurs,
		f.relayhost != "" && !dtOurs:
		return transport.MailRelayStateOwner
	case f.servicePresent && !mailRelayServiceIsOurs(f.serviceFields, f.serviceParams),
		f.mapPresent && !f.mapOurs:
		return transport.MailRelayStateEdited
	case dtOurs && f.relayhost != "" && f.servicePresent && f.mapPresent:
		return transport.MailRelayStateOn
	case !dtOurs && f.relayhost == "" && !f.servicePresent && !f.mapPresent:
		return transport.MailRelayStateOff
	default:
		return transport.MailRelayStatePartial
	}
}

func mailRelayPostconfValue(ctx context.Context, name string) (string, error) {
	out, err := mailRelayRun(ctx, "postconf", "-n", "-h", name)
	if err != nil {
		return "", fmt.Errorf("postconf -n -h %s: %s", name, boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
	}
	// A setting main.cf does not set prints nothing at all with -n (measured
	// with Postfix 3.10 on Debian 13): that is "not set", not an unknown.
	if mailRelayOnlyPostconfMessages(out) {
		return "", nil
	}
	value, known := postconfOneValue(out)
	if !known {
		return "", fmt.Errorf("postconf -n -h %s did not answer one value", name)
	}
	return value, nil
}

// mailRelayOnlyPostconfMessages: the output holds no line other than
// postconf's own warnings (or nothing).
func mailRelayOnlyPostconfMessages(out []byte) bool {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" && !postconfOwnMessage.MatchString(line) {
			return false
		}
	}
	return true
}

func readMailRelayFacts(ctx context.Context) (mailRelayFacts, error) {
	var facts mailRelayFacts
	var err error
	if facts.relayhost, err = mailRelayPostconfValue(ctx, "relayhost"); err != nil {
		return facts, err
	}
	if facts.defaultTransport, err = mailRelayPostconfValue(ctx, "default_transport"); err != nil {
		return facts, err
	}
	for _, name := range mailRelayOwnerParams {
		value, err := mailRelayPostconfValue(ctx, name)
		if err != nil {
			return facts, err
		}
		if value != "" {
			facts.ownerParams = append(facts.ownerParams, name)
		}
	}
	run := func(name string, args ...string) ([]byte, error) { return mailRelayRun(ctx, name, args...) }
	if fields, ok := masterServiceFields(run, mailRelayServiceEntry); ok {
		facts.servicePresent = true
		facts.serviceFields = fields
		out, err := mailRelayRun(ctx, "postconf", "-P", mailRelayServiceEntry)
		if err != nil {
			return facts, fmt.Errorf("postconf -P %s: %s", mailRelayServiceEntry, boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
		}
		facts.serviceParams = parseMailRelayServiceParams(string(out))
	}
	raw, exists, err := mailRelayRead(mailRelayMapPath)
	if err != nil {
		return facts, fmt.Errorf("read %s: %s", mailRelayMapPath, boundedNativeDetail(err.Error()))
	}
	facts.mapPresent = exists
	facts.mapOurs = exists && strings.HasPrefix(string(raw), mailRelayMapHeader+"\n")
	return facts, nil
}

func parseMailRelayServiceParams(out string) map[string]string {
	params := map[string]string{}
	prefix := mailRelayServiceEntry + "/"
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		name, value, found := strings.Cut(strings.TrimPrefix(line, prefix), "=")
		if found {
			params[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	return params
}

// mailRelayServiceParams is the exact -o set CelikPanel writes.
func mailRelayServiceParams(mapType string, port int) map[string]string {
	params := map[string]string{
		"smtp_sasl_auth_enable":          "yes",
		"smtp_sasl_password_maps":        mapType + ":" + mailRelayMapPath,
		"smtp_sasl_security_options":     "noanonymous",
		"smtp_sasl_tls_security_options": "noanonymous",
		"smtp_tls_security_level":        "encrypt",
	}
	if port == 465 {
		params["smtp_tls_wrappermode"] = "yes"
	}
	return params
}

// mailRelayServiceIsOurs: the service line and its options are one of the
// forms CelikPanel writes (with or without TLS from the first byte, any map
// type over CelikPanel's map path).
func mailRelayServiceIsOurs(fields, params map[string]string) bool {
	// postconf -F shows the -o options inside the command field ("smtp -o
	// a=b -o c=d"); the options themselves are compared through -P below.
	command := strings.Fields(fields["command"])
	if len(command) == 0 || command[0] != "smtp" || len(command)%2 == 0 || fields["chroot"] != "n" {
		return false
	}
	for i := 1; i < len(command); i += 2 {
		if command[i] != "-o" {
			return false
		}
	}
	maps := params["smtp_sasl_password_maps"]
	mapType, path, found := strings.Cut(maps, ":")
	if !found || path != mailRelayMapPath || postfixIndexExt(mapType) == "" && mapType != "texthash" {
		return false
	}
	for _, port := range []int{587, 465} {
		want := mailRelayServiceParams(mapType, port)
		if len(want) != len(params) {
			continue
		}
		same := true
		for key, value := range want {
			same = same && params[key] == value
		}
		if same {
			return true
		}
	}
	return false
}

// mailRelaySASLReady: Postfix's SMTP client has Cyrus SASL and a PLAIN or
// LOGIN module is installed (Debian/Ubuntu: libsasl2-modules).
func mailRelaySASLReady(ctx context.Context) bool {
	out, err := mailRelayRun(ctx, "postconf", "-A")
	if err != nil || !strings.Contains(string(out), "cyrus") {
		return false
	}
	for _, pattern := range []string{
		"/usr/lib/*/sasl2/libplain.so*", "/usr/lib/*/sasl2/liblogin.so*",
		"/usr/lib/sasl2/libplain.so*", "/usr/lib/sasl2/liblogin.so*",
		"/usr/lib64/sasl2/libplain.so*", "/usr/lib64/sasl2/liblogin.so*",
	} {
		if found, _ := mailRelayGlob(pattern); len(found) > 0 {
			return true
		}
	}
	return false
}

func mailRelayOwnerDetail(facts mailRelayFacts) string {
	var parts []string
	if facts.relayhost != "" && facts.defaultTransport != mailRelayService {
		parts = append(parts, "relayhost = "+facts.relayhost)
	}
	if facts.defaultTransport != "" && facts.defaultTransport != mailRelayService {
		parts = append(parts, "default_transport = "+facts.defaultTransport)
	}
	parts = append(parts, facts.ownerParams...)
	return "main.cf already has the server owner's relay setting (" + strings.Join(parts, ", ") +
		"). CelikPanel keeps it and does not change it. To let CelikPanel send through a relay, the server owner removes that line (for example `sudo postconf -X relayhost`) and saves again"
}

func mailRelayEditedDetail(facts mailRelayFacts) string {
	var parts []string
	if facts.servicePresent && !mailRelayServiceIsOurs(facts.serviceFields, facts.serviceParams) {
		parts = append(parts, "the master.cf service "+mailRelayService+" (`sudo postconf -MX "+mailRelayServiceEntry+"` removes it)")
	}
	if facts.mapPresent && !facts.mapOurs {
		parts = append(parts, mailRelayMapPath)
	}
	return strings.Join(parts, " and ") + " was changed on the server, so CelikPanel leaves it as it is and changed nothing. The server owner keeps it, or moves it aside and saves again"
}

func readMailRelayStatus(ctx context.Context) transport.MailRelayStatus {
	if _, err := mailRelayLookPath("postconf"); err != nil {
		return transport.MailRelayStatus{}
	}
	status := transport.MailRelayStatus{Postfix: true, SASLReady: mailRelaySASLReady(ctx)}
	facts, err := readMailRelayFacts(ctx)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.State = facts.state()
	status.Relayhost = facts.relayhost
	switch status.State {
	case transport.MailRelayStateOwner:
		status.Detail = mailRelayOwnerDetail(facts)
	case transport.MailRelayStateEdited:
		status.Detail = mailRelayEditedDetail(facts)
	}
	return status
}

// MailRelayStatusV1 reads the relay the server holds. Read only.
func (a *Agent) MailRelayStatusV1(_ *transport.Empty, resp *transport.MailRelayStatus) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	*resp = readMailRelayStatus(ctx)
	return nil
}

// ---- Applying ---------------------------------------------------------------

// MailRelayApplyV1 makes the server hold the wanted relay (or none).
func (a *Agent) MailRelayApplyV1(req *transport.MailRelayApplyRequest, resp *transport.MailRelayApplyResponse) error {
	if req == nil || resp == nil {
		return errors.New("relay request and response are required")
	}
	*resp = transport.MailRelayApplyResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing the mail relay"); err != nil {
		resp.Code, resp.Error = transport.MailRelayInvalid, err.Error()
		return nil
	}
	if !mailRelayMu.TryLock() {
		resp.Code, resp.Error = transport.MailRelayBusy, "another change of the mail relay is running; nothing was changed"
		return nil
	}
	defer mailRelayMu.Unlock()
	if !mailMutex.TryLock() {
		resp.Code, resp.Error = transport.MailRelayBusy, "another mail change is running; nothing was changed"
		return nil
	}
	defer mailMutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	applyMailRelay(ctx, req, resp)
	return nil
}

func applyMailRelay(ctx context.Context, req *transport.MailRelayApplyRequest, resp *transport.MailRelayApplyResponse) {
	step := func(text string) { resp.Steps = append(resp.Steps, text) }
	if _, err := mailRelayLookPath("postconf"); err != nil {
		resp.Code, resp.Error = transport.MailRelayNoPostfix, "Postfix is not installed on this server; nothing was changed"
		return
	}
	var nexthop string
	if req.Enabled {
		var err error
		if nexthop, err = transport.MailRelayNexthop(req.Host, req.Port); err != nil {
			resp.Code, resp.Error = transport.MailRelayInvalid, err.Error()+"; nothing was changed"
			return
		}
		if err := transport.ValidateMailRelayUsername(req.Username); err != nil {
			resp.Code, resp.Error = transport.MailRelayInvalid, err.Error()+"; nothing was changed"
			return
		}
		if err := transport.ValidateMailRelayPassword(req.Password); err != nil {
			resp.Code, resp.Error = transport.MailRelayInvalid, err.Error()+"; nothing was changed"
			return
		}
	}
	facts, err := readMailRelayFacts(ctx)
	if err != nil {
		resp.Code, resp.Error = transport.MailRelayUnreadable, "the Postfix settings could not be read ("+err.Error()+"); nothing was changed"
		return
	}
	state := facts.state()
	resp.State = state
	switch state {
	case transport.MailRelayStateEdited:
		resp.Code, resp.Error = transport.MailRelayEditedConfig, mailRelayEditedDetail(facts)
		return
	case transport.MailRelayStateOwner:
		if req.Enabled {
			resp.Code, resp.Error = transport.MailRelayOwnerConfig, mailRelayOwnerDetail(facts)
			return
		}
	}
	if req.Enabled && !mailRelaySASLReady(ctx) {
		resp.Code, resp.Error = transport.MailRelaySASLMissing,
			"Postfix cannot sign in to another server yet: its SASL login modules are not installed. The server administrator installs them from the distribution (Debian and Ubuntu: `sudo apt install libsasl2-modules`; Arch: `sudo pacman -S libsasl`) and saves again; nothing was changed"
		return
	}
	oursPresent := facts.defaultTransport == mailRelayService || facts.servicePresent || facts.mapPresent
	if !req.Enabled && !oursPresent {
		// Nothing of CelikPanel's relay is there: nothing to remove, and an
		// owner's relay stays exactly as it is.
		resp.Applied = true
		return
	}

	mapType := mailRelayMapType(ctx)
	files := append([]string{}, mailRelayPostfixFiles...)
	files = append(files, mailRelayMapPath, mailRelayMapPath+".db", mailRelayMapPath+".lmdb", mailRelayMapPath+".cdb")
	snapshots := make([]mailFileSnapshot, 0, len(files))
	for _, path := range files {
		snapshot, err := mailRelaySnapshot(path)
		if err != nil {
			resp.Code, resp.Error = transport.MailRelayUnreadable, "the Postfix files could not be read before the change ("+boundedNativeDetail(err.Error())+"); nothing was changed"
			return
		}
		snapshots = append(snapshots, snapshot)
	}
	putBack := func(cause string) {
		var errs []string
		for _, snapshot := range snapshots {
			if err := mailRelayRestore(snapshot); err != nil {
				errs = append(errs, boundedNativeDetail(err.Error()))
			}
		}
		if len(errs) > 0 {
			resp.Code = transport.MailRelayRollbackFailed
			resp.Error = cause + "; the earlier Postfix files could not all be put back (" + strings.Join(errs, "; ") + "). The server administrator runs `sudo postfix check` and compares /etc/postfix/main.cf and master.cf"
			return
		}
		if reloadErr := mailRelayReload(); reloadErr != nil {
			resp.Code = transport.MailRelayRollbackFailed
			resp.Error = cause + "; the earlier Postfix files were put back but Postfix did not reload them (" + boundedNativeDetail(reloadErr.Error()) + "). The server administrator runs `sudo postfix check` and `sudo systemctl reload postfix`"
			return
		}
		resp.Code = transport.MailRelayPostfixFailed
		resp.Error = cause + "; the earlier settings were put back and mail is sent as before"
	}
	run := func(args ...string) error {
		out, err := mailRelayRun(ctx, args[0], args[1:]...)
		if err != nil {
			return fmt.Errorf("%s: %s", strings.Join(args[:2], " "), boundedNativeDetail(firstNonEmpty(string(out), err.Error())))
		}
		return nil
	}

	if req.Enabled {
		content := mailRelayMapHeader + "\n" + nexthop + " " + req.Username + ":" + req.Password + "\n"
		if err := mailRelayWriteSecret(mailRelayMapPath, []byte(content)); err != nil {
			putBack("the relay sign-in file could not be written (" + boundedNativeDetail(err.Error()) + ")")
			return
		}
		if mapType != "texthash" {
			// umask 077: the index holds the password and is never readable by
			// anyone but root, not even for a moment.
			if err := run("sh", "-c", `umask 077 && exec postmap "$1"`, "sh", mapType+":"+mailRelayMapPath); err != nil {
				putBack("postmap could not build the relay sign-in map (" + err.Error() + ")")
				return
			}
			if err := mailRelayChmodSecret(mailRelayMapPath + postfixIndexExt(mapType)); err != nil {
				putBack("the relay sign-in map could not be made root-only (" + boundedNativeDetail(err.Error()) + ")")
				return
			}
		}
		step("the relay sign-in is in " + mailRelayMapPath + " (root only)")
		if err := run("postconf", "-M", mailRelayServiceEntry+"="+mailRelayServiceLine); err != nil {
			putBack(err.Error())
			return
		}
		want := mailRelayServiceParams(mapType, req.Port)
		for key := range facts.serviceParams {
			if _, keep := want[key]; !keep {
				if err := run("postconf", "-PX", mailRelayServiceEntry+"/"+key); err != nil {
					putBack(err.Error())
					return
				}
			}
		}
		keys := make([]string, 0, len(want))
		for key := range want {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := run("postconf", "-P", mailRelayServiceEntry+"/"+key+"="+want[key]); err != nil {
				putBack(err.Error())
				return
			}
		}
		step("the master.cf service " + mailRelayService + " signs in over TLS only")
		if err := run("postconf", "-e", "default_transport="+mailRelayService); err != nil {
			putBack(err.Error())
			return
		}
		if err := run("postconf", "-e", "relayhost="+nexthop); err != nil {
			putBack(err.Error())
			return
		}
		step("main.cf sends mail for other servers through " + nexthop)
	} else {
		if state != transport.MailRelayStateOwner && facts.relayhost != "" {
			if err := run("postconf", "-X", "relayhost"); err != nil {
				putBack(err.Error())
				return
			}
		}
		if facts.defaultTransport == mailRelayService {
			if err := run("postconf", "-X", "default_transport"); err != nil {
				putBack(err.Error())
				return
			}
		}
		if facts.servicePresent {
			if err := run("postconf", "-MX", mailRelayServiceEntry); err != nil {
				putBack(err.Error())
				return
			}
		}
		for _, path := range []string{mailRelayMapPath, mailRelayMapPath + ".db", mailRelayMapPath + ".lmdb", mailRelayMapPath + ".cdb"} {
			if err := mailRelayRemove(path); err != nil {
				putBack("the relay sign-in file " + path + " could not be removed (" + boundedNativeDetail(err.Error()) + ")")
				return
			}
		}
		step("CelikPanel's relay lines, its service and its sign-in file were removed")
	}
	if err := mailRelayReload(); err != nil {
		putBack("Postfix did not take the change (" + boundedNativeDetail(err.Error()) + ")")
		return
	}
	step("Postfix took the change")
	resp.Applied, resp.Changed = true, true
	if req.Enabled {
		resp.State = transport.MailRelayStateOn
	} else if state == transport.MailRelayStateOwner {
		resp.State = transport.MailRelayStateOwner
	} else {
		resp.State = transport.MailRelayStateOff
	}
}
