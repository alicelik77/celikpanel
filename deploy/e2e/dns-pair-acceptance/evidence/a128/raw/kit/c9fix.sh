# alpha.127 case 9 guard fix check: build a lab 130 label of release/alpha127 e129b5d8a, update the main through its
# Update API, then the guard and systemctl --failed. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit
bash $A/build130.sh
export CELL=r7 IP=192.0.3.190 Z=r7.test
source $A/lib.sh; source $A/lib5.sh
bash $A/portalF.sh latest v0.1.0-alpha.130
CELL=r7 IP=192.0.3.190 bash $A/upd.sh main > $A/c9fix-upd.out 2>&1
exec > >(tee -a $LOGS/C9egress-r7-main.log) 2>&1
echo "== main 127 -> lab 130 (e129b5d8a)"; grep -E "^  [0-9]+s |schema_version|^\(end\)|unverified" $A/c9fix-upd.out | cut -c1-250
sleep 70
grun main g_eg4.sh
