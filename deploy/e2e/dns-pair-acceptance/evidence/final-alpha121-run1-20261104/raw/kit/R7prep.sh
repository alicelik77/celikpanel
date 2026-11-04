# alpha.119 A prep: customers, sites with certificates, another site's 0644 file; guide checks. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R7prep-$CELL.log) 2>&1
echo "== R7prep $(ts)"
for c in cust119a cust119b cust119n; do mkcred $c
  python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-$c.json $T/u.json
  adm POST /api/v1/users @$T/u.json --label create-$c | sbrief 200; done; rm -f $T/u.json
echo $(sub_of cust119a) > $T/suba; echo $(sub_of cust119b) > $T/subb; echo $(sub_of cust119n) > $T/subn
for spec in cust119a:suba:prog119.test cust119b:subb:py119.test cust119b:subb:flask119.test cust119b:subb:fast119.test cust119n:subn:n8n119.test; do
  IFS=: read cu sf dn <<< "$spec"
  infra delegate $dn ns1.$Z $IP ns2.$Z $IP > /dev/null
  id=$(newdomain $cu $(cat $T/$sf) $dn letsencrypt); echo "$dn id=$id"; echo $id > $T/id-$dn
done
OID=$(newdomain cust119b $(cat $T/subb) other119.test); echo "other119.test id=$OID"
python3 $A/mkup.py $T/t.json secret.txt text:other-site-secret-119 2>/dev/null; as cust119b POST "/api/v1/domains/$OID/files?path=/" @$T/t.json | sbrief 100
sleep 20
for spec in cust119a:prog119.test cust119b:py119.test cust119b:flask119.test cust119b:fast119.test cust119n:n8n119.test; do IFS=: read cu dn <<< "$spec"; echo "  $dn certificate: $(as $cu GET /api/v1/domains/$(cat $T/id-$dn)/runtime-apps | jget certificate | cut -c1-200)"; done
echo "== item 1 guide $(ts)"
gsh1 'curl -sk -o /tmp/help.html -w "help http %{http_code} bytes %{size_download}\n" https://127.0.0.1:1923/help; curl -sk -o /dev/null -w "help chapter http %{http_code}\n" https://127.0.0.1:1923/help/getting-started; grep -o "<title>[^<]*" /tmp/help.html'
GQT=60 gq g_guide7.sh
echo "-- guide-state as customer"; as cust119a GET /api/v1/me/guide-state | sbrief 200
as cust119a PUT /api/v1/me/guide-state '{"tour_version":1,"tour_state":"skipped"}' | sbrief 200
as cust119a GET /api/v1/me/guide-state | sbrief 200
echo "-- guide-state as admin"; adm GET /api/v1/me/guide-state --label gs | sbrief 200
echo "== R7prep done $(ts)"
