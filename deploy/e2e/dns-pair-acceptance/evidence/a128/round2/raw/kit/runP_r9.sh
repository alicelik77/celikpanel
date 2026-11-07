# alpha.128 round: the owner's pair at the published alpha.127 (lab copy): Debian 13 PowerDNS main + mail,
# Ubuntu 24.04 BIND backup joined by code, both through get.sh. The lab resolver's early SERVFAIL is flushed. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit
source $A/env.sh; source $H/common.sh
dmesg | grep -q "sub channel open failed" && { echo "STOP: host network adapter failure in dmesg"; exit 2; }
( for i in $(seq 1 50); do sleep 45; grep -q "== pair done" $LOGS/pair-r9.log 2>/dev/null && break
    a=$(lab ssh beta1-lab labnet -- "dig +short @192.0.2.53 frankfurt.r9.test A" 2>/dev/null); b=$(lab ssh beta1-lab labnet -- "dig +short +norec @192.0.3.190 frankfurt.r9.test A" 2>/dev/null)
    [ -n "$b" ] && [ -z "$a" ] && { echo "lab: resolver had no answer for frankfurt.r9.test while ns1 answers; delegation re-run (flush) $(date -u +%T)"; infra delegate r9.test ns1.r9.test 192.0.3.190 ns2.r9.test 192.0.3.191 > /dev/null; }
  done ) > $A/flushP.out 2>&1 &
CELL=r9 MIP=192.0.3.190 BIP=192.0.3.191 Z=r9.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.127 BVER=v0.1.0-alpha.127 MKIND=pair-main BKIND=pair-backup-web BENGINE=bind MMEM=3584 BMEM=3072 TRUST=1 bash $A/pairX.sh > $A/pairP.out 2>&1
wait
for n in main backup; do opnow r9 $n | head -1 | cut -c1-300; done
free -m | head -2
echo "runP done $(date -u +%T)"
