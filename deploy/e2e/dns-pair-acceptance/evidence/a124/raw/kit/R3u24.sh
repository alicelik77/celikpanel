# alpha.124 case 3 (PHP part) on u24 (Ubuntu 24.04, distribution PHP 8.3): PHP 8.4 beside it from the Sury lane,
# phpMyAdmin on 8.4, a customer PHP site on 8.4, removing 8.4 refused naming phpMyAdmin and the site. Lab only.
export CELL=u24 IP=192.0.3.195 Z=u24.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
source $H/common.sh; source $A/lib11.sh; source $A/lib123.sh; source $A/lib124.sh
exec > >(tee -a $LOGS/R3-$CELL.log) 2>&1
echo "== 3u.1 PHP 8.4 beside 8.3 (Sury lane from Components, then php8.4-fpm) $(ts)"
comps php-fpm,phpmyadmin
adm GET "/api/v1/repo/lane?service_id=php-fpm" --label lane-php > $T/lane.json
SHA=$(python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("plan") or {}).get("sha256",""))' $T/lane.json)
[ -n "$SHA" ] && adm POST /api/v1/repo/lane "{\"service_id\":\"php-fpm\",\"expected_sha256\":\"$SHA\"}" --label lane-enable-php --timeout 800 | sbrief 300
installc php-fpm php8.4-fpm
comps php-fpm
echo "== 3u.2 phpMyAdmin on PHP 8.4 (not the distribution's 8.3) $(ts)"
prepare phpmyadmin 8.4
svcstart "$(cat $T/body-phpmyadmin.json)" /api/v1/service/install && svcwait $(cat $T/rid-phpmyadmin) 900
adm GET "/api/v1/service/operation?request_id=$(cat $T/rid-phpmyadmin)" --label op-pma | sbrief 900
echo "== 3u.3 a customer PHP site on 8.4 $(ts)"
mkcred cust3u
python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); json.dump({"username":c["username"],"email":c["username"]+"@beta1-lab.test","password":c["password"],"role":"customer"}, open(sys.argv[2],"w"))' $T/cred-cust3u.json $T/u.json
adm POST /api/v1/users @$T/u.json --label create-cust3u | sbrief 200; rm -f $T/u.json
SUB=$(sub_of cust3u)
infra delegate php84.test ns1.$Z $IP ns2.$Z $IP > /dev/null
DID=$(newdomain cust3u $SUB php84.test); echo "php84.test id=$DID"
sleep 10
as cust3u POST /api/v1/domains/$DID/php '{"php_version":"8.4"}' | sbrief 400
as cust3u GET /api/v1/domains/$DID/php | sbrief 300
echo "== 3u.4 remove PHP 8.4: refused, naming phpMyAdmin and the site $(ts)"
impact "service_id=php-fpm&package=php8.4-fpm"
uninst '{"service_id":"php-fpm","package":"php8.4-fpm"}' 2500
comps php-fpm
echo "== R3u24 done $(ts)"
