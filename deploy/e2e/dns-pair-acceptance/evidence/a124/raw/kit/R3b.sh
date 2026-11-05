# alpha.124 case 3 (site part) on p124 main: the main site moved to PHP 8.3, then removing 8.3 is refused
# naming phpMyAdmin and the site; the site moved back. Lab only.
export CELL=p124 IP=192.0.3.190 Z=p124.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a124lab/kit/lib.sh
source $H/common.sh; source $A/lib11.sh; source $A/lib123.sh; source $A/lib124.sh
exec > >(tee -a $LOGS/R3b-$CELL.log) 2>&1
DID=$(domain_id p124.test); echo "p124.test id=$DID"
echo "== 3.4b the site p124.test on PHP 8.3 $(ts)"
adm GET /api/v1/domains/$DID/php --label php-get | sbrief 300
adm POST /api/v1/domains/$DID/php '{"php_version":"8.3"}' --label php-83 | sbrief 400
adm GET /api/v1/domains/$DID/php --label php-get2 | sbrief 300
echo "== 3.5b remove PHP 8.3: refused, naming phpMyAdmin and the site $(ts)"
impact "service_id=php-fpm&package=php8.3-fpm"
uninst '{"service_id":"php-fpm","package":"php8.3-fpm"}' 2500
comps php-fpm
echo "-- the site back on 8.4"; adm POST /api/v1/domains/$DID/php '{"php_version":"8.4"}' --label php-84 | sbrief 300
echo "== R3b done $(ts)"
