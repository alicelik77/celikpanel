export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C4mail3-$CELL.log) 2>&1
echo "== 4.5' after the password change: the pause list and postfwd (read-only) $(ts)"
grun main g_pf5.sh
adm GET /api/v1/mail/outgoing --label outgoing | python3 -c 'import json,sys; b=json.loads(json.load(sys.stdin)["body"]); print("paused", json.dumps(b.get("paused")), "apply", json.dumps(b.get("apply")))'
nup n_swaks.sh >/dev/null; echo " msg now:"; pw box1 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 box1@site1.$Z box2@site1.$Z box1@site1.$Z"
