# alpha.127 case 6: administrator vs customer on the main: the Access tab's SSH tools notice (customer: provider
# wording; administrator: "Install the tools" in the panel's window, SSH option without reload), the webmail card. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=main
exec > >(tee -a $LOGS/C6-$CELL-$N.log) 2>&1
echo "== a customer with its own site $(ts)"
mkcred cust7
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust7.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-cust7 | sbrief 300; rm -f $T/u.json
SUB=$(sub_of cust7); echo "sub $SUB"
infra delegate cust7.test ns1.$Z $MIP ns2.$Z $BIP > /dev/null
CID=$(newdomain cust7 $SUB cust7.test); echo "cust7.test id $CID"
as cust7 PUT /api/v1/me/guide-state '{"tour_version":1,"tour_state":"skipped"}' | sbrief 200
SID=$(cat $T/site7.id)
echo "-- the access view's server facts (admin, API) $(ts)"
adm GET /api/v1/domains/$SID/access --label access | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b.get("server") or {}; print({k:s.get(k) for k in s if "jail" in k.lower() or "missing" in k.lower() or "ssh" in k.lower()}); print("rights", b.get("rights"))'
as cust7 GET /api/v1/domains/$CID/access | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); s=b.get("server") or {}; print("customer view:", {k:s.get(k) for k in s if "jail" in k.lower() or "missing" in k.lower()}, "rights", b.get("rights"))'
python3 $A/uiitems.py access-view cust7.test $T/acc-c.json customer
python3 $A/uiitems.py webmail cust7.test $T/wm-c.json customer
python3 -c 'import json,sys; json.dump(json.load(open(sys.argv[1]))+json.load(open(sys.argv[2])), open(sys.argv[3],"w"))' $T/acc-c.json $T/wm-c.json $T/c6-cust.json
echo "== customer: Access tab and webmail card (EN, TR) $(ts)"
timeout 1200 python3 -I $H/shots.py $CELL $N c6-cust $T/c6-cust.json --langs en,tr --widths desktop --user cust7 --pwfile $T/cred-cust7.json | python3 $A/shotsum.py
python3 $A/uiitems.py access-view site7.test $T/acc-a.json admin
python3 $A/uiitems.py webmail site7.test $T/wm-a.json admin
python3 -c 'import json,sys; json.dump(json.load(open(sys.argv[1]))+json.load(open(sys.argv[2])), open(sys.argv[3],"w"))' $T/acc-a.json $T/wm-a.json $T/c6-adm.json
echo "== administrator: Access tab and webmail card (EN, TR) $(ts)"
timeout 1200 python3 -I $H/shots.py $CELL $N c6-adm $T/c6-adm.json --langs en,tr --widths desktop | python3 $A/shotsum.py
echo "== administrator: Install the tools (EN) $(ts)"
python3 $A/uiitems.py access-install site7.test $T/acc-i.json
timeout 1500 python3 -I $H/shots.py $CELL $N c6-install $T/acc-i.json --langs en --widths desktop | python3 $A/shotsum.py
echo "== customer again after the install (EN) $(ts)"
timeout 900 python3 -I $H/shots.py $CELL $N c6-cust-after $T/acc-c.json --langs en --widths desktop --user cust7 --pwfile $T/cred-cust7.json | python3 $A/shotsum.py
echo "== c6 done $(ts)"
