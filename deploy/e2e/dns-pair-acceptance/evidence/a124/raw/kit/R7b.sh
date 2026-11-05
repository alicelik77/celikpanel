# alpha.124 case 7, second pass on p124 main: the site user sends to another server (PHP mail() and SMTP to
# localhost:25 without a password); the Postfix page's list; the sending mailbox's 587 send. Lab only.
export CELL=p124 IP=192.0.3.190 Z=p124.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
exec > >(tee -a $LOGS/R7b-$CELL.log) 2>&1
pwrep() { adm GET /api/v1/mail/passwordless --label passwordless --timeout 90 | python3 -c '
import json,sys
d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print("  report", d.get("status"), b[:300]); sys.exit()
print("  report", d.get("status"), "state", j.get("state"), "reason", j.get("reason"), "window_days", j.get("window_days"), "covered_since", j.get("covered_since"))
for s in j.get("sites") or []: print("    site", json.dumps(s))
for o in j.get("others") or []: print("    other", json.dumps(o))'; }
echo "== 7.2b the site p124.test sends to outside@example.net $(ts)"
lab ssh $CELL main -- "sudo -n tee /root/g_mail7b.sh >/dev/null" < $A/g_mail7b.sh
timeout 200 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/g_mail7b.sh p124.test p124_test outside@example.net"
echo "== 7.3b the Postfix page's 30-day list $(ts)"
for i in 1 2 3 4 5 6; do pwrep > $T/pw.txt; cat $T/pw.txt; grep -q '"domain": "p124.test"' $T/pw.txt && break; sleep 20; done
echo "== 7.5b authenticated submission 587 with noreply@p124.test to another server $(ts)"
lab ssh $CELL main -- "sudo -n tee /root/g_mail587.sh >/dev/null" < $A/g_mail587.sh
pw mbx7 | timeout 120 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/g_mail587.sh noreply@p124.test outside@example.net"
sleep 10; pwrep
gsh1 'postconf -h authorized_submit_users smtpd_relay_restrictions | tr "\n" ";"; echo; postqueue -j | wc -l'
echo "== R7b done $(ts)"
