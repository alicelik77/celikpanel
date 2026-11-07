# Ubuntu 22.04 fix check: the lab 129 label of fix/a129-lab (4a841cfe4) on the 127 server whose 128 update failed. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
bash $A/portalF.sh latest v0.1.0-alpha.129
export S=u22 IPU=192.0.3.221
bash $A/upd128.sh > /dev/null 2>&1
bash $A/portalF.sh latest v0.1.0-alpha.128
mv $LOGS/C2upd-u22.log $LOGS/C2upd-u22-fix129.log; mv $LOGS/C2-u22u-main.log $LOGS/C2-u22u-main-fix129.log 2>/dev/null; grep -E "s 200|schema_version|differences|^<|^>|failed|: [0-9]{3}$|snapshot|== press" $LOGS/C2upd-u22-fix129.log | cut -c1-400
