//go:build linux

package dnspeerenrollowner

// TestRenderDistroConfigMatrix renders the sudoers rule, sshd Match block,
// sshd_config Include line and forced-command wrapper of both DNS peer
// inspector profiles (BIND and PowerDNS) into CELIKPANEL_CONFIG_MATRIX_OUT.
// See deploy/test-distro-config-matrix.sh; it runs inside a clean container
// of each release, so the stock /etc/ssh/sshd_config is the distribution's.

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/alicelik/celikpanel/internal/configmatrix"
)

func TestRenderDistroConfigMatrix(t *testing.T) {
	out := configmatrix.OutDir()
	if out == "" {
		t.Skip("set CELIKPANEL_CONFIG_MATRIX_OUT (deploy/test-distro-config-matrix.sh) to render the distro config matrix")
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	stockSSHD, _ := os.ReadFile("/etc/ssh/sshd_config")
	for _, v := range []struct {
		id      string
		p       *profile
		options SecondaryOptions
	}{
		{"bind", bindProfile, SecondaryOptions{PrimaryIP: "192.0.2.10", PeerIP: "192.0.2.20", CatalogName: "catalog-c000020a.celikpanel.invalid"}},
		{"pdns", pdnsProfile, SecondaryOptions{PrimaryIP: "192.0.2.10", PeerIP: "192.0.2.20", CatalogName: "catalog-c000020a.celikpanel.invalid", CatalogAccount: "celikpanel"}},
	} {
		_, wrapper, sudoers, sshd, _ := v.p.renderFiles(v.options, key)
		source := "internal/dnspeerenrollowner/secondary_linux.go:131 renderFiles (" + v.p.label + " profile, engine_linux.go)"
		write := func(c *configmatrix.Case) {
			if err := c.Write(out); err != nil {
				t.Fatalf("write case %s: %v", c.ID, err)
			}
		}
		c := &configmatrix.Case{ID: "sudo.peer-inspector-" + v.id, Program: "sudo", Variant: v.p.label + " peer inspector sudoers rule",
			Validator: "sudoers", Source: source, Target: v.p.sudoers}
		c.File(v.p.sudoers, sudoers, 0o440)
		write(c)

		c = &configmatrix.Case{ID: "openssh.peer-inspector-" + v.id, Program: "openssh", Variant: v.p.label + " peer inspector Match block + sshd_config Include",
			Validator: "sshd", Source: source + " + engine_linux.go:188 sshdInclude",
			Target: "/etc/ssh/sshd_config", Extra: map[string]string{"USERS": v.p.account}}
		c.File(v.p.sshdConfig, sshd, 0o644)
		if stockSSHD != nil {
			c.File("/etc/ssh/sshd_config", append(append([]byte(nil), stockSSHD...), []byte(v.p.sshdInclude())...), 0o644)
		}
		write(c)

		c = &configmatrix.Case{ID: "hooks.peer-inspector-wrapper-" + v.id, Program: "openssh", Variant: v.p.label + " forced-command wrapper",
			Validator: "shell", Source: source, Target: v.p.wrapper}
		c.File(v.p.wrapper, wrapper, 0o755)
		write(c)
	}
}
