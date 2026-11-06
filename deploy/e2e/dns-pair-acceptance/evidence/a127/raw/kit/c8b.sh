# alpha.127 case 8 (part 2): confirm the address through the mailed link; stop a service outside the panel; the alert
# opens after the 5-minute delay; its email arrives in the lab mail capture; start it again. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=main; SVC=${SVC:-cron}
exec > >(tee -a $LOGS/C8-$CELL-$N.log) 2>&1
alerts() { admn $N GET /api/v1/metrics/alerts --label malerts | python3 -c '
import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("  settings", b.get("settings"), "email", b.get("email"), "unseen", b.get("unseen"))
for a in b.get("alerts") or []: print("  ", {k:a.get(k) for k in ("id","kind","name","severity","state","first_seen","opened_at","resolved_at","mail_state","mail_detail")})'; }
echo "== the confirmation mail in the lab mail capture $(ts)"; grun $N g_mail.sh alerts@$Z list 5
TOK=$(grun $N g_mail.sh alerts@$Z token 2>/dev/null | tail -1)
[ -n "$TOK" ] && echo "token found (not shown)" || echo "NO TOKEN"
python3 -c 'import json,sys; print(json.dumps({"token":sys.argv[1]}))' "$TOK" > $T/tok.json; chmod 600 $T/tok.json
admn $N POST /api/v1/auth/recovery/verify-email @$T/tok.json --label verify-email | sbrief 300; rm -f $T/tok.json
admn $N GET /api/v1/auth/email/verification --label emailver | sbrief 300
alerts
echo "== stop $SVC outside the panel (lab load) $(ts)"; T0=$(date +%s)
gshn $N "systemctl stop $SVC; systemctl is-active $SVC"
for i in $(seq 1 40); do sleep 30
  s=$(admn $N GET /api/v1/metrics/alerts --label malerts | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(" ".join("%s:%s:%s" % (a.get("kind"), a.get("state"), a.get("mail_state")) for a in b.get("alerts") or [] if a.get("kind")!="security_updates"))')
  echo "  +$(( $(date +%s) - T0 ))s $s"
  echo "$s" | grep -qE "open:(sent|failed|not_available|suppressed)" && break
done
alerts
echo "-- problems now"; admn $N GET /api/v1/metrics/problems --label problems | sbrief 800
echo "== the alert email in the lab mail capture $(ts)"; sleep 20; grun $N g_mail.sh alerts@$Z list 3
echo "== start $SVC again $(ts)"; gshn $N "systemctl start $SVC; systemctl is-active $SVC"
echo "== c8b done $(ts)"
