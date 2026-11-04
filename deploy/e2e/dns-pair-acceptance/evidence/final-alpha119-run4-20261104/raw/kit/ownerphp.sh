source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/php-$CELL.log) 2>&1
echo "== OWNER ACTION $(ts): apt-get install php8.4-curl php8.4-gd php8.4-mysql php8.4-bcmath (per APP_EXTENSIONS_MISSING guidance)"
GQT=600 gq g_owner_php.sh
