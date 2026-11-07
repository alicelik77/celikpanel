# alpha.128 case 2 (part 2): the customer's pool and nginx file, ini_set/.user.ini/set_time_limit, a 90 s script. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C2php-$CELL.log) 2>&1
D=cust1.$Z
echo "== 2b': the pool and nginx file after the plan clamp (own values 256 MB / 120 s / 64 MB / 3000 / 4) $(ts)"; grun main g_phpt.sh $D pool
echo "== 2d: the application cannot raise them (ini_set, set_time_limit, .user.ini) $(ts)"
grun main g_phpt.sh $D put; sleep 2; grun main g_phpt.sh $D ini
echo "== 2e: a 90 s script with run time 120 s (fastcgi_read_timeout) $(ts)"; GQT=500 grun main g_phpt.sh $D sleep 90
echo "   a 140 s script (above the run time; stopped):"; GQT=500 grun main g_phpt.sh $D sleep 140
echo "== c2b done $(ts)"
