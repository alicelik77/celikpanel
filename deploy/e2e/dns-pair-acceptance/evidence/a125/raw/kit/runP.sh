# alpha.125 round: lab portal (124 + 125 signed releases, latest 125), the owner's pair at 124
# (Debian 13 PowerDNS main + mail, Ubuntu 24.04 BIND backup joined by code) and a fresh Debian 12 at 125. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit
PREVV=v0.1.0-alpha.124 NEXTV=v0.1.0-alpha.125 bash $A/portalF.sh up v0.1.0-alpha.125 > $A/portalnet-up.out 2>&1
( CELL=p5 MIP=192.0.3.190 BIP=192.0.3.191 Z=p5.test MOS=debian13 BOS=ubuntu2404 MVER=v0.1.0-alpha.124 BVER=v0.1.0-alpha.124 MKIND=pair-main BKIND=pair-backup-web BENGINE=bind MMEM=4096 BMEM=2560 TRUST=1 bash $A/pairX.sh > $A/pairP.out 2>&1 ) &
sleep 60
( CELL=d5 IP=192.0.3.193 Z=d5.test VER=v0.1.0-alpha.125 OS=debian12 MEM=3072 SHORT=frankfurt DNSENGINE=bind TRUST=1 bash $A/single124.sh > $A/singleD5.out 2>&1 ) &
wait
echo "runP done $(date -u +%T)"
