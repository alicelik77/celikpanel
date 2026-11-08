export CELL=r7 IP=192.0.3.190 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
exec > >(tee -a $LOGS/C7-r7-main.log) 2>&1
echo "== SSL issue through the lab ACME CA (EN) $(ts)"
python3 $A/uiitems.py op-ssl site7.test $T/ssl.json
timeout 1200 python3 -I $H/shots.py r7 main c7ssl $T/ssl.json --langs en --widths desktop > /dev/null
python3 $A/jsum.py c7ssl
echo "== c7ssl done $(ts)"
