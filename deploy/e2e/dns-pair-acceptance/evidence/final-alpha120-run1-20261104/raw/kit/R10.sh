# alpha.120 check: on Ubuntu 24.04 at 119 install Python, update to 120 through the panel, Components API,
# security emergency list (+ withdraw/cancel for PostgreSQL), SPA routes. Lab only.
export CELL=u119 IP=192.0.3.191 Z=u119.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R10-$CELL.log) 2>&1
row() { adm GET /api/v1/managed-services --label ms | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
for r in rows:
    if r.get("id") in sys.argv[1:]:
        keep={k:v for k,v in r.items() if k in ("id","is_installed","status","kind","not_offered","install","versions","package_version","package_version_short","installed_minors","instances","requires_missing","emergency","security_emergency","withdrawn") or "emerg" in k or "version" in k or "minor" in k or "instance" in k}
        print("  row", json.dumps(keep)[:900])' "$@"; }
echo "== on 119 $(ts)"; gsh1 'cat /opt/celikpanel/release.commit 2>/dev/null; grep PRETTY /etc/os-release'
adm GET "/api/v1/service/install-preview?step=python" --label pyprev > $T/pyprev.json
CONF=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); n=b.get("notable") or []; print(json.dumps(sorted({(x.get("name") or x.get("package")) for x in n if isinstance(x,dict) and (x.get("name") or x.get("package"))})))' $T/pyprev.json); echo "confirming: $CONF"
R=$(rid); svcstart "{\"service_id\":\"python\",\"request_id\":\"$R\",\"check_installs\":true,\"confirmed_installs\":$CONF}" /api/v1/service/install && svcwait $R 900
row python postgresql
echo "== update to 120 through the panel $(ts)"
CHK=$(adm GET /api/v1/panel/update/check --label update-check --timeout 120); echo "$CHK" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("check", d["status"], d["body"][:500].replace("\n"," "))'
echo "$CHK" | python3 -c '
import json,sys,secrets
chk=json.loads(json.load(sys.stdin)["body"])
body={"request_id":secrets.token_hex(16),"confirmed":True,"current_version":chk["current_version"],"current_commit":chk["current_commit"],**chk["target"]}
json.dump(body,open(sys.argv[1],"w")); print(body["request_id"])' $T/upd.json > $T/upd.rid
RID=$(cat $T/upd.rid)
for k in 1 2 3 4 5 6; do r=$(adm POST /api/v1/panel/update/start @$T/upd.json --label update-start --timeout 120 | sbrief 200); echo "$r"; case "$r" in 409*PANEL_UPDATE_BUSY*) sleep 30;; *) break;; esac; done
UPT=$(date +%s); last=""
for i in $(seq 1 120); do sleep 10
  s=$(adm GET "/api/v1/panel/update/status?request_id=$RID" --label update-status --timeout 30 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}") if d.get("status")==200 else {}; print(d.get("status"), b.get("status"), b.get("phase"))' 2>/dev/null)
  case "$s" in 401*) b1 login $CELL main >/dev/null 2>&1;; esac
  [ "$s" != "$last" ] && { echo "  $(( $(date +%s) - UPT ))s $s"; last="$s"; }
  case "$s" in "200 succeeded "*|"200 failed"*|"200 rolled_back"*|"200 recovered"*|"200 abandoned"*) [ "${s##* }" != verifying ] && break;; esac
done
echo "== after update $(ts)"; sleep 20
gsh1 'cat /opt/celikpanel/release.commit 2>/dev/null; systemctl is-active celikpanel-panel celikpanel-agent nginx postfix dovecot mariadb named | tr "\n" " "; echo; systemctl --failed --no-legend --no-pager | head -3; nginx -t 2>&1 | tail -1; echo "panics: $(journalctl -u celikpanel-panel -u celikpanel-agent --no-pager | grep -ciE "panic:|goroutine [0-9]+ \[")"'
echo "== Components API $(ts)"; row python postgresql
adm GET "/api/v1/service/candidate?id=postgresql" --label cand | sbrief 200
echo "== security emergencies $(ts)"
adm GET /api/v1/security-emergencies --label emerg | sbrief 400
adm GET "/api/v1/security-emergencies/preview?component=postgresql" --label emerg-prev | sbrief 600
adm POST /api/v1/security-emergencies '{"component":"postgresql","version":"","reason":"lab test only: withdraw step, then undo"}' --label emerg-open > $T/em.json; sbrief 500 < $T/em.json
EID=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); e=b.get("emergency",b); print(e.get("id",""))' $T/em.json); echo "emergency id $EID"
row postgresql
[ -n "$EID" ] && { adm POST /api/v1/security-emergencies/$EID/cancel '{}' --label emerg-cancel | sbrief 400; row postgresql; adm GET /api/v1/security-emergencies --label emerg2 | sbrief 300; }
echo "== SPA routes $(ts)"
gsh1 'for p in /components /services /help; do curl -sk -o /dev/null -w "$p %{http_code}\n" https://127.0.0.1:1923$p; done'
echo "== R10 done $(ts)"
