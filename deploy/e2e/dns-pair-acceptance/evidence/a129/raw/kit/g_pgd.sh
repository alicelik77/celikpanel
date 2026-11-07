systemctl is-active postgresql; journalctl -u postgresql --since -10min -o short-iso --no-pager | tail -25 | cut -c1-300
D=$(ls -d /var/lib/postgres/data 2>/dev/null); echo "data $D"; cat $D/postgresql.auto.conf 2>&1; grep -nE "^(ssl|shared_preload|listen)" $D/postgresql.conf 2>/dev/null; ls -la /etc/ssl/celikpanel-database/postgresql/ 2>&1; tail -8 $D/pg_hba.conf
