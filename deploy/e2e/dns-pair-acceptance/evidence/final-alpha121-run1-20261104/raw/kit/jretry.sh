source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/jretry-$CELL.log) 2>&1
DID=$(domain_id bjoom118.test); OP=$(as cust118b GET /api/v1/domains/$DID/apps | jget installed.0.operation_id)
echo "== Joomla retry (guidance) $(ts) domain $DID op $OP"
AS_TIMEOUT=600 as cust118b POST /api/v1/domains/$DID/apps/$OP/retry | sbrief 700
lc get - bjoom118.test ""
cat > $A/g_tmp4.sh <<'G'
journalctl --no-pager --since -3min 2>/dev/null | grep -iE 'joomla|installation/joomla.php|PHP (Fatal|Warning)' | tail -12 | cut -c1-400
ls /var/www/celikpanel/subscriptions/4/sites/15/ 2>&1 | head; ls /var/www/celikpanel/subscriptions/4/sites/15/public_html/installation 2>&1 | head -5
G
gq g_tmp4.sh
