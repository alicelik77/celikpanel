# guest (root, read-only): exact BIND vendor unit + environment bytes and package facts. Lab only.
grep PRETTY /etc/os-release
for f in /usr/lib/systemd/system/named.service /etc/default/named; do echo "-- $f $(sha256sum < $f | cut -c1-64) $(stat -c '%a %U:%G %s' $f)"; od -c $f | tail -3; python3 -c 'import sys; print(repr(open(sys.argv[1]).read()))' $f; done
dpkg -S /etc/default/named 2>&1; grep -A1 'named' /var/lib/dpkg/info/bind9.conffiles 2>/dev/null
grep 'systemd/system/named.service' /var/lib/dpkg/info/bind9.md5sums
systemctl show named.service -p Type -p ExecStart -p Names -p FragmentPath -p DropInPaths -p SourcePath -p Transient -p Id
