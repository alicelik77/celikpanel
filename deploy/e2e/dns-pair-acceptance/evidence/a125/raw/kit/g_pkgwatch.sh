# guest (root), lab load standing for a package task that starts while the update prepares its recovery runtime
# (e.g. Debian's daily apt-listchanges/dpkg-db-backup at midnight): as soon as the staged recovery runtime's
# read-only checker runs, a process named "dpkg" runs for $1 s. A transient unit, bounded to 300 s.
cp /usr/bin/sleep /root/dpkg
systemd-run --unit=lab-pkgwatch -p RuntimeMaxSec=300 bash -c "
end=\$((SECONDS+280))
while [ \$SECONDS -lt \$end ]; do
  if pgrep -f '/usr/libexec/celikpanel/recovery-runtimes/v1/[0-9a-f]+/bin/(panel|agent)-checker' >/dev/null; then echo \"checker seen \$(date -u +%T.%N)\"; /root/dpkg ${1:-20}; echo \"dpkg-named process ended \$(date -u +%T.%N)\"; exit 0; fi
  sleep 0.05; done; echo 'no checker seen'"
sleep 1; systemctl is-active lab-pkgwatch
