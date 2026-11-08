# alpha.130 round: owner-path screenshots of A (lab). usage: ui.sh NODE PREFIX ITEMS.json [langs] [widths]
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
b1 login s0 $1 > /dev/null 2>&1
timeout 1500 python3 -I $A/shots130.py s0 $1 $2 $A/$3 --langs ${4:-en,tr} --widths ${5:-desktop} 2>&1 | tail -c 3000
ls /var/tmp/a130o/shots | grep "^$2" | head -40
