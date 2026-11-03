# guest (root, read only): PHP 8.4 packages and apt sources. Lab only.
dpkg -l 'php8.4-*' | grep ^ii | awk '{print $2}' | tr '\n' ' '; echo
grep -rhE '^(deb|URIs|Suites)' /etc/apt/sources.list /etc/apt/sources.list.d/ 2>/dev/null | sort -u; ls /etc/apt/sources.list.d/
