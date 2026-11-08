# case 5: web settings of site1: missing error page refused, custom 404, page cache MISS/HIT, one allowed directive
# applied, one nginx refuses put back (tree hash), one forbidden refused. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/hdr.sh
exec > >(tee -a $LOGS/C5web-$S.log) 2>&1
D=site1.$Z
gw() { grun main g_web.sh $D $S1 "$@"; }
ws() { adm POST /api/v1/domains/$S1/web-settings "$1" --label web-settings | python3 -c '
import json,sys; d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print(d["status"], b[:300]); sys.exit()
if d["status"]==200: print(200, "saved", json.dumps(j.get("settings"))[:500])
else: print(d["status"], json.dumps({k:j.get(k) for k in ("code","reason","error","vars","details")})[:700])'; }
echo "== 5.0 files $(ts)"; gw files
echo "== 5.1 an error page whose file is missing -> refused $(ts)"; ws '{"settings":{"error_pages":[{"code":500,"path":"/errors/500.html"}]}}'
echo "== 5.2 index + custom 404 + page cache 5 min $(ts)"
ws '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}]},"page_cache":true,"page_cache_minutes":5}'
gw check
grun main g_pc3.sh $D
adm POST /api/v1/domains/$S1/web-settings/cache-clear '{}' --label cache-clear | sbrief 200
echo "   after clear:"; grun main g_pc3.sh $D
gshn main 'stat -c "%A %U:%G %n" /var/cache/celikpanel /var/cache/celikpanel/pagecache 2>&1'
echo "== 5.3 directives $(ts)"
H0=$(gw hash | tail -1); echo "   nginx tree before $H0"
echo "   nginx refuses (client_max_body_size abc):"; adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"client_max_body_size abc;"}' --label dir-bad | sbrief 600
H1=$(gw hash | tail -1); echo "   nginx tree after  $H1 (same: $([ "$H0" = "$H1" ] && echo yes || echo NO))"
gshn main "nginx -t 2>&1 | tail -1; systemctl is-active nginx"
echo "   forbidden (root /etc):"; adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"root /etc;"}' --label dir-forbidden | sbrief 500
echo "   allowed (add_header):"; adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"add_header X-Lab-Directive yes always;"}' --label dir-good | sbrief 300
grun main g_curl.sh $D /home.html x-lab
echo "== 5.4 page cache off $(ts)"; ws '{"settings":{"index_files":["home.html","index.php"],"error_pages":[{"code":404,"path":"/errors/404.html"}]}}'
grun main g_curl.sh $D /missing-a129
echo "== k5web $S done $(ts)"
