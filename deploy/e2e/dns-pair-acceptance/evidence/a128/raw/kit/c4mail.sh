# alpha.128 case 4: outgoing mail limits with the real Postfix + postfwd on the updated main, and the mail work items. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C4mail-$CELL.log) 2>&1
S1=$(cat $T/site1.id); C1=$(cat $T/cust1.id); D=site1.$Z
nup n_swaks.sh >/dev/null
# send PORT FROM TO [AUTHMAILBOX]
send() { if [ -n "${4:-}" ]; then pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2"; else echo x | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3"; fi; }
mid() { adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' $1; }
outg() { adm GET /api/v1/mail/outgoing --label outgoing | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], "settings", json.dumps(b.get("settings")), "\n   apply", json.dumps(b.get("apply")), "\n   server", json.dumps(b.get("server"))[:600], "\n   paused", json.dumps(b.get("paused"))[:500])'; }
echo "== 4.0 before: Postfix wiring (limits off by default) $(ts)"; outg
gshn main 'postconf -P | grep -iE "policy|end_of_data"; echo "(no policy lines above = untouched)"; systemctl is-active postfwd 2>&1'
echo "== 4.1 install postfwd from Components $(ts)"; installc main postfwd
gshn main 'dpkg -l postfwd | tail -1; systemctl is-active postfwd; cat /etc/systemd/system/postfwd.service.d/celikpanel.conf 2>&1 | head -8; postconf -P | grep -iE "policy|end_of_data"; echo "(after install, before turning on)"'
mkcred box2; adm POST /api/v1/domains/$S1/mail/accounts "{\"address\":\"box2@$D\",\"password\":\"$(pw box2)\",\"quota_mb\":200}" --label mbox2 | sbrief 300
B1=$(mid box1@$D); B2=$(mid box2@$D); echo "box1 id $B1, box2 id $B2"
echo "== 4.2 turn the limits on (owner values; refusal pause after 3 refusals for the lab) $(ts)"
adm POST /api/v1/mail/outgoing '{"enabled":true,"mailbox_per_hour":500,"domain_per_hour":750,"subscription_per_hour":1000,"new_mailbox_per_hour":50,"new_mailbox_hours":24,"spike_factor":5,"spike_minimum":100,"refusal_pause":3}' --label outgoing-on --timeout 300 | sbrief 1200
outg
gshn main 'postconf -P | grep -iE "policy|end_of_data"; systemctl is-active postfwd postfix; ls /etc/postfix/celikpanel-postfwd/ 2>&1'
echo "-- box1's own limit: 3 recipients per hour"
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":3}" --label mb-limit | sbrief 400
echo "== 4.3 box1 sends 5 messages (one recipient each) through submission $(ts)"
for i in 1 2 3 4 5; do echo " msg $i:"; send 587 box1@$D box2@$D box1; done
echo "== 4.4 the panel's five-minute round: the refusals pause box1 $(ts)"
for i in $(seq 1 26); do sleep 30; p=$(adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(json.dumps(b.get("paused") or []))'); [ "$p" != "[]" ] && { echo "  paused after $((i*30)) s: $p" | cut -c1-500; break; }; done
echo " msg after the pause:"; send 587 box1@$D box2@$D box1
adm GET /api/v1/mail-outgoing-notices --label notices | sbrief 1200
adm GET "/api/v1/mail/outgoing/report?days=1" --label report | sbrief 800
echo "== 4.5 password change resumes (box1's limit raised to 100 first) $(ts)"
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":100}" --label mb-limit100 | sbrief 300
rm -f $T/cred-box1.json; mkcred box1
adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B1,\"new_password\":\"$(pw box1)\"}" --label mb-pass | sbrief 400
sleep 70; echo " msg after the password change:"; send 587 box1@$D box2@$D box1
echo "-- postfwd stopped (lab load): mail still flows"
gshn main 'systemctl stop postfwd; systemctl is-active postfwd'
echo " msg with postfwd stopped:"; send 587 box1@$D box2@$D box1
gshn main 'systemctl start postfwd; systemctl is-active postfwd'
echo "== 4.6 empty a mailbox (box2 received the messages above) $(ts)"
adm POST /api/v1/domains/$S1/mail/accounts/empty "{\"id\":$B2}" --label mb-empty | sbrief 400
echo "== 4.7 send-only domain and the reply for unknown addresses $(ts)"
echo " before: unknown address from outside"; send 25 lab@labnet.test nobody@$D
adm PUT /api/v1/domains/$S1/mail/options '{"incoming_off":true,"unknown_reply":""}' --label mopt-sendonly | sbrief 300
sleep 5; echo " send-only: to box2 from outside"; send 25 lab@labnet.test box2@$D
echo " send-only: box2 can still send out:"; send 587 box2@$D box1@$D box2
adm PUT /api/v1/domains/$S1/mail/options '{"incoming_off":false,"unknown_reply":"No such mailbox at site1. Please write to box2@site1.r8.test."}' --label mopt-reply | sbrief 300
sleep 5; echo " unknown address with own reply:"; send 25 lab@labnet.test nobody@$D
echo " known address still receives:"; send 25 lab@labnet.test box2@$D
adm PUT /api/v1/domains/$S1/mail/options '{"incoming_off":false,"unknown_reply":""}' --label mopt-clear | sbrief 200
echo "== 4.8 delivery lookup scoped to the domain $(ts)"
adm GET "/api/v1/domains/$S1/mail/delivery?days=1" --label delivery | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); r=b.get("result") or {}
e=r.get("entries") or []; print(d["status"], "entries", len(e), "read", r.get("read"), "incomplete", r.get("incomplete"))
doms=set()
for x in e: doms.update(a.split("@")[-1] for a in (x.get("from") or "", x.get("to") or "") if "@" in a)
print("   addresses domains:", sorted(doms)); [print("   ", json.dumps(x)[:220]) for x in e[-6:]]'
echo " customer cust1 asks site1's lookup:"; as cust1 GET "/api/v1/domains/$S1/mail/delivery?days=1" | sbrief 200
echo " customer cust1 on its own domain:"; as cust1 GET "/api/v1/domains/$C1/mail/delivery?days=1" | sbrief 300
echo "== 4.9 bulk (the screen's loop): box1+box2 limit 200 $(ts)"
for b in $B1 $B2; do adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$b,\"mode\":\"custom\",\"per_hour\":200}" --label bulk | sbrief 200; done
gshn main 'systemctl list-units --failed --no-legend --plain | cat; echo "(failed end)"'
echo "== c4mail done $(ts)"
