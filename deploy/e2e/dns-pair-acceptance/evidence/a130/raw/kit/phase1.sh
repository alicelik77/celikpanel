# alpha.130 round phase 1: labnet + infra, the published alpha.129 (acceptance licence), lab portal. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
mkdir -p $B; bash $A/sync.sh
echo "start $(ts)"; pgrep -c qemu-system; ls -d /var/tmp/cp-beta1 /var/tmp/cp-pair-accept 2>&1
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet > /dev/null 2>&1
( timeout 2700 bash $L/infra-up.sh > $B/infra-up.out 2>&1; echo "infra-up exit $? $(ts)" ) &
bash $A/build.sh refs/tags/v0.1.0-alpha.129 v0.1.0-alpha.129 > $B/out-build129.out 2>&1
wait
tail -3 $B/infra-up.out
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp swaks git >/dev/null 2>&1; echo labnet-tools-ok"
grep -E "^commit|EXIT|budget|FAILED|\"commit\"|\"sha256\"" $B/out-build129.out
bash $A/portal.sh $C129 > $B/portal129.out 2>&1; echo "portal129 exit $?"; grep -E "^(version|commit|sequence)=" $B/portal129.out
PREVV=v0.1.0-alpha.129 NEXTV=v0.1.0-alpha.129 bash $A/portalF.sh up v0.1.0-alpha.129 > $B/portalnet-up.out 2>&1; tail -1 $B/portalnet-up.out
free -m | sed -n 2p; df -h /var/tmp | tail -1; echo "phase1 done $(ts)"
