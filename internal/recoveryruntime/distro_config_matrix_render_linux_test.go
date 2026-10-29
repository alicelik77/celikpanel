//go:build linux

package recoveryruntime

// TestRenderDistroConfigMatrix renders the boot-resume unit the mail
// enrollment arms (mail_enrollment_boot_linux.go:43) into
// CELIKPANEL_CONFIG_MATRIX_OUT for systemd-analyze verify on every release.
// See deploy/test-distro-config-matrix.sh.

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/configmatrix"
)

func TestRenderDistroConfigMatrix(t *testing.T) {
	out := configmatrix.OutDir()
	if out == "" {
		t.Skip("set CELIKPANEL_CONFIG_MATRIX_OUT (deploy/test-distro-config-matrix.sh) to render the distro config matrix")
	}
	operation := strings.Repeat("c", 32)
	scope := mailEnrollmentScope{mailEnrollmentSchema, operation, strings.Repeat("d", 64), strings.Repeat("e", 64), "g" + strings.Repeat("f", 63)}
	target := "/etc/systemd/system/" + mailEnrollmentBootName(operation)
	c := &configmatrix.Case{ID: "systemd.mail-enrollment-resume", Program: "systemd", Variant: "mail enrollment boot-resume unit",
		Validator: "systemd-unit", Source: "internal/recoveryruntime/mail_enrollment_boot_linux.go:43 mailEnrollmentBootUnit", Target: target}
	c.File(target, mailEnrollmentBootUnit(scope), 0o644)
	if err := c.Write(out); err != nil {
		t.Fatal(err)
	}
}
