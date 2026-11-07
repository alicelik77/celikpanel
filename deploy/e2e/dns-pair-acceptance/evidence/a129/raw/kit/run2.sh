# alpha.127 case 2: the owner's pair 126 -> 127 through the panel's own Update (joined server first, then the main). Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit
source $A/env.sh
bash $A/portalF.sh latest v0.1.0-alpha.127
lab ssh beta1-lab labnet -- "sudo -n find /root/a114-portal/site/releases -maxdepth 1"
CELL=r7 IP=192.0.3.191 bash $A/upd.sh backup > $A/c2-backup.out 2>&1
CELL=r7 IP=192.0.3.190 bash $A/upd.sh main > $A/c2-main.out 2>&1
for n in backup main; do echo "== $n"; grep -E "^  [0-9]+s |succeeded|schema|systemctl|unverified|^\(end\)|failed" $A/c2-$n.out | cut -c1-260; done
echo "run2 done $(date -u +%T)"
