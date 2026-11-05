# Wave 2b on 894fd63c9: fresh Ubuntu 24.04 (u24, PowerDNS), case 4 retried with an intermittent DB write-lock hold (7 s held / 1 s free x 12). Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit
source $A/env.sh
true
CELL=u24 IP=192.0.3.193 Z=u24.test OS=ubuntu2404 SHORT=boston HOLDDB=1 HOLDPAT="12 7 1" HOLDAFTER=3 WATCHN=60 WATCHS=2 bash $A/singleX.sh > $A/singleU24.out 2>&1
CELLS=u24 bash $A/post2.sh > $A/post2U24.out 2>&1
echo "wave2b done $(date -u +%T)"
