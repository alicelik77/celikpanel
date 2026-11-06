export CELL=r7 IP=192.0.3.190 Z=r7.test MIP=192.0.3.190 BIP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a127lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C6-r7-main.log) 2>&1
echo "== (the notice shows once File access is ticked; items now tick it) $(ts)"
python3 $A/uiitems.py access-view cust7.test $T/acc-c.json customer; python3 $A/uiitems.py access-install site7.test $T/acc-i.json
echo "== customer: Access tab, file access ticked (EN, TR) $(ts)"
timeout 900 python3 -I $H/shots.py $CELL main c6c-cust $T/acc-c.json --langs en,tr --widths desktop --user cust7 --pwfile $T/cred-cust7.json | python3 $A/shotsum.py
echo "== administrator: Install the tools (EN) $(ts)"
timeout 1500 python3 -I $H/shots.py $CELL main c6c-install $T/acc-i.json --langs en --widths desktop | python3 $A/shotsum.py
grun main g_pkgs.sh
echo "== customer after the install (EN) $(ts)"
timeout 900 python3 -I $H/shots.py $CELL main c6c-cust-after $T/acc-c.json --langs en --widths desktop --user cust7 --pwfile $T/cred-cust7.json | python3 $A/shotsum.py
echo "== c6c done $(ts)"
