# guest (root): LAB LOAD for case 4 (intermittent) - one extra SQLite writer holds the panel database's write lock
# (BEGIN IMMEDIATE) H s, frees it F s, N times, as the panel's own user; every hold is rolled back (writes nothing). Lab only.
N=${1:-12}; H=${2:-7}; F=${3:-1}
U=$(systemctl show -p User --value celikpanel-panel.service); [ -n "$U" ] || U=root
DB=/var/lib/celikpanel/celikpanel.db
systemctl reset-failed lab-dbhold 2>/dev/null
systemd-run --unit=lab-dbhold -p User=$U python3 -c "
import sqlite3,time,sys
c=sqlite3.connect(sys.argv[1], isolation_level=None, timeout=30)
n,h,f=int(sys.argv[2]),float(sys.argv[3]),float(sys.argv[4])
print('cycles start', time.strftime('%T'), flush=True)
for i in range(n):
    c.execute('BEGIN IMMEDIATE'); time.sleep(h); c.execute('ROLLBACK'); time.sleep(f)
print('cycles end', time.strftime('%T'), flush=True)" "$DB" "$N" "$H" "$F"
sleep 1; systemctl is-active lab-dbhold; journalctl -u lab-dbhold -o cat --no-pager | tail -1
