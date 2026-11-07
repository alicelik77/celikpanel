export S=ar IPU=192.0.3.222
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/hdr.sh
exec > >(tee -a $LOGS/C6db-ar-mysql.log) 2>&1
echo "== 5.1 again on the lab 130 label (mariadb client) $(ts)"; adm GET /api/v1/domains/$S1/databases/1/tools --label tools130 | sbrief 500; adm GET /api/v1/domains/4/databases/3/tools --label tools130b | sbrief 500
