# the owner presses Start again in Server setup after the 127 mail profile step failed (lab). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/env.sh
source $H/common.sh
exec > >(tee -a $LOGS/pairR-r9.log) 2>&1

echo "== retry main setup $(ts)"
b1 api r9 main GET /api/v1/setup/operation --label op --limit 400000 | python3 $H/opsum.py | head -2 | cut -c1-300
b1 plan r9 main --limit 60000 > /tmp/plan9.json; python3 $H/plansum2.py /tmp/plan9.json "plan main" | cut -c1-400
b1 start r9 main --limit 3000 | st start-main 600; sleep 20
TRIES=60 EVERY=15 wait_status r9 main succeeded failed
for ip in 192.0.3.190 192.0.3.191; do echo "  @$ip A frankfurt.r9.test: $(dig1 $ip frankfurt.r9.test A)"; done
echo "== retry done $(ts)"
