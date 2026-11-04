# Lab images round, Ubuntu 22.04 (u22): sandbox probe; if full, Go probe as an uploaded program (item 9) + run.sh quick (item 18). Lab only.
export CELL=u22 IP=192.0.3.191 Z=u22.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit/lib.sh
source $A/probe.sh
exec > >(tee -a $LOGS/R12U-$CELL.log) 2>&1
echo "== host facts $(ts)"; GQT=60 gq g_facts.sh
echo "== sandbox probe $(ts)"; probe
LEVEL=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("sandbox") or {}).get("level",""))' $T/rta.json)
echo "level=$LEVEL"
[ "$LEVEL" = full ] || { echo "== level is not full: stop here $(ts)"; exit 0; }
echo "== uploaded program (Go probe) via panel-runtime.sh $(ts)"
mkcred custu
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-custu.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-custu | sbrief 200; rm -f $T/u.json
SUB=$(sub_of custu)
infra delegate progu.test ns1.$Z $IP ns2.$Z $IP > /dev/null
PR=$(newdomain custu $SUB progu.test letsencrypt); echo "progu.test id=$PR"
AS_TIMEOUT=300 as custu POST /api/v1/domains/$PR/ssl/letsencrypt '{"auto_renew":true}' | sbrief 200
DGP=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print([k for k in b["kinds"] if k["kind"]=="program"][0]["risk_digest"])' $T/rta.json)
adm POST /api/v1/admin/runtime-apps/program/offer "{\"risk_digest\":\"$DGP\"}" --label offer-prog | python3 -c 'import json,sys; d=json.load(sys.stdin); print("offer program", d["status"], d["body"][:300] if d["status"]!=200 else "")'
bash $A/kitpush.sh $CELL > /dev/null 2>&1; echo "kit pushed"
as custu GET /api/v1/me/guide-state > /dev/null
cat $T/cred-custu.json.cookie | timeout 1500 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash /root/kit/deploy/e2e/sandbox-escape/prt.sh $PR /nonexistent program" > $T/prt-progu.txt 2>&1; echo "rc=$?"
cut -c1-300 $T/prt-progu.txt; cp $T/prt-progu.txt $LOGS/panel-runtime-program-$CELL.txt
echo "-- admin view after the program run"; probe
echo "== run.sh quick (item 18) $(ts)"
timeout 1200 python3 -I $L/lab.py ssh $CELL main -- "sudo -n bash -c 'cd /root/kit/deploy/e2e/sandbox-escape && ./run.sh quick 2>&1'" | grep -vE "^\s*$" > $LOGS/runsh-quick-$CELL.txt; tail -45 $LOGS/runsh-quick-$CELL.txt
echo "== R12U done $(ts)"
