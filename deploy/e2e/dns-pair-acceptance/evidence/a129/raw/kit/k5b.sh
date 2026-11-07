# case 5 (part 2): page cache on (inside settings) MISS/HIT/clear, custom 404 for a missing PHP file, a directive
# nginx itself refuses (unknown) with the tree hash, then cache off. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/hdr.sh
exec > >(tee -a $LOGS/C5web-$S.log) 2>&1
D=site1.$Z
gw() { grun main g_web.sh $D $S1 "$@"; }
echo "== 5b.1 page cache 5 minutes (in settings) $(ts)"
adm POST /api/v1/domains/$S1/web-settings '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}],"page_cache":true,"page_cache_minutes":5}}' --label pc-on | sbrief 160
gshn main 'stat -c "%A %U:%G %n" /var/cache/celikpanel /var/cache/celikpanel/pagecache 2>&1'
grun main g_pc3.sh $D
adm POST /api/v1/domains/$S1/web-settings/cache-clear '{}' --label cache-clear | sbrief 200
echo "   after clear:"; grun main g_pc3.sh $D
echo "== 5b.2 custom 404 for a missing PHP file $(ts)"; grun main g_curl.sh $D /nope-a129.php
echo "== 5b.3 a directive nginx refuses (unknown directive) $(ts)"
H0=$(gw hash | tail -1)
adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"add_header X-Lab-Directive yes always;\nlab_unknown_directive on;"}' --label dir-unknown | sbrief 500
H1=$(gw hash | tail -1); echo "   nginx tree before $H0 after $H1 (same: $([ "$H0" = "$H1" ] && echo yes || echo NO))"
grun main g_curl.sh $D /home.html x-lab
echo "== 5b.4 page cache off $(ts)"
adm POST /api/v1/domains/$S1/web-settings '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}]}}' --label pc-off | sbrief 120
grun main g_pc3.sh $D
echo "== k5b $S done $(ts)"
