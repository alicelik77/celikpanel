CELL=u117 IP=192.0.3.191 Z=u117.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/warnB-$CELL.log) 2>&1
echo "== B 90% warning check $(ts)"; GQT=60 gq gW.sh q@cb117.test
