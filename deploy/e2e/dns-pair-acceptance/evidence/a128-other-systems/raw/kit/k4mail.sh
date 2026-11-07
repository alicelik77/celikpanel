# case 4: outgoing mail limits: postfwd from Components, limits on, 450 at the mailbox limit, the security pause (550,
# triggered by refusal_pause 3 as in the a128 lab), password change reopens, postfwd stopped -> mail flows. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/hdr.sh
exec > >(tee -a $LOGS/C4mail-$S.log) 2>&1
D=site1.$Z
nup n_swaks.sh >/dev/null
send() { pw $4 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP $1 $2 $3 $2"; }
mid() { adm GET /api/v1/domains/$S1/mail/accounts --label mboxes | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("accounts") if isinstance(b,dict) else b; print([a["id"] for a in l if a["address"]==sys.argv[1]][0])' $1; }
ON='{"enabled":true,"mailbox_per_hour":500,"domain_per_hour":750,"subscription_per_hour":1000,"new_mailbox_per_hour":50,"new_mailbox_hours":24,"spike_factor":5,"spike_minimum":100,"refusal_pause":3}'
on() { adm POST /api/v1/mail/outgoing "$ON" --label outgoing-on --timeout 300 | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]) if d["body"].startswith("{") else {}; r=b.get("result") or {}; print(d["status"], "applied", r.get("applied"), "verified", r.get("verified"), "code", r.get("code") or b.get("code"), "|", (r.get("error") or b.get("error") or "")[:400], "| steps", r.get("steps"))'; }
echo "== 4.0 Components: postfwd row $(ts)"
admn main GET /api/v1/managed-services --label ms | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
[print("  row", json.dumps({k:r.get(k) for k in ("id","is_installed","status","package","available","reason","notice")})[:600]) for r in rows if "postfwd" in str(r.get("id"))]'
echo "== 4.1 postfwd from Components $(ts)"; installc main postfwd
gshn main 'command -v postfwd postfwd2 postfwd3 2>/dev/null; systemctl is-active postfwd 2>&1; systemctl cat postfwd 2>/dev/null | grep -E "^ExecStart|^Environment|cache" | head -6; (pacman -Q postfwd 2>/dev/null || dpkg -l postfwd 2>/dev/null | tail -1); ls /etc/systemd/system/postfwd.service.d/ 2>/dev/null && cat /etc/systemd/system/postfwd.service.d/*.conf 2>/dev/null; ps -eo args | grep -i "[p]ostfwd" | head -3 | cut -c1-200'
B1=$(mid box1@$D); B2=$(mid box2@$D); echo "box1 $B1 box2 $B2"
echo "== 4.2 limits on $(ts)"; on
gshn main 'postconf -P | grep -iE "policy|end_of_data"; stat -c "%A %U:%G %n" /etc/postfix/celikpanel-postfwd 2>&1'
echo "-- one message before any own limit:"; send 587 box1@$D box2@$D box1
echo "== 4.3 box1 own limit 3; 6 messages $(ts)"
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":3}" --label mb-limit | sbrief 300
for i in 1 2 3 4 5 6; do echo " msg $i:"; send 587 box1@$D box2@$D box1; done
echo "== 4.4 the pause (worker round, refusal_pause 3) $(ts)"
for i in $(seq 1 26); do sleep 30; p=$(adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(json.dumps(b.get("paused") or []))'); [ "$p" != "[]" ] && { echo "  paused after $((i*30)) s: $p" | cut -c1-300; break; }; done
echo " msg while paused:"; send 587 box1@$D box2@$D box1
adm GET /api/v1/notifications --label notif | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); [print("  ", i.get("source"), i.get("kind"), i.get("needs_action"), i.get("fix_path")) for i in (b.get("items") or []) if "mail" in str(i.get("source"))+str(i.get("kind"))]'
echo "== 4.5 password change reopens $(ts)"
adm PUT /api/v1/domains/$S1/mail/outgoing "{\"id\":$B1,\"mode\":\"custom\",\"per_hour\":100}" --label mb-limit100 | sbrief 200
rm -f $T/cred-box1.json; mkcred box1
T0=$(date +%s); adm PUT /api/v1/domains/$S1/mail/accounts/password "{\"id\":$B1,\"new_password\":\"$(pw box1)\"}" --label mb-pass --timeout 120 | sbrief 300; echo "   answer after $(( $(date +%s) - T0 )) s"
for i in 1 2; do echo " msg $i right after the password change:"; send 587 box1@$D box2@$D box1; done
echo "== 4.6 postfwd stopped (lab load): mail still flows $(ts)"
gshn main 'systemctl stop postfwd; systemctl is-active postfwd'
echo " msg with postfwd stopped:"; send 587 box1@$D box2@$D box1
gshn main 'systemctl start postfwd; systemctl is-active postfwd; systemctl list-units --failed --no-legend --plain | cat; echo "(failed end)"'
echo "== k4mail $S done $(ts)"
