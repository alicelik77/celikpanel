# run 2 D4: 1 MB disk limit closes PHP uploads, lifting it opens them again. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R2lim-$CELL.log) 2>&1
SUBA=$(cat $T/suba); DID=$(cat $T/did)
pool() { gsh1 "grep -H file_uploads /etc/php/*/fpm/pool.d/site*.conf; ls -la --time-style=+%T /etc/php/8.4/fpm/pool.d/ | grep site"; }
echo "== R2lim $(ts)"
python3 $A/mkup.py $T/big.json big.bin gen:2500000; python3 $A/mkup.py $T/php.json up.php php
for f in big php; do as cust118a POST "/api/v1/domains/$DID/files?path=/" @$T/$f.json | sbrief 200; done; rm -f $T/big.json
lc phpup - c118.test
echo "-- first measurement"; waitround; subusage cust118a
echo "== disk limit 1 MB $(ts)"; setq $SUBA '{"disk_quota_mb":1}'
waitround; subusage cust118a; pool; lc phpup - c118.test
echo "== disk limit lifted $(ts)"; setq $SUBA '{"disk_quota_mb":0}'
waitround; subusage cust118a; pool; lc phpup - c118.test
echo "== R2lim done $(ts)"
