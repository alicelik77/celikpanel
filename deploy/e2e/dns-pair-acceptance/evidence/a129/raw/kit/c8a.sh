# alpha.127 case 8 (part 1): monitoring reads on the main; alert delay 5 min; the administrator's address set to a
# mailbox on this server (the lab mail capture) and confirmed through the link the panel mails. Lab only.
export CELL=${CELL:-r7} Z=${Z:-r7.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
N=main
exec > >(tee -a $LOGS/C8-$CELL-$N.log) 2>&1
echo "== services $(ts)"
admn $N GET /api/v1/metrics/services --label msvc > $T/msvc.json; python3 -c '
import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print("state", b.get("state"), "read_at", b.get("read_at"), "rows", len(b.get("services") or []))
for s in b.get("services") or []: print("  ", {k:s.get(k) for k in ("id","unit","active_state","sub_state","active_since","restarts_last_hour","memory_bytes","cpu")})' $T/msvc.json
echo "== sites $(ts)"; admn $N GET /api/v1/metrics/sites --label msites | sbrief 2000
echo "== alerts (before) $(ts)"; admn $N GET /api/v1/metrics/alerts --label malerts | sbrief 1500
admn $N PUT /api/v1/metrics/alerts/settings '{"delay_minutes":7,"email":true,"site_checks":true}' --label alert-bad | sbrief 300
admn $N PUT /api/v1/metrics/alerts/settings '{"delay_minutes":5,"email":true,"site_checks":true}' --label alert-settings | sbrief 300
echo "== the lab mail capture: mailbox alerts@$Z on this server $(ts)"
DID=$(domain_id $Z $N); echo "$Z id $DID"
MPW=$(python3 -c 'import secrets; print(secrets.token_urlsafe(14)+"-Aa1")')
admn $N POST /api/v1/domains/$DID/mail/accounts "{\"address\":\"alerts@$Z\",\"password\":\"$MPW\",\"quota_mb\":200}" --label mbox-alerts | sbrief 400
echo "-- the administrator's address (My account) $(ts)"
python3 -c 'import json,sys; print(json.dumps({"email":"alerts@"+sys.argv[2],"current_password":json.load(open(sys.argv[1]))["password"]}))' /var/tmp/cp-beta1/state/$CELL/$N.json $Z > $T/email.json; chmod 600 $T/email.json
admn $N PUT /api/v1/auth/email @$T/email.json --label set-email | sbrief 400; rm -f $T/email.json
admn $N GET /api/v1/auth/email/verification --label emailver | sbrief 400
echo "== c8a done $(ts)"
