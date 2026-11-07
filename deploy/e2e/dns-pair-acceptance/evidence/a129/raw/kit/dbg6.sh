source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
source $H/common.sh
pgrep -af "pairX|runP_r9|chainR2" | cut -c1-120
b1 api r9 main GET /api/v1/setup/operation --label op --limit 3000 | python3 $H/opsum.py | head -3
b1 plan r9 main --limit 60000 > /tmp/plan9.json; python3 $H/plansum2.py /tmp/plan9.json "plan main" | cut -c1-600
