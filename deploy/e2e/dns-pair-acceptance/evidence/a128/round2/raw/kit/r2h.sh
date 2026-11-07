# alpha.128 round 2: lab 131 mail part (box3).
# limit -> paused (550) -> password change -> its next message goes. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2c-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
nup n_swaks.sh >/dev/null
send() { pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2"; }
rm -f $T/cred-box3.json; mkcred box3; adm POST /api/v1/domains/$S1/mail/accounts "{\"address\":\"box3@$D\",\"password\":\"$(pw box3)\",\"quota_mb\":100}" --label mbox3 | sbrief 200
B3=$(adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' box3@$D)
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B3,\"mode\":\"custom\",\"per_hour\":3}" --label mb-limit --timeout 120 | sbrief 200
for i in 1 2 3 4 5 6; do send 587 box3@$D box1@$D box3 | tail -1; done
for i in $(seq 1 26); do sleep 30; p=$(adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(json.dumps([x["address"] for x in b.get("paused") or []]))'); case "$p" in *box3*) echo "  box3 paused after $((i*30)) s"; break;; esac; done
sleep 15; echo " msg while paused:"; send 587 box3@$D box1@$D box3 | tail -1
rm -f $T/cred-box3.json; mkcred box3
T0=$(date +%s); adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B3,\"new_password\":\"$(pw box3)\"}" --label mb-pass --timeout 120 | sbrief 300; echo "   answer after $(( $(date +%s) - T0 )) s"
for i in 1 2; do echo " msg $i right after the password change:"; send 587 box3@$D box1@$D box3 | tail -1; done
gshn main 'systemctl list-units --failed --no-legend --plain | cat; echo "(failed end)"'
echo "== r2h done $(ts)"
