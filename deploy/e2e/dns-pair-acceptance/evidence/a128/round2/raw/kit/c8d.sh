# alpha.127 case 8: confirm the administrator's address through a new mailed link (the first try parsed the
# quoted-printable link wrongly in the lab script). Lab only.
export CELL=r7 IP=192.0.3.190 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C8-r7-main.log) 2>&1
echo "== ask for a new confirmation link $(ts)"
admn main POST /api/v1/auth/email/verification '{"lang":"en"}' --label emailver-send | sbrief 300
sleep 15; grun main g_mail.sh alerts@$Z list 2
TOK=$(grun main g_mail.sh alerts@$Z token 2>/dev/null | tail -1); [ -n "$TOK" ] && echo "token found (not shown, ${#TOK} chars)" || echo "NO TOKEN"
python3 -c 'import json,sys; print(json.dumps({"token":sys.argv[1]}))' "$TOK" > $T/tok.json; chmod 600 $T/tok.json
admn main POST /api/v1/auth/recovery/verify-email @$T/tok.json --label verify-email | sbrief 300; rm -f $T/tok.json
admn main GET /api/v1/auth/email/verification --label emailver | sbrief 300
admn main GET /api/v1/metrics/alerts --label malerts | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(b.get("email")); [print("  ", {k:a.get(k) for k in ("id","kind","state","opened_at","resolved_at","mail_state","mail_detail")}) for a in b.get("alerts") or []]'
