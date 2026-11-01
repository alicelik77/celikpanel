#!/bin/bash
# v3.sh CELL NODE : the PowerDNS native evidence documents (digest + schema), read only
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
lab ssh $1 $2 -- "sudo -n sh -s" <<'R'
cd /var/lib/celikpanel-agent-private && stat -c 'dir %a' .
for f in dns-engine-state.json dns-engine-ownership-pdns.json dns-engine-switch-journal.json; do
  if [ -f "$f" ]; then echo "$f $(sha256sum < "$f" | cut -c1-16) $(grep -o '"schema" *: *"[^"]*"' "$f" | head -2 | tr '\n' ' ')"; else echo "$f absent"; fi
done
R
