export CELL=d117 IP=192.0.3.190 Z=d117.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R9db-$CELL.log) 2>&1
echo "== database after the update $(ts)"; dbt db9 $(cat $T/dbu)
