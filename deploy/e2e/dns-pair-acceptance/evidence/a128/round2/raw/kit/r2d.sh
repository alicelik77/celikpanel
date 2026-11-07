# alpha.128 round 2, lab 130: box2 (never paused) over its limit -> paused -> answered with the pause (550) ->
# password change -> sends at once. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/R2c-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
nup n_swaks.sh >/dev/null
send() { pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2"; }
B2=$(adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' box2@$D)
echo "== D.1 box2 limit 3, 6 messages $(ts)"
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B2,\"mode\":\"custom\",\"per_hour\":3}" --label mb-limit --timeout 120 | sbrief 200
for i in 1 2 3 4 5 6; do send 587 box2@$D box1@$D box2 | tail -1; done
for i in $(seq 1 26); do sleep 30; p=$(adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(json.dumps([x["address"] for x in b.get("paused") or []]))'); case "$p" in *box2*) echo "  box2 paused after $((i*30)) s"; break;; esac; done
sleep 15; echo " msg while paused:"; send 587 box2@$D box1@$D box2 | tail -1
rm -f $T/cred-box2.json; mkcred box2
T0=$(date +%s); adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B2,\"new_password\":\"$(pw box2)\"}" --label mb-pass --timeout 120 | sbrief 300; echo "   answer after $(( $(date +%s) - T0 )) s"
echo " msg right after the password change:"; send 587 box2@$D box1@$D box2 | tail -1
gshn main 'journalctl -u celikpanel-agent --since -3min -o cat --no-pager | grep -iE "delrate|counters|resumed" | tail -3 | cut -c1-200'
echo "== r2d done $(ts)"
