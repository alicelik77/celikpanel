# guest (root, read-only): the BIND unit as systemd and dpkg see it; usrmerge state. Lab only.
grep PRETTY /etc/os-release
systemctl show -p FragmentPath -p ActiveState -p SubState named.service
dpkg -S /lib/systemd/system/named.service 2>&1 | head -2; dpkg -S /usr/lib/systemd/system/named.service 2>&1 | head -2
stat -c '%N %U:%G' /lib; dpkg -l bind9 | grep ^ii | awk '{print $2, $3}'
dig +short @127.0.0.1 SOA $(hostname -d) 2>/dev/null | head -1
