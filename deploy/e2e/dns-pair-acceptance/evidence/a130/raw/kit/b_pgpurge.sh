# B lane item 5 (purge): the lab database deleted, then the whole PostgreSQL component removed through the panel:
# packages purged (no rc, no client-NN), cluster files gone, archive if non-fresh. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B5pg-$CELL.log) 2>&1
echo "== purge: before $(ts)"; grun $N g_pgl.sh
admn $N DELETE /api/v1/domains/$(cat $T/pgb.id)/databases/1 --label db-del --timeout 300 | sbrief 400
admn $N GET "/api/v1/service/removal-impact?service_id=postgresql" --label impact-pg | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); s=b.get("simulation") or {}
print(d["status"], "removal", b.get("removal"), "packages", b.get("packages"), "remove", s.get("remove"), "removes_clusters", s.get("removes_clusters"), "holds", s.get("holds_databases"), "refuses", s.get("refuses"), "plan", json.dumps(b.get("plan"))[:300])'
admn $N POST /api/v1/service/uninstall '{"service_id":"postgresql"}' --label rm-pg --timeout 900 | sbrief 1200
sleep 3; grun $N g_pgl.sh; gshn $N 'dpkg -l | grep -E "^rc" | awk "{print \$1,\$2}"; ls /etc/postgresql /var/lib/postgresql /var/log/postgresql 2>&1'
audit 'postgresql|uninstall' 8
echo "== purge done $(ts)"
