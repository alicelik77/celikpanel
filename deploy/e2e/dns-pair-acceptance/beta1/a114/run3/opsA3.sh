#!/bin/bash
# alpha.114 run 2, topology A (Debian 13 first server + Ubuntu 24.04 joined, both candidate). Lab only.
# E2, E8 (Components mail in place against an alpha.114 first server), E6, E4 (Debian + Ubuntu), E3,
# checks 2/6/7/9/10, the forgot-password command, deletes, reboots, panel-off.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
source $H/common.sh
CELL=a4-a; MIP=192.0.3.150; BIP=192.0.3.151; Z=fa114.test; S=shop114.test; JD=custb114.test; CD=cust114.test
SH="python3 -I $H/shots.py"
OW="python3 -I $H/owner.py $CELL"
export CELL MIP BIP
exec > >(tee -a $LOGS/opsA.log) 2>&1
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
labq() { local f=$1; shift; lab ssh beta1-lab labnet -- "sudo -n tee /root/q-$CELL.sh >/dev/null" < "$H/$f"; lab ssh beta1-lab labnet -- "sudo -n bash /root/q-$CELL.sh $*"; }
pwfile() { python3 -c 'import json,sys,os; d=json.load(open(sys.argv[1])); f=sys.argv[3]; open(f,"w").write(json.dumps({"password": d.get(sys.argv[2]) or d[sys.argv[2].replace("-new","")]})); os.chmod(f,0o600)' /var/tmp/cp-beta1/state/$CELL/owner-secrets.json $1 /var/tmp/a114r3/pw-$1.json; echo /var/tmp/a114r3/pw-$1.json; }
dnsrec() { for ip in $MIP $BIP; do echo "   @$ip $1: MX=[$(dig1 $ip $1 MX)] SPF=[$(dig1 $ip $1 TXT | tr -d '"' | grep -o 'v=spf1[^"]*' | head -1)] DMARC=[$(dig1 $ip _dmarc.$1 TXT | cut -c1-60)] DKIM=[$(dig1 $ip celik._domainkey.$1 TXT | cut -c1-40)]"; done; }
opwatch() { # NODE SECONDS LOGFILE : the setup operation every 15 s (typed waits) until it ends
  local n=$1 secs=$2 f=$3 t0=$(date +%s) last=""
  while [ $(( $(date +%s) - t0 )) -lt $secs ]; do
    o=$(b1 api $CELL $n GET /api/v1/setup/operation --label mailext-watch --limit 400000 2>/dev/null | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}"); op=b.get("operation",b)
chk=[(c.get("id"),c.get("state") or c.get("status"),c.get("reason"),c.get("vars")) for c in (op.get("checks") or op.get("verification",{}).get("checks",[]) if isinstance(op.get("verification"),dict) else op.get("checks") or [])]
print(op.get("status"), op.get("phase"), json.dumps(op.get("error"))[:300], " ".join(f"{s.get(\"id\")}={s.get(\"status\")}" for s in op.get("steps",[])), json.dumps(chk)[:600])' 2>&1 | cut -c1-1400)
    [ "$o" != "$last" ] && { echo "$(ts) $o" >> $f; last=$o; }
    case "$o" in succeeded*|failed*) break;; esac
    sleep 15
  done
}

echo "== check 2: the main domain's website opened at setup $(ts)"
labq webcheck.sh $Z | grep -E '^resolves|^---|code=|^location|issuer|Verify|subject='
echo "-- main domain mail records before anything else (must not change later)"
dnsrec $Z | tee /var/tmp/a114r3/mainrec-before.txt

echo "== run-6 ops on the first server $(ts)"
CELL=$CELL MIP=$MIP BIP=$BIP Z=$Z S=$S PANEL=frankfurt ZONEDEL=gone114.test timeout 2400 bash $H/ops4.sh 2>&1 | grep -vE '^\{|^ +"' | cut -c1-400

