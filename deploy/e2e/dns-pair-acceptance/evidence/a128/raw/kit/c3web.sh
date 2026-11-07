# alpha.128 case 3a: web server settings of the administrator's site site1.$Z on the updated main. Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C3web-$CELL.log) 2>&1
S1=$(cat $T/site1.id); D=site1.$Z
gw() { grun main g_web.sh $D $S1 "$@"; }
fromlab() { lab ssh beta1-lab labnet -- "curl -sS -m 10 -o /dev/null -w '%{http_code}' --resolve $D:80:$MIP --resolve $D:443:$MIP -k http://$D/home.html; echo" 2>/dev/null; }
ws() { adm POST /api/v1/domains/$S1/web-settings "$1" --label web-settings | python3 -c '
import json,sys; d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print(d["status"], b[:300]); sys.exit()
if d["status"]==200: print(200, "saved", json.dumps(j.get("settings"))[:700])
else: print(d["status"], json.dumps({k:j.get(k) for k in ("code","reason","error","vars","details")})[:900])'; }
ssl() { local P; P=$(adm GET /api/v1/ssl/providers --label prov | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); l=b.get("providers") if isinstance(b,dict) else b; print(l[0]["id"])')
  adm POST /api/v1/domains/$S1/ssl/letsencrypt "{\"email\":\"admin@$Z\",\"auto_renew\":true,\"include_mail\":false,\"provider\":\"$P\",\"reissue\":$1}" --label ssl-$1 --timeout 600 | sbrief 500
  gshn main "echo | openssl s_client -connect 127.0.0.1:443 -servername $D 2>/dev/null | openssl x509 -noout -issuer -serial -enddate 2>&1 | tr '\n' ' '"; echo; }
LABIPS=$(lab ssh beta1-lab labnet -- "ip -4 -o addr show | awk '{print \$4}' | cut -d/ -f1 | grep -v '^127\.'" 2>/dev/null | tr '\n' ' '); echo "labnet addresses: $LABIPS"
DENY=$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1].split()))' "$LABIPS")
echo "== 3a.0 GET $(ts)"; adm GET /api/v1/domains/$S1/web-settings --label web-get | sbrief 1500
gw files
echo "== 3a.1 certificate from the lab CA before any setting $(ts)"; ssl false
echo "== 3a.2 an error page whose file is missing -> refused $(ts)"
ws '{"settings":{"error_pages":[{"code":500,"path":"/errors/500.html"}]}}'
echo "== 3a.3 index, MIME, browser cache 7 days, header, IP deny of the lab network, custom 404 $(ts)"
BASE="{\"index_files\":[\"home.html\",\"index.php\"],\"mime_types\":[{\"type\":\"application/x-lab\",\"extensions\":[\"lab\"]}],\"browser_cache_days\":7,\"headers\":[{\"name\":\"X-Lab\",\"value\":\"a128\"}],\"access_mode\":\"deny\",\"access_list\":$DENY,\"error_pages\":[{\"code\":404,\"path\":\"/errors/404.html\"}]"
ws "{\"settings\":$BASE}}"
gw check; echo "   from the lab network (denied): $(fromlab)"
echo "== 3a.4 certificate renewal (reissue) while the lab network is denied $(ts)"; ssl true
echo "== 3a.5 page cache 5 minutes + clear $(ts)"
ws "{\"settings\":$BASE,\"page_cache\":true,\"page_cache_minutes\":5}}"
gw check
adm POST /api/v1/domains/$S1/web-settings/cache-clear '{}' --label cache-clear | sbrief 300
gshn main "curl -sSk -m 10 --resolve $D:443:127.0.0.1 --resolve $D:80:127.0.0.1 -L http://$D/clock.php"
echo "== 3a.6 a directive nginx refuses -> refused, files put back $(ts)"
H0=$(gw hash | tail -1); echo "   nginx tree before $H0"
adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"client_max_body_size abc;"}' --label dir-bad | sbrief 800
H1=$(gw hash | tail -1); echo "   nginx tree after  $H1 (same: $([ "$H0" = "$H1" ] && echo yes || echo NO))"
gshn main "nginx -t 2>&1 | tail -1; systemctl is-active nginx"
echo "   forbidden directive:"; adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"root /etc;"}' --label dir-forbidden | sbrief 600
echo "   good directive:"; adm POST /api/v1/domains/$S1/web-settings/directives '{"text":"add_header X-Lab-Directive yes always;"}' --label dir-good | sbrief 300
gshn main "curl -sSk -m 10 -D - -o /dev/null --resolve $D:443:127.0.0.1 --resolve $D:80:127.0.0.1 -L http://$D/home.html | grep -i x-lab"
gw conf
echo "== 3a.7 access open again $(ts)"; ws "{\"settings\":{\"index_files\":[\"home.html\",\"index.php\"],\"error_pages\":[{\"code\":404,\"path\":\"/errors/404.html\"}]}}"; echo "   from the lab network: $(fromlab)"
echo "== c3web done $(ts)"
