package hostname

import (
	"os"
	"strings"
)

// LocalFQDN returns this server's full name the way `hostname -f` finds it.
// Cloud images often keep the kernel name short ("boston") and write the full
// name only to /etc/hosts ("127.0.1.1 boston.example.com boston"). A kernel
// name that already has dots is used as is; otherwise the canonical name of
// the first /etc/hosts line listing the short name is used when it extends
// that name. Anything else falls back to the kernel name.
// LocalFQDN sunucunun tam adını `hostname -f` gibi bulur; çekirdek adı kısaysa
// /etc/hosts içinde kısa adı listeleyen ilk satırın kanonik adı kullanılır.
func LocalFQDN() (string, error) {
	return localFQDN(os.Hostname, "/etc/hosts")
}

func localFQDN(kernel func() (string, error), hostsPath string) (string, error) {
	name, err := kernel()
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, ".") {
		return name, nil
	}
	raw, err := os.ReadFile(hostsPath)
	if err != nil {
		return name, nil
	}
	short := strings.ToLower(name)
	for _, line := range strings.Split(string(raw), "\n") {
		if hash := strings.IndexByte(line, '#'); hash >= 0 {
			line = line[:hash]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		listed := false
		for _, alias := range fields[1:] {
			if strings.EqualFold(alias, short) {
				listed = true
				break
			}
		}
		if !listed {
			continue
		}
		// glibc stops at the first matching line, as `hostname -f` does.
		canonical := strings.ToLower(strings.TrimSuffix(fields[1], "."))
		if strings.HasPrefix(canonical, short+".") {
			if full, err := CanonicalFQDN(canonical); err == nil {
				return full, nil
			}
		}
		return name, nil
	}
	return name, nil
}
