# topology A prep: two Unlimited customers, c118.test (php) with mailboxes and file access. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/A0-$CELL.log) 2>&1
echo "== A0 prep $(ts)"
for c in cust118a cust118b; do mkcred $c
  python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-$c.json $T/u.json
  adm POST /api/v1/users @$T/u.json --label create-$c | sbrief 300; done; rm -f $T/u.json
SUBA=$(sub_of cust118a); SUBB=$(sub_of cust118b); echo "subA=$SUBA subB=$SUBB"; echo $SUBA > $T/suba; echo $SUBB > $T/subb
infra delegate c118.test ns1.$Z $IP ns2.$Z $IP > /dev/null
DID=$(newdomain cust118a $SUBA c118.test); echo "c118.test id=$DID"; echo $DID > $T/did
for m in box:100 q:1; do mkcred mbx-${m%%:*}
  python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":sys.argv[2],"password":c["password"],"quota_mb":int(sys.argv[3])},open(sys.argv[4],"w"))' $T/cred-mbx-${m%%:*}.json ${m%%:*}@c118.test ${m#*:} $T/m.json
  echo "-- mailbox ${m%%:*}@c118.test quota ${m#*:} MB"; as cust118a POST /api/v1/domains/$DID/mail/accounts @$T/m.json | sbrief 300; done; rm -f $T/m.json
mkcred fa118
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(json.dumps({"enabled":True,"ssh":False,"password":c["password"],"add_keys":[],"remove_keys":[]}))' $T/cred-fa118.json > $T/fa.json
echo "-- file access on"; as cust118a PUT /api/v1/domains/$DID/access @$T/fa.json | sbrief 300; rm -f $T/fa.json
as cust118a GET /api/v1/domains/$DID/access > $T/access.json; FU=$(jget connection.username < $T/access.json); echo "file user=$FU"; echo $FU > $T/fu
echo "-- files at /"; as cust118a GET "/api/v1/domains/$DID/files?path=/" | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], b.get("current_path"), [f["name"] for f in b.get("files") or []])'
echo "-- subscription usage"; as cust118a GET /api/v1/my/subscription | sbrief 1500
echo "-- usage status"; adm GET /api/v1/usage/status --label usage-status | sbrief 1500
GQT=60 gq gA0.sh $DID
echo "== A0 done $(ts)"
