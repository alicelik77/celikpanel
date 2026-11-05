# alpha.124 case 7 on p124 main (Debian 13 with mail): passwordless mail phase 1 (report only). Lab only.
export CELL=p124 IP=192.0.3.190 Z=p124.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
source $H/common.sh
exec > >(tee -a $LOGS/R7-$CELL.log) 2>&1
pwrep() { adm GET /api/v1/mail/passwordless --label passwordless --timeout 90 | python3 -c '
import json,sys
d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print("  report", d.get("status"), b[:300]); sys.exit()
print("  report", d.get("status"), "state", j.get("state"), "reason", j.get("reason"), "window_days", j.get("window_days"), "covered_since", j.get("covered_since"))
for s in j.get("sites") or []: print("    site", json.dumps(s))
for o in j.get("others") or []: print("    other", json.dumps(o))'; }
docroot() { python3 -c '
import json,sys
b=json.loads(json.load(open(sys.argv[1]))["body"])
def f(x):
    if isinstance(x,dict):
        for k,v in x.items():
            if k in ("document_root","doc_root","docroot","DocumentRoot") and isinstance(v,str) and v: return v
            r=f(v)
            if r: return r
    if isinstance(x,list):
        for v in x:
            r=f(v)
            if r: return r
print(f(b) or "")' $1; }
echo "== 7.0 the main site p124.test (new domains wait for the stopped backup DNS server) and a mailbox to receive $(ts)"
mkcred mbx7in; mkcred mbx7; D7=p124.test
DID=$(domain_id p124.test); echo "p124.test id=$DID"; echo $DID > $T/did7
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"inbox7@p124.test","password":c["password"],"quota_mb":50},open(sys.argv[2],"w"))' $T/cred-mbx7in.json $T/m.json
adm POST /api/v1/domains/$DID/mail/accounts @$T/m.json --label create-inbox | sbrief 200; rm -f $T/m.json
DR=/var/www/celikpanel/subscriptions/2/sites/1/public_html  # the nginx root of p124.test on the guest (read from sites-enabled)
echo "docroot: $DR"
echo "== 7.1 report before $(ts)"; pwrep
echo "== 7.2 the site sends with PHP mail() and with SMTP to localhost:25 without a password $(ts)"
lab ssh $CELL main -- "sudo -n tee /root/g_mail7.sh >/dev/null" < $A/g_mail7.sh
timeout 200 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/g_mail7.sh p124.test $DR inbox7@p124.test"
echo "== 7.3 the Postfix page's 30-day list $(ts)"
for i in 1 2 3 4 5 6; do pwrep > $T/pw.txt; cat $T/pw.txt; grep -q p124.test $T/pw.txt && break; sleep 20; done
echo "== 7.4 Create a sending mailbox (as the notice's button does) $(ts)"
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"noreply@p124.test","password":c["password"],"quota_mb":1024},open(sys.argv[2],"w"))' $T/cred-mbx7.json $T/m.json
adm POST /api/v1/domains/$DID/mail/accounts @$T/m.json --label create-sender | sbrief 400; rm -f $T/m.json
adm GET /api/v1/domains/$DID/mail/setup --label mail-setup | sbrief 1500
pwrep
echo "== 7.5 authenticated submission 587 with noreply@p124.test $(ts)"
lab ssh $CELL main -- "sudo -n tee /root/g_mail587.sh >/dev/null" < $A/g_mail587.sh
pw mbx7 | timeout 120 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/g_mail587.sh noreply@p124.test inbox7@p124.test"
echo "== 7.6 nothing blocked; delivered messages $(ts)"
gsh1 'postconf -h authorized_submit_users smtpd_relay_restrictions | tr "\n" ";"; echo; find / -xdev -path "*p124.test*inbox7*" -type f 2>/dev/null | grep -v "/proc/" | wc -l; postqueue -j | wc -l'
echo "== R7 done $(ts)"
