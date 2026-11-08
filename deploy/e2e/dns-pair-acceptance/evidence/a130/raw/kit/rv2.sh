# alpha.130 re-verify on B (Ubuntu 24.04, lab 131): leftover purge >= 10 min after start; one PostgreSQL version
# removed; whole PostgreSQL removal leaves nothing; Postfix status; an owner drop-in survives Postfix removal. Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2b-$CELL.log) 2>&1
N=backup
b1 login $CELL $N > /dev/null 2>&1
audit() { admn $N GET "/api/v1/audit-logs?limit=60" --label audit | python3 -c '
import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("logs") or b.get("items") or b.get("entries") or (b if isinstance(b,list) else [])
for e in l:
    a=e.get("action","")
    if any(k in a for k in sys.argv[1].split(",")): print("  audit", e.get("created_at"), a[:160], json.dumps(e.get("details"), ensure_ascii=False)[:300])' "$1"; }
mon() { admn $N GET /api/v1/metrics/services --label msvc | python3 -c '
import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  service", s.get("id"), s.get("unit"), s.get("active_state"), s.get("sub_state"), "running" if s.get("running") else s.get("running")) for s in b.get("services",[]) if s.get("id") in ("postfix","dovecot")]'
  admn $N GET /api/v1/metrics/problems --label mprob | python3 -c '
import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("problems") if isinstance(b,dict) else b
print("  problems:", [(p.get("kind"), p.get("subject")) for p in (l or [])])'; }
case "${1:-all}" in
purge|all)
  S0=$(cat $T/b-start.epoch); W=$(( S0 + 630 - $(date +%s) )); [ $W -gt 0 ] && { echo "== waiting $W s until 10.5 min after B's start $(ts)"; sleep $W; }
  echo "== B >= 10 min after start $(ts)"; grun $N g_pgl.sh; audit postgresql.cluster
  gshn $N 'journalctl -u celikpanel-panel -u celikpanel-agent --since -15min -o cat --no-pager | grep -iE "leftover|purge" | tail -6 | cut -c1-250' ;;
esac
case "${1:-all}" in
pgver|all)
  echo "== one version removed $(ts)"
  admn $N GET "/api/v1/repo/lane?service_id=postgresql" --label lane > $T/lane.json; sbrief 200 < $T/lane.json
  SHA=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); p=b.get("plan") or b; print("" if p.get("enabled") else p.get("sha256",""))' $T/lane.json)
  [ -n "$SHA" ] && admn $N POST /api/v1/repo/lane "{\"service_id\":\"postgresql\",\"expected_sha256\":\"$SHA\"}" --label lane-enable --timeout 600 | sbrief 200
  installc $N postgresql postgresql-18; installc $N postgresql postgresql-16; grun $N g_pgl.sh
  admn $N GET "/api/v1/service/removal-impact?service_id=postgresql&package=postgresql-16" --label impact16 | sbrief 900
  admn $N POST /api/v1/service/uninstall '{"service_id":"postgresql","package":"postgresql-16"}' --label pg16-remove --timeout 900 | sbrief 900
  sleep 5; grun $N g_pgl.sh; audit postgresql ;;
esac
case "${1:-all}" in
pgall|all)
  echo "== whole PostgreSQL removal $(ts)"
  admn $N GET "/api/v1/service/removal-impact?service_id=postgresql" --label impact | sbrief 1200
  admn $N POST /api/v1/service/uninstall '{"service_id":"postgresql"}' --label pg-remove --timeout 900 | sbrief 900
  sleep 5; grun $N g_pgl.sh
  gshn $N 'dpkg -l | grep -E "^(ii|rc)" | grep -iE "postgres|libpq" | awk "{print \$1, \$2, \$3}"; ls /etc/apt/sources.list.d /etc/apt/preferences.d /etc/apt/keyrings 2>/dev/null | grep -i -E "pgdg|postgres"; ls -d /etc/postgresql /var/lib/postgresql /var/log/postgresql 2>&1' ;;
esac
case "${1:-all}" in
postfix|all)
  echo "== Postfix on Ubuntu: status, owner drop-in kept on removal $(ts)"
  installc $N postfix; sleep 70; mon
  gshn $N 'mkdir -p /etc/systemd/system/postfix.service.d; printf "[Service]\n# owner (lab)\nNice=5\n" > /etc/systemd/system/postfix.service.d/owner.conf; systemctl daemon-reload; ls -la /etc/systemd/system/postfix.service.d/'
  admn $N POST /api/v1/service/uninstall '{"service_id":"postfix"}' --label postfix-remove --timeout 900 | sbrief 1500
  gshn $N 'ls -la /etc/systemd/system/postfix.service.d/ 2>&1; cat /etc/systemd/system/postfix.service.d/owner.conf 2>&1; dpkg -l postfix 2>/dev/null | tail -1 | cut -c1-60' ;;
esac
echo "== rv2 ${1:-all} done $(ts)"
