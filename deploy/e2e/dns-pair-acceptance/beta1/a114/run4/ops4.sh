#!/bin/bash
# Run 4 (lean). On the main through the panel API as the owner's screens call
# it: site with Enable SSL at creation, 301, PHP page; mailboxes (DKIM card
# never pressed); 587/465 send, IMAPS, Roundcube; PHP mail(); a bounce from a
# nonexistent recipient; DKIM verified against each DNS server; rspamd's log.
# Pair only: a DNS-only zone created and deleted at once, deletion-status
# polled during the DELETE. Lab only.
# usage: env CELL MIP BIP Z S PANEL [ZONEDEL=1]
set -u
export APC=${APC:-e8e7ab4e6f3cd7564bfaa612838dfa1c3647a75d}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
source $H/common.sh
CELL=${CELL:?}; MIP=${MIP:?}; BIP=${BIP:?}; Z=${Z:?}; S=${S:?}; PANEL=${PANEL:?}
export CELL MIP BIP
DNSIPS=$MIP; [ "$BIP" != "$MIP" ] && DNSIPS=$MIP,$BIP
T=/var/tmp/a114r4
exec > >(tee -a $LOGS/ops-$CELL.log) 2>&1
pw2() { python3 -c 'import json,sys; print(json.dumps({"from": json.load(open(sys.argv[1]))["password"], "to": json.load(open(sys.argv[2]))["password"]}))' /var/tmp/cp-beta1/state/$CELL/mbx-$1.json /var/tmp/cp-beta1/state/$CELL/mbx-$2.json; }
echo "== clocks as the lab gives them $(ts)"
lab ssh beta1-lab labnet -- "echo labnet-CA \$(date -u +%T.%N)"; lab ssh $CELL main -- "echo $PANEL \$(date -u +%T.%N)"
echo "== site $S with Enable SSL at creation $(ts)"
infra delegate $S ns1.$Z $MIP ns2.$Z $BIP > /dev/null
b1 api $CELL main POST /api/v1/domains/create "{\"domain\":\"$S\",\"project_type\":\"static\",\"ssl_type\":\"letsencrypt\"}" --label site-create --limit 4000 | brief
SID=$(domain_id $S); echo "   site id $SID"; echo $SID > $T/sid-$CELL-$S
poll_dig $MIP $S A "$MIP" 60; [ "$BIP" != "$MIP" ] && poll_dig $BIP $S A "$MIP" 180
t0=$(date +%s)
b1 api $CELL main POST /api/v1/domains/$SID/ssl/letsencrypt '{"auto_renew":true,"include_mail":false,"provider":"letsencrypt","reissue":false,"redirect_http":true}' --label site-ssl --limit 4000 --timeout 480 | brief
echo "   certificate answer after $(( $(date +%s) - t0 ))s"
b1 api $CELL main GET /api/v1/domains/$SID/hosting --label hosting-get --limit 4000 > $T/hosting-$CELL.json
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); b["project_type"]="php"; json.dump(b, open(sys.argv[2],"w"))' $T/hosting-$CELL.json $T/hosting-php-$CELL.json
b1 api $CELL main PUT /api/v1/domains/$SID/hosting @$T/hosting-php-$CELL.json --label hosting-php --limit 4000 | brief
for f in hello mail bounce; do b1 api $CELL main POST "/api/v1/domains/$SID/files?path=/$f.php" '{"action":"create"}' --label file-create-$f --limit 2000 | brief; done
b1 api $CELL main POST "/api/v1/domains/$SID/files?path=/hello.php" '{"action":"write","content":"<?php echo \"php-ok \" . (2+3);"}' --label file-write --limit 2000 | brief
b1 api $CELL main POST "/api/v1/domains/$SID/files?path=/mail.php" "{\"action\":\"write\",\"content\":\"<?php \$s = 'php-mail '.bin2hex(random_bytes(6)); \$ok = mail('bob@$S', \$s, 'php mail() test', 'From: alice@$S'); echo (\$ok ? 'sent ' : 'fail ').\$s;\"}" --label file-write-mail --limit 2000 | brief
b1 api $CELL main POST "/api/v1/domains/$SID/files?path=/bounce.php" "{\"action\":\"write\",\"content\":\"<?php \$s = 'bounce-test '.bin2hex(random_bytes(6)); \$ok = mail('nobody113@$S', \$s, 'to a mailbox that does not exist', 'From: alice@$S', '-falice@$S'); echo (\$ok ? 'sent ' : 'fail ').\$s;\"}" --label file-write-bounce --limit 2000 | brief
labq webcheck.sh $S | grep -E '^---|code=|^Server|^server|^Location|Verify|subject='
labq phpcheck.sh $S
echo "== mailboxes; DKIM by itself (card read only) $(ts)"
python3 - $CELL <<'PY'
import json, secrets, sys, pathlib
p = pathlib.Path(f"/var/tmp/cp-beta1/state/{sys.argv[1]}")
for who in ("alice", "bob"):
    f = p / f"mbx-{who}.json"
    if not f.exists():
        f.write_text(json.dumps({"password": secrets.token_urlsafe(18)})); f.chmod(0o600)
