# alpha.126 case 1 other order: stop p6, install p7 at 125 (the lab resolver's early SERVFAIL is flushed by re-running the
# delegation while setup waits for public DNS), reproduce, then update main first and the joined server second. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a126lab/kit
source $A/env.sh; source $H/common.sh
dmesg | grep -q "sub channel open failed" && { echo "STOP: host network adapter failure in dmesg"; exit 2; }
bash $A/portalF.sh latest v0.1.0-alpha.125
for n in main backup; do python3 -I $L/lab.py stop p6 $n | tr -d '\n '; echo; done
( for i in $(seq 1 40); do sleep 45; grep -q "== pair done" $LOGS/pair-p7.log 2>/dev/null && break
    a=$(lab ssh beta1-lab labnet -- "dig +short @192.0.2.53 frankfurt.p7.test A" 2>/dev/null); b=$(lab ssh beta1-lab labnet -- "dig +short +norec @192.0.3.192 frankfurt.p7.test A" 2>/dev/null)
    [ -n "$b" ] && [ -z "$a" ] && { echo "lab: resolver had no answer for frankfurt.p7.test while ns1 answers; delegation re-run (flush) $(date -u +%T)"; infra delegate p7.test ns1.p7.test 192.0.3.192 ns2.p7.test 192.0.3.193 > /dev/null; }
  done ) > $A/flushB.out 2>&1 &
CELL=p7 MIP=192.0.3.192 BIP=192.0.3.193 Z=p7.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.125 BVER=v0.1.0-alpha.125 MKIND=pair-main BKIND=pair-backup-web BENGINE=bind MMEM=3584 BMEM=2304 TRUST=1 bash $A/pairX.sh > $A/pairB.out 2>&1
wait
for n in main backup; do opnow p7 $n | head -1 | cut -c1-200; done
bash $A/c1preB.sh > $A/c1preB.out 2>&1
CELL=p7 Z=p7.test MIP=192.0.3.192 BIP=192.0.3.193 IP=192.0.3.192 ORDER=main-first D=inc7.test bash $A/c1run.sh > $A/c1runB.out 2>&1
free -m | sed -n 2p
echo "runB done $(date -u +%T)"
