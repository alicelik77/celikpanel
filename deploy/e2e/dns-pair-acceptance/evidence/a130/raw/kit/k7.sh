# alpha.130 round, item 7: mail on A (Debian 13, PowerDNS primary; B is the BIND secondary). usage: k7.sh PART... Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/K7mail-$CELL.log) 2>&1
b1 login $CELL main > /dev/null 2>&1
nup n_mail.sh > /dev/null; nup n_swaks.sh > /dev/null
nm() { timeout 120 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_mail.sh $*"; }
C1=$(cat $T/cust1.id); S1=$(cat $T/site1.id)
MS=$(domain_id $Z main)
setup() { adm GET /api/v1/domains/$1/mail/setup --label mailsetup | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if (d.get("body") or "").startswith("{") else {}
print("  setup", d["status"], json.dumps(b.get("autoconfig"))[:900])'; }
dnsset() { local D=$1; for ip in $MIP $BIP; do echo "  @$ip: autoconfig.$D CNAME=$(nm dig autoconfig.$D CNAME $ip) autodiscover.$D CNAME=$(nm dig autodiscover.$D CNAME $ip) _autodiscover._tcp.$D SRV=$(nm dig _autodiscover._tcp.$D SRV $ip)"; done; }
srvs() { adm GET /api/v1/mail/server-settings --label mss | sbrief ${1:-1500}; }
relay() { adm GET /api/v1/mail/relay --label relay | sbrief 1200; }
relaybody() { python3 -c 'import json,os,sys; fd=os.open(sys.argv[1],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600); os.write(fd,json.dumps({"enabled":sys.argv[2]=="on","host":"192.0.3.1","port":int(sys.argv[3]),"username":"relayuser@relay.lab.test","password":sys.argv[4]}).encode())' $T/relay.json $1 $2 "$3"; }
pf() { gshn main 'postconf -n | grep -E "^(relayhost|default_transport|smtp_sasl|smtp_tls_security_level|recipient_delimiter|smtpd_sender_restrictions|smtp_tls_wrappermode)" ; ls /etc/postfix | grep -iE "celikpanel|relay|sender" | xargs'; }
for part in "$@"; do echo; echo "######## $part $(ts)"; case $part in
ac)
  gshn main 'nginx -t 2>&1 | tail -1'
  for d in $Z site1.$Z cust1.$Z; do echo "-- $d"; setup $(domain_id $d main); done
  for d in $Z cust1.$Z; do echo "-- $d answers"; nm autoconf $d $MIP; dnsset $d; done ;;
acnow)
  for d in site1.$Z cust1.$Z; do echo "-- Set up now: $d"; adm POST /api/v1/domains/$(domain_id $d main)/mail/autoconfig "{}" --label ac-now --timeout 300 | sbrief 900; setup $(domain_id $d main); done; sleep 20; dnsset cust1.$Z; for ip in $MIP $BIP; do echo "  @$ip mail.cust1.$Z A=$(nm dig mail.cust1.$Z A $ip)"; done ;;
newdom)
  echo "-- a new domain with mail: mnew.test"; infra delegate mnew.test ns1.$Z $MIP ns2.$Z $BIP > /dev/null
  adm POST /api/v1/domains/create '{"domain":"mnew.test","project_type":"php","ssl_type":"none"}' --label create-mnew --timeout 300 | sbrief 300
  ND=$(domain_id mnew.test main); echo "mnew id $ND"; echo $ND > $T/mnew.id
  mkcred mnewbox; adm POST /api/v1/domains/$ND/mail/accounts "{\"address\":\"info@mnew.test\",\"password\":\"$(pw mnewbox)\",\"quota_mb\":100}" --label mbox-mnew --timeout 300 | sbrief 300
  sleep 30; setup $ND; dnsset mnew.test; gshn main 'nginx -t 2>&1 | tail -1'; nm autoconf mnew.test $MIP ;;
deldom)
  ND=$(cat $T/mnew.id); echo "-- delete mnew.test ($ND)"; adm DELETE /api/v1/domains/$ND --label del-mnew --timeout 300 | sbrief 400
  for i in $(seq 1 24); do s=$(adm GET /api/v1/domains/$ND/deletion-status --label delst | sbrief 200); echo "$s" | grep -qE "^(404|204)" && break; sleep 10; done; echo "  deletion: $s"
  sleep 20; dnsset mnew.test; gshn main 'ls /etc/nginx/sites-available | grep -c mnew; grep -rl "mnew.test" /etc/nginx 2>/dev/null | head -3' ;;
