# Arch fix check: lab 130 label of fix/a129-lab 74a7dda8e (mariadb client) on aru (128): update once, then the refused copy again. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
bash $A/portalF.sh latest v0.1.0-alpha.130
export S=ar IPU=192.0.3.222
cp $LOGS/C2upd-ar.log $LOGS/C2upd-ar-128.log; : > $LOGS/C2upd-ar.log
lab ssh aru main -- "sudo -n bash /root/g_snap.sh pre127 >/dev/null 2>&1; echo resnap"
bash $A/upd128.sh > /dev/null 2>&1
bash $A/portalF.sh latest v0.1.0-alpha.128
mv $LOGS/C2upd-ar.log $LOGS/C2upd-ar-fix130.log; grep -E "s 200|schema_version|differences|^<|^>|failed|: [0-9]{3}$|snapshot|== press" $LOGS/C2upd-ar-fix130.log | cut -c1-300
bash $A/k9b.sh
