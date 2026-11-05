# alpha.124 helpers (sourced after lib.sh, lib11.sh, lib123.sh). Lab only.
# planread SVC: the component-level install plan, as the install window reads it
planread() { adm GET "/api/v1/service/install-plan?id=$1" --label "plan-$1" > $T/plan-$1.json; python3 -c '
import json,sys; d=json.load(open(sys.argv[1])); b=d["body"]
try: j=json.loads(b)
except Exception: print("  plan", d["status"], b[:300]); sys.exit()
p=j.get("plan") or {}
print("  plan", d["status"], "target", p.get("target"), "together", p.get("together"), "blocked", p.get("blocked"))
for r in p.get("requires") or []: print("    requires", json.dumps(r))' $T/plan-$1.json; }
together() { python3 -c 'import json,sys; print(json.dumps((json.loads(json.load(open(sys.argv[1]))["body"]).get("plan") or {}).get("together") or []))' $T/plan-$1.json; }
# prepare SVC [PHP] -> writes $T/body-SVC.json (preview of together + target, every notable confirmed, as the window does)
prepare() { local svc=$1 php=${2:-} TOG q CONF R
  planread $svc; TOG=$(together $svc)
  q=$(python3 -c 'import json,sys,urllib.parse; t=json.loads(sys.argv[1]); s=sys.argv[2]; php=sys.argv[3]
steps=[x for x in t]+[s+("::"+php if php else "")]
print("&".join("step="+urllib.parse.quote(x) for x in steps))' "$TOG" $svc "$php")
  adm GET "/api/v1/service/install-preview?$q" --label "prev-$svc" --timeout 150 > $T/prev.json; prevsum $T/prev.json
  CONF=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print(json.dumps(sorted({x["package"] for x in (b.get("notable") or []) if x.get("package")})))' $T/prev.json)
  R=$(rid); echo $R > $T/rid-$svc
  python3 -c 'import json,sys; t=json.loads(sys.argv[2]); b={"service_id":sys.argv[1],"check_installs":True,"confirmed_installs":json.loads(sys.argv[3]),"request_id":sys.argv[4]}
if t: b["install_together"]=t
if sys.argv[5]: b["php_version"]=sys.argv[5]
json.dump(b,open(sys.argv[6],"w")); print("  body", json.dumps(b))' $svc "$TOG" "$CONF" $R "$php" $T/body-$svc.json; }
# auditcount: rows in the panel audit table (read-only, as the panel user)
auditcount() { gsh1 'U=$(systemctl show -p User --value celikpanel-panel.service); DB=$(ls /var/lib/celikpanel/celikpanel.db); runuser -u $U -- python3 -c "import sqlite3,sys; c=sqlite3.connect(\"file:\"+sys.argv[1]+\"?mode=ro\", uri=True, timeout=60); print(\"audit rows\", c.execute(\"select count(*) from audit_logs\").fetchone()[0]); [print(\"  \", r) for r in c.execute(\"select id, action, created_at from audit_logs order by id desc limit \"+sys.argv[2])]" $DB '"${1:-0}"'; ls -la /var/lib/celikpanel/audit-pending.json 2>&1 | tail -1'; }
# followr QUERY [N]: read an operation (by request_id=... or id=...) every 3 s until it ends, printing each change
followr() { local q=$1 last="" s i; for i in $(seq 1 ${2:-200}); do s=$(opread "$q"); [ "$s" != "$last" ] && { echo "  $(ts)$s"; last="$s"; }; case "$s" in *'"status": "succeeded"'*|*'"status": "failed"'*) return 0;; esac; sleep 3; done; echo "  still running after bound"; }
