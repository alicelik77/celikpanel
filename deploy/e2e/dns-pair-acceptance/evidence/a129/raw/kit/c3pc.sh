export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C3web-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
echo "== 3a.5' page cache diagnosis $(ts)"
adm POST /api/v1/domains/$S1/web-settings '{"settings":{"index_files":["home.html","index.php"],"page_cache":true,"page_cache_minutes":5}}' --label pc-on | sbrief 300
grun main g_pc.sh $D
adm POST /api/v1/domains/$S1/web-settings '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}]}}' --label pc-off | sbrief 200
gshn main "curl -sk -o /dev/null -w 'after off: clock %{http_code} nope.php %{http_code}\n' --resolve $D:80:127.0.0.1 -L http://$D/clock.php; curl -sk --resolve $D:80:127.0.0.1 -L http://$D/nope-a128.php | head -c 120"
