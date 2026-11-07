# alpha.128 other systems round: builds of the published 127 and 128 tags (acceptance licence), lab portals, labnet. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
mkdir -p $B; bash $A/sync.sh
echo "start $(ts)"; pgrep -c qemu-system
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet > /dev/null 2>&1
( timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)" ) &
bash $A/build.sh refs/tags/v0.1.0-alpha.127 v0.1.0-alpha.127 > $B/out-build127.out 2>&1
bash $A/build.sh refs/tags/v0.1.0-alpha.128 v0.1.0-alpha.128 > $B/out-build128.out 2>&1
wait
tail -3 $B/infra-up.out
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp swaks git >/dev/null 2>&1; echo labnet-tools-ok"
for v in 127 128; do grep -E "^commit|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build$v.out; done
bash $A/portal.sh $C127 > $B/portal127.out 2>&1; echo "portal127 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal127.out
bash $A/portal.sh $C128 > $B/portal128.out 2>&1; echo "portal128 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal128.out
PREVV=v0.1.0-alpha.127 NEXTV=v0.1.0-alpha.128 bash $A/portalF.sh up v0.1.0-alpha.128 > $B/portalnet-up.out 2>&1; tail -1 $B/portalnet-up.out
free -m | sed -n 2p; echo "phase1 done $(ts)"
