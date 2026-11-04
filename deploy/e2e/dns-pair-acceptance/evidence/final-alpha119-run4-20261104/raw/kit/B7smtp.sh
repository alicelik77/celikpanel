export CELL=u118 IP=192.0.3.191 Z=u118.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/B7smtp-$CELL.log) 2>&1
echo "== B SMTP from the n8n unit cgroup after refresh $(ts)"
GQT=90 gq g_n8n7.sh celiksvc-1 | grep -E "smtp|counter|cgroup"
