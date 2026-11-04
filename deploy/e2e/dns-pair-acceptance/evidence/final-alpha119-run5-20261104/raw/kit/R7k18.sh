source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R7item18-$CELL.log) 2>&1
echo "== item 18 detail run (run.sh quick, instrumented) $(ts)"; GQT=900 gq g_kit18.sh
echo "== done $(ts)"
