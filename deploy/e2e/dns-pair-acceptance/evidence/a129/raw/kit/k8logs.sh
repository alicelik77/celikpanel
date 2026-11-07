# once: log retention of cust1.$Z (subscription scope): only the site's rotated files older than the limit go. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/hdr.sh
exec > >(tee -a $LOGS/C8logs-$S.log) 2>&1
D=cust1.$Z
echo "== 8.0 what the screen reads before $(ts)"; adm GET /api/v1/domains/$C1/logs/retention --label ret-get | sbrief 1500
echo "   customer reads it:"; as cust1 GET /api/v1/domains/$C1/logs/retention | sbrief 600
echo "== 8.1 lab load: rotated files with old and young dates, another site's, another name, a link $(ts)"
grun main g_logs.sh $D site1.$Z make
echo "== 8.2 administrator sets 7 days for the subscription $(ts)"
adm POST /api/v1/domains/$C1/logs/retention '{"scope":"subscription","keep_days":7}' --label ret-set --timeout 200 | sbrief 1500
grun main g_logs.sh $D site1.$Z list
echo "== 8.3 the screen after $(ts)"; adm GET /api/v1/domains/$C1/logs/retention --label ret-get2 | sbrief 1500
echo "   customer tries to change it:"; as cust1 POST /api/v1/domains/$C1/logs/retention '{"scope":"subscription","keep_days":1}' | sbrief 400
echo "   0 / 4000 days:"; adm POST /api/v1/domains/$C1/logs/retention '{"scope":"subscription","keep_days":4000}' --label ret-bad | sbrief 400
gshn main 'ls -la /etc/logrotate.d/ | grep -i nginx; grep -hE "rotate|daily|weekly" /etc/logrotate.d/nginx* 2>/dev/null | head -4; sha256sum /etc/logrotate.d/nginx* 2>/dev/null | cut -c1-16'
echo "== k8logs $S done $(ts)"