PY
for who in alice bob; do
  python3 -c 'import json,sys; print(json.dumps({"address": sys.argv[2]+"@"+sys.argv[3], "password": json.load(open(sys.argv[1]))["password"], "quota_mb": 1024}))' /var/tmp/cp-beta1/state/$CELL/mbx-$who.json $who $S > $T/mbx-$CELL.json
  b1 api $CELL main POST /api/v1/domains/$SID/mail/accounts @$T/mbx-$CELL.json --label mailbox-$who --limit 2000 | brief
  rm -f $T/mbx-$CELL.json
done
t0=$(date +%s)
for i in $(seq 1 40); do
  sum=$(b1 api $CELL main GET /api/v1/domains/$SID/mail/auth --label dkim-card --limit 20000 | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}"); dk=b.get("dkim") or {}
print(json.dumps({"status": d.get("status"), "dkim_status": dk.get("status"), "dkim_signed": b.get("dkim_signed"), "signing_installed": b.get("signing_installed"), "owner_record": b.get("dkim_owner_record"), "automatic": b.get("automatic")}))')
  txt1=$(dig1 $MIP celik._domainkey.$S TXT | cut -c1-40); txt2=$(dig1 $BIP celik._domainkey.$S TXT | cut -c1-40)
  [ -n "$txt1" ] && [ -n "$txt2" ] && echo "$sum" | grep -q '"dkim_signed": true' && echo "$sum" | grep -q '"automatic": null' && break
  sleep 3
done
echo "   after $(( $(date +%s) - t0 ))s: $sum"
echo "   TXT @$MIP '$txt1...'  @$BIP '$txt2...'"
echo "   main zone TXT celik._domainkey.$Z @$MIP '$(dig1 $MIP celik._domainkey.$Z TXT | cut -c1-40)...'  @$BIP '$(dig1 $BIP celik._domainkey.$Z TXT | cut -c1-40)...'"
gsh main "journalctl -u celikpanel-panel --no-pager -o short-iso --since -10min | grep -F '[dkim-auto]' | cut -c1-220" dkim-auto 3000
echo "== webmail address (card enable, owner action) $(ts)"
RID=$(python3 -c 'import secrets; print(secrets.token_hex(16))')
b1 api $CELL main POST /api/v1/service/roundcube/address "{\"host\":\"mail.$Z\",\"request_id\":\"$RID\"}" --label webmail-enable --limit 6000 | brief
for i in $(seq 1 20); do out=$(b1 api $CELL main GET /api/v1/service/roundcube/address --label webmail-address --limit 6000 | brief); echo "$out" | grep -q '"address": {' && break; sleep 5; done
echo "== 587 + 465 send, IMAPS read $(ts)"
pw2 alice bob > $T/mpw-$CELL.json
lab ssh beta1-lab labnet -- "sudo -n python3 -I /root/mailtest.py mail.$Z alice@$S bob@$S" < $T/mpw-$CELL.json > $T/mailtest-$CELL.json
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print("   587", d.get("smtp_send"), "| 465", d.get("smtp_465_send"), "| imaps", d.get("imap_read"), d.get("imap_error",""), "| cert", (d.get("smtp_587_cert") or {}).get("san"))' $T/mailtest-$CELL.json
echo "== PHP mail() and a bounce $(ts)"
PHPOUT=$(lab ssh beta1-lab labnet -- "sudo -n curl -sS --resolve $S:443:$MIP --cacert /root/beta1/pebble-root.pem https://$S/mail.php"); echo "   mail.php: $PHPOUT"
BOUT=$(lab ssh beta1-lab labnet -- "sudo -n curl -sS --resolve $S:443:$MIP --cacert /root/beta1/pebble-root.pem https://$S/bounce.php"); echo "   bounce.php: $BOUT"
PHPSUBJ=$(echo "$PHPOUT" | sed -n 's/^sent \(php-mail [0-9a-f]*\)$/\1/p')
sleep 20
ARGS=""; for s in $(python3 -c 'import json,sys; print(" ".join(s.replace(" ","_") for s in json.load(open(sys.argv[1])).get("subjects",[])))' $T/mailtest-$CELL.json); do ARGS="$ARGS '${s//_/ }'"; done
[ -n "$PHPSUBJ" ] && ARGS="$ARGS '$PHPSUBJ'"
echo "== DKIM verified independently against the TXT from each DNS server ($DNSIPS) $(ts)"
lab ssh beta1-lab labnet -- "sudo -n python3 -I /root/dkimcheck.py mail.$Z bob@$S $DNSIPS $ARGS" < $T/mpw-$CELL.json > $T/dkim-$CELL.json
python3 - $T/dkim-$CELL.json <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
for m in d["mails"]:
    print("  ", m["subject"], "| found" if m.get("found", True) else "| NOT FOUND", "| sigs", m.get("dkim_signatures"), "|", [{k: v for k, v in c.items() if k in ("d","s") or k.startswith("verify")} for c in m.get("dkim", [])], "| mid", m.get("message_id"))
