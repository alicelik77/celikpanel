# alpha.130 round, item 5: on B after the update the leftover PostgreSQL 16 cluster (initdb only) goes by itself,
# with an audit line and an archive; Components edits 18's files. Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=backup
exec > >(tee -a $LOGS/C5pg-$CELL.log) 2>&1
echo "== $(ts) B clusters now"; grun $N g_pgl.sh
gshn $N 'journalctl -u celikpanel-panel -u celikpanel-agent --since -20min -o short-iso --no-pager | grep -iE "leftover|pg_leftover|cluster" | tail -12 | cut -c1-300'
admn $N GET "/api/v1/audit-logs?limit=40" --label audit > $T/audit-b.json; python3 -c '
import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); l=b.get("logs") or b.get("items") or b.get("entries") or (b if isinstance(b,list) else [])
for e in l:
    a=e.get("action","")
    if any(k in a for k in ("postgresql","offering","database.remote")): print("  audit", e.get("created_at"), a[:200], (e.get("details") or "")[:200])' $T/audit-b.json
admn $N GET /api/v1/service/postgresql/clusters --label pgclusters | sbrief 1500
echo "== c5pg done $(ts)"
