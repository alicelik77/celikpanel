source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/wpchk-$CELL.log) 2>&1
echo "== $(ts) site checks"
for d in wp118.test stage2.wp118.test bdrup118.test bjoom118.test; do lc get - $d ""; done
cat > $A/g_tmp3.sh <<'G'
grep -h "wp118" /var/log/nginx/error.log 2>/dev/null | tail -5 | cut -c1-300
ls /run/php/ 2>/dev/null; systemctl is-active php8.4-fpm
G
gq g_tmp3.sh
