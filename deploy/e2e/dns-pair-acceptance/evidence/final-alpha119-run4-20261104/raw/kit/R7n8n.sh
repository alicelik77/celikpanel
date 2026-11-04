# alpha.119 A item 5: n8n on sandbox generation 3. Lab only.
export STOPAFTERRUN=1
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R7n8n-$CELL.log) 2>&1
echo "== R7n8n $(ts)"
sed -e 's/n8n118\.test/n8n119.test/g; s/R3n8n-/R7n8nrun-/; s/cust118n/cust119n/g; s#^NID=$(newdomain cust119n $SUBN n8n119.test letsencrypt).*#NID=$(cat $T/nid); echo "n8n119.test id=$NID"#' $A/R3n8n.sh > $B/R7n8nrun.sh; grep -n "^NID=" $B/R7n8nrun.sh
echo "$(cat $T/id-n8n119.test)" > $T/nid
bash $B/R7n8nrun.sh
GQT=90 gq g_n8n7.sh celiksvc-1
adm GET /api/v1/admin/service-apps --label sa | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  admin sandbox:", json.dumps(b.get("sandbox"))[:600])'
echo "== R7n8n done $(ts)"
