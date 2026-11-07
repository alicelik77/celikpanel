# alpha.128 case 2i: PHP version change while nginx is stopped -> WEB_SERVER_STOPPED and nothing moved; then the
# customer's site on the second PHP version keeps its limits (ini_set/.user.ini on 8.3 too). Lab only.
export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C2ver-$CELL.log) 2>&1
N=main; S1=$(cat $T/site1.id); C1=$(cat $T/cust1.id)
phpv() { admn $N GET /api/v1/domains/$1/php --label php-get | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("   version", b.get("php_version"), "available", b.get("available_versions"))'; }
echo "== second PHP version: Sury lane + PHP-FPM 8.3 $(ts)"
admn $N GET "/api/v1/repo/lane?service_id=php-fpm" --label lane > $T/lanep.json; sbrief 300 < $T/lanep.json
SHA=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); p=b.get("plan") or b; print(p.get("sha256",""))' $T/lanep.json)
admn $N POST /api/v1/repo/lane "{\"service_id\":\"php-fpm\",\"expected_sha256\":\"$SHA\"}" --label lane-enable --timeout 600 | sbrief 300
installc $N php-fpm php8.3-fpm
phpv $S1
H0=$(gshn $N 'find /etc/php /etc/nginx -type f | sort | xargs sha256sum | sha256sum | cut -c1-16'); echo "   files before $H0"
echo "== nginx stopped outside the panel (lab load); change site1 to 8.3 $(ts)"
gshn $N 'systemctl stop nginx; systemctl is-active nginx'
admn $N POST /api/v1/domains/$S1/php '{"php_version":"8.3"}' --label php-stopped --timeout 300 | sbrief 900
H1=$(gshn $N 'find /etc/php /etc/nginx -type f | sort | xargs sha256sum | sha256sum | cut -c1-16'); echo "   files after  $H1 (unchanged: $([ "$H0" = "$H1" ] && echo yes || echo NO))"
phpv $S1
gshn $N 'systemctl start nginx; systemctl is-active nginx'
echo "== with nginx running: site1 -> 8.3 and back $(ts)"
admn $N POST /api/v1/domains/$S1/php '{"php_version":"8.3"}' --label php-83 --timeout 300 | sbrief 300; phpv $S1
gshn $N "curl -sSk -m 10 -o /dev/null -w 'site1 %{http_code}\n' --resolve site1.$Z:443:127.0.0.1 --resolve site1.$Z:80:127.0.0.1 -L http://site1.$Z/clock.php"
echo "== cust1.$Z on 8.3: limits still hold $(ts)"
admn $N POST /api/v1/domains/$C1/php '{"php_version":"8.3"}' --label php-c83 --timeout 300 | sbrief 300; phpv $C1
grun $N g_phpt.sh cust1.$Z pool; grun $N g_phpt.sh cust1.$Z ini
echo "== c2ver done $(ts)"