certwith)
  P=$(adm GET /api/v1/ssl/providers --label prov | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("providers") if isinstance(b,dict) else b; print(l[0]["id"])')
  echo "-- certificate for cust1.$Z (autoconfig names resolve here)"; adm POST /api/v1/domains/$C1/ssl/letsencrypt "{\"email\":\"admin@$Z\",\"auto_renew\":true,\"include_mail\":false,\"provider\":\"$P\",\"reissue\":false}" --label ssl-cust1 --timeout 600 | sbrief 900
  gshn main "echo | openssl s_client -connect 127.0.0.1:443 -servername cust1.$Z 2>/dev/null | openssl x509 -noout -issuer -ext subjectAltName 2>&1 | tr '\n' ' '"; echo
  nm autoconf cust1.$Z $MIP | head -8 ;;
certwithout)
  P=$(adm GET /api/v1/ssl/providers --label prov | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("providers") if isinstance(b,dict) else b; print(l[0]["id"])')
  echo "-- remove the autoconfig/autodiscover CNAMEs of $Z, then a certificate for $Z"
  for n in autoconfig autodiscover; do RI=$(record_id $MS $n.$Z CNAME); echo "   record $n $RI"; [ -n "$RI" ] && adm DELETE /api/v1/domains/$MS/dns/records/$RI --label rec-del --timeout 120 | sbrief 200; done
  sleep 20; dnsset $Z
  adm POST /api/v1/domains/$MS/ssl/letsencrypt "{\"email\":\"admin@$Z\",\"auto_renew\":true,\"include_mail\":false,\"provider\":\"$P\",\"reissue\":false}" --label ssl-main --timeout 600 | sbrief 900
  gshn main "echo | openssl s_client -connect 127.0.0.1:443 -servername $Z 2>/dev/null | openssl x509 -noout -issuer -ext subjectAltName 2>&1 | tr '\n' ' '"; echo
  setup $MS
  echo "-- Set up now puts the missing records back"; adm POST /api/v1/domains/$MS/mail/autoconfig '{}' --label ac-now-main --timeout 300 | sbrief 600; sleep 20; dnsset $Z; setup $MS ;;
relay)
  relay; pf
  echo "-- test sign-in, wrong password (587)"; relaybody on 587 "Wrong-$(date +%s)-pass"; adm POST /api/v1/mail/relay/test @$T/relay.json --label relay-test-wrong --timeout 120 | sbrief 500
  echo "-- test sign-in, right password (587)"; relaybody on 587 "$(pw relayuser)"; adm POST /api/v1/mail/relay/test @$T/relay.json --label relay-test --timeout 120 | sbrief 500
  echo "-- save relay 587"; adm POST /api/v1/mail/relay @$T/relay.json --label relay-save --timeout 300 | sbrief 900; rm -f $T/relay.json
  sleep 15; relay; pf
  echo "-- a mail out through the relay (box1 -> someone@example.com, submitted on A:587)"
  pw box1 | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 box1@site1.$Z someone@example.com box1@site1.$Z"
  sleep 15; nm relaylog 8
  echo "-- port 465"; relaybody on 465 ""; adm POST /api/v1/mail/relay @$T/relay.json --label relay-save-465 --timeout 300 | sbrief 600; rm -f $T/relay.json
  sleep 15; pf
  pw box1 | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 box1@site1.$Z other@example.com box1@site1.$Z"
  sleep 15; nm relaylog 6 ;;
relayoff)
  relaybody off 465 ""; adm POST /api/v1/mail/relay @$T/relay.json --label relay-off --timeout 300 | sbrief 600; rm -f $T/relay.json
  sleep 10; pf; relay
  echo "-- the owner's own relayhost: save refused"; gshn main 'postconf -e "relayhost = [192.0.3.1]:587"; postfix reload 2>&1 | tail -1'
  relaybody on 587 "$(pw relayuser)"; adm POST /api/v1/mail/relay @$T/relay.json --label relay-owner --timeout 300 | sbrief 600; rm -f $T/relay.json
  gshn main 'postconf -X relayhost; postfix reload 2>&1 | tail -1; postconf -n | grep -c relayhost' ;;
subaddr)
  srvs 1500
  echo "-- info+lab@cust1.$Z from labnet"; nm swaks tester@lab.test info+lab@cust1.$Z
  sleep 8; gshn main "grep -rl 'a130 lab' /var/vmail/cust1.$Z/info 2>/dev/null | head -3; find / -path /proc -prune -o -type d -name 'info' -path '*cust1*' -print 2>/dev/null | head -3; journalctl -u postfix@- --since -2min -o cat --no-pager | grep -E 'info\\+lab|status=' | tail -3 | cut -c1-220" ;;