echo "== mail on the joined server again (API; E8 was proven in the browser before the host reboot) $(ts)"
$OW backup mailext_taken mail.$Z
$OW backup mailext mail.boston.$Z
echo "-- mail name published, main domain records unchanged"
for ip in $MIP $BIP; do echo "   @$ip mail.boston.$Z A=[$(dig1 $ip mail.boston.$Z A)]"; done
dnsrec $Z > /var/tmp/a114r3/mainrec-after-e8.txt; diff /var/tmp/a114r3/mainrec-before.txt /var/tmp/a114r3/mainrec-after-e8.txt && echo "   main domain MX/SPF/DMARC/DKIM unchanged"
#$SH $CELL backup a-e8-after '[{"name":"components","path":"/services","wait":3500},{"name":"dns-page","path":"/dns-infrastructure","wait":3500}]' --langs en,tr --widths desktop

echo "== E6: a customer domain with a mailbox on the joined server $(ts)"
infra delegate $JD ns1.$Z $MIP ns2.$Z $BIP > /dev/null
$OW backup mailpair $JD mail.boston.$Z $S mail.$Z
sleep 20
echo "-- the joined server's customer domain records, from both DNS servers"
dnsrec $JD | tee /var/tmp/a114r3/jdrec.txt
echo "-- DKIM of the mail carol@$JD sent to alice@$S, verified against each DNS server"
python3 -c 'import json,sys; print(json.dumps({"to": json.load(open(sys.argv[1]))["password"]}))' /var/tmp/cp-beta1/state/$CELL/mbx-alice.json > /var/tmp/a114r3/apw.json
lab ssh beta1-lab labnet -- "sudo -n python3 -I /root/dkimcheck.py mail.$Z alice@$S $MIP,$BIP 'joined to first'" < /var/tmp/a114r3/apw.json | python3 -c '
import json,sys
d=json.load(sys.stdin)
for m in d["mails"]: print("  ", m["subject"], "| found" if m.get("found", True) else "| NOT FOUND", "| sigs", m.get("dkim_signatures"), "|", [{k: v for k, v in c.items() if k in ("d","s") or k.startswith("verify")} for c in m.get("dkim", [])])'
rm -f /var/tmp/a114r3/apw.json
dnsrec $Z > /var/tmp/a114r3/mainrec-after-e6.txt; diff /var/tmp/a114r3/mainrec-before.txt /var/tmp/a114r3/mainrec-after-e6.txt && echo "   main domain MX/SPF/DMARC/DKIM unchanged"

echo "== check 6: accounts $(ts)"
infra delegate $CD ns1.$Z $MIP ns2.$Z $BIP > /dev/null
$OW main accounts $S mail.$Z $CD
$OW main invitelink $S mail.$Z
echo "== check 7: site features $(ts)"
$OW main sitefeatures $S mail.$Z
for n in main backup; do b1 sh $CELL $n "nginx -t 2>&1 | tail -1" --label nginx-t | python3 -c 'import json,sys; print(sys.argv[1], json.load(sys.stdin)["stdout"].strip())' $n; done

