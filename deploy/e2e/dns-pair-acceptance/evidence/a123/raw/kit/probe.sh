# Read the host sandbox level the way the Programs offering does (admin, read-only). usage: source after lib.sh; probe NAME. Lab only.
probe() { adm GET /api/v1/admin/runtime-apps --label rta > $T/rta.json
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("  runtime-apps", d["status"], "sandbox", json.dumps(b.get("sandbox")))
for k in b.get("kinds") or []: print("  kind", json.dumps({x:k.get(x) for x in ("kind","offering_id","offered","accepted_level","accepted_gaps","level_gap","available","reason")})[:600])' $T/rta.json
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); json.dump({"status":d["status"],"sandbox":b.get("sandbox"),"kinds":[{x:k.get(x) for x in k if x not in ("risk_digest",)} for k in b.get("kinds") or []]}, open(sys.argv[2],"w"), indent=1)' $T/rta.json $LOGS/runtime-apps-$CELL.json
  adm GET /api/v1/admin/service-apps --label sva > $T/sva.json
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("  service-apps", d["status"], "sandbox", json.dumps(b.get("sandbox"))[:900])' $T/sva.json; }
