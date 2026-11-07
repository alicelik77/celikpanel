export CELL=r7 IP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C9esc-r7.log) 2>&1
echo "== the joined Ubuntu 24.04 server's Node.js sandbox level as Components shows it $(ts)"
admn backup GET /api/v1/runtimes/node/sandbox --label sandbox | sbrief 800
echo "== escape kit item 10 detail on both"; for n in main backup; do echo "-- $n"; grep -iE "item 10|10 |wildcard|0\.0\.0\.0|reference|veth|outside" $LOGS/C9esc-$n.txt | head -14 | cut -c1-260; done
