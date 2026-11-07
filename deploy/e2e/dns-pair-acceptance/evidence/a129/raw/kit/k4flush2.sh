# alpha.129 case 4.10b: during the window after `nft flush ruleset`, can an unlisted address sign in (engine-level limits)?
# And is the panel firewall table back afterwards? Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C4flush2-$CELL.log) 2>&1
R=/api/v1/database-remote
command -v psql >/dev/null || pacman -S --noconfirm --needed postgresql-libs postgresql >/dev/null 2>&1
echo "== the rule for 192.0.3.2 (k4ban) removed $(ts)"
RID=$(as cust1 GET $R/databases/$(cat $T/rdb-mysql.id) | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print(" ".join(str(r["id"]) for r in b.get("rules") or [] if r.get("address")=="192.0.3.2"))')
for r in $RID; do as cust1 DELETE $R/rules/$r | sbrief 200; done; sleep 45
my() { MYSQL_PWD="$(pw rdb-mysql)" timeout 12 mariadb --connect-timeout=6 -h $MIP -u $(cat $T/rdb-mysql.user) --ssl --skip-ssl-verify-server-cert -N -B $(cat $T/rdb-mysql.name) -e "select 'ok'" 2>&1 | tail -1 | cut -c1-200; }
pg() { PGPASSWORD="$(pw rdb-postgresql)" PGCONNECT_TIMEOUT=6 timeout 12 psql -X -A -t "host=$MIP dbname=$(cat $T/rdb-postgresql.name) user=$(cat $T/rdb-postgresql.user) sslmode=require" -c "select 'ok'" 2>&1 | tail -1 | cut -c1-200; }
echo "   before: my [$(my)] pg [$(pg)]"
echo "== nft flush ruleset; right passwords from the unlisted 192.0.3.2 at once $(ts)"
gshn main "nft flush ruleset; date -u +%T"
echo "   +0s my [$(my)]"; echo "   pg [$(pg)]"
sleep 40; echo "== after 40 s $(ts)"; gshn main "nft list tables | xargs"
sleep 60; echo "== after 100 s $(ts)"; gshn main "nft list tables | xargs; systemctl is-active fail2ban"
echo "== k4flush2 done $(ts)"