sender)
  srvs 600
  adm POST /api/v1/mail/sender-rules '{"op":"add","value":"spam.example","action":"block"}' --label sr-add1 --timeout 300 | sbrief 500
  adm POST /api/v1/mail/sender-rules '{"op":"add","value":"boss@spam.example","action":"allow"}' --label sr-add2 --timeout 300 | sbrief 500
  echo "-- own domain refused:"; adm POST /api/v1/mail/sender-rules "{\"op\":\"add\",\"value\":\"cust1.$Z\",\"action\":\"block\"}" --label sr-own --timeout 120 | sbrief 300
  sleep 10; srvs 1500; pf
  echo "-- x@spam.example -> info@cust1"; nm swaks x@spam.example info@cust1.$Z
  echo "-- boss@spam.example -> info@cust1"; nm swaks boss@spam.example info@cust1.$Z
  adm GET /api/v1/mail/server-settings --label mss > $T/mss.json
  for id in $(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(" ".join(str(r["id"]) for r in b["sender"]["rules"]))' $T/mss.json); do adm POST /api/v1/mail/sender-rules "{\"op\":\"remove\",\"id\":$id}" --label sr-rm --timeout 300 | sbrief 300; done
  sleep 10; srvs 800; pf; gshn main 'ls -la /etc/postfix | grep -iE "sender|celikpanel"' ;;
sasl)
  echo "-- the owner installs the SASL modules as the refusal says"; gshn main 'DEBIAN_FRONTEND=noninteractive apt-get install -y -q libsasl2-modules >/dev/null 2>&1; echo rc=$?; dpkg -l libsasl2-modules | tail -1 | cut -c1-80'
  relaybody on 587 "$(pw relayuser)"; adm POST /api/v1/mail/relay @$T/relay.json --label relay-save2 --timeout 300 | sbrief 900; rm -f $T/relay.json
  sleep 10; relay; pf; gshn main 'ls -la /etc/postfix | grep -i relay'
  pw box1 | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 box1@site1.$Z someone@example.com box1@site1.$Z"
  sleep 15; nm relaylog 12
  relaybody on 465 ""; adm POST /api/v1/mail/relay @$T/relay.json --label relay-465b --timeout 300 | sbrief 600; rm -f $T/relay.json
  sleep 10; pf
  pw box1 | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 box1@site1.$Z other@example.com box1@site1.$Z"
  sleep 15; nm relaylog 8
  relaybody off 465 ""; adm POST /api/v1/mail/relay @$T/relay.json --label relay-off2 --timeout 300 | sbrief 600; rm -f $T/relay.json
  sleep 10; pf; gshn main 'ls -la /etc/postfix | grep -i relay; postconf -n | grep -E "relay|default_transport|sasl" '
  echo "-- subaddressing delivery and the sender list allow line"
  nm swaks tester@lab.test info+lab@cust1.$Z; sleep 5
  gshn main "journalctl --since -1min -o cat --no-pager | grep -E \"info\+lab|to=<info\" | tail -3 | cut -c1-200"
  adm POST /api/v1/mail/sender-rules '{"op":"add","value":"spam.example","action":"block"}' --label sr-add1 --timeout 300 | sbrief 200
  adm POST /api/v1/mail/sender-rules '{"op":"add","value":"boss@spam.example","action":"allow"}' --label sr-add2 --timeout 300 | sbrief 200
  sleep 5; echo "   x:"; nm swaks x@spam.example info@cust1.$Z; echo "   boss:"; nm swaks boss@spam.example info@cust1.$Z
  adm GET /api/v1/mail/server-settings --label mss > $T/mss.json
  for id in $(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(" ".join(str(r["id"]) for r in b["sender"]["rules"]))' $T/mss.json); do adm POST /api/v1/mail/sender-rules "{\"op\":\"remove\",\"id\":$id}" --label sr-rm --timeout 300 | sbrief 200; done
  sleep 5; pf ;;
r465)
  nup n_fix465.sh >/dev/null; nrun n_fix465.sh
  relaybody on 465 "$(pw relayuser)"; adm POST /api/v1/mail/relay/test @$T/relay.json --label relay-test-465 --timeout 120 | sbrief 400
  adm POST /api/v1/mail/relay @$T/relay.json --label relay-465c --timeout 300 | sbrief 600; rm -f $T/relay.json
  sleep 8; gshn main 'postqueue -f; postconf -P | grep celikpanel-relay'; sleep 15; nm relaylog 6
  pw box1 | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 box1@site1.$Z third@example.com box1@site1.$Z"
  sleep 15; nm relaylog 4
  relaybody off 465 ""; adm POST /api/v1/mail/relay @$T/relay.json --label relay-off3 --timeout 300 | sbrief 400; rm -f $T/relay.json
  sleep 8; gshn main 'ls -la /etc/postfix | grep -ci relay; postconf -n | grep -cE "relayhost|default_transport"; postconf -M | grep -c celikpanel-relay'
  echo "-- boss allowed"; adm POST /api/v1/mail/sender-rules '{"op":"add","value":"spam.example","action":"block"}' --label sr1 --timeout 300 | sbrief 150
  adm POST /api/v1/mail/sender-rules '{"op":"add","value":"boss@spam.example","action":"allow"}' --label sr2 --timeout 300 | sbrief 150
  sleep 5; timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "swaks --server $MIP --port 25 --from boss@spam.example --to info@cust1.$Z --timeout 30 2>&1 | grep -E '^<' | tail -3"
  adm GET /api/v1/mail/server-settings --label mss > $T/mss.json
  for id in $(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(" ".join(str(r["id"]) for r in b["sender"]["rules"]))' $T/mss.json); do adm POST /api/v1/mail/sender-rules "{\"op\":\"remove\",\"id\":$id}" --label sr-rm --timeout 300 | sbrief 150; done ;;
esac; done
echo "== k7 $* done $(ts)"
