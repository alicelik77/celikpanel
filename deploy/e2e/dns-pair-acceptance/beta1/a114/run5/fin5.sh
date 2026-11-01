#!/bin/bash
# run 5 stop: the evidence of the stopping failure, then collection, then cleanup. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab5/env.sh
{ echo "run 5 stopped (fail-fast) at Sanity Debian 13 / E-new-3 on a7-a main $(date -u)"; grep -E '^PASS|^FAIL' /var/tmp/a114r5/opsA.out; echo; cat /var/tmp/a114r5/lmtp-diag.out /var/tmp/a114r5/lmtp-diag2.out; } > /var/tmp/a114r5/stop-evidence.out
bash $H/collect5.sh "a7-a:main a7-a:backup" 2>&1 | tail -25
