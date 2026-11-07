# alpha.129 case 5: a mailbox reopened by a password change is paused again in the same hour (main, Debian 13). Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C5mail-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
nup n_swaks.sh >/dev/null
send() { pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2" | grep -v "235 2.7.0"; }
mid() { adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' $1; }
paused() { adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(json.dumps(b.get("paused") or []))'; }
waitpause() { for i in $(seq 1 30); do sleep 30; p=$(paused); [ "$p" != "[]" ] && { echo "  paused after $((i*30)) s: $p" | cut -c1-300; return 0; }; done; echo "  NOT paused within 15 min"; return 1; }
ON='{"enabled":true,"mailbox_per_hour":500,"domain_per_hour":750,"subscription_per_hour":1000,"new_mailbox_per_hour":50,"new_mailbox_hours":24,"spike_factor":5,"spike_minimum":100,"refusal_pause":3}'
echo "== 5.0 postfwd from Components, limits on $(ts)"; installc main postfwd
mkcred box2; adm POST /api/v1/domains/$S1/mail/accounts "{\"address\":\"box2@$D\",\"password\":\"$(pw box2)\",\"quota_mb\":200}" --label mbox2 | sbrief 200
adm POST /api/v1/mail/outgoing "$ON" --label outgoing-on --timeout 300 | sbrief 500
gshn main 'dpkg -l postfwd | tail -1; cat /etc/systemd/system/postfwd.service.d/celikpanel.conf | grep -E "^Exec"; ls -la /var/lib/celikpanel-postfwd 2>&1 | tail -1'
B1=$(mid box1@$D)
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":3}" --label mb-limit --timeout 120 | sbrief 200
echo "== 5.1 six messages -> 450 from the 4th $(ts)"; for i in 1 2 3 4 5 6; do echo " msg $i:"; send 587 box1@$D box2@$D box1; done
echo "== 5.2 first pause $(ts)"; waitpause; echo " msg while paused:"; send 587 box1@$D box2@$D box1
echo "== 5.3 password change reopens $(ts)"
rm -f $T/cred-box1.json; mkcred box1
T0=$(date +%s); adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B1,\"new_password\":\"$(pw box1)\"}" --label mb-pass --timeout 120 | sbrief 300; echo "   answer after $(( $(date +%s) - T0 )) s"
echo "== 5.4 after the change: 3 pass, then 450 x3 (same hour) $(ts)"; for i in 1 2 3 4 5 6; do echo " msg $i:"; send 587 box1@$D box2@$D box1; done
echo "== 5.5 the second pause within the same hour $(ts)"; waitpause; echo " msg while paused again:"; send 587 box1@$D box2@$D box1
adm GET /api/v1/notifications --label notif | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  ", i.get("source"), i.get("kind"), i.get("needs_action"), (i.get("created_at") or "")[:19]) for i in (b.get("items") or []) if "mail" in str(i.get("source"))]'
gshn main "python3 -c 'import sqlite3; c=sqlite3.connect(\"file:/var/lib/celikpanel/celikpanel.db?mode=ro\",uri=True); print([r for r in c.execute(\"select address, resume_base_recipients, resume_base_refusals from mailbox_outgoing m join mail_accounts a on a.id=m.mailbox_id\")])' 2>&1 | head -3"
echo "== 5.6 reopen at the end (password change) $(ts)"
rm -f $T/cred-box1.json; mkcred box1
adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B1,\"new_password\":\"$(pw box1)\"}" --label mb-pass2 --timeout 120 | sbrief 200
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":100}" --label mb-limit100 --timeout 120 | sbrief 200
echo "== k5repause done $(ts)"
