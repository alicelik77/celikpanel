export CELL=r7 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C9egress-r7-main.log) 2>&1
echo "== on lab 130: the guard file is written at the next sandbox write; Install packages and build again (node7n.test) $(ts)"
admn main POST /api/v1/domains/3/app/packages "{\"request_id\":\"node-$(rid)\",\"build\":true}" --label app-packages --timeout 900 | sbrief 300
sleep 75; grun main g_eg4.sh
