# alpha.128 round 2, mail fixes on the candidate: postfwd from Components, limits on (A3: a list postfwd cannot read
# fails the self-test), mailbox limit 450, pause 550, password change reopens at once (A2), bulk answers (A5). Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2mail-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
nup n_swaks.sh >/dev/null
send() { pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2"; }
mid() { adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' $1; }
ON='{"enabled":true,"mailbox_per_hour":500,"domain_per_hour":750,"subscription_per_hour":1000,"new_mailbox_per_hour":50,"new_mailbox_hours":24,"spike_factor":5,"spike_minimum":100,"refusal_pause":3}'
on() { adm POST /api/v1/mail/outgoing "$ON" --label outgoing-on --timeout 300 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); r=b.get("result") or {}; print(d["status"], "applied", r.get("applied"), "verified", r.get("verified"), "code", r.get("code"), "|", (r.get("error") or "")[:300], "| steps", r.get("steps"))'; }
echo "== M.1 postfwd from Components $(ts)"; installc main postfwd
mkcred box2; adm POST /api/v1/domains/$S1/mail/accounts "{\"address\":\"box2@$D\",\"password\":\"$(pw box2)\",\"quota_mb\":200}" --label mbox2 | sbrief 200
B1=$(mid box1@$D); B2=$(mid box2@$D); echo "box1 $B1 box2 $B2"
echo "== M.2 limits on $(ts)"; on
gshn main 'grep -c "sender_domain==file:" /etc/postfix/postfwd.cf; cat /etc/postfix/celikpanel-postfwd/selftest.domains | grep -v "^#"; stat -c "%A %n" /etc/postfix/celikpanel-postfwd /etc/postfix/celikpanel-postfwd/selftest.domains; postconf -P | grep -c end_of_data'
echo "== M.3 (A3) the self-test list made unreadable for postfwd (lab load): the next apply must fail and leave Postfix unwired $(ts)"
gshn main 'chmod 0600 /etc/postfix/celikpanel-postfwd/selftest.domains; chown root:root /etc/postfix/celikpanel-postfwd/selftest.domains; postconf -PX submission/inet/smtpd_end_of_data_restrictions smtps/inet/smtpd_end_of_data_restrictions 2>/dev/null; systemctl reload postfwd postfix; sleep 2; postconf -P | grep -c end_of_data'
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":3}" --label mb-limit | sbrief 300
on
gshn main 'postconf -P | grep -c end_of_data; journalctl -u postfwd --since -2min -o cat --no-pager | grep -iE "selftest|not found|permission" | tail -3 | cut -c1-200'
echo "-- the list readable again (lab load), apply again"
gshn main 'chmod 0644 /etc/postfix/celikpanel-postfwd/selftest.domains'; on
gshn main 'postconf -P | grep -c end_of_data; ls /etc/postfix/celikpanel-postfwd/'
echo "== M.4 box1 limit 3: 6 messages $(ts)"
for i in 1 2 3 4 5 6; do echo " msg $i:"; send 587 box1@$D box2@$D box1; done
echo "== M.5 the pause (worker round, refusal_pause 3) $(ts)"
for i in $(seq 1 26); do sleep 30; p=$(adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(json.dumps(b.get("paused") or []))'); [ "$p" != "[]" ] && { echo "  paused after $((i*30)) s: $p" | cut -c1-300; break; }; done
echo " msg while paused:"; send 587 box1@$D box2@$D box1
echo "== M.6 (A2) password change: open at once $(ts)"
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":100}" --label mb-limit100 | sbrief 200
rm -f $T/cred-box1.json; mkcred box1
T0=$(date +%s); adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B1,\"new_password\":\"$(pw box1)\"}" --label mb-pass --timeout 120 | sbrief 300; echo "   answer after $(( $(date +%s) - T0 )) s"
for i in 1 2 3; do echo " msg $i right after the password change:"; send 587 box1@$D box2@$D box1; done
gshn main 'journalctl -u postfwd --since -3min -o short --no-pager | grep -E "CP_PAUSED" | tail -4 | cut -c1-220; echo "(paused hits end)"'
echo "== M.7 (A5) bulk answers $(ts)"
for b in $B1 $B2; do adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$b,\"mode\":\"custom\",\"per_hour\":200}" --label bulk | sbrief 200; done
adm GET /api/v1/notifications --label notif | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  ", i.get("source"), i.get("kind"), i.get("needs_action"), i.get("fix_path")) for i in b.get("items") or []]'
gshn main 'systemctl list-units --failed --no-legend --plain | cat; echo "(failed end)"'
echo "== r2mail done $(ts)"
