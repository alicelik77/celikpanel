# alpha.128 case 3a.5'': page cache with the cache folders made traversable for nginx's worker (lab load standing for the fix). Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C3web-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
echo "== 3a.5'' page cache after chmod 0755 of /var/cache/celikpanel and its pagecache folder (lab load = the fix) $(ts)"
gshn main 'chmod 0755 /var/cache/celikpanel /var/cache/celikpanel/pagecache; stat -c "%A %U:%G %n" /var/cache/celikpanel /var/cache/celikpanel/pagecache'
adm POST /api/v1/domains/$S1/web-settings '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}],"page_cache":true,"page_cache_minutes":5}}' --label pc-on2 | sbrief 120
grun main g_pc3.sh $D
adm POST /api/v1/domains/$S1/web-settings/cache-clear '{}' --label cache-clear2 | sbrief 200
grun main g_pc3.sh $D
echo "-- custom 404 for a missing PHP file (nginx's own 404):"; gshn main "curl -sk -o /tmp/n.out -w '%{http_code} ' --resolve $D:80:127.0.0.1 -L http://$D/nope-a128.php; head -c 80 /tmp/n.out; echo"
adm POST /api/v1/domains/$S1/web-settings '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}]}}' --label pc-off2 | sbrief 100
