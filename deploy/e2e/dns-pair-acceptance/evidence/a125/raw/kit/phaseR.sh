# alpha.125 round, after the owner-approved WSL restart (the host's network adapter broke at 21:31 UTC under memory
# pressure): labnet again, the old cells removed, the portal up, then the pair at 124 with less memory. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/env.sh
[ -d $B/logs-run1 ] || mv $B/logs $B/logs-run1; mkdir -p $B/logs
rm -rf /var/tmp/cp-beta1/state/p5 /var/tmp/cp-beta1/lab/cells/p5 /var/tmp/cp-beta1/state/d5 /var/tmp/cp-beta1/lab/cells/d5 /var/tmp/cp-beta1/evidence/p5 /var/tmp/cp-beta1/evidence/d5 $B/t-p5 $B/t-d5 $B/wiz-p5 $B/wiz-d5 $B/perf
bash $A/sync.sh
echo "start $(ts)"
python3 -I $L/lab.py net-up > /dev/null
python3 -I $L/lab.py stop beta1-lab labnet; echo
timeout 2700 bash $L/infra-up.sh > $B/infra-up2.out 2>&1; echo "infra-up exit $? $(ts)"
lab ssh beta1-lab labnet -- "sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y -q python3-dkim sshpass lftp swaks >/dev/null 2>&1; echo labnet-tools-ok"
PREVV=v0.1.0-alpha.124 NEXTV=v0.1.0-alpha.125 bash $A/portalF.sh up v0.1.0-alpha.125 > $A/portalnet-up.out 2>&1; tail -1 $A/portalnet-up.out
CELL=p5 MIP=192.0.3.190 BIP=192.0.3.191 Z=p5.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.124 BVER=v0.1.0-alpha.124 MKIND=pair-main BKIND=pair-backup-web BENGINE=bind MMEM=3584 BMEM=2048 TRUST=1 bash $A/pairX.sh > $A/pairP.out 2>&1
for n in main backup; do opnow p5 $n | head -1 | cut -c1-300; done
free -m | head -2
echo "phaseR done $(ts)"
