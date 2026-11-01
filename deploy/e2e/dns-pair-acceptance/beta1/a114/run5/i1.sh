#!/bin/bash
# run 5, I1: setup finished on both servers (13-mail_profile included), no firewall_sync_failed; the firewall
# waiting phase (journal "the firewall update waits") if the race was met; no "fail-closed" in Agent journals. Read only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab5/env.sh
CELL=a7-a; bad=0
for n in main backup; do
  echo "===== $CELL $n"
  o=$(opnow $CELL $n); echo "$o" | head -3 | cut -c1-900; case "$(echo "$o" | head -1)" in succeeded*) ;; *) bad=1;; esac
  echo "$o" | grep -q 'firewall_sync_failed\|firewall_sync_host_busy' && bad=1
  bash $H/gq.sh $CELL $n "grep PRETTY_NAME /etc/os-release; echo \"   [firewall waits] panel=\$(journalctl -b all -u celikpanel-panel --no-pager -o cat | grep -c 'the firewall update waits')\"; echo \"   [firewall_sync_failed] panel=\$(journalctl -b all -u celikpanel-panel --no-pager -o cat | grep -c firewall_sync_failed)\"; echo \"   [firewall_sync_host_busy] panel=\$(journalctl -b all -u celikpanel-panel --no-pager -o cat | grep -c firewall_sync_host_busy)\"; for p in 'fail-closed' 'could not be checked'; do echo \"   [\$p] agent=\$(journalctl -b all -u celikpanel-agent --no-pager -o cat | grep -ci \"\$p\") panel=\$(journalctl -b all -u celikpanel-panel --no-pager -o cat | grep -ci \"\$p\")\"; done; echo '-- firewall lines'; journalctl -b all -u celikpanel-panel --no-pager -o short-iso | grep -E 'firewall update waits|firewall_sync|in profile/.*/firewall|firewall_waiting|server setup step 1[3-9]' | cut -c1-330 | tail -14; echo '-- fail-closed lines'; journalctl -b all -u celikpanel-agent -u celikpanel-panel --no-pager -o short-iso | grep -iE 'fail-closed|could not be checked' | head -4 | cut -c1-300" > /var/tmp/a114r5/i1-$n.out
  cat /var/tmp/a114r5/i1-$n.out
  grep -qE '^\s+\[firewall_sync_failed\] panel=[1-9]' /var/tmp/a114r5/i1-$n.out && bad=1
  grep -E '^\s+\[(fail-closed|could not be checked)\]' /var/tmp/a114r5/i1-$n.out | grep -vq 'agent=0 panel=0' && bad=1
done
[ $bad = 0 ] && echo "I1 PASS" || echo "I1 FAIL"
