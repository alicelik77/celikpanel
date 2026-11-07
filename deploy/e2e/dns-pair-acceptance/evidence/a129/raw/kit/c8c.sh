export CELL=r7 IP=192.0.3.190 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C8-r7-main.log) 2>&1
echo "== (c8b stopped by the lab after the cron alert opened: the address was not confirmed yet, the token parse was wrong) $(ts)"
gshn main "systemctl start cron; systemctl is-active cron"
admn main GET /api/v1/metrics/alerts --label malerts | sbrief 1500
