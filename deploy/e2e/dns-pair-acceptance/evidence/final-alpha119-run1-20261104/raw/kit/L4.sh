# topology A line 4: suspension of subscription B stops its n8n unit, resume restarts it. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/L4-$CELL.log) 2>&1
SUBB=$(cat $T/subb); NID=$(cat $T/nid)
susp() {
  local want=$1 i s
  for i in $(seq 1 40); do
    s=$(adm GET /api/v1/subscriptions/$SUBB/suspension --label sus-get | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); o=b.get("operation") or {}; print(b.get("status"), o.get("state"), json.dumps([(e.get("kind"),e.get("target"),e.get("state")) for e in o.get("effects") or []])[:700])')
    case "$s" in "$want "*) case "$s" in *running*|*pending*|*queued*) ;; *) echo "  $s ($(ts))"; return 0 ;; esac ;; esac
    sleep 6
  done
  echo "  still: $s"
}
echo "== L4 suspend subB=$SUBB $(ts)"
adm POST /api/v1/subscriptions/$SUBB/suspension '{"action":"suspend","reason":"lab acceptance: suspension stops service apps"}' --label suspend | sbrief 500
susp suspended
GQT=60 gq gn8n.sh
lc https - n8n118.test ""
echo "== L4 resume $(ts)"
adm POST /api/v1/subscriptions/$SUBB/suspension '{"action":"resume"}' --label resume | sbrief 500
susp active
sleep 20; GQT=60 gq gn8n.sh
lc https - n8n118.test ""
as cust118b GET /api/v1/domains/$NID/service-apps | python3 $A/sastat.py
echo "== L4 done $(ts)"
