source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/php-$CELL.log) 2>&1
echo "== admin: Components -> PHP 8.4 (explicit version, companions) $(ts)"
adm POST /api/v1/service/install-preview '{"service_id":"php-fpm","package":"php8.4-fpm"}' --label php-preview | sbrief 1500
R=$(rid); svcstart "{\"service_id\":\"php-fpm\",\"package\":\"php8.4-fpm\",\"request_id\":\"$R\"}" /api/v1/service/install; svcwait $R 900
adm GET "/api/v1/service/operation?request_id=$R" --label php-op | sbrief 1500
gsh1 'dpkg -l "php8.4-*" | grep ^ii | awk "{print \$2}" | tr "\n" " "'
