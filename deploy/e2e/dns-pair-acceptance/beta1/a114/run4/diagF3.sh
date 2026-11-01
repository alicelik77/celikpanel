#!/bin/bash
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
exec > >(tee -a /var/tmp/a114r4/stop-evidence.out) 2>&1
echo "===== a6-a main: agent + panel units 14:36:00-14:37:45, all levels (read only)"
bash $H/gq.sh a6-a main "journalctl -u celikpanel-agent -u celikpanel-panel --no-pager -o short-iso --since '2026-11-01 14:36:00' --until '2026-11-01 14:37:45' | cut -c1-300 | grep -vE 'GET /api|sudo' | tail -60; echo '-- service-mutation units then'; journalctl --no-pager -o short-iso --since '2026-11-01 14:37:00' --until '2026-11-01 14:37:45' | grep -iE 'service-mutation|mutation|apt|dpkg|package' | cut -c1-250 | tail -20" 200000
