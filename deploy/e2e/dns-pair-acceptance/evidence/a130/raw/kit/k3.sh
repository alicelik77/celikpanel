# alpha.130 round, item 3 (API part): the Offered tab and the Databases switch write the same offer. Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/K3rights-$CELL.log) 2>&1
b1 login $CELL main > /dev/null 2>&1
ov() { adm GET /api/v1/offered-versions --label offered | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"])
keep={k:v for k,v in b.items() if k not in ("hidden",)}
print("  offered-versions", d["status"], json.dumps(keep, ensure_ascii=False)[:900])'; }
sw() { adm GET /api/v1/database-remote/server --label rs | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  databases switch offered =", b.get("offered"))'; }
put() { adm PUT /api/v1/offered-versions "{\"offering_id\":\"$1\",\"offered\":$2}" --label offer-$2 | sbrief 700; }
echo "== $(ts) start"; ov; sw
echo "-- Offered tab: untick remote access"; put permission:database_remote false; ov; sw
echo "-- Offered tab: tick remote access"; put permission:database_remote true; ov; sw
echo "-- Databases switch off"; adm PUT /api/v1/database-remote/server '{"offered":false}' --label rs-off | sbrief 200; ov; sw
echo "-- Databases switch on"; adm PUT /api/v1/database-remote/server '{"offered":true}' --label rs-on | sbrief 200; ov; sw
echo "-- nginx directives: tick, read, untick"; put permission:nginx_directives true; ov; put permission:nginx_directives false; ov
echo "-- a program/Python row is read-only here"; put service_app:openwebui true; put runtime:python-3.12 false
echo "-- the audit"; adm GET "/api/v1/audit-logs?limit=30" --label audit | python3 -c '
import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("logs") or b.get("items") or b.get("entries") or (b if isinstance(b,list) else [])
for e in l:
    a=e.get("action","")
    if "offer" in a or "database.remote.offer" in a: print("  audit", e.get("created_at"), a, json.dumps(e.get("details"), ensure_ascii=False)[:200])'
echo "== k3 done $(ts)"
