# alpha.124 case 3 (Debian 12 part): the PostgreSQL 18 install preview names the PGDG repository. Lab only.
export CELL=d12 IP=192.0.3.193 Z=d12.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
source $A/lib11.sh; source $A/lib123.sh; source $A/lib124.sh
exec > >(tee -a $LOGS/R2-$CELL.log) 2>&1
echo "== 3d12.1 enable PGDG from Components (repository lane) $(ts)"; lane
echo "== 3d12.2 preview PostgreSQL 18 $(ts)"
adm GET "/api/v1/service/install-preview?step=postgresql:postgresql-18" --label prev18 --timeout 150 > $T/p18.json; prevsum $T/p18.json
cp $T/p18.json $LOGS/preview-pgdg18-d12.json
echo "== 3d12.3 install plan of PostgreSQL 18 (component level) $(ts)"; planread postgresql
echo "== R2d12 done $(ts)"
