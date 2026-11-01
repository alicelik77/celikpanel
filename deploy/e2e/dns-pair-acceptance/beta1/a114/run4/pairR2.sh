#!/bin/bash
# A redo, resumed: the first server waits for the joined server's setup to start (by design), so the joined
# setup starts now. E1 forced: the first server's panel is stopped for 90 s when the joined server reaches its
# certificate/publisher steps. Check 8: when the publisher has passed and the joined setup still runs, the
# joined server's panel is held stopped until the first server has finished, so the first server's DNS page
# shows the state "waiting for the backup"; then it is started again and the state must clear by itself. Lab only.
set -u
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
CELL=a4-a; MIP=192.0.3.150; BIP=192.0.3.151; Z=fa114.test
SH="python3 -I $H/shots.py"
exec > >(tee -a $LOGS/pair-$CELL.log) 2>&1
gsh1() { b1 sh $CELL $1 "$2" --label lab-$3 --limit 400 > /dev/null; }
ovl() { b1 api $CELL main GET /api/v1/dns/overview --label dns-overview-during --limit 400000 2>/dev/null | python3 $H/dnswatch.py 2>&1 | cut -c1-500; }
echo "== resume: joined setup starts now (the first server waits for it) $(ts)"
b1 plan $CELL backup --limit 60000 > $LOGS/plan-$CELL-backup.json; python3 $H/plansum2.py $LOGS/plan-$CELL-backup.json "plan backup"
b1 start $CELL backup --limit 300 | st start-backup 100
stopped=0; held=0; lastpub=""; last=""; lastcode=""; shots=0
for i in $(seq 1 400); do
  b1 api $CELL backup GET /api/v1/setup/operation --label op-watch --limit 400000 > /var/tmp/a114r4/e1e/op.json 2>/dev/null
  pub=$(python3 $H/e1watch.py < /var/tmp/a114r4/e1e/op.json 2>&1 | cut -c1-1500)
  cur=$(echo "$pub" | grep -o 'cur=[^ ]*' | head -1)
  mop=$(opnow $CELL main 2>/dev/null | head -1 | cut -c1-70)
  [ "$pub" != "$lastpub" ] && { echo "$(ts) E1 backup: $pub"; echo "         main: $mop"; lastpub=$pub; cp /var/tmp/a114r4/e1e/op.json /var/tmp/a114r4/e1e/op-r$i.json; }
  if [ $stopped = 0 ] && echo "$cur" | grep -qE 'cur=09-|cur=10-dns_publisher'; then
    echo "$(ts) LAB: the first server's panel is stopped for 90 s (as in run 1, a restart while the joined server publishes)"
    gsh1 main "systemctl stop celikpanel-panel" stop-main; stopped=$(date +%s)
  fi
  if [ $stopped -gt 1 ] && [ $shots -lt 2 ] && [ $(( $(date +%s) - stopped )) -ge $(( 25 + shots * 40 )) ]; then
    $SH $CELL backup a2-backup-publisher-wait-$shots '[{"name":"setup-progress","path":"/setup","wait":3000}]' > /dev/null; shots=$((shots+1)); echo "$(ts) photographed the joined server's setup page ($shots)"
  fi
  if [ $stopped -gt 1 ] && [ $(( $(date +%s) - stopped )) -ge 90 ]; then
    gsh1 main "systemctl start celikpanel-panel; sleep 3" start-main; echo "$(ts) LAB: the first server's panel started again"; stopped=1
  fi
  if [ $stopped = 1 ] && [ $held = 0 ] && echo "$pub" | grep -q '"status": "succeeded", "target"' && echo "$pub" | grep -q '^running\|^waiting'; then
    echo "$(ts) LAB: the joined server's panel is held stopped until the first server finished (check 8)"
    gsh1 backup "systemctl stop celikpanel-panel" stop-backup; held=$(date +%s)
  fi
  if [ $held -gt 1 ]; then
    case "$mop" in succeeded*) if [ -z "${mdone:-}" ]; then mdone=$(date +%s); echo "$(ts) first server finished; its DNS page while the backup is unfinished:"; fi;; esac
    ov=$(ovl); [ "$ov" != "$last" ] && { echo "$(ts) check8 main: $ov | main: $mop"; last=$ov; }
    code=$(echo "$ov" | cut -d' ' -f1-2); [ "$code" != "$lastcode" ] && { $SH $CELL main a2-main-during-$i '[{"name":"dns-during-join","path":"/dns-infrastructure","wait":2500}]' > /dev/null; lastcode=$code; }
    if [ -n "${mdone:-}" ] && [ $(( $(date +%s) - mdone )) -ge 150 ] || [ $(( $(date +%s) - held )) -ge 2400 ]; then
      $SH $CELL main a2-main-waiting-backup '[{"name":"dns-page","path":"/dns-infrastructure","wait":2500},{"name":"dashboard","path":"/","wait":3000}]' > /dev/null
      gsh1 backup "systemctl start celikpanel-panel; sleep 3" start-backup; echo "$(ts) LAB: the joined server's panel started again"; held=1
    fi
  fi
  case "$pub" in succeeded*|failed*) [ $held -le 1 ] && break;; esac
  sleep 5
done
TRIES=60 wait_status $CELL main succeeded failed
echo "== after both finished: the first server's DNS page settles by itself $(ts)"
for i in $(seq 1 16); do ov=$(ovl); echo "$(ts) $ov"; echo "$ov" | grep -q '^dns_overview_ok' && break; sleep 30; done
b1 api $CELL backup GET /api/v1/dns/overview --label dns-overview-backup --limit 400000 2>/dev/null | python3 $H/dnswatch.py | cut -c1-300
$SH $CELL main a2-main-after '[{"name":"dns-page","path":"/dns-infrastructure","wait":3000},{"name":"setup-ready","path":"/setup","wait":2500}]'
$SH $CELL backup a2-backup-after '[{"name":"setup","path":"/setup","wait":3000},{"name":"dns-page","path":"/dns-infrastructure","wait":3000}]' --widths desktop
grep -l '_refused\|new join code\|create a new' /var/tmp/a114r4/e1e/op-r*.json 2>/dev/null | head -3 | sed 's/^/RAW REFUSAL WORDING IN: /'
echo "== pair done $(ts)"
