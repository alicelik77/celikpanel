export CELL=u118 IP=192.0.3.191 Z=u118.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/B7data-$CELL.log) 2>&1
echo "== B data on 118 $(ts)"
mkcred custb
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-custb.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-custb | sbrief 200; rm -f $T/u.json
SUB=$(sub_of custb); echo $SUB > $T/sub
infra delegate cb118.test ns1.$Z $IP ns2.$Z $IP > /dev/null
DID=$(newdomain custb $SUB cb118.test); echo "cb118.test id=$DID"; echo $DID > $T/did
