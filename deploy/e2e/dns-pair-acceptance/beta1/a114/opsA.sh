#!/bin/bash
# A (Debian 13 first server + Ubuntu 24.04 joined, alpha.114) checks 2-10. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab/env.sh
source $H/common.sh
CELL=a4-a; MIP=192.0.3.150; BIP=192.0.3.151; Z=fa114.test; S=shop114.test; JD=custb114.test; CD=cust114.test
SH="python3 -I $H/shots.py"
OW="python3 -I $H/owner.py $CELL"
export CELL MIP BIP
exec > >(tee -a $LOGS/opsA.log) 2>&1
lab ssh beta1-lab labnet -- "sudo -n tee /root/mailx.py >/dev/null" < $H/mailx.py
labq() { local f=$1; shift; lab ssh beta1-lab labnet -- "sudo -n tee /root/q-$CELL.sh >/dev/null" < "$H/$f"; lab ssh beta1-lab labnet -- "sudo -n bash /root/q-$CELL.sh $*"; }
pwfile() { python3 -c 'import json,sys,os; d=json.load(open(sys.argv[1])); f=sys.argv[3]; open(f,"w").write(json.dumps({"password": d.get(sys.argv[2]) or d[sys.argv[2].replace("-new","")]})); os.chmod(f,0o600)' /var/tmp/cp-beta1/state/$CELL/owner-secrets.json $1 /var/tmp/a114r1/pw-$1.json; echo /var/tmp/a114r1/pw-$1.json; }
dnsrec() { for ip in $MIP $BIP; do echo "   @$ip $1: MX=[$(dig1 $ip $1 MX)] SPF=[$(dig1 $ip $1 TXT | tr -d '"' | grep -o 'v=spf1[^ ]* [^"]*' | head -1)] DMARC=[$(dig1 $ip _dmarc.$1 TXT | cut -c1-60)] DKIM=[$(dig1 $ip celik._domainkey.$1 TXT | cut -c1-40)]"; done; }

echo "== check 2: the main domain's website opened at setup $(ts)"
labq webcheck.sh $Z | grep -E '^resolves|^---|code=|^location|issuer|Verify|subject='
echo "-- main domain mail records before anything else (must not change later)"
dnsrec $Z | tee /var/tmp/a114r1/mainrec-before.txt

echo "== run-6 ops on the first server $(ts)"
CELL=$CELL MIP=$MIP BIP=$BIP Z=$Z S=$S PANEL=frankfurt ZONEDEL=gone114.test timeout 2400 bash $H/ops4.sh 2>&1 | grep -vE '^\{|^ +"' | cut -c1-400

echo "== check 3a: a customer domain on the joined server sends and receives both ways $(ts)"
infra delegate $JD ns1.$Z $MIP ns2.$Z $BIP > /dev/null
$OW backup mailpair $JD mail.boston.$Z $S mail.$Z
sleep 20
echo "-- the joined server's customer domain records, from both DNS servers"
dnsrec $JD
echo "-- the main domain's mail records afterwards"
dnsrec $Z | tee /var/tmp/a114r1/mainrec-after.txt
diff /var/tmp/a114r1/mainrec-before.txt /var/tmp/a114r1/mainrec-after.txt && echo "   main domain MX/SPF/DMARC/DKIM unchanged"

echo "== check 4: port change refused on a paired server $(ts)"
$OW main port_paired
$OW backup port_paired

echo "== check 5: OS updates (Debian, Ubuntu) $(ts)"
$OW main osupdates
$OW backup osupdates

echo "== check 6: accounts $(ts)"
infra delegate $CD ns1.$Z $MIP ns2.$Z $BIP > /dev/null
$OW main accounts $S mail.$Z $CD
$OW main invitelink $S mail.$Z

echo "== check 7: site features $(ts)"
$OW main sitefeatures $S mail.$Z
$OW main autoresponder $S mail.$Z
for n in main backup; do b1 sh $CELL $n "nginx -t 2>&1 | tail -1" --label nginx-t | python3 -c 'import json,sys; print(sys.argv[1], json.load(sys.stdin)["stdout"].strip())' $n; done

echo "== check 9 + 10: owner screens (admin) $(ts)"
cat > /var/tmp/a114r1/items-owner.json <<'J'
[{"name":"dashboard","path":"/","wait":3500},
 {"name":"domains","path":"/domains","wait":3000},
 {"name":"domain-shop","path":"/domains/shop114.test","wait":3000},
 {"name":"domain-shop-hosting-protect","path":"/domains/shop114.test","wait":2500,"actions":[{"clickText":["button,[role=tab]","Hosting|Barındırma"]},{"wait":1500},{"clickText":["button,[role=tab]","Protected folders|Korumalı klasörler"]},{"wait":2000}]},
 {"name":"domain-shop-hosting-redirects","path":"/domains/shop114.test","wait":2500,"actions":[{"clickText":["button,[role=tab]","Hosting|Barındırma"]},{"wait":1500},{"clickText":["button,[role=tab]","Redirects|Yönlendirmeler"]},{"wait":2000}]},
 {"name":"domain-shop-mail","path":"/domains/shop114.test?tab=mail","wait":3000},
 {"name":"domain-shop-dns","path":"/domains/shop114.test?tab=dns","wait":3000},
 {"name":"domain-shop-ssl","path":"/domains/shop114.test?tab=ssl","wait":3000},
 {"name":"domain-main","path":"/domains/fa114.test","wait":3000},
 {"name":"components","path":"/services","wait":3500},
 {"name":"dns-page","path":"/dns-infrastructure","wait":3500},
 {"name":"security","path":"/security","wait":3500},
 {"name":"users","path":"/users","wait":3000},
 {"name":"users-plans","path":"/users","wait":2500,"actions":[{"clickText":["button,[role=tab]","Plans|Planlar"]},{"wait":1500}]},
 {"name":"account","path":"/account","wait":3000},
 {"name":"settings-panel-port","path":"/settings?section=panel","wait":3000},
 {"name":"audit","path":"/audit","wait":3000},
 {"name":"server-setup","path":"/server-setup","wait":3000}]
J
$SH $CELL main a-owner /var/tmp/a114r1/items-owner.json
$SH $CELL backup a-backup-owner '[{"name":"dashboard","path":"/","wait":3500},{"name":"components","path":"/services","wait":3500},{"name":"dns-page","path":"/dns-infrastructure","wait":3500},{"name":"security","path":"/security","wait":3500},{"name":"settings-panel-port","path":"/settings?section=panel","wait":3000}]'
echo "-- reseller and customer views"
$SH $CELL main a-reseller '[{"name":"dashboard","path":"/","wait":3500},{"name":"users","path":"/users","wait":3000},{"name":"users-plans","path":"/users","wait":2500,"actions":[{"clickText":["button,[role=tab]","Plans|Planlar"]},{"wait":1500}]},{"name":"account","path":"/account","wait":3000}]' --user res1 --pwfile $(pwfile res1-new)
$SH $CELL main a-customer '[{"name":"dashboard","path":"/","wait":3500},{"name":"domains","path":"/domains","wait":3000},{"name":"domain","path":"/domains/cust114.test","wait":3000},{"name":"domain-mail","path":"/domains/cust114.test?tab=mail","wait":3000},{"name":"users-team","path":"/users","wait":3000},{"name":"account","path":"/account","wait":3000}]' --user cust1 --pwfile $(pwfile cust1)
$SH $CELL main a-team '[{"name":"dashboard","path":"/","wait":3500},{"name":"domain","path":"/domains/cust114.test?tab=dns","wait":3000}]' --user team1 --pwfile $(pwfile team1) --langs en
echo "-- public pages: login, forgot password, invitation"
$SH $CELL main a-public '[{"name":"login","path":"/","wait":2500},{"name":"forgot-password","path":"/","wait":2500,"actions":[{"click":"[data-forgot-open]"},{"wait":1500}]}]' --no-login
[ -f /var/tmp/cp-beta1/state/$CELL/invite-link-path ] && $SH $CELL main a-public '[{"name":"invitation-accept","path":"@file:/var/tmp/cp-beta1/state/a4-a/invite-link-path","wait":3000}]' --no-login --langs en --widths desktop
echo "-- log in as: the banner"
CID=$(python3 -c 'import json; print(json.load(open("/var/tmp/cp-beta1/state/a4-a/owner-secrets.json"))["ids"]["cust"])')
$SH $CELL main a-loginas "[{\"name\":\"log-in-as-banner\",\"path\":\"/\",\"wait\":2500,\"actions\":[{\"eval\":\"fetch('/api/v1/users/$CID/impersonate',{method:'POST'})\"},{\"wait\":2000},{\"eval\":\"location.assign('/domains')\"},{\"wait\":4000},{\"shot\":\"customer-domains\"},{\"eval\":\"fetch('/api/v1/auth/unimpersonate',{method:'POST'})\"},{\"wait\":1500}]}]" --widths desktop
echo "== check 6b: 2FA recovery codes $(ts)"
$OW main twofa
echo "== check 9: Add domain in the browser, the row appears without a reload $(ts)"
infra delegate ui114.test ns1.$Z $MIP ns2.$Z $BIP > /dev/null
$SH $CELL main a-ui-add '[{"name":"add-domain","path":"/domains","wait":3000,"actions":[{"shot":"before"},{"clickText":["button","Add domain"]},{"wait":1500},{"type":["input[placeholder=\"example.com\"]","ui114.test"]},{"shot":"dialog"},{"click":"form button[type=submit]"},{"waitText":["ui114.test",120000]},{"wait":20000},{"shot":"after20s"},{"wait":40000},{"shot":"after60s"}]}]' --langs en --widths desktop
$SH $CELL main a-ui-dash '[{"name":"dashboard-after-add","path":"/","wait":3500}]' --langs en --widths desktop
rm -f /var/tmp/a114r1/pw-*.json
echo "== A ops done $(ts)"
