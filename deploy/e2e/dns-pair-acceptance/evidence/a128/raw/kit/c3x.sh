# alpha.128 case 3a (again, on the lab 129 label with the page cache fix): the cache folders made 0750 as the
# 128 Agent left them; the next save corrects them; MISS/HIT/BYPASS/clear; a directive with alias /etc; nginx -t refusal
# with the tree hash; custom 404 for a missing PHP file. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C3x-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
gw() { grun main g_web.sh $D $S1 "$@"; }
echo "== versions $(ts)"; ver main
echo "== 3x.1 the 128 state: cache folders 0750 (lab load) $(ts)"
gshn main 'chmod 0750 /var/cache/celikpanel /var/cache/celikpanel/pagecache; stat -c "%A %U:%G %n" /var/cache/celikpanel /var/cache/celikpanel/pagecache'
adm POST /api/v1/domains/$S1/web-settings '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}],"page_cache":true,"page_cache_minutes":5}}' --label pc-on | sbrief 100
gshn main 'stat -c "%A %U:%G %n" /var/cache/celikpanel /var/cache/celikpanel/pagecache'
grun main g_pc3.sh $D
adm POST /api/v1/domains/$S1/web-settings/cache-clear '{}' --label cache-clear | sbrief 200
grun main g_pc3.sh $D
echo "== 3x.2 page cache off; custom 404 for a missing PHP file $(ts)"
adm POST /api/v1/domains/$S1/web-settings '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}]}}' --label pc-off | sbrief 100
grun main g_curl.sh $D /nope-a128.php
echo "== 3x.3 directives: a location with alias /etc (administrator) $(ts)"
H0=$(gw hash | tail -1)
adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"location /lab-etc/ { alias /etc/; }"}' --label dir-alias | sbrief 600
grun main g_curl.sh $D /lab-etc/hostname
echo "   a directive nginx itself refuses (unknown directive):"
adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"lab_unknown_directive on;"}' --label dir-unknown | sbrief 600
H1=$(gw hash | tail -1); echo "   nginx tree before $H0 after $H1"
adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"add_header X-Lab-Directive yes always;"}' --label dir-good | sbrief 120
echo "== 3x.4 a reseller/customer may not edit directives (not offered) $(ts)"
C1=$(cat $T/cust1.id); as cust1 POST /api/v1/domains/$C1/web-settings/directives '{"text":"add_header X-C yes;"}' | sbrief 400
echo "== c3x done $(ts)"
