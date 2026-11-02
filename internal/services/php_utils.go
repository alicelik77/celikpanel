package services

import (
	"os/exec"

	"github.com/alicelik/celikpanel/internal/transport"
)

// reloadPHPFPM is a shared utility to reload PHP-FPM. A variable so tests can
// stub it — pool-writer tests must not depend on a real unit being present on
// the machine running them.
// reloadPHPFPM, PHP-FPM'i yeniden yüklemek için ortak yardımcıdır. Testler
// taklit edebilsin diye değişkendir — havuz yazıcısı testleri, koştukları
// makinede gerçek bir unit'in varlığına bağımlı olmamalıdır.
var reloadPHPFPM = func(version string) error {
	// The unit is the host's own: `php<version>-fpm` on Debian and Sury, the
	// single `php-fpm` on Arch (php_layout.go).
	cmd := exec.Command("systemctl", "reload", phpLayoutFor(version).unit())
	return cmd.Run()
}

// reloadPHPFPMIfRunning reloads a PHP-FPM unless systemd says it is not
// running: a stopped PHP-FPM reads its pools when it next starts. A site
// moving off a version stopped in a security emergency (D-043) must not fail
// because that version cannot be reloaded (a masked unit refuses it). An
// unknown answer still reloads, as before.
func reloadPHPFPMIfRunning(version string) error {
	if phpFPMActive(version) == transport.SiteFileServiceActiveFalse {
		return nil
	}
	return reloadPHPFPM(version)
}
