#!/bin/bash
# Delete a domain (owner action) while polling deletion-status every 2 s as the
# Domains screen does; then DNS on both servers, the domains list, DKIM tables,
# and the peer inspector's own journal when a proof attempt was unavailable.
# usage: env CELL MIP BIP; delpoll.sh DOMAIN_ID DOMAIN   Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
source $H/common.sh
ID=$1; D=$2; T=/var/tmp/a114r3
for ip in $MIP $BIP; do echo "   before @$ip: SOA $(rcode $ip $D) | TXT celik._domainkey '$(dig1 $ip celik._domainkey.$D TXT | cut -c1-30)'"; done
since=$(date -u '+%Y-%m-%d %H:%M:%S')
t0=$(date +%s)
( b1 api $CELL main DELETE /api/v1/domains/$ID --label delete-$D --limit 6000 --timeout 400 > $T/del-$CELL-$D.json ) &
dpid=$!
last=""
while kill -0 $dpid 2>/dev/null; do
  s=$(b1 api $CELL main GET /api/v1/domains/$ID/deletion-status --label delstatus-$D --limit 3000 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b); print(d.get("status"), j.get("status"), j.get("stage"), (j.get("message") or "")[:160])
except Exception: print(d.get("status"), b[:120])')
  [ "$s" != "$last" ] && echo "   +$(( $(date +%s) - t0 ))s status: $s"; last=$s
  sleep 2
done
wait $dpid
echo "   DELETE answered after $(( $(date +%s) - t0 ))s: $(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d.get("status"), (d.get("body") or "").replace(chr(10)," ")[:400])' $T/del-$CELL-$D.json)"
sleep 5
for ip in $MIP $BIP; do echo "   after @$ip: SOA $(rcode $ip $D) | TXT celik._domainkey '$(dig1 $ip celik._domainkey.$D TXT)'"; done
echo "   domains list names it: $( [ -n "$(domain_id $D)" ] && echo yes || echo no)"
q $CELL main guestmail.sh "$D" | grep -E 'lines for'
gsh main "journalctl -u celikpanel-agent -u celikpanel-panel --no-pager -o short-iso --since '$since' | grep -iE 'peer proof|inspector|pending|fail-closed|ambiguous|held|FOREIGN|dkim' | cut -c1-280" delete-log-$D 8000
if [ "$BIP" != "$MIP" ]; then
  echo "   backup inspector's own journal (journalctl -t celikpanel-pdns-peer-inspect) for this deletion:"
  lab ssh $CELL backup -- "sudo -n journalctl -t celikpanel-pdns-peer-inspect --no-pager -o short-iso --since '$since' | cut -c1-300"
fi
if grep -q peer_inspection_unavailable <(lab ssh $CELL main -- "sudo -n journalctl -u celikpanel-agent --no-pager --since '$since'") && [ "$BIP" != "$MIP" ]; then
  echo "   peer_inspection_unavailable seen: backup-side inspector and sshd journal"
  lab ssh $CELL backup -- "sudo -n journalctl --no-pager -o short-iso --since '$since' | grep -iE 'inspect|celikpeer|sshd|sudo|pdns_server' | grep -viE 'systemd\[[0-9]+\]: (Listening|Reached|Started|Starting|Created|Queued)' | cut -c1-300 | head -40"
fi
