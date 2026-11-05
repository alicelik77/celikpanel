# guest (root): LAB LOAD for case 4 - one extra SQLite writer holds the panel database's write lock
# (BEGIN IMMEDIATE) for N seconds, as the panel's own user, then rolls back (writes nothing). Lab only.
N=${1:-60}
U=$(systemctl show -p User --value celikpanel-panel.service); [ -n "$U" ] || U=root
DB=$(ls /var/lib/celikpanel/celikpanel.db 2>/dev/null || find /var/lib -maxdepth 3 -name celikpanel.db 2>/dev/null | head -1)
echo "panel user=$U db=$DB journal_mode=$(runuser -u $U -- python3 -c "import sqlite3,sys; print(sqlite3.connect(sys.argv[1]).execute('pragma journal_mode').fetchone()[0])" $DB)"
systemctl reset-failed lab-dbhold 2>/dev/null
systemd-run --unit=lab-dbhold -p User=$U python3 -c "import sqlite3,time,sys; c=sqlite3.connect(sys.argv[1], isolation_level=None, timeout=30); c.execute('BEGIN IMMEDIATE'); print('lock held', time.strftime('%T'), flush=True); time.sleep(int(sys.argv[2])); c.execute('ROLLBACK'); print('lock released', time.strftime('%T'), flush=True)" "$DB" "$N"
sleep 1; systemctl is-active lab-dbhold; journalctl -u lab-dbhold -o cat --no-pager | tail -2
