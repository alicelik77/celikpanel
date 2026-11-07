# once: the copy refused because the target has content (database asked): is the copy database removed? Lab only.
export S=ar IPU=192.0.3.222
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/hdr.sh
exec > >(tee -a $LOGS/C9copy-ar.log) 2>&1
BIG=$(cat $T/big.id); TGT=$(cat $T/tgt.id)
echo "== 9.4 before $(ts)"; gshn main "mariadb -N -e \"show databases like \\\"tgt%\\\"\" | xargs; mariadb -N -e \"select user from mysql.user where user like \\\"tgt%\\\"\" | xargs"
echo "== 9.5 copy (database 3) without overwrite onto tgt with content $(ts)"; T0=$(date +%s)
adm POST /api/v1/domains/$BIG/copy "{\"target_domain_id\":$TGT,\"database_id\":3,\"overwrite\":false}" --label copy-no2 --timeout 3000 | sbrief 900; echo "   answer after $(( $(date +%s) - T0 )) s"
gshn main "mariadb -N -e \"show databases like \\\"tgt%\\\"\" | xargs; mariadb -N -e \"select user from mysql.user where user like \\\"tgt%\\\"\" | xargs"
admn main GET /api/v1/domains/$TGT/databases --label tdbs2 | sbrief 500
gshn main "journalctl -u celikpanel-agent --since -10min -o cat --no-pager | grep -iE \"delete|drop|database|copy\" | tail -6 | cut -c1-300"
echo "== 9.5 done $(ts)"
