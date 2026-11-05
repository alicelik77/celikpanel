# alpha.124 case 6 on the PowerDNS main + backup pair (p124): the DNSSEC gate on a customer domain. Lab only.
export CELL=p2 IP=192.0.3.190 Z=p2.test BIP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
source $H/common.sh
exec > >(tee -a $LOGS/R6r-$CELL.log) 2>&1
zone() { echo "  @$1 SOA $(dig1 $1 $2 SOA) | DNSKEY [$(dig1 $1 $2 DNSKEY | cut -c1-60)]"; }
echo "== 6.0 a customer and a domain on the pair $(ts)"
mkcred cust6r
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust6r.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-cust6r | sbrief 200; rm -f $T/u.json
SUB=$(sub_of cust6r); echo $SUB > $T/sub6
infra delegate sec2.test ns1.$Z $IP ns2.$Z $BIP > /dev/null
DID=$(newdomain cust6r $SUB sec2.test); echo "sec2.test id=$DID"; echo $DID > $T/did6
for i in $(seq 1 20); do s=$(dig1 $BIP sec2.test SOA); [ -n "$s" ] && break; sleep 6; done
zone $IP sec2.test; zone $BIP sec2.test
gsh1 'pdnsutil show-zone sec2.test 2>&1 | head -5; pdnsutil list-keys sec2.test 2>&1 | head -4' | sed 's/^/  /'
echo "-- quiet window: wait until the zone serial stops moving $(ts)"; prev=""; for i in $(seq 1 20); do cur=$(dig1 $IP sec2.test SOA); [ -n "$cur" ] && [ "$cur" = "$prev" ] && break; prev=$cur; sleep 30; done; echo "  stable: $cur"; zone $IP sec2.test; zone $BIP sec2.test
echo "== 6.1 the DNS tab's read (GET /dnssec) as the admin and as the customer $(ts)"
adm GET /api/v1/domains/$DID/dnssec --label dnssec-get | sbrief 1500
as cust6r GET /api/v1/domains/$DID/dnssec | sbrief 1500
echo "== 6.2 sign called anyway (POST /dnssec) $(ts)"
adm POST /api/v1/domains/$DID/dnssec --label dnssec-post | sbrief 1500
as cust6r POST /api/v1/domains/$DID/dnssec | sbrief 800
echo "== 6.3 nothing changed in the zone $(ts)"
sleep 5; zone $IP sec2.test; zone $BIP sec2.test
gsh1 'pdnsutil show-zone sec2.test 2>&1 | head -5; pdnsutil list-keys sec2.test 2>&1 | head -4; journalctl -u celikpanel-agent -u celikpanel-panel --since -5min -o cat --no-pager | grep -iE "dnssec" | tail -5' | sed 's/^/  /'
echo "== R6r done $(ts)"
