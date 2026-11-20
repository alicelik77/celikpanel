//go:build linux

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/configmatrix"
)

// OpenDKIM (20 Nov 2026, mail signed only for the sender's own domain;
// mail_signing_bind.go). The configuration CelikPanel writes on this release
// (with SenderMacro where the release's OpenDKIM knows it) is checked with
// `opendkim -n`, and, where miltertest is installed, OpenDKIM is started with
// it and asked which domain it signs for: a login of one domain whose From:
// names another domain, a login sending with the null sender, mail made on
// the server without a login, and an unauthenticated client.

const matrixDKIMScript = "/run/celikpanel-matrix/dkim-sign.lua"

// matrixDKIMKey is a fresh RSA key in the PEM form opendkim-genkey writes.
func matrixDKIMKey(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

// matrixDKIMLua is the miltertest script: each line of `want` is one message
// and the d= domain it must be signed with ("-": no signature).
func matrixDKIMLua(cases []matrixDKIMMessage) string {
	var b strings.Builder
	b.WriteString(`-- CelikPanel distro config matrix: which domain does OpenDKIM sign for?
local unpack = table.unpack or unpack
local function message(name, ip, macros, sender, from, want)
	local conn = mt.connect("inet:8891@127.0.0.1", 40, 0.25)
	if conn == nil then error(name .. ": cannot connect to OpenDKIM") end
	if mt.conninfo(conn, "client.matrix.test", ip) ~= nil then error(name .. ": conninfo failed") end
	if #macros > 0 then mt.macro(conn, SMFIC_MAIL, unpack(macros)) end
	if mt.mailfrom(conn, sender) ~= nil then error(name .. ": mailfrom failed") end
	if mt.rcptto(conn, "<someone@outside.test>") ~= nil then error(name .. ": rcptto failed") end
	if mt.header(conn, "From", from) ~= nil then error(name .. ": header failed") end
	if mt.header(conn, "Subject", "matrix") ~= nil then error(name .. ": header failed") end
	if mt.header(conn, "Date", "Fri, 20 Nov 2026 12:00:00 +0000") ~= nil then error(name .. ": header failed") end
	if mt.eoh(conn) ~= nil then error(name .. ": eoh failed") end
	if mt.bodystring(conn, "matrix\r\n") ~= nil then error(name .. ": body failed") end
	if mt.eom(conn) ~= nil then error(name .. ": eom failed") end
	local signed = mt.eom_check(conn, MT_HDRINSERT, "DKIM-Signature") or mt.eom_check(conn, MT_HDRADD, "DKIM-Signature")
	local got = "-"
	if signed then
		local sig = mt.getheader(conn, "DKIM-Signature", 0)
		got = string.match(sig or "", "d=([^;%s]+)") or "?"
	end
	mt.disconnect(conn)
	if got ~= want then error(name .. ": signed for " .. got .. ", want " .. want) end
	print(name .. ": " .. got)
end
`)
	for _, c := range cases {
		macros := make([]string, 0, len(c.macros))
		for _, m := range c.macros {
			macros = append(macros, fmt.Sprintf("%q", m))
		}
		fmt.Fprintf(&b, "message(%q, %q, {%s}, %q, %q, %q)\n", c.name, c.ip, strings.Join(macros, ", "), c.sender, c.from, c.want)
	}
	return b.String()
}

type matrixDKIMMessage struct {
	name, ip     string
	macros       []string
	sender, from string
	want         string
}

func renderMatrixOpenDKIM(t *testing.T, h *matrixHost) {
	layout, ok := opendkimLayoutFor(h.family)
	if !ok {
		h.notApplicable(t, "opendkim.conf", "opendkim", "configuration", "cmd/agent/dkim_signing_rpc.go renderOpenDKIMConf", "no OpenDKIM layout for this family")
		return
	}
	if _, err := exec.LookPath("opendkim"); err != nil {
		h.notApplicable(t, "opendkim.conf", "opendkim", "configuration", "cmd/agent/dkim_signing_rpc.go renderOpenDKIMConf", "OpenDKIM is not installed on this release")
		return
	}
	run := func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).CombinedOutput() }
	supported, known := openDKIMSupportsSenderMacro(run)
	if !known {
		h.renderFailure(t, "opendkim.conf", "opendkim", "configuration", "cmd/agent/mail_signing_bind.go openDKIMSupportsSenderMacro",
			fmt.Errorf("the SenderMacro probe could not be run"))
		return
	}
	c := &configmatrix.Case{
		ID: "opendkim.conf", Program: "opendkim", Validator: "opendkim",
		Variant: fmt.Sprintf("CelikPanel's opendkim.conf (SenderMacro: %v) and two signing domains", supported),
		Source:  "cmd/agent/dkim_signing_rpc.go renderOpenDKIMConf / mail_signing_bind.go desiredOpenDKIMConf",
		Target:  layout.confPath,
		Extra:   map[string]string{"DKIM_SENDER_MACRO": map[bool]string{true: "yes", false: "no"}[supported], "DKIM_SCRIPT": matrixDKIMScript},
	}
	c.File(layout.confPath, desiredOpenDKIMConf(layout, supported), 0o644)
	domains := []string{"a.matrix.test", "b.matrix.test"}
	keyTable, signingTable, _, err := renderDKIMTables(nil, nil, domains)
	if err != nil {
		h.renderFailure(t, "opendkim.conf", "opendkim", "configuration", "cmd/agent/dkim_origin.go renderDKIMTables", err)
		return
	}
	c.File(filepath.Join(opendkimTablesDir, "keytable"), keyTable, 0o640)
	c.File(filepath.Join(opendkimTablesDir, "signingtable"), signingTable, 0o640)
	c.File(filepath.Join(opendkimTablesDir, "trustedhosts"), []byte("127.0.0.1\n::1\nlocalhost\n"), 0o640)
	owners := []string{dkimStoreRoot + "=root:opendkim", opendkimTablesDir + "=root:opendkim", dkimBaseDir + "=root:opendkim"}
	for _, table := range []string{"keytable", "signingtable", "trustedhosts"} {
		owners = append(owners, filepath.Join(opendkimTablesDir, table)+"=root:opendkim")
	}
	for _, domain := range domains {
		key, err := dkimKeyPath(domain, signingSelector)
		if err != nil {
			h.renderFailure(t, "opendkim.conf", "opendkim", "configuration", "cmd/agent/dkim_rpc.go dkimKeyPath", err)
			return
		}
		c.File(key, matrixDKIMKey(t), 0o640)
		owners = append(owners, filepath.Dir(key)+"=root:opendkim", key+"=root:opendkim")
	}
	c.Extra["OWNERS"] = strings.Join(owners, " ")
	c.Extra["RESTORE_DIRS"] = dkimStoreRoot
	login := []string{"{auth_type}", "PLAIN", "{auth_authen}", "anna@a.matrix.test", "{mail_addr}", "anna@a.matrix.test"}
	nullLogin := []string{"{auth_type}", "PLAIN", "{auth_authen}", "anna@a.matrix.test", "{mail_addr}", ""}
	c.File(matrixDKIMScript, []byte(matrixDKIMLua([]matrixDKIMMessage{
		{"login, own From", "203.0.113.9", login, "<anna@a.matrix.test>", "Anna <anna@a.matrix.test>", "a.matrix.test"},
		{"login, another customer's From", "203.0.113.9", login, "<anna@a.matrix.test>", "CEO <ceo@b.matrix.test>", "a.matrix.test"},
		{"login, null sender, another customer's From", "203.0.113.9", nullLogin, "<>", "CEO <ceo@b.matrix.test>", "a.matrix.test"},
		{"made on the server (sendmail of root or vmail)", "127.0.0.1", nil, "<notices@b.matrix.test>", "Notices <notices@b.matrix.test>", "b.matrix.test"},
		{"no login, not this server", "203.0.113.9", nil, "<x@b.matrix.test>", "X <x@b.matrix.test>", "-"},
	})), 0o644)
	h.write(t, c)
}
