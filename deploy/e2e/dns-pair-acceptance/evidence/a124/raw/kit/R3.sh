# alpha.124 case 3 on p124 main (Debian 13): phpPgAdmin with PostgreSQL installed together (reload + panel restart
# in the middle), PHP 8.3 beside 8.4, phpMyAdmin on 8.3, removing 8.3 refused naming phpMyAdmin and the site. Lab only.
export CELL=p124 IP=192.0.3.190 Z=p124.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
source $A/lib11.sh; source $A/lib123.sh; source $A/lib124.sh
exec > >(tee -a $LOGS/R3-$CELL.log) 2>&1
IDS=postgresql,phppgadmin,phpmyadmin,php-fpm,nginx,mariadb
echo "== 3.0 components $(ts)"; comps $IDS
echo "== 3.1 phpPgAdmin: the plan lists PostgreSQL installed together $(ts)"
prepare phppgadmin
R=$(cat $T/rid-phppgadmin)
adm POST /api/v1/service/install @$T/body-phppgadmin.json --label install-together > $T/it.json; sbrief 900 < $T/it.json
OID=$(jget operation.id < $T/it.json); [ -z "$OID" ] && OID=$(jget id < $T/it.json); echo "  operation id=$OID"
sleep 4
echo "-- reload: a new page reads the active operation; the tab's marker reads by request id $(ts)"
opread "active=1"; opread "request_id=$R"
sleep 8; opread "request_id=$R"
echo "-- panel restart in the middle (as the owner would: systemctl restart celikpanel-panel) $(ts)"
gsh1 'systemctl restart celikpanel-panel; systemctl is-active celikpanel-panel'
panelup
echo "-- after the restart: new page and tab marker $(ts)"
opread "active=1"; opread "request_id=$R"
followr "request_id=$R" 300
adm GET "/api/v1/service/operation?request_id=$R" --label op-together | sbrief 1500
comps $IDS
gsh1 'pg_lsclusters 2>&1 | tail -2; systemctl is-active postgresql; ls /etc/nginx/conf.d /etc/nginx/sites-enabled 2>/dev/null | grep -i pgadmin; journalctl -u celikpanel-panel --since "-15min" -o cat --no-pager | grep -iE "together|resum" | cut -c1-250 | tail -8'
echo "== 3.2 PHP 8.3 beside 8.4 (Sury lane from Components, then php8.3-fpm) $(ts)"
adm GET "/api/v1/repo/lane?service_id=php-fpm" --label lane-php > $T/lane.json; python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); p=b.get("plan") or {}; print("  lane", b.get("state"), "ready", p.get("ready"), "steps", len(p.get("steps") or []), "error", p.get("error") or b.get("error"))' $T/lane.json
SHA=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("plan") or {}).get("sha256",""))' $T/lane.json)
[ -n "$SHA" ] && adm POST /api/v1/repo/lane "{\"service_id\":\"php-fpm\",\"expected_sha256\":\"$SHA\"}" --label lane-enable-php --timeout 800 | sbrief 500
installc php-fpm php8.3-fpm
comps php-fpm
echo "== 3.3 phpMyAdmin on PHP 8.3 (not the distribution's 8.4) $(ts)"
prepare phpmyadmin 8.3
svcstart "$(cat $T/body-phpmyadmin.json)" /api/v1/service/install && svcwait $(cat $T/rid-phpmyadmin) 900
adm GET "/api/v1/service/operation?request_id=$(cat $T/rid-phpmyadmin)" --label op-pma | sbrief 900
comps phpmyadmin,php-fpm
gsh1 'grep -rlE "php8\.[0-9]" /etc/nginx/ 2>/dev/null | xargs grep -hE "fastcgi_pass|php8\.[0-9]-fpm" 2>/dev/null | grep -iE "pma|phpmyadmin|php8" | sort -u | head -8; ls /etc/php/*/fpm/pool.d/ 2>/dev/null'
echo "== 3.4 a site on PHP 8.3 $(ts)"
mkcred cust3
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust3.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-cust3 | sbrief 200; rm -f $T/u.json
SUB=$(sub_of cust3)
infra delegate php83.test ns1.$Z $IP ns2.$Z 192.0.3.191 > /dev/null
DID=$(newdomain cust3 $SUB php83.test); echo "php83.test id=$DID"; echo $DID > $T/did3
as cust3 POST /api/v1/domains/$DID/php '{"php_version":"8.3"}' | sbrief 400
as cust3 GET /api/v1/domains/$DID/php | sbrief 400
echo "== 3.5 remove PHP 8.3: refused, naming phpMyAdmin and the site $(ts)"
impact "service_id=php-fpm&package=php8.3-fpm"
adm GET "/api/v1/service/removal-impact?service_id=php-fpm&package=php8.3-fpm" --label impact-php83 | sbrief 2500
uninst '{"service_id":"php-fpm","package":"php8.3-fpm"}' 2500
comps php-fpm
echo "== R3 done $(ts)"
