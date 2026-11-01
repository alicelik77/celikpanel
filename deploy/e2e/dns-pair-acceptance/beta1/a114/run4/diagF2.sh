#!/bin/bash
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
exec > >(tee -a /var/tmp/a114r4/stop-evidence.out) 2>&1
echo "===== a6-a main: the window 14:36:30-14:38:30 (read only)"
bash $H/gq.sh a6-a main "journalctl --no-pager -o short-iso --since '2026-11-01 14:36:30' --until '2026-11-01 14:38:30' | grep -vE 'sudo\[|pam_unix|level=debug|kernel:' | cut -c1-330 | tail -90" 200000
echo "===== H1 phrases, plain"
for n in main backup; do bash $H/gq.sh a6-a $n "echo $n could-not-be-checked=\$(journalctl -b all -u celikpanel-agent -u celikpanel-panel --no-pager -o cat | grep -ci 'could not be checked') fail-closed=\$(journalctl -b all -u celikpanel-agent -u celikpanel-panel --no-pager -o cat | grep -ci 'fail-closed')"; done
echo "===== backup setup operation window (what the joined server did then)"
bash $H/gq.sh a6-a backup "journalctl -u celikpanel-panel --no-pager -o short-iso --since '2026-11-01 14:36:30' --until '2026-11-01 14:38:30' | grep -iE 'setup step|operation' | cut -c1-260 | tail -15"
