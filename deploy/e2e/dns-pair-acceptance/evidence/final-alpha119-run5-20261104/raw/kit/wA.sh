source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/warnA-$CELL.log) 2>&1
echo "== A mail quota after the hourly reconcile $(ts)"
adm GET /api/v1/usage/status --label usage-status | sbrief 800
GQT=60 gq gW.sh q@c118.test
echo "-- 300 KB to q@c118.test (now 1.25 MB in a 1 MB box; expect 552 5.2.2)"; lc smtpsize - q@c118.test 300000
echo "-- 300 KB to box@c118.test (100 MB; expect accepted)"; lc smtpsize - box@c118.test 300000
GQT=60 gq gW.sh q@c118.test
