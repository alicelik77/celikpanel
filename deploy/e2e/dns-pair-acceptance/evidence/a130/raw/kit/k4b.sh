# case 4 (diagnosis): a mailbox limit changed while the limits are on: does postfwd 1.35's reload reach the
# policy children? box2 limit 2: 3 messages after the save; then postfwd restarted (lab load): 3 more. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/hdr.sh
exec > >(tee -a $LOGS/C4mail-$S.log) 2>&1
D=site1.$Z
nup n_swaks.sh >/dev/null
send() { pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2"; }
mid() { adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' $1; }
B2=$(mid box2@$D)
echo "== 4.7 box2 own limit 2 (limits already on) $(ts)"
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B2,\"mode\":\"custom\",\"per_hour\":2}" --label mb2-limit | sbrief 300
sleep 3
for i in 1 2 3; do echo " box2 msg $i:"; send 587 box2@$D box1@$D box2; done
gshn main 'journalctl -u postfwd --since -2min -o cat --no-pager | grep -E "user=box2" | sed -E "s/.*(hits=[^ ]*).*(action=.*)/\1 \2/" | cut -c1-200; ps -o pid,etimes,args -C postfwd 2>/dev/null | cut -c1-120; pgrep -af "postfwd2::policy" | head'
echo "== 4.8 postfwd restarted (lab load) $(ts)"
gshn main 'systemctl restart postfwd; sleep 2; systemctl is-active postfwd'
for i in 4 5 6; do echo " box2 msg $i:"; send 587 box2@$D box1@$D box2; done
gshn main 'journalctl -u postfwd --since -1min -o cat --no-pager | grep -E "user=box2" | sed -E "s/.*(hits=[^ ]*).*(action=.*)/\1 \2/" | cut -c1-200'
echo "== k4b $S done $(ts)"
