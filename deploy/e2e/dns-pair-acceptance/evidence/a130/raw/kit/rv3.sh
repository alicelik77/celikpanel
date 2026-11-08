# alpha.130 re-verify on A (Debian 13, lab 131): Postfix status; mail "Set up now" and a certificate with the names
# resolving; a held "starting" boot: remove an address (works), add one (503), create a domain (503 HOST_STARTING). Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R3a-$CELL.log) 2>&1
b1 login $CELL main > /dev/null 2>&1
nup n_mail.sh > /dev/null
nm() { timeout 120 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_mail.sh $*"; }
dnsset() { local D=$1; for ip in $MIP $BIP; do echo "  @$ip: autoconfig.$D CNAME=$(nm dig autoconfig.$D CNAME $ip) autodiscover.$D CNAME=$(nm dig autodiscover.$D CNAME $ip) SRV=$(nm dig _autodiscover._tcp.$D SRV $ip)"; done; }
R=/api/v1/database-remote
case "${1:-all}" in
postfix|all)
  echo "== Postfix status on Debian 13 $(ts)"
  admn main GET /api/v1/metrics/services --label msvc | python3 -c '
import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  service", s.get("id"), s.get("unit"), s.get("active_state"), s.get("sub_state"), s.get("running"), s.get("memory_bytes")) for s in b.get("services",[]) if s.get("id") in ("postfix","dovecot")]'
  admn main GET /api/v1/metrics/problems --label mprob | python3 -c '
import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("problems") if isinstance(b,dict) else b
print("  problems:", [(p.get("kind"), p.get("subject")) for p in (l or [])])' ;;
esac
case "${1:-all}" in
mail|all)
  for d in cust1.$Z mnew.test; do DID=$(domain_id $d main); echo "== Set up now: $d ($DID) $(ts)"
    adm POST /api/v1/domains/$DID/mail/autoconfig '{}' --label ac-now --timeout 300 | sbrief 700
    adm GET /api/v1/domains/$DID/mail/setup --label mailsetup | sbrief 900; done
  sleep 20; dnsset cust1.$Z; gshn main 'nginx -t 2>&1 | tail -1'
  nm autoconf mnew.test $MIP | grep -E "http |LoginName|^--" | head -10
  echo "== certificate again for cust1.$Z with the names resolving $(ts)"
  P=$(adm GET /api/v1/ssl/providers --label prov | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("providers") if isinstance(b,dict) else b; print(l[0]["id"])')
  adm POST /api/v1/domains/$(domain_id cust1.$Z main)/ssl/letsencrypt "{\"email\":\"admin@$Z\",\"auto_renew\":true,\"include_mail\":false,\"provider\":\"$P\",\"reissue\":true}" --label ssl-cust1-2 --timeout 600 | sbrief 900
  gshn main "echo | openssl s_client -connect 127.0.0.1:443 -servername autodiscover.cust1.$Z 2>/dev/null | openssl x509 -noout -ext subjectAltName"
  nm autoconf cust1.$Z $MIP | grep -E "http |LoginName|^--|Server>" | head -12 ;;
esac
case "${1:-all}" in
starting|all)
  echo "== a held starting boot $(ts)"
  grun main g_k2.sh hang; lab reboot $CELL main > /dev/null 2>&1; lab wait-ssh $CELL main 300 > /dev/null 2>&1
  for i in $(seq 1 18); do b1 login $CELL main > /dev/null 2>&1 && break; sleep 5; done
  grun main g_k2.sh state
  RM=$(as cust1 GET $R/databases/$(cat $T/rdb-mysql.id) | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=[r for r in (b.get("rules") or []) if r.get("address")=="192.0.3.1"]; print(l[0]["id"] if l else "")')
  echo "-- remove the MariaDB address (rule $RM)"; as cust1 DELETE $R/rules/$RM | sbrief 500; T1=$(date +%s)
  for i in $(seq 1 18); do s=$(grun main g_k2.sh state); echo "$s" | grep -q "3306 ip saddr 192.0.3.1" || { echo "   3306 address gone from the filter after $(( $(date +%s) - T1 )) s"; break; }; sleep 5; done
  echo "$s"
  as cust1 GET $R/databases/$(cat $T/rdb-mysql.id) | sbrief 900
  echo "-- add an address while starting:"; as cust1 POST $R/databases/$(cat $T/rdb-mysql.id)/rules "{\"user_id\":$(cat $T/rdb-mysql.uid),\"address\":\"192.0.3.1\",\"duration\":\"1d\"}" | sbrief 500
  echo "-- create a domain while starting:"; infra delegate boot.test ns1.$Z $MIP ns2.$Z $BIP > /dev/null; adm POST /api/v1/domains/create '{"domain":"boot.test","project_type":"php","ssl_type":"none"}' --label create-boot --timeout 120 | sbrief 900
  echo "-- wait for the boot to finish (up to 7 min)"; for i in $(seq 1 42); do st=$(gshn main 'systemctl is-system-running'); case "$st" in running|degraded) break;; esac; sleep 10; done; echo "   is-system-running: $st $(ts)"
  sleep 60; grun main g_k2.sh nft; as cust1 GET $R/databases/$(cat $T/rdb-mysql.id) | sbrief 600
  grun main g_k2.sh unhang; gshn main 'systemctl reset-failed lab-hang.service 2>/dev/null; systemctl is-system-running' ;;
esac
echo "== rv3 ${1:-all} done $(ts)"
