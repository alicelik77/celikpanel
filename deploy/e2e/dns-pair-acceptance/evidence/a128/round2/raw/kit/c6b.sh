# alpha.127 case 6 (part 2): Boston's state staged on the main (unzip, zip removed: lab load); customer then admin. Lab only.
export CELL=r7 IP=192.0.3.190 Z=r7.test MIP=192.0.3.190 BIP=192.0.3.191
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/C6-r7-main.log) 2>&1
echo "== stage: remove unzip and zip (lab load) $(ts)"; grun main g_rmzip.sh
adm GET /api/v1/domains/4/access --label access > $T/acc.json
python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); s=b["state"]["server"]; print({k:v for k,v in s.items() if "jail" in k})' $T/acc.json
echo "== customer: Access tab (EN, TR) $(ts)"
timeout 900 python3 -I $H/shots.py $CELL main c6b-cust $T/acc-c.json --langs en,tr --widths desktop --user cust7 --pwfile $T/cred-cust7.json | python3 $A/shotsum.py
echo "== administrator: Install the tools (EN) $(ts)"
timeout 1500 python3 -I $H/shots.py $CELL main c6b-install $T/acc-i.json --langs en --widths desktop | python3 $A/shotsum.py
grun main g_pkgs.sh
echo "== customer after the install (EN) $(ts)"
timeout 900 python3 -I $H/shots.py $CELL main c6b-cust-after $T/acc-c.json --langs en --widths desktop --user cust7 --pwfile $T/cred-cust7.json | python3 $A/shotsum.py
echo "== c6b done $(ts)"
