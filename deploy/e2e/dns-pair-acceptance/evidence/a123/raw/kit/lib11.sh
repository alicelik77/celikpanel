# alpha.121 helpers (sourced after lib.sh). Lab only.
# installc SERVICE [PACKAGE]: preview, confirm every notable package (as the screen does), install, wait.
installc() { local svc=$1 pkg=${2:-} step R CONF
  step=$svc; [ -n "$pkg" ] && step="$svc:$pkg"
  adm GET "/api/v1/service/install-preview?step=$step" --label "prev-$svc" --timeout 150 > $T/prev.json; prevsum $T/prev.json
  CONF=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(json.dumps(sorted({x["package"] for x in (b.get("notable") or []) if x.get("package")})))' $T/prev.json)
  R=$(rid)
  if [ -n "$pkg" ]; then body="{\"service_id\":\"$svc\",\"package\":\"$pkg\",\"request_id\":\"$R\",\"check_installs\":true,\"confirmed_installs\":$CONF}"
  else body="{\"service_id\":\"$svc\",\"request_id\":\"$R\",\"check_installs\":true,\"confirmed_installs\":$CONF}"; fi
  echo "  install $step confirming $CONF"; svcstart "$body" /api/v1/service/install && svcwait $R 1200
  adm GET "/api/v1/service/operation?request_id=$R" --label "op-$svc" | sbrief 600; }
prevsum() { python3 -c '
import json,sys; d=json.load(open(sys.argv[1]));
try: b=json.loads(d["body"])
except Exception: print("  preview", d.get("status"), d.get("body","")[:300]); sys.exit()
print("  preview", d.get("status"), "state", b.get("state"), "lists_refreshed", b.get("lists_refreshed"), "reason_code", b.get("reason_code"), "reason", b.get("reason"), "refusals", json.dumps(b.get("refusals"))[:200])
for n in b.get("notable") or []: print("    notable", n.get("package"), n.get("reason"), n.get("component"), n.get("from"), "->", n.get("to"), "point_update", n.get("point_update"))
print("    installs", (b.get("installs") or [])[:14])' $1; }
lane() { adm GET "/api/v1/repo/lane?service_id=postgresql" --label lane > $T/lane.json; python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); p=b.get("plan") or {}; print("  lane", b.get("state"), "ready", p.get("ready"), "sha", (p.get("sha256") or "")[:12], "steps", len(p.get("steps") or []), "error", p.get("error") or b.get("error"))' $T/lane.json
  SHA=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("plan") or {}).get("sha256",""))' $T/lane.json)
  [ -n "$SHA" ] && adm POST /api/v1/repo/lane "{\"service_id\":\"postgresql\",\"expected_sha256\":\"$SHA\"}" --label lane-enable --timeout 800 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else {}; r=b.get("repo") or {}; print("  lane enable", d["status"], b.get("code"), b.get("written"), "repo enabled", r.get("enabled"), "packages", r.get("packages"), (b.get("error") or "")[:300])'; }
