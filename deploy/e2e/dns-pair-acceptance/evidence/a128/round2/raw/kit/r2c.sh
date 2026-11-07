# alpha.128 round 2, fix check on the lab 130 label of fix/a128-lab (5edae8f22): main 129 -> 130 (one press), then A1
# (clamped site runs at the limit), mail (raised limit applies at once, pause answers 550, password change reopens at
# once), notify-only view, SFTP smoke again, idle timeout. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2c-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
echo "== C.0 main 129 -> 130 $(ts)"; bash $A/portalF.sh latest v0.1.0-alpha.130
grun main g_snap.sh pre130 > /dev/null
CELL=r9 bash $A/upd.sh main
grun main g_snap.sh post130 pre130
bash $A/portalF.sh latest v0.1.0-alpha.128
bash $A/r2idle.sh
echo "== C.1 A1 again $(ts)"; bash $A/r2a.sh
echo "== C.2 mail $(ts)"
nup n_swaks.sh >/dev/null
send() { pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2"; }
mid() { adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' $1; }
B1=$(mid box1@$D)
lim() { adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":$1}" --label mb-limit --timeout 120 | sbrief 200; }
echo "-- limit 3 (its counter starts again): 4 messages"; lim 3
for i in 1 2 3 4; do echo " msg $i:"; send 587 box1@$D box2@$D box1; done
echo "-- raised to 100: the next message goes at once"; lim 100
send 587 box1@$D box2@$D box1
echo "-- limit 3 again, 6 messages -> pause"; lim 3
for i in 1 2 3 4 5 6; do send 587 box1@$D box2@$D box1 | tail -1; done
for i in $(seq 1 26); do sleep 30; p=$(adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(json.dumps(b.get("paused") or []))'); [ "$p" != "[]" ] && { echo "  paused after $((i*30)) s" ; break; }; done
sleep 20; echo " msg while paused:"; send 587 box1@$D box2@$D box1
rm -f $T/cred-box1.json; mkcred box1
T0=$(date +%s); adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B1,\"new_password\":\"$(pw box1)\"}" --label mb-pass --timeout 120 | sbrief 300; echo "   answer after $(( $(date +%s) - T0 )) s"
echo " msg right after the password change:"; send 587 box1@$D box2@$D box1
adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("   apply", json.dumps(b.get("apply")))'
echo "== C.3 notify-only view (cust2) $(ts)"
adm GET /api/v1/subscriptions --label subs | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  ", s.get("owner"), json.dumps((s.get("usage") or {}).get("disk"))[:300]) for s in b["subscriptions"] if s.get("owner")=="cust2"]'
echo "== C.4 SFTP again $(ts)"; bash $A/r2sftp.sh
echo "== r2c done $(ts)"
