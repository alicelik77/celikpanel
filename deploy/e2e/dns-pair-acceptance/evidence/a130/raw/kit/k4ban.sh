# alpha.129 case 4 (fix check): Fail2ban bans a listed address after wrong MariaDB passwords. The client is the lab
# host 192.0.3.2 (no reverse name, so MariaDB logs its address). Lab only.
export CELL=${CELL:-r9} Z=${Z:-r9.test} MIP=${MIP:-192.0.3.190} BIP=${BIP:-192.0.3.191} IP=${IP:-192.0.3.190}
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C4ban-$CELL.log) 2>&1
R=/api/v1/database-remote; ENG=mysql
command -v mariadb >/dev/null || pacman -S --noconfirm --needed mariadb-clients >/dev/null 2>&1
DB=$(cat $T/rdb-$ENG.id); U=$(cat $T/rdb-$ENG.user); N=$(cat $T/rdb-$ENG.name)
echo "== b.1 a rule for 192.0.3.2 $(ts)"
as cust1 POST $R/databases/$DB/rules "{\"user_id\":$(cat $T/rdb-$ENG.uid),\"address\":\"192.0.3.2\"}" | sbrief 300
sleep 40
my() { MYSQL_PWD="$1" timeout 12 mariadb --connect-timeout=8 -h $MIP -u $U --ssl --skip-ssl-verify-server-cert -N -B $N -e "select 'ok'" 2>&1 | tail -1 | cut -c1-160; }
echo "   right password: $(my "$(pw rdb-$ENG)")"
echo "== b.2 seven wrong passwords from 192.0.3.2 $(ts)"
for i in 1 2 3 4 5 6 7; do echo "   $i: $(my "Wrong-$i-x")"; done
sleep 8
grun main g_f2b.sh status
echo "   right password after the wrong ones: $(my "$(pw rdb-$ENG)")"
grun main g_f2b.sh unban; sleep 3; echo "   after unban: $(my "$(pw rdb-$ENG)")"
echo "== k4ban done $(ts)"
