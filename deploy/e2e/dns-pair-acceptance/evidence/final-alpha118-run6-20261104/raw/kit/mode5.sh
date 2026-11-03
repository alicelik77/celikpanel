source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R5mode-$CELL.log) 2>&1
DID=$(cat $T/did)
echo "== $(ts) docroot modes after restore"; GQT=60 gq g_mode5.sh
echo "-- upload a new file through the file manager after the restore"
python3 $A/mkup.py $T/t.json after-restore.txt text:new 2>/dev/null; as cust118a POST "/api/v1/domains/$DID/files?path=/" @$T/t.json | sbrief 200
GQT=60 gq g_mode5.sh | grep -E "after-restore|sites/2/public_html "
lc get - c118.test "after-restore.txt"
