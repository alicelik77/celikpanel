# alpha.128 round 2 (fix/a128-lab head, merged release/alpha128): builds 127 tag, candidate (no bump: the branch carries 128), lab 129 label; portals; labnet. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
mkdir -p $B; bash $A/sync.sh
echo "start $(ts)"; pgrep -c qemu-system
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet > /dev/null 2>&1
( timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)" ) &
bash $A/build.sh refs/tags/v0.1.0-alpha.127 v0.1.0-alpha.127 > $B/out-build127.out 2>&1
bash $A/build.sh refs/heads/fix/a128-lab v0.1.0-alpha.128 > $B/out-build128.out 2>&1
BUMP=129 bash $A/build.sh refs/heads/fix/a128-lab v0.1.0-alpha.129 > $B/out-build129.out 2>&1
wait
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp swaks git >/dev/null 2>&1; echo labnet-tools-ok"
for v in 127 128 129; do grep -E "^commit|^lab bump|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build$v.out; done
bash $A/portal.sh $C127 > $B/portal127.out 2>&1; echo "portal127 exit $?"
C=$(sed -n 's/^commit //p' $B/out-build128.out | head -1); echo $C > $B/c128
bash $A/portal.sh $C > $B/portal128.out 2>&1; echo "portal128 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal128.out
C9=$(sed -n 's/^lab bump commit //p' $B/out-build129.out | head -1); echo $C9 > $B/c129
bash $A/portal.sh $C9 > $B/portal129.out 2>&1; echo "portal129 exit $?"
PREVV=v0.1.0-alpha.127 NEXTV=v0.1.0-alpha.128 bash $A/portalF.sh up v0.1.0-alpha.127 > $A/portalnet-up.out 2>&1; tail -1 $A/portalnet-up.out
bash $A/portalF.sh add v0.1.0-alpha.129 | tail -1
free -m | sed -n 2p; echo "phase2 done $(ts)"
