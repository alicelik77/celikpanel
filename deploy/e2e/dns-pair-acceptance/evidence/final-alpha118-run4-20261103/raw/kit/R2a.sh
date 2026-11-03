# run 2 prep + D2 (mailbox quota right after mail) + logrotate. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R2a-$CELL.log) 2>&1
echo "== R2a prep $(ts)"
for c in cust118a cust118b cust118n; do mkcred $c
  python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-$c.json $T/u.json
  adm POST /api/v1/users @$T/u.json --label create-$c | sbrief 200; done; rm -f $T/u.json
echo $(sub_of cust118a) > $T/suba; echo $(sub_of cust118b) > $T/subb; echo $(sub_of cust118n) > $T/subn
SUBA=$(cat $T/suba); echo "subA=$SUBA subB=$(cat $T/subb) subN=$(cat $T/subn)"
DID=$(newdomain cust118a $SUBA c118.test); echo "c118.test id=$DID"; echo $DID > $T/did
echo "-- usage status before the mailbox"; adm GET /api/v1/usage/status --label us | sbrief 900
mkcred mbx-q
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"address":"q@c118.test","password":c["password"],"quota_mb":1},open(sys.argv[2],"w"))' $T/cred-mbx-q.json $T/m.json
echo "-- D2: 1 MB mailbox q@c118.test $(ts)"; as cust118a POST /api/v1/domains/$DID/mail/accounts @$T/m.json | sbrief 200; rm -f $T/m.json
T0=$(date +%s)
sleep 15; echo "-- 950 KB $(ts)"; lc smtpsize - q@c118.test 950000
for i in $(seq 1 12); do
  r=$(lc smtpsize - q@c118.test 300000 | grep RCPT); echo "  $(ts) +$(( $(date +%s) - T0 ))s 300 KB: $r"
  echo "$r" | grep -q "552" && break; sleep 30
done
adm GET /api/v1/usage/status --label us | sbrief 900
GQT=60 gq gW.sh q@c118.test
echo "-- logrotate"; gsh1 'command -v logrotate; dpkg -l logrotate | tail -1'
echo "== R2a done $(ts)"
