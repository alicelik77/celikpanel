# alpha.123 helpers (sourced after lib.sh, lib11.sh). Lab only.
comps() { adm GET /api/v1/managed-services --label ms > $T/ms.json; python3 -c '
import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); L=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
want=sys.argv[2].split(",")
for s in L:
  if s.get("id") in want: print("  comp", json.dumps({k:s.get(k) for k in ("id","is_installed","status","versions","needs_missing","requires","install_blocked_by")})[:500])' $T/ms.json "$1"; }
impact() { adm GET "/api/v1/service/removal-impact?$1" --label impact > $T/imp.json; python3 -c '
import json,sys; d=json.load(open(sys.argv[1])); b=d["body"]
try: j=json.loads(b)
except Exception: print("  impact", d["status"], b[:400]); sys.exit()
p=j.get("plan") or {}
print("  impact", d["status"], "together", p.get("together"), "stops", json.dumps(p.get("stops")), "withdraws", p.get("withdraws"))
for x in p.get("blockers") or []: print("    blocker", json.dumps(x)[:600])' $T/imp.json; }
together_of() { python3 -c 'import json,sys; j=json.loads(json.load(open(sys.argv[1]))["body"]); print(json.dumps([x for x in (j.get("plan") or {}).get("together") or []]))' $T/imp.json; }
uninst() { adm POST /api/v1/service/uninstall "$1" --label "uninstall" --timeout 600 | sbrief ${2:-1500}; }
opread() { adm GET "/api/v1/service/operation?$1" --label opread | python3 -c '
import json,sys; d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print("  read", d.get("status"), b[:300]); sys.exit()
op=j.get("operation") if isinstance(j,dict) else None
if not op: print("  read", d.get("status"), json.dumps(j)[:400]); sys.exit()
print("  read", d.get("status"), json.dumps({k:op.get(k) for k in ("id","kind","service_id","package_name","status","phase","request_id","result","error")})[:700])'; }
# follow an operation by id until it ends, printing each new phase (reads only)
follow() { local id=$1 last="" s i; for i in $(seq 1 ${2:-120}); do s=$(opread "id=$id"); [ "$s" != "$last" ] && { echo "  $(ts)$s"; last="$s"; }; case "$s" in *'"status": "succeeded"'*|*'"status": "failed"'*) return 0;; esac; sleep 3; done; echo "  still running after bound"; }
panelup() { local i; for i in $(seq 1 60); do adm GET /api/v1/panel/version --label ver 2>/dev/null | grep -q '"status": 200' && { echo "  panel answers again $(ts)"; return 0; }; sleep 2; done; echo "  panel not back"; }
