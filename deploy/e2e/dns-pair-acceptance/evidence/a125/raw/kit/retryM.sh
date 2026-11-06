# the owner presses "Review the plan" then Start again after a lab-network apt fetch failure (deb.debian.org unreachable for a moment). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/env.sh
source $H/common.sh
exec > >(tee -a $LOGS/pair-p5.log) 2>&1
echo "== main: start again after the "another change was running" refusal (mail_tls_sync_host_busy) $(ts)"
b1 plan p5 main --limit 60000 > $LOGS/plan-p5-main-2.json; python3 $H/plansum2.py $LOGS/plan-p5-main-2.json "plan main again" | cut -c1-600
b1 start p5 main --limit 3000 | st start-main-again 600
EVERY=20 TRIES=60 wait_status p5 main succeeded failed
opnow p5 main | head -3 | cut -c1-600
echo "== retry done $(ts)"