echo "== check 9 + 10: owner screens (admin) $(ts)"
$SH $CELL main a-owner $H/items-owner.json
$SH $CELL backup a-backup-owner '[{"name":"dashboard","path":"/","wait":3500},{"name":"components","path":"/services","wait":3500},{"name":"dns-page","path":"/dns-infrastructure","wait":3500},{"name":"domains","path":"/domains","wait":3000},{"name":"settings-panel-port","path":"/settings?section=panel","wait":3000}]'
echo "-- reseller and customer views"
$SH $CELL main a-reseller '[{"name":"dashboard","path":"/","wait":3500},{"name":"users","path":"/users","wait":3000},{"name":"users-plans","path":"/users","wait":2500,"actions":[{"clickText":["button,[role=tab]","Service plans|Servis planları|Plans|Planlar"]},{"wait":1500}]},{"name":"account","path":"/account","wait":3000}]' --user res1 --pwfile $(pwfile res1-new)
$SH $CELL main a-customer '[{"name":"dashboard","path":"/","wait":3500},{"name":"domains","path":"/domains","wait":3000},{"name":"domain","path":"/domains/cust114.test","wait":3000},{"name":"domain-mail","path":"/domains/cust114.test?tab=mail","wait":3000},{"name":"users-team","path":"/users","wait":3000},{"name":"account","path":"/account","wait":3000}]' --user cust1 --pwfile $(pwfile cust1)
$SH $CELL main a-team '[{"name":"dashboard","path":"/","wait":3500},{"name":"domain","path":"/domains/cust114.test?tab=dns","wait":3000}]' --user team1 --pwfile $(pwfile team1) --langs en
echo "-- public pages: login, forgot password, invitation"
$SH $CELL main a-public '[{"name":"login","path":"/","wait":2500},{"name":"forgot-password","path":"/","wait":2500,"actions":[{"click":"[data-forgot-open]"},{"wait":1500},{"probe":"(document.querySelector(\"[data-owner-reset-command] code\")||{}).innerText","label":"owner command"}]}]' --no-login
[ -f /var/tmp/cp-beta1/state/$CELL/invite-link-path ] && $SH $CELL main a-public '[{"name":"invitation-accept","path":"@file:/var/tmp/cp-beta1/state/a4-a/invite-link-path","wait":3000}]' --no-login --langs en,tr --widths desktop
echo "-- S23: the command from the forgot-password page in a root terminal"
$OW main forgotcmd
echo "-- log in as: the banner"
CID=$(python3 -c 'import json; print(json.load(open("/var/tmp/cp-beta1/state/a4-a/owner-secrets.json"))["ids"]["cust"])')
$SH $CELL main a-loginas "[{\"name\":\"log-in-as-banner\",\"path\":\"/\",\"wait\":2500,\"actions\":[{\"eval\":\"fetch('/api/v1/users/$CID/impersonate',{method:'POST'})\"},{\"wait\":2000},{\"eval\":\"location.assign('/domains')\"},{\"wait\":4000},{\"shot\":\"customer-domains\"},{\"eval\":\"fetch('/api/v1/auth/unimpersonate',{method:'POST'})\"},{\"wait\":1500}]}]" --widths desktop
echo "== check 6b: 2FA recovery codes $(ts)"
$OW main twofa
echo "== check 9: Add domain in the browser, the row appears without a reload $(ts)"
infra delegate ui114.test ns1.$Z $MIP ns2.$Z $BIP > /dev/null
$SH $CELL main a-ui-add '[{"name":"add-domain","path":"/domains","wait":3000,"actions":[{"shot":"before"},{"clickText":["button","Add domain"]},{"wait":1500},{"type":["input[placeholder=\"example.com\"]","ui114.test"]},{"shot":"dialog"},{"click":"form button[type=submit]"},{"waitText":["ui114.test",120000]},{"wait":20000},{"shot":"after20s"},{"probe":"[...document.querySelectorAll(\"nav a\")].map(a=>a.innerText.replace(/\\s+/g,\" \")).filter(t=>/Domains|Alan/.test(t)).join(\" | \")","label":"nav counter"}]}]' --langs en --widths desktop
$SH $CELL main a-ui-dash '[{"name":"dashboard-after-add","path":"/","wait":3500}]' --langs en --widths desktop
rm -f /var/tmp/a114r3/pw-*.json

echo "== E6 delete: the joined server's customer domain is deleted; its mail records go from both DNS servers $(ts)"
JID=$(domain_id $JD backup); echo "   $JD id on the joined server: $JID"
b1 api $CELL backup DELETE /api/v1/domains/$JID --label delete-jd --limit 6000 --timeout 400 | brief
sleep 30
dnsrec $JD
for ip in $MIP $BIP; do echo "   @$ip SOA $JD: $(rcode $ip $JD)"; done
dnsrec $Z > /var/tmp/a114r3/mainrec-after-del.txt; diff /var/tmp/a114r3/mainrec-before.txt /var/tmp/a114r3/mainrec-after-del.txt && echo "   main domain MX/SPF/DMARC/DKIM unchanged"

echo "== run-6 basics: reboot both, panel off on the first server, site delete $(ts)"
CELL=$CELL MIP=$MIP BIP=$BIP Z=$Z S=$S PANEL=frankfurt NODES="main backup" PANELOFF=1 timeout 3600 bash $H/after4.sh 2>&1 | grep -vE '^\{|^ +"' | cut -c1-400
echo "== A ops done $(ts)"
