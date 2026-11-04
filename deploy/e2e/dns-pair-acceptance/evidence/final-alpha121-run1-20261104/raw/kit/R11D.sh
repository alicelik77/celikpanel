# alpha.121 D (Debian 13 at 121, fresh): PGDG 18 preview + install, host-name notice. Lab only.
export CELL=d121 IP=192.0.3.190 Z=d121.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
source $A/lib11.sh
exec > >(tee -a $LOGS/R11D-$CELL.log) 2>&1
echo "== 8 host-name notice $(ts)"; gsh1 'hostname -f; cat /opt/celikpanel/release.commit'
adm GET "/api/v1/dns/server-hostname?recheck=1" --label hostname | sbrief 1200
echo "== 7 PGDG 18 $(ts)"; lane
installc postgresql postgresql-18
gsh1 'pg_lsclusters; dpkg -l postgresql-common postgresql-client-common libpq5 | grep ^ii | awk "{print \$2, \$3}"'
adm GET /api/v1/database-servers --label dbs | sbrief 600
echo "== R11D done $(ts)"
