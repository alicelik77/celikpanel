# guest (root, read only): traffic log, limit state, upload blocks, mail quota. usage: gL1.sh DOMAINID MAILBOX. Lab only.
D=$1; M=$2
echo "## $(date -u +%T) traffic log"; ls -la /var/log/celikpanel/traffic/ 2>&1 | grep -v total; wc -l /var/log/celikpanel/traffic/$D.log 2>&1
echo "## nginx -t"; nginx -t 2>&1 | tail -1
echo "## traffic include"; cat /etc/nginx/celikpanel-managed.d/c118.test/access-log.traffic 2>&1 | head -3; grep -l "celikpanel-traffic-limit" /etc/nginx/sites-enabled/* 2>/dev/null
echo "## php pool upload flag"; grep -rh "file_uploads" /etc/php/*/fpm/pool.d/ 2>/dev/null | head -3
echo "## sftp force command"; grep -n "ForceCommand\|Match" /etc/ssh/sshd_config.d/60-celikpanel-file-access.conf 2>&1 | head -6
echo "## db usage rows"; python3 - <<'P'
import sqlite3
db = sqlite3.connect("file:/var/lib/celikpanel/celikpanel.db?mode=ro", uri=True, timeout=10)
for t in ("subscription_disk_usage", "traffic_web_blocks", "traffic_daily"):
    try:
        cols=[r[1] for r in db.execute("pragma table_info(%s)" % t)]
        print(t, cols); [print("  ", r) for r in db.execute("select * from %s order by rowid desc limit 4" % t)]
    except Exception as e: print(t, "n/a", e)
P
echo "## mail quota $M"; doveadm quota get -u $M 2>&1 | head -4
echo "## maildir of $M"; d=$(doveadm user -f home $M 2>/dev/null); echo "home=$d"; find $d -type f -path '*/cur/*' -o -type f -path '*/new/*' 2>/dev/null | wc -l; grep -l -ri "quota" $d/Maildir/new $d/Maildir/cur $d/new $d/cur 2>/dev/null | head -3
echo "## postfix log (quota)"; journalctl -u postfix@- -u postfix --no-pager --since -20min 2>/dev/null | grep -iE "552|quota|5\.2\.2" | tail -5; grep -iE "552|5\.2\.2" /var/log/mail.log 2>/dev/null | tail -3
