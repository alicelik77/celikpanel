# case 4 (pause, after the restart): box2 over its limit 2 refuses twice more (4 refusals in the hour), the
# panel's round pauses it (550), a password change reopens it (250). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/hdr.sh
exec > >(tee -a $LOGS/C4mail-$S.log) 2>&1
D=site1.$Z
nup n_swaks.sh >/dev/null
send() { pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2"; }
mid() { adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' $1; }
B2=$(mid box2@$D)
echo "== 4.9 box2 two more messages over its limit $(ts)"
for i in 7 8; do echo " box2 msg $i:"; send 587 box2@$D box1@$D box2; done
echo "== 4.10 the pause (worker round) $(ts)"
for i in $(seq 1 26); do sleep 30; p=$(adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(json.dumps(b.get("paused") or []))'); [ "$p" != "[]" ] && { echo "  paused after $((i*30)) s: $p" | cut -c1-300; break; }; done
echo " box2 msg while paused:"; send 587 box2@$D box1@$D box2
echo "== 4.11 password change reopens $(ts)"
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B2,\"mode\":\"custom\",\"per_hour\":100}" --label mb2-limit100 | sbrief 200
rm -f $T/cred-box2.json; mkcred box2
adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B2,\"new_password\":\"$(pw box2)\"}" --label mb2-pass --timeout 120 | sbrief 300
for i in 1 2; do echo " box2 msg $i after the password change:"; send 587 box2@$D box1@$D box2; done
gshn main 'systemctl list-units --failed --no-legend --plain | cat; echo "(failed end)"'
echo "== k4c $S done $(ts)"