PY
echo "-- bounce in alice's INBOX"
python3 -c 'import json,sys; print(json.dumps({"to": json.load(open(sys.argv[1]))["password"]}))' /var/tmp/cp-beta1/state/$CELL/mbx-alice.json > $T/apw-$CELL.json
lab ssh beta1-lab labnet -- "sudo -n python3 -I /root/dkimcheck.py mail.$Z alice@$S $DNSIPS 'Undelivered Mail Returned to Sender'" < $T/apw-$CELL.json | python3 -c '
import json,sys
d=json.load(sys.stdin)
for m in d["mails"]: print("  ", m["subject"], "found" if m.get("found", True) else "NOT FOUND", m.get("from"), m.get("message_id"))'
rm -f $T/apw-$CELL.json
MIDS=$(python3 -c 'import json,sys; print(" ".join((m.get("message_id") or "").strip("<>") for m in json.load(open(sys.argv[1]))["mails"] if m.get("message_id")))' $T/dkim-$CELL.json)
echo "-- server side: rspamd verdicts, postfix bounce lines, DKIM tables (read only)"
q $CELL main guestmail.sh "$S $MIDS" | grep -E 'rspamd|default|lines for|milter'
gsh main "journalctl -u postfix -u 'postfix@*' --no-pager -o short-iso --since -5min | grep -E 'nobody113|status=bounced|sender non-delivery' | cut -c1-230" bounce-log 4000
echo "-- Roundcube (bob)"
lab ssh beta1-lab labnet -- "sudo -n python3 -I /root/webmailtest.py mail.$Z bob@$S" < $T/mpw-$CELL.json | grep -E 'logged_in|count_hint|error'
rm -f $T/mpw-$CELL.json
echo "-- DKIM card now (read only)"
b1 api $CELL main GET /api/v1/domains/$SID/mail/auth --label dkim-card-late --limit 20000 | python3 -c '
import json,sys
d=json.load(sys.stdin); b=json.loads(d.get("body") or "{}"); dk=b.get("dkim") or {}
print("   ", d.get("status"), json.dumps({"dkim.status": dk.get("status"), "dkim_signed": b.get("dkim_signed"), "automatic": b.get("automatic")}))'
if [ -n "${ZONEDEL:-}" ]; then
  echo "== DNS-only zone $ZONEDEL created and deleted at once $(ts)"
  infra delegate $ZONEDEL ns1.$Z $MIP ns2.$Z $BIP > /dev/null
  b1 api $CELL main POST /api/v1/domains/create "{\"domain\":\"$ZONEDEL\",\"project_type\":\"dnsonly\",\"ssl_type\":\"none\"}" --label zone-create --limit 4000 | brief
  ZID=$(domain_id $ZONEDEL); echo "   zone id $ZID"
  bash $H/delpoll.sh $ZID $ZONEDEL
  gsh main "python3 -c \"import sqlite3; db=sqlite3.connect('file:/var/lib/celikpanel/celikpanel.db?mode=ro', uri=True); print('dkim_automatic rows:', db.execute(\\\"select key, substr(value,1,160) from panel_settings where key like 'dkim_automatic%'\\\").fetchall())\"; journalctl -b all -q | grep -c 'FOREIGN KEY' | sed 's/^/FOREIGN KEY lines: /'" dkim-leftover 3000
fi
echo "== ops done $(ts)"
