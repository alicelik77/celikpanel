# guest (root, read-only): certbot scheduler states and the update snapshot's service-state ledger.
for u in certbot.timer certbot-renew.timer; do echo "$u load=$(systemctl show -p LoadState --value $u) enabled=$(systemctl is-enabled $u 2>&1) active=$(systemctl is-active $u 2>&1)"; done
dpkg -l certbot 2>/dev/null | tail -1 | cut -c1-80
for f in $(find / -xdev -name 'service-states.tsv' -newermt '2026-11-06 00:20' 2>/dev/null); do echo "== $f"; cat "$f"; done
ls -la /var/lib/celikpanel-release-state/ 2>&1 | head -20
